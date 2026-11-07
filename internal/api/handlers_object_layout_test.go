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

// affichage-objets-2026-11-006 : disposition des lumières et des volets (gateway).

func (r *namesRig) layout(t *testing.T, kind, role string) map[string]any {
	t.Helper()
	rec := r.do(http.MethodGet, "/api/"+kind+"/layout", "", role)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", kind, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestObjectLayout_getTousUtilisateursEtDefaut(t *testing.T) {
	rig := newNamesRig(t, true, true)
	for _, kind := range []string{"lights", "shutters"} {
		for _, role := range []string{models.LanRoleUser, models.LanRoleAdmin} {
			if rig.do(http.MethodGet, "/api/"+kind+"/layout", "", role).Code != http.StatusOK {
				t.Fatalf("%s %s", kind, role)
			}
		}
		if rig.do(http.MethodGet, "/api/"+kind+"/layout", "", "").Code != http.StatusUnauthorized {
			t.Fatalf("%s : sans session 401 attendu", kind)
		}
		if body := rig.do(http.MethodGet, "/api/"+kind+"/layout", "", models.LanRoleUser).Body.String(); strings.TrimSpace(body) != `{"order":[],"disabled":[],"version":0,"updated_at":null}` {
			t.Fatalf("%s : défaut inattendu %s", kind, body)
		}
	}
}

func TestObjectLayout_getIAMDesactiveEtSansStockage(t *testing.T) {
	rig := newNamesRig(t, false, true)
	if rig.layout(t, "lights", "")["version"] != float64(0) {
		t.Fatal("ouvert sans IAM")
	}
	rig = newNamesRig(t, true, false)
	if rig.layout(t, "shutters", models.LanRoleUser)["version"] != float64(0) {
		t.Fatal("sans stockage : disposition par défaut")
	}
}

func TestObjectLayout_getSansUpdatedBy(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":["salon"],"disabled":["wc2"]}`, models.LanRoleAdmin)
	if body := rig.do(http.MethodGet, "/api/lights/layout", "", models.LanRoleUser).Body.String(); strings.Contains(body, "updated_by") || strings.Contains(body, "example.org") {
		t.Fatalf("updated_by ne doit pas être exposé : %s", body)
	}
}

func TestObjectLayout_putAdminEtLecture(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rec := rig.do(http.MethodPut, "/api/admin/shutters/layout", `{"order":["volet2salon","volet1salon"],"disabled":["voletbureau","store"]}`, models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["status"] != "saved" || out["version"] != float64(1) || out["updated_at"] == nil || out["updated_by"] != nil {
		t.Fatalf("%+v", out)
	}
	if rig.shuttersLayoutSynced != 1 || rig.lightsLayoutSynced != 0 || rig.synced != 0 || rig.shutterSynced != 0 {
		t.Fatalf("synchro immédiate de la disposition des volets seulement : %d %d %d %d", rig.shuttersLayoutSynced, rig.lightsLayoutSynced, rig.synced, rig.shutterSynced)
	}
	got := rig.layout(t, "shutters", models.LanRoleUser)
	if got["version"] != float64(1) || len(got["disabled"].([]any)) != 2 {
		t.Fatalf("%+v", got)
	}
	if rig.layout(t, "lights", models.LanRoleUser)["version"] != float64(0) {
		t.Fatal("genres indépendants")
	}
	// reset : version n+1
	rec = rig.do(http.MethodPut, "/api/admin/shutters/layout", `{"reset":true}`, models.LanRoleAdmin)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out["version"] != float64(2) || len(out["order"].([]any)) != 0 || len(out["disabled"].([]any)) != 0 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestObjectLayout_putDroitsEtValidation(t *testing.T) {
	rig := newNamesRig(t, true, true)
	if rec := rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":[],"disabled":["wc2"]}`, models.LanRoleUser); rec.Code != http.StatusForbidden || errKey(t, rec) != "forbidden" {
		t.Fatalf("lan_user : %d %s", rec.Code, rec.Body.String())
	}
	if rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":[],"disabled":[]}`, "").Code != http.StatusUnauthorized {
		t.Fatal("sans session : 401")
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("le fichier ne doit pas exister")
	}
	cases := []struct{ kind, body, key string }{
		{"lights", `{"order":["nimporte"],"disabled":[]}`, "unknown_light"},
		{"lights", `{"order":[],"disabled":["volet1salon"]}`, "unknown_light"},
		{"shutters", `{"order":["salon"],"disabled":[]}`, "unknown_shutter"},
		{"lights", `{"order":["salon","salon"],"disabled":[]}`, "invalid_layout"},
		{"lights", `{"order":[],"disabled":["wc2","wc2"]}`, "invalid_layout"},
		{"lights", `{"disabled":[]}`, "invalid_layout"},
		{"lights", `{"order":[]}`, "invalid_layout"},
		{"lights", `{"order":null,"disabled":[]}`, "invalid_layout"},
		{"lights", `{"order":"salon","disabled":[]}`, "invalid_layout"},
		{"lights", `{"order":[1],"disabled":[]}`, "invalid_layout"},
		{"lights", `{}`, "invalid_layout"},
		{"lights", `pas du json`, "invalid_layout"},
		{"lights", `{"order":[],"disabled":[],"pad":"` + strings.Repeat("a", 17000) + `"}`, "invalid_layout"},
	}
	for _, c := range cases {
		rec := rig.do(http.MethodPut, "/api/admin/"+c.kind+"/layout", c.body, models.LanRoleAdmin)
		if rec.Code != http.StatusBadRequest || errKey(t, rec) != c.key {
			t.Fatalf("%s %.60s : %d %s", c.kind, c.body, rec.Code, rec.Body.String())
		}
	}
	if rig.lightsLayoutSynced+rig.shuttersLayoutSynced != 0 {
		t.Fatal("aucune synchro sur une requête refusée")
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("aucune écriture sur une requête refusée")
	}
	// tout désactivé accepté
	all, _ := json.Marshal(map[string]any{"order": []string{}, "disabled": core.LayoutLights.Catalogue()})
	if rig.do(http.MethodPut, "/api/admin/lights/layout", string(all), models.LanRoleAdmin).Code != http.StatusOK {
		t.Fatal("tout désactivé doit être accepté")
	}
}

