package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/essensys-hub/essensys-server-backend/internal/unifi"
)

// sonnette-plages-push-2026-11-008 : plages horaires, routes de la gateway.

func scheduleBody(schedule string) string {
	return `{"enabled":true,"camera_id":"` + camOK + `","lights":["salon"],"blinks":3,"schedule":` + schedule + `}`
}

func gridJSON(row1 string) string {
	zero := `"` + strings.Repeat("0", 24) + `"`
	rows := []string{zero, `"` + row1 + `"`, zero, zero, zero, zero, zero}
	return "[" + strings.Join(rows, ",") + "]"
}

func TestDoorbellSchedule_defautEtEnregistrement(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rec := rig.do(http.MethodGet, "/api/admin/doorbell/settings", "", models.LanRoleAdmin)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	sc := m["schedule"].(map[string]any)
	if sc["mode"] != "always" || sc["timezone"] != "Europe/Paris" || len(sc["grid"].([]any)) != 7 {
		t.Fatalf("%s", rec.Body.String())
	}
	body := scheduleBody(`{"mode":"custom","timezone":"Europe/Paris","grid":` + gridJSON("000000011111111111111100") + `}`)
	rig.save(t, body)
	// relecture après « redémarrage »
	rec = rig.do(http.MethodGet, "/api/admin/doorbell/settings", "", models.LanRoleAdmin)
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	sc = m["schedule"].(map[string]any)
	if sc["mode"] != "custom" || sc["grid"].([]any)[1] != "000000011111111111111100" {
		t.Fatalf("%s", rec.Body.String())
	}
	// un PUT sans schedule conserve la plage
	rig.save(t, goodSettings)
	_ = json.Unmarshal(rig.do(http.MethodGet, "/api/admin/doorbell/settings", "", models.LanRoleAdmin).Body.Bytes(), &m)
	if m["schedule"].(map[string]any)["mode"] != "custom" {
		t.Fatal("la plage doit être conservée quand le corps n'en porte pas")
	}
}

func TestDoorbellSchedule_valeursRefusees(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rig.save(t, goodSettings)
	before, _ := os.ReadFile(rig.path)
	ok23 := strings.Repeat("0", 23)
	zero := `"` + strings.Repeat("0", 24) + `"`
	cases := map[string]string{
		"6 lignes":      `{"mode":"custom","timezone":"Europe/Paris","grid":[` + strings.Repeat(zero+",", 5) + zero + `]}`,
		"23 caractères": `{"mode":"custom","timezone":"Europe/Paris","grid":` + gridJSON(ok23) + `}`,
		"caractère 2":   `{"mode":"custom","timezone":"Europe/Paris","grid":` + gridJSON(ok23+"2") + `}`,
		"fuseau":        `{"mode":"custom","timezone":"Mars/Olympus","grid":` + gridJSON(strings.Repeat("1", 24)) + `}`,
		"mode":          `{"mode":"parfois","timezone":"Europe/Paris","grid":` + gridJSON(strings.Repeat("1", 24)) + `}`,
		"pas un objet":  `"toujours"`,
		"null":          `null`,
	}
	for name, sc := range cases {
		rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", scheduleBody(sc), models.LanRoleAdmin)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_schedule") {
			t.Fatalf("%s : %d %s", name, rec.Code, rec.Body.String())
		}
	}
	after, _ := os.ReadFile(rig.path)
	if string(before) != string(after) {
		t.Fatal("aucun changement attendu après un refus")
	}
}

func TestDoorbellSchedule_lanUserRefuse(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	body := scheduleBody(`{"mode":"always","timezone":"Europe/Paris","grid":` + gridJSON(strings.Repeat("1", 24)) + `}`)
	if rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", body, models.LanRoleUser); rec.Code != http.StatusForbidden {
		t.Fatalf("lan_user : %d", rec.Code)
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("aucun fichier écrit par un refus")
	}
}

func TestDoorbellSchedule_detailsDAudit(t *testing.T) {
	def := core.DefaultDoorbellSchedule()
	custom := def
	custom.Mode = "custom"
	prev := core.DoorbellSettings{Schedule: def, Version: 1}
	if doorbellScheduleAuditDetails(prev, core.DoorbellSettings{Schedule: def, Version: 2}) != nil {
		t.Fatal("plage inchangée : pas d'événement doorbell_schedule")
	}
	d := doorbellScheduleAuditDetails(prev, core.DoorbellSettings{Schedule: custom, Version: 2})
	if d == nil || d["event"] != "doorbell_schedule" || d["version"] != 2 {
		t.Fatalf("%v", d)
	}
}

// NR: NR-server-backend-35 essensys-hub/essensys-feature-lifecycle#61
// Plages horaires de la sonnette : section doorbell.schedule dans le fichier existant sans
// toucher aux autres sections, erreur invalid_schedule, écriture réservée à lan_admin,
// in_window dans l'état, « Essayer » qui ignore la plage, aucune écriture dans l'armoire hors
// plage, aucun legacy IoT modifié.
func Test_NR_server_backend_35_sonnette_plages_horaires(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	name := "Salon TV"
	_, _ = rig.names.Set("salon", &name, "admin@example.org", time.Now())
	before := readSections(t, rig.path)

	// droits : lan_user refusé, rien d'écrit
	custom := scheduleBody(`{"mode":"custom","timezone":"Europe/Paris","grid":` + gridJSON(strings.Repeat("0", 24)) + `}`)
	if rig.do(http.MethodPut, "/api/admin/doorbell/settings", custom, models.LanRoleUser).Code != http.StatusForbidden {
		t.Fatal("lan_user doit être refusé")
	}
	// grille invalide
	bad := scheduleBody(`{"mode":"custom","timezone":"Europe/Paris","grid":["0"]}`)
	if rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", bad, models.LanRoleAdmin); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_schedule") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// grille entièrement inactive : valide ; hors plage en permanence
	rig.save(t, custom)
	if after := readSections(t, rig.path); before["lights"] != after["lights"] {
		t.Fatal("la section des noms doit rester intacte")
	}
	if rig.status(t, models.LanRoleUser)["in_window"] != false {
		t.Fatal("in_window doit valoir false avec une grille inactive")
	}
	// une vraie pression hors plage : ringing local, aucune écriture dans l'armoire
	rig.svc.HandleRing(unifi.RingEvent{ID: "e1", CameraID: camOK, At: time.Now()})
	rig.svc.WaitIdle()
	if st := rig.status(t, models.LanRoleUser); st["ringing"] != true || st["in_window"] != false {
		t.Fatalf("%v", st)
	}
	if !rig.queueEmpty() {
		t.Fatal("hors plage : aucun ordre vers l'armoire")
	}
	// « Essayer » ignore la plage (à sec : rien dans l'armoire, mais la séquence est annoncée)
	rec := rig.do(http.MethodPost, "/api/admin/doorbell/test", "", models.LanRoleAdmin, "X-Essensys-Test-Mode", "dry-run")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sequence":true`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// legacy IoT : aucune route de l'armoire n'évoque la plage
	for _, p := range []string{"/api/serverinfos", "/api/mystatus", "/api/myactions", "/api/done/x"} {
		if rec := rig.do(http.MethodPost, p, "", ""); strings.Contains(rec.Body.String(), "schedule") {
			t.Fatalf("%s ne doit pas évoquer la plage", p)
		}
	}
}
