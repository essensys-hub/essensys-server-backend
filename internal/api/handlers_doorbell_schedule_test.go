package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
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
