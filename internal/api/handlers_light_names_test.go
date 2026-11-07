package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type namesRig struct {
	router  http.Handler
	cookies map[string]*http.Cookie
	path    string
	store   data.Store
	synced  int
	// noms-volets-2026-11-005 : même magasin (même fichier), notification distincte.
	shutterSynced int
}

func newNamesRig(t *testing.T, iamEnabled bool, withStorage bool) *namesRig {
	t.Helper()
	store := data.NewMemoryStore()
	h := NewHandler(core.NewActionService(store), core.NewStatusService(store), store, nil, nil, config.ArmoireConfig{})
	rig := &namesRig{store: store, cookies: map[string]*http.Cookie{}}
	if withStorage {
		rig.path = filepath.Join(t.TempDir(), "light-names.json")
	}
	names := core.NewLightNameStore(rig.path)
	h.SetLightNames(names, func() { rig.synced++ })
	h.SetShutterNames(names, func() { rig.shutterSynced++ })
	sessions := laniam.NewSessionStore(1)
	for i, role := range []string{models.LanRoleAdmin, models.LanRoleUser} {
		sess, err := sessions.CreateSession(&models.LanUser{ID: i + 1, Email: role + "@example.org", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		rig.cookies[role] = &http.Cookie{Name: middleware.LanSessionCookie, Value: sess.ID}
	}
	rig.router = NewRouter(RouterConfig{Handler: h, LanSessionStore: sessions, LanIAMEnabled: iamEnabled, Store: store})
	return rig
}

func (r *namesRig) do(method, path, body, role string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if c, ok := r.cookies[role]; ok {
		req.AddCookie(c)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	r.router.ServeHTTP(rec, req)
	return rec
}

func (r *namesRig) names(t *testing.T, role string) []core.LightNameView {
	t.Helper()
	rec := r.do(http.MethodGet, "/api/lights/names", "", role)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lights []core.LightNameView `json:"lights"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Lights
}

func TestLightNames_getTousUtilisateurs(t *testing.T) {
	rig := newNamesRig(t, true, true)
	for _, role := range []string{models.LanRoleUser, models.LanRoleAdmin} {
		lights := rig.names(t, role)
		if len(lights) != 33 || lights[0].ID != "terrasse" || lights[0].Name != nil || lights[0].Default != "Terrasse" || lights[0].Version != 0 {
			t.Fatalf("%s : %d %+v", role, len(lights), lights[0])
		}
	}
	rec := rig.do(http.MethodGet, "/api/lights/names", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	if !strings.Contains(rig.do(http.MethodGet, "/api/lights/names", "", models.LanRoleUser).Body.String(), `"name":null,"version":0,"updated_at":null`) {
		t.Fatal("name et updated_at doivent valoir null par défaut")
	}
}

func TestLightNames_getIAMDesactive(t *testing.T) {
	rig := newNamesRig(t, false, true)
	if len(rig.names(t, "")) != 33 {
		t.Fatal("33 lumières attendues sans IAM")
	}
}

func TestLightNames_getSansStockage(t *testing.T) {
	rig := newNamesRig(t, true, false)
	lights := rig.names(t, models.LanRoleUser)
	if len(lights) != 33 || lights[20].Default != "Dressing" || lights[20].Name != nil {
		t.Fatalf("%+v", lights[20])
	}
}

func TestLightNames_putAdmin(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rec := rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"  WC 3 "}`, models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["id"] != "dressing" || out["default"] != "Dressing" || out["name"] != "WC 3" || out["version"] != float64(1) || out["status"] != "saved" || out["updated_at"] == nil {
		t.Fatalf("%+v", out)
	}
	if rig.synced != 1 {
		t.Fatalf("synchro immédiate attendue, got %d", rig.synced)
	}
	// visible par un lan_user
	for _, l := range rig.names(t, models.LanRoleUser) {
		if l.ID == "dressing" && (l.Name == nil || *l.Name != "WC 3") {
			t.Fatalf("%+v", l)
		}
	}
	// remise à zéro
	rec = rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":null}`, models.LanRoleAdmin)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["name"] != nil || out["version"] != float64(2) {
		t.Fatalf("%d %+v", rec.Code, out)
	}
	// aucune écriture dans l'armoire
	if len(rig.store.DequeueActions("default")) != 0 {
		t.Fatal("aucune action ne doit être mise en file")
	}
}

func TestLightNames_putDroitsEtValidation(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rec := rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"X"}`, models.LanRoleUser)
	if rec.Code != http.StatusForbidden || errKey(t, rec) != "forbidden" {
		t.Fatalf("lan_user : %d %s", rec.Code, rec.Body.String())
	}
	if rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"X"}`, "").Code != http.StatusUnauthorized {
		t.Fatal("sans session : 401 attendu")
	}
	for _, l := range rig.names(t, models.LanRoleAdmin) {
		if l.Name != nil {
			t.Fatal("rien ne doit avoir été écrit")
		}
	}
	cases := []struct{ path, body, key string }{
		{"/api/admin/lights/names/nimporte", `{"name":"X"}`, "unknown_light"},
		{"/api/admin/lights/names/", `{"name":"X"}`, "unknown_light"},
		{"/api/admin/lights/names/dressing", `{"name":"` + strings.Repeat("a", 41) + `"}`, "invalid_name"},
		{"/api/admin/lights/names/dressing", `{"name":"<b>"}`, "invalid_name"},
		{"/api/admin/lights/names/dressing", `{"name":12}`, "invalid_name"},
		{"/api/admin/lights/names/dressing", `{}`, "invalid_name"},
		{"/api/admin/lights/names/dressing", `pas du json`, "invalid_name"},
	}
	for _, c := range cases {
		rec := rig.do(http.MethodPut, c.path, c.body, models.LanRoleAdmin)
		if rec.Code != http.StatusBadRequest || errKey(t, rec) != c.key {
			t.Fatalf("%s %s : %d %s", c.path, c.body, rec.Code, rec.Body.String())
		}
	}
	if rig.synced != 0 {
		t.Fatal("aucune synchro sur une requête refusée")
	}
}