func TestObjectLayout_putIAMDesactive(t *testing.T) {
	rig := newNamesRig(t, false, true)
	if rec := rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":[],"disabled":["wc2"]}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestObjectLayout_putSansStockage503(t *testing.T) {
	rig := newNamesRig(t, true, false)
	rec := rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":[],"disabled":[]}`, models.LanRoleAdmin)
	if rec.Code != http.StatusServiceUnavailable || errKey(t, rec) != "names_unavailable" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestObjectLayout_dryRun(t *testing.T) {
	rig := newNamesRig(t, true, true)
	for _, c := range []struct{ path, hk, hv string }{
		{"/api/admin/lights/layout", "X-Essensys-Test-Mode", "dry-run"},
		{"/api/admin/lights/layout?test_mode=dry_run", "", ""},
	} {
		rec := rig.do(http.MethodPut, c.path, `{"order":["salon"],"disabled":["wc2"]}`, models.LanRoleAdmin, c.hk, c.hv)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != 200 || out["status"] != "dry_run" || out["version"] != float64(1) {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	}
	if rig.layout(t, "lights", models.LanRoleAdmin)["version"] != float64(0) {
		t.Fatal("dry-run a écrit")
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("le fichier ne doit pas exister après un dry-run")
	}
	if rig.lightsLayoutSynced != 0 {
		t.Fatal("pas de synchro en dry-run")
	}
	rec := rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":["nimporte"],"disabled":[]}`, models.LanRoleAdmin, "X-Essensys-Test-Mode", "dry-run")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("le dry-run valide aussi : %d", rec.Code)
	}
}

func TestObjectLayout_methodeRefusee(t *testing.T) {
	rig := newNamesRig(t, true, true)
	if rig.do(http.MethodPost, "/api/lights/layout", `{}`, models.LanRoleAdmin).Code != http.StatusMethodNotAllowed {
		t.Fatal("POST sur la lecture : 405")
	}
	if rig.do(http.MethodGet, "/api/admin/lights/layout", "", models.LanRoleAdmin).Code != http.StatusMethodNotAllowed {
		t.Fatal("GET sur l'écriture : 405")
	}
}

func TestObjectLayoutAuditDetails(t *testing.T) {
	prev := core.ObjectLayout{Order: []string{}, Disabled: []string{}, Version: 1}
	next := core.ObjectLayout{Order: []string{"salon"}, Disabled: []string{"wc2"}, Version: 2}
	d := objectLayoutAuditDetails(core.LayoutShutters, prev, next)
	if d["event"] != "object_layout" || d["kind"] != "shutters" || d["version"] != 2 || d["previous"] == nil || d["new"] == nil {
		t.Fatalf("%+v", d)
	}
}

// Disposition et noms vivent dans le même fichier sans se perdre mutuellement.
func TestObjectLayout_noms_et_dispositions_cohabitent(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"WC 3"}`, models.LanRoleAdmin)
	rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"Banne"}`, models.LanRoleAdmin)
	rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":["salon"],"disabled":["wc2"]}`, models.LanRoleAdmin)
	rig.do(http.MethodPut, "/api/admin/shutters/layout", `{"order":[],"disabled":["store"]}`, models.LanRoleAdmin)
	rig.do(http.MethodPut, "/api/admin/lights/names/wc1", `{"name":"Toilettes"}`, models.LanRoleAdmin)
	for _, l := range rig.names(t, models.LanRoleUser) {
		if (l.ID == "dressing" && (l.Name == nil || *l.Name != "WC 3")) || (l.ID == "wc1" && (l.Name == nil || *l.Name != "Toilettes")) {
			t.Fatalf("%+v", l)
		}
	}
	for _, v := range rig.shutters(t, models.LanRoleUser) {
		if v.ID == "store" && (v.Name == nil || *v.Name != "Banne") {
			t.Fatalf("%+v", v)
		}
	}
	if rig.layout(t, "lights", models.LanRoleUser)["version"] != float64(1) {
		t.Fatal("disposition des lumières perdue")
	}
	if rig.layout(t, "shutters", models.LanRoleUser)["version"] != float64(1) {
		t.Fatal("disposition des volets perdue")
	}
}

