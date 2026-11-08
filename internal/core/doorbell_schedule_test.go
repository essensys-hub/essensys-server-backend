package core

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// sonnette-plages-push-2026-11-008 : plages horaires de la sonnette (gateway).

func customGrid(rows map[int]string) []string {
	g := make([]string, 7)
	for i := range g {
		g[i] = strings.Repeat("0", 24)
		if r, ok := rows[i]; ok {
			g[i] = r
		}
	}
	return g
}

func TestDoorbellSchedule_defauts(t *testing.T) {
	st, _ := doorbellStore(t)
	sc := st.Doorbell().Schedule
	if sc.Mode != "always" || sc.Timezone != "Europe/Paris" || len(sc.Grid) != 7 || sc.Grid[3] != strings.Repeat("1", 24) {
		t.Fatalf("%+v", sc)
	}
	if def := NewLightNameStore("").Doorbell().Schedule; def.Mode != "always" {
		t.Fatalf("sans stockage : %+v", def)
	}
}

func TestDoorbellSchedule_ecritureRelectureEtConservation(t *testing.T) {
	st, p := doorbellStore(t)
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	sched := DoorbellSchedule{Mode: "custom", Timezone: "Europe/Paris", Grid: customGrid(map[int]string{1: "000000011111111111111100"})}
	saved, err := st.SetDoorbellSchedule(true, testDoorbellCam, []string{"salon"}, 3, &sched, "admin@example.org", now)
	if err != nil {
		t.Fatal(err)
	}
	if again := NewLightNameStore(p).Doorbell(); !reflect.DeepEqual(again, saved) || again.Schedule.Mode != "custom" {
		t.Fatalf("relecture : %+v au lieu de %+v", again, saved)
	}
	// SetDoorbell (sans plage) conserve la plage actuelle.
	next, err := st.SetDoorbell(true, testDoorbellCam, []string{"salon"}, 4, "admin@example.org", now)
	if err != nil || !reflect.DeepEqual(next.Schedule, saved.Schedule) {
		t.Fatalf("%v %+v", err, next.Schedule)
	}
}

func TestDoorbellSchedule_validation(t *testing.T) {
	ok := strings.Repeat("1", 24)
	bad := map[string]DoorbellSchedule{
		"6 lignes":      {Mode: "custom", Timezone: "Europe/Paris", Grid: customGrid(nil)[:6]},
		"23 caractères": {Mode: "custom", Timezone: "Europe/Paris", Grid: append(customGrid(nil)[:6], ok[:23])},
		"caractère 2":   {Mode: "custom", Timezone: "Europe/Paris", Grid: append(customGrid(nil)[:6], strings.Repeat("1", 23)+"2")},
		"fuseau":        {Mode: "custom", Timezone: "Mars/Olympus", Grid: customGrid(nil)},
		"fuseau Local":  {Mode: "custom", Timezone: "Local", Grid: customGrid(nil)},
		"mode":          {Mode: "parfois", Timezone: "Europe/Paris", Grid: customGrid(nil)},
		"custom vide":   {Mode: "custom", Timezone: "Europe/Paris"},
	}
	st, _ := doorbellStore(t)
	for name, sc := range bad {
		sc := sc
		if _, err := st.SetDoorbellSchedule(false, "", []string{}, 5, &sc, "x", time.Now()); err != ErrInvalidSchedule {
			t.Fatalf("%s : %v", name, err)
		}
	}
	if st.Doorbell().Version != 0 {
		t.Fatal("aucun changement attendu")
	}
	// Un fichier sans schedule se lit comme always ; une grille « always » vide est complétée.
	sc := DoorbellSchedule{Mode: "always"}
	got, err := st.SetDoorbellSchedule(false, "", []string{}, 5, &sc, "x", time.Now())
	if err != nil || got.Schedule.Timezone != "Europe/Paris" || len(got.Schedule.Grid) != 7 {
		t.Fatalf("%v %+v", err, got.Schedule)
	}
}