func TestLightNames_putIAMDesactive(t *testing.T) {
	rig := newNamesRig(t, false, true)
	rec := rig.do(http.MethodPut, "/api/admin/lights/names/wc1", `{"name":"Toilettes"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestLightNames_dryRun(t *testing.T) {
	rig := newNamesRig(t, true, true)
	for _, c := range []struct{ path, hk, hv string }{
		{"/api/admin/lights/names/wc1", "X-Essensys-Test-Mode", "dry-run"},
		{"/api/admin/lights/names/wc1?test_mode=dry_run", "", ""},
	} {
		rec := rig.do(http.MethodPut, c.path, `{"name":"Essai"}`, models.LanRoleAdmin, c.hk, c.hv)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 || out["status"] != "dry_run" {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	for _, l := range rig.names(t, models.LanRoleAdmin) {
		if l.Name != nil || l.Version != 0 {
			t.Fatalf("dry-run a écrit : %+v", l)
		}
	}
	if rig.synced != 0 {
		t.Fatal("pas de synchro en dry-run")
	}
	rec := rig.do(http.MethodPut, "/api/admin/lights/names/wc1", `{"name":"<b>"}`, models.LanRoleAdmin, "X-Essensys-Test-Mode", "dry-run")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("le dry-run valide aussi : %d", rec.Code)
	}
}

func TestLightNames_putSansStockage503(t *testing.T) {
	rig := newNamesRig(t, true, false)
	rec := rig.do(http.MethodPut, "/api/admin/lights/names/wc1", `{"name":"X"}`, models.LanRoleAdmin)
	if rec.Code != http.StatusServiceUnavailable || errKey(t, rec) != "names_unavailable" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// NR: NR-server-backend-29 essensys-hub/essensys-feature-lifecycle#41
// Le renommage d'une lumière est réservé à lan_admin, lisible par tout utilisateur
// authentifié, et n'écrit jamais dans l'armoire.
func Test_NR_server_backend_29_renommer_une_lumiere_reserve_a_lan_admin(t *testing.T) {
	rig := newNamesRig(t, true, true)
	if rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"WC 3"}`, models.LanRoleUser).Code != http.StatusForbidden {
		t.Fatal("lan_user doit être refusé")
	}
	if rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"WC 3"}`, models.LanRoleAdmin).Code != http.StatusOK {
		t.Fatal("lan_admin doit être accepté")
	}
	if len(rig.names(t, models.LanRoleUser)) != 33 {
		t.Fatal("lecture ouverte à lan_user")
	}
	if len(rig.store.DequeueActions("default")) != 0 {
		t.Fatal("aucune écriture dans l'armoire")
	}
}