// NR: NR-server-backend-32 essensys-hub/essensys-feature-lifecycle#55
// L'écriture de la disposition est réservée à lan_admin, lisible par tout utilisateur
// authentifié, n'écrit jamais dans l'armoire et ne change pas les noms des lumières et des
// volets (#41 et #52).
func Test_NR_server_backend_32_disposition_des_objets_reservee_a_lan_admin(t *testing.T) {
	rig := newNamesRig(t, true, true)
	rig.do(http.MethodPut, "/api/admin/lights/names/dressing", `{"name":"WC 3"}`, models.LanRoleAdmin)
	rig.do(http.MethodPut, "/api/admin/shutters/names/store", `{"name":"Banne"}`, models.LanRoleAdmin)
	for _, kind := range []string{"lights", "shutters"} {
		if rig.do(http.MethodPut, "/api/admin/"+kind+"/layout", `{"order":[],"disabled":[]}`, models.LanRoleUser).Code != http.StatusForbidden {
			t.Fatalf("%s : lan_user doit être refusé", kind)
		}
	}
	if rig.do(http.MethodPut, "/api/admin/lights/layout", `{"order":["salon"],"disabled":["wc2"]}`, models.LanRoleAdmin).Code != http.StatusOK ||
		rig.do(http.MethodPut, "/api/admin/shutters/layout", `{"order":["store"],"disabled":["voletbureau"]}`, models.LanRoleAdmin).Code != http.StatusOK {
		t.Fatal("lan_admin doit être accepté")
	}
	for _, kind := range []string{"lights", "shutters"} {
		if rig.do(http.MethodGet, "/api/"+kind+"/layout", "", models.LanRoleUser).Code != http.StatusOK {
			t.Fatalf("%s : lecture ouverte à lan_user", kind)
		}
	}
	for _, l := range rig.names(t, models.LanRoleUser) {
		if l.ID == "dressing" && (l.Name == nil || *l.Name != "WC 3") || l.ID != "dressing" && l.Name != nil {
			t.Fatalf("noms des lumières (#41) modifiés : %+v", l)
		}
	}
	for _, v := range rig.shutters(t, models.LanRoleUser) {
		if v.ID == "store" && (v.Name == nil || *v.Name != "Banne") || v.ID != "store" && v.Name != nil {
			t.Fatalf("noms des volets (#52) modifiés : %+v", v)
		}
	}
	if len(rig.store.DequeueActions("default")) != 0 {
		t.Fatal("aucune écriture dans l'armoire")
	}
}