func TestDoorbellSchedule_autresSectionsPreservees(t *testing.T) {
	st, p := doorbellStore(t)
	now := time.Now()
	name := "Salon TV"
	if _, err := st.Set("salon", &name, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p)
	var b map[string]json.RawMessage
	_ = json.Unmarshal(before, &b)
	sc := DoorbellSchedule{Mode: "custom", Timezone: "Europe/Paris", Grid: customGrid(nil)}
	if _, err := st.SetDoorbellSchedule(true, testDoorbellCam, []string{"salon"}, 3, &sc, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	var a map[string]json.RawMessage
	_ = json.Unmarshal(after, &a)
	for _, k := range []string{"version", "lights"} {
		if string(a[k]) != string(b[k]) {
			t.Fatalf("section %q modifiée", k)
		}
	}
}

func TestDoorbellSchedule_ActiveAt(t *testing.T) {
	sc, _ := DoorbellSchedule{Mode: "custom", Timezone: "Europe/Paris", Grid: customGrid(map[int]string{
		1: "000000000000001111000000", // mardi 14 h à 17 h 59
		6: "100000000000000000000000", // dimanche 0 h
	})}.Normalize()
	paris, _ := time.LoadLocation("Europe/Paris")
	at := func(y int, m time.Month, d, h, mi int) time.Time { return time.Date(y, m, d, h, mi, 0, 0, paris).UTC() }
	// 2026-11-10 est un mardi
	if !sc.ActiveAt(at(2026, 11, 10, 14, 5)) {
		t.Fatal("mardi 14 h 05 doit être actif")
	}
	if sc.ActiveAt(at(2026, 11, 10, 3, 10)) {
		t.Fatal("mardi 3 h 10 doit être inactif")
	}
	if sc.ActiveAt(at(2026, 11, 9, 14, 5)) {
		t.Fatal("lundi 14 h 05 doit être inactif (lundi en premier)")
	}
	if !sc.ActiveAt(at(2026, 11, 15, 0, 30)) {
		t.Fatal("dimanche 0 h 30 doit être actif")
	}
	// Le fuseau compte : mardi 14 h 05 à Paris = 13 h 05 UTC, mais 8 h 05 à New York.
	ny := sc
	ny.Timezone = "America/New_York"
	if ny.ActiveAt(at(2026, 11, 10, 14, 5)) {
		t.Fatal("même instant, autre fuseau : mardi 8 h, inactif")
	}
	// mode always : grille ignorée
	al := DoorbellSchedule{Mode: "always", Timezone: "Europe/Paris", Grid: customGrid(nil)}
	if !al.ActiveAt(at(2026, 11, 10, 3, 10)) {
		t.Fatal("mode always : toujours actif")
	}
}

func TestDoorbellSchedule_changementDHeure(t *testing.T) {
	// Passage à l'heure d'été 2026 en Europe/Paris : dimanche 29 mars, 2 h -> 3 h.
	// La case de dimanche n'est active qu'à 3 h locale ; 01:30 UTC (= 3 h 30 CEST) doit l'être,
	// 00:30 UTC (= 1 h 30 CET) ne doit pas l'être.
	sc, _ := DoorbellSchedule{Mode: "custom", Timezone: "Europe/Paris", Grid: customGrid(map[int]string{
		6: "000100000000000000000000",
	})}.Normalize()
	if !sc.ActiveAt(time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC)) {
		t.Fatal("01:30 UTC = 3 h 30 locale (heure d'été) : actif")
	}
	if sc.ActiveAt(time.Date(2026, 3, 29, 0, 30, 0, 0, time.UTC)) {
		t.Fatal("00:30 UTC = 1 h 30 locale : inactif")
	}
	// Le jour d'avant (heure d'hiver), 02:30 UTC = 3 h 30 locale... mais c'est un samedi
	if sc.ActiveAt(time.Date(2026, 3, 28, 2, 30, 0, 0, time.UTC)) {
		t.Fatal("samedi : inactif")
	}
}
