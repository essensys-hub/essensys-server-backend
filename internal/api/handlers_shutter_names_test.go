package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

// noms-volets-2026-11-005 : noms personnalisés des volets et stores (gateway).

func (r *namesRig) shutters(t *testing.T, role string) []core.ShutterNameView {
	t.Helper()
	rec := r.do(http.MethodGet, "/api/shutters/names", "", role)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Shutters []core.ShutterNameView `json:"shutters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Shutters
}

func TestShutterNames_getTousUtilisateurs(t *testing.T) {
	rig := newNamesRig(t, true, true)
	for _, role := range []string{models.LanRoleUser, models.LanRoleAdmin} {
		s := rig.shutters(t, role)
		if len(s) != 16 || s[0].ID != "volet1salon" || s[0].Name != nil || s[0].Default != "Volet 1 Salon" || s[0].Version != 0 || s[14].Default != `Volet "Store"` {
			t.Fatalf("%s : %d %+v", role, len(s), s[0])
		}
	}
	if rig.do(http.MethodGet, "/api/shutters/names", "", "").Code != http.StatusUnauthorized {
		t.Fatal("sans session : 401 attendu")
	}
	if !strings.Contains(rig.do(http.MethodGet, "/api/shutters/names", "", models.LanRoleUser).Body.String(), `"name":null,"version":0,"updated_at":null`) {
		t.Fatal("name et updated_at doivent valoir null par défaut")
	}
}

func TestShutterNames_getIAMDesactive(t *testing.T) {
	rig := newNamesRig(t, false, true)
	if len(rig.shutters(t, "")) != 16 {
		t.Fatal("16 volets attendus sans IAM")
	}
}

func TestShutterNames_getSansStockage(t *testing.T) {
	rig := newNamesRig(t, true, false)
	s := rig.shutters(t, models.LanRoleUser)
	if len(s) != 16 || s[15].Default != "Store (banne)" || s[15].Name != nil {
		t.Fatalf("%+v", s[15])
	}
}

func TestShutterNames_putAdmin(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rec := rig.do(http.MethodPut, "/api/admin/shutters/names/volet1salon", `{"name":"  Volet baie vitrée "}`, models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["id"] != "volet1salon" || out["default"] != "Volet 1 Salon" || out["name"] != "Volet baie vitrée" || out["version"] != float64(1) || out["status"] != "saved" || out["updated_at"] == nil {
		t.Fatalf("%+v", out)
	}
	if rig.shutterSynced != 1 || rig.synced != 0 {
		t.Fatalf("synchro immédiate des volets seulement : %d/%d", rig.shutterSynced, rig.synced)
	}
	for _, v := range rig.shutters(t, models.LanRoleUser) {
		if v.ID == "volet1salon" && (v.Name == nil || *v.Name != "Volet baie vitrée") {
			t.Fatalf("%+v", v)
		}
	}
	rec = rig.do(http.MethodPut, "/api/admin/shutters/names/volet1salon", `{"name":null}`, models.LanRoleAdmin)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["name"] != nil || out["version"] != float64(2) {
		t.Fatalf("%d %+v", rec.Code, out)
	}
	rec = rig.do(http.MethodPut, "/api/admin/shutters/names/voletstore", `{"name":"Store \"nord\""}`, models.LanRoleAdmin)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["name"] != `Store "nord"` {
		t.Fatalf("guillemets : %d %+v", rec.Code, out)
	}
	if len(rig.store.DequeueActions("default")) != 0 {
		t.Fatal("aucune action ne doit être mise en file")
	}
}

func TestShutterNames_putDroitsEtValidation(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rec := rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"X"}`, models.LanRoleUser)
	if rec.Code != http.StatusForbidden || errKey(t, rec) != "forbidden" {
		t.Fatalf("lan_user : %d %s", rec.Code, rec.Body.String())
	}
	if rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"X"}`, "").Code != http.StatusUnauthorized {
		t.Fatal("sans session : 401 attendu")
	}
	for _, v := range rig.shutters(t, models.LanRoleAdmin) {
		if v.Name != nil {
			t.Fatal("rien ne doit avoir été écrit")
		}
	}
	cases := []struct{ path, body, key string }{
		{"/api/admin/shutters/names/nimporte", `{"name":"X"}`, "unknown_shutter"},
		{"/api/admin/shutters/names/", `{"name":"X"}`, "unknown_shutter"},
		{"/api/admin/shutters/names/dressing", `{"name":"X"}`, "unknown_shutter"},
		{"/api/admin/shutters/names/store", `{"name":"` + strings.Repeat("a", 41) + `"}`, "invalid_name"},
		{"/api/admin/shutters/names/store", `{"name":"<b>"}`, "invalid_name"},
		{"/api/admin/shutters/names/store", `{"name":12}`, "invalid_name"},
		{"/api/admin/shutters/names/store", `{}`, "invalid_name"},
		{"/api/admin/shutters/names/store", `pas du json`, "invalid_name"},
	}
	for _, c := range cases {
		rec := rig.do(http.MethodPut, c.path, c.body, models.LanRoleAdmin)
		if rec.Code != http.StatusBadRequest || errKey(t, rec) != c.key {
			t.Fatalf("%s %s : %d %s", c.path, c.body, rec.Code, rec.Body.String())
		}
	}
	// un identifiant de volet est refusé côté lumières
	if rec := rig.do(http.MethodPut, "/api/admin/lights/names/volet1salon", `{"name":"X"}`, models.LanRoleAdmin); rec.Code != http.StatusBadRequest || errKey(t, rec) != "unknown_light" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rig.shutterSynced != 0 {
		t.Fatal("aucune synchro sur une requête refusée")
	}
}

func TestShutterNames_putIAMDesactive(t *testing.T) {
	rig := newNamesRig(t, false, true)
	if rec := rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"Banne"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestShutterNames_dryRun(t *testing.T) {
	rig := newNamesRig(t, true, true)
	for _, c := range []struct{ path, hk, hv string }{
		{"/api/admin/shutters/names/store", "X-Essensys-Test-Mode", "dry-run"},
		{"/api/admin/shutters/names/store?test_mode=dry_run", "", ""},
	} {
		rec := rig.do(http.MethodPut, c.path, `{"name":"Essai"}`, models.LanRoleAdmin, c.hk, c.hv)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 || out["status"] != "dry_run" {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	for _, v := range rig.shutters(t, models.LanRoleAdmin) {
		if v.Name != nil || v.Version != 0 {
			t.Fatalf("dry-run a écrit : %+v", v)
		}
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("le fichier ne doit pas exister après un dry-run")
	}
	if rig.shutterSynced != 0 {
		t.Fatal("pas de synchro en dry-run")
	}
	rec := rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"<b>"}`, models.LanRoleAdmin, "X-Essensys-Test-Mode", "dry-run")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("le dry-run valide aussi : %d", rec.Code)
	}
}

func TestShutterNames_putSansStockage503(t *testing.T) {
	rig := newNamesRig(t, true, false)
	rec := rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"X"}`, models.LanRoleAdmin)
	if rec.Code != http.StatusServiceUnavailable || errKey(t, rec) != "names_unavailable" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

// Renommer un volet puis une lumière : les deux restent lisibles, y compris après relecture du fichier.
func TestShutterNames_lumieresInchangees(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"Banne"}`, models.LanRoleAdmin)
	rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"WC 3"}`, models.LanRoleAdmin)
	for _, l := range rig.names(t, models.LanRoleUser) {
		if l.ID == "dressing" && (l.Name == nil || *l.Name != "WC 3") {
			t.Fatalf("%+v", l)
		}
		if l.ID != "dressing" && l.Name != nil {
			t.Fatalf("lumière modifiée par un volet : %+v", l)
		}
	}
	for _, v := range rig.shutters(t, models.LanRoleUser) {
		if v.ID == "store" && (v.Name == nil || *v.Name != "Banne") {
			t.Fatalf("%+v", v)
		}
	}
}

// NR: NR-server-backend-30 essensys-hub/essensys-feature-lifecycle#41
// Le renommage d'un volet est réservé à lan_admin, lisible par tout utilisateur authentifié,
// n'écrit jamais dans l'armoire et ne change pas les noms des lumières.
func Test_NR_server_backend_30_renommer_un_volet_reserve_a_lan_admin(t *testing.T) {
	rig := newNamesRig(t, true, true)
	if rig.do(http.MethodPut, "/api/admin/shutters/names/volet1salon", `{"name":"Volet baie vitrée"}`, models.LanRoleUser).Code != http.StatusForbidden {
		t.Fatal("lan_user doit être refusé")
	}
	if rig.do(http.MethodPut, "/api/admin/shutters/names/volet1salon", `{"name":"Volet baie vitrée"}`, models.LanRoleAdmin).Code != http.StatusOK {
		t.Fatal("lan_admin doit être accepté")
	}
	if len(rig.shutters(t, models.LanRoleUser)) != 16 {
		t.Fatal("lecture ouverte à lan_user")
	}
	for _, l := range rig.names(t, models.LanRoleUser) {
		if l.Name != nil {
			t.Fatalf("les noms des lumières doivent rester inchangés : %+v", l)
		}
	}
	if len(rig.store.DequeueActions("default")) != 0 {
		t.Fatal("aucune écriture dans l'armoire")
	}
}
