package core

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// affichage-objets-2026-11-006 : disposition des objets (gateway), sections « lights_layout »
// et « shutters_layout » du fichier des noms.

func TestLayout_catalogues(t *testing.T) {
	if n := len(LayoutLights.Catalogue()); n != 33 {
		t.Fatalf("lumières : %d", n)
	}
	if n := len(LayoutShutters.Catalogue()); n != 16 {
		t.Fatalf("volets : %d", n)
	}
}

func TestValidateLayout(t *testing.T) {
	ok := func(k LayoutKind, o, d []string) {
		t.Helper()
		if err := ValidateLayout(k, o, d); err != nil {
			t.Fatalf("%v %v : %v", o, d, err)
		}
	}
	ok(LayoutLights, []string{}, []string{})
	ok(LayoutLights, []string{"salon"}, []string{"wc2"}) // order incomplet accepté
	ok(LayoutShutters, []string{"volet2salon", "volet1salon"}, []string{"voletbureau", "store"})
	ok(LayoutLights, []string{}, LayoutLights.Catalogue()) // tout désactivé accepté
	if err := ValidateLayout(LayoutLights, []string{"salon", "salon"}, []string{}); err != ErrInvalidLayout {
		t.Fatalf("doublon dans order : %v", err)
	}
	if err := ValidateLayout(LayoutLights, []string{}, []string{"wc2", "wc2"}); err != ErrInvalidLayout {
		t.Fatalf("doublon dans disabled : %v", err)
	}
	if err := ValidateLayout(LayoutLights, []string{"volet1salon"}, []string{}); err != ErrUnknownLight {
		t.Fatalf("identifiant de volet refusé pour les lumières : %v", err)
	}
	if err := ValidateLayout(LayoutLights, []string{}, []string{"nimporte"}); err != ErrUnknownLight {
		t.Fatalf("%v", err)
	}
	if err := ValidateLayout(LayoutShutters, []string{}, []string{"salon"}); err != ErrUnknownShutter {
		t.Fatalf("identifiant de lumière refusé pour les volets : %v", err)
	}
	if err := ValidateLayout(LayoutLights, nil, []string{}); err != ErrInvalidLayout {
		t.Fatalf("clé order absente : %v", err)
	}
}

func TestEffectiveLayout(t *testing.T) {
	ordered, dis := EffectiveLayout(LayoutLights, ObjectLayout{Order: []string{"salon", "inconnu", "entree", "salon"}, Disabled: []string{"wc2", "inconnu"}})
	if ordered[0] != "salon" || ordered[1] != "entree" || ordered[2] != "terrasse" || len(ordered) != 33 {
		t.Fatalf("%v", ordered)
	}
	if !dis["wc2"] || len(dis) != 1 {
		t.Fatalf("%v", dis)
	}
	def, none := EffectiveLayout(LayoutShutters, ObjectLayout{})
	if !reflect.DeepEqual(def, LayoutShutters.Catalogue()) || len(none) != 0 {
		t.Fatal("défaut = ordre du catalogue")
	}
}

func TestLayoutStore_defautEtVersion(t *testing.T) {
	s, _ := newStore(t)
	l := s.Layout(LayoutLights)
	if l.Version != 0 || l.UpdatedAt != nil || l.Order == nil || len(l.Order) != 0 || l.Disabled == nil {
		t.Fatalf("%+v", l)
	}
	raw, _ := json.Marshal(l)
	if string(raw) != `{"order":[],"disabled":[],"version":0,"updated_at":null}` {
		t.Fatalf("%s", raw)
	}
}

func TestLayoutStore_ecritureRelectureEtReset(t *testing.T) {
	s, p := newStore(t)
	l, err := s.SetLayout(LayoutLights, []string{"salon", "entree"}, []string{"wc2"}, false, "admin-local", t0)
	if err != nil || l.Version != 1 || l.UpdatedAt == nil || *l.UpdatedAt != "2026-11-07T10:15:30.123Z" {
		t.Fatalf("%+v %v", l, err)
	}
	again := NewLightNameStore(p) // redémarrage
	got := again.Layout(LayoutLights)
	if !reflect.DeepEqual(got.Order, []string{"salon", "entree"}) || !reflect.DeepEqual(got.Disabled, []string{"wc2"}) || got.Version != 1 {
		t.Fatalf("%+v", got)
	}
	if again.Layout(LayoutShutters).Version != 0 {
		t.Fatal("les genres sont indépendants")
	}
	l, err = again.SetLayout(LayoutLights, nil, nil, true, "admin-local", t0.Add(time.Second))
	if err != nil || l.Version != 2 || len(l.Order) != 0 || len(l.Disabled) != 0 {
		t.Fatalf("reset : %+v %v", l, err)
	}
	if _, err := again.SetLayout(LayoutLights, []string{"volet1salon"}, []string{}, false, "a", t0); err != ErrUnknownLight {
		t.Fatalf("%v", err)
	}
	if again.Layout(LayoutLights).Version != 2 {
		t.Fatal("une écriture refusée ne change rien")
	}
	if string(readRaw(t, p)["version"]) != "1" {
		t.Fatal("version du fichier inchangée")
	}
}

// Fichier de #52 (sans section de disposition) : lecture par défaut, écriture sans toucher les noms.
func TestLayoutStore_fichierDeNomsRelutEtComplete(t *testing.T) {
	s, p := newStore(t)
	legacy := `{"version":1,"lights":{"dressing":{"name":"WC 3","version":2,"updated_at":"2026-11-07T10:15:30.123Z","updated_by":"nicolas@example.org"}},"shutters":{"store":{"name":"Banne","version":1,"updated_at":"2026-11-07T10:15:30.123Z","updated_by":"nicolas@example.org"}}}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if s.Layout(LayoutLights).Version != 0 {
		t.Fatal("défaut attendu")
	}
	before := readRaw(t, p)
	if _, err := s.SetLayout(LayoutShutters, []string{}, []string{"voletbureau"}, false, "a@example.org", t0); err != nil {
		t.Fatal(err)
	}
	after := readRaw(t, p)
	for _, k := range []string{"lights", "shutters", "version"} {
		if string(after[k]) != string(before[k]) {
			t.Fatalf("section %s altérée : %s -> %s", k, before[k], after[k])
		}
	}
	if _, ok := after["shutters_layout"]; !ok {
		t.Fatal("section shutters_layout ajoutée")
	}
	if _, ok := after["lights_layout"]; ok {
		t.Fatal("lights_layout ne doit pas être créée par une écriture de volets")
	}
}

// Préservation croisée : chaque écriture (nom de lumière, nom de volet, chaque disposition,
// fusion) laisse les autres sections intactes, section inconnue comprise.
func TestLayoutFile_ecrituresCroiseesPreserventTout(t *testing.T) {
	s, p := newStore(t)
	seed := `{"version":1,"lights":{},"shutters":{},"lights_layout":{"order":["salon"],"disabled":[],"version":3,"updated_at":"2026-11-07T08:00:00.000Z","updated_by":"x"},"shutters_layout":{"order":[],"disabled":["store"],"version":1,"updated_at":"2026-11-07T08:00:00.000Z","updated_by":"y"},"zones":{"a":{"x":1}},"note":"ajout manuel"}`
	if err := os.WriteFile(p, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := func(step string, except ...string) {
		t.Helper()
		var want, got map[string]json.RawMessage
		_ = json.Unmarshal([]byte(seedAfter[0]), &want)
		got = readRaw(t, p)
		for k, w := range want {
			skip := false
			for _, e := range except {
				skip = skip || e == k
			}
			if !skip && string(got[k]) != string(w) {
				t.Fatalf("%s : section %s altérée : %s -> %s", step, k, w, got[k])
			}
		}
		seedAfter[0] = string(mustMarshal(t, got))
	}
	seedAfter[0] = seed
	if _, err := s.Set("dressing", strp("WC 3"), "a@example.org", t0); err != nil {
		t.Fatal(err)
	}
	keep("nom de lumière", "lights")
	if _, err := s.SetShutter("store", strp("Banne"), "a@example.org", t0); err != nil {
		t.Fatal(err)
	}
	keep("nom de volet", "shutters")
	if _, err := s.SetLayout(LayoutLights, []string{"entree"}, []string{"wc2"}, false, "a@example.org", t0); err != nil {
		t.Fatal(err)
	}
	keep("disposition des lumières", "lights_layout")
	if _, err := s.SetLayout(LayoutShutters, []string{}, []string{}, true, "a@example.org", t0); err != nil {
		t.Fatal(err)
	}
	keep("disposition des volets", "shutters_layout")
	at := "2026-11-08T08:00:00.000Z"
	if ch, err := s.MergeLayout(LayoutLights, LayoutSyncDoc{Order: []string{"terrasse"}, Disabled: []string{}, Version: 9, UpdatedAt: &at, UpdatedBy: "cloud"}); err != nil || !ch {
		t.Fatalf("%v %v", ch, err)
	}
	keep("fusion des lumières", "lights_layout")
	if ch, err := s.MergeShutters([]ShutterNameView{{ID: "volet1salon", Name: strp("N"), Version: 1, UpdatedAt: &at}}); err != nil || !ch {
		t.Fatalf("%v %v", ch, err)
	}
	keep("fusion des noms de volets", "shutters")
	if got := s.Layout(LayoutLights); got.Version != 9 || got.Order[0] != "terrasse" {
		t.Fatalf("%+v", got)
	}
	if l := s.Layout(LayoutShutters); l.Version != 2 {
		t.Fatalf("le reset de volets est conservé à la version 2 : %+v", l)
	}
}

var seedAfter [1]string

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Les sections de disposition sont des objets : elles ne sont jamais décodées comme des
// tables de noms (une table de noms n'accepterait pas « order »: [...]).
func TestLayoutFile_sectionsObjetsPasTablesDeNoms(t *testing.T) {
	s, p := newStore(t)
	seed := `{"version":1,"lights_layout":{"order":["salon"],"disabled":["wc2"],"version":3,"updated_at":"2026-11-07T08:00:00.000Z","updated_by":"x"}}`
	_ = os.WriteFile(p, []byte(seed), 0o644)
	if got := s.List(); len(got) != 33 || got[0].Name != nil {
		t.Fatal("les noms ignorent les dispositions")
	}
	if l := s.Layout(LayoutLights); l.Version != 3 || l.Disabled[0] != "wc2" {
		t.Fatalf("%+v", l)
	}
}

func TestLayoutStore_stockageIndisponible(t *testing.T) {
	s := NewLightNameStore("")
	if l := s.Layout(LayoutLights); l.Version != 0 || len(l.Order) != 0 {
		t.Fatalf("%+v", l)
	}
	if _, err := s.SetLayout(LayoutLights, []string{}, []string{}, false, "a", t0); err != ErrLightNamesUnavailable {
		t.Fatalf("%v", err)
	}
	var nilStore *LightNameStore
	if nilStore.Layout(LayoutShutters).Version != 0 {
		t.Fatal("magasin nil : défaut")
	}
}

func TestLayoutStore_fichierCorrompuOuSectionIllisibleNonEcrase(t *testing.T) {
	s, p := newStore(t)
	for _, content := range []string{`{"version":1,"lights_layout":[1,2]`, `{"version":1,"lights_layout":[1,2]}`, `{"version":1,"lights_layout":"x"}`} {
		_ = os.WriteFile(p, []byte(content), 0o644)
		if _, err := s.SetLayout(LayoutLights, []string{}, []string{}, false, "a", t0); err != ErrLightNamesUnavailable {
			t.Fatalf("%s : %v", content, err)
		}
		if raw, _ := os.ReadFile(p); string(raw) != content {
			t.Fatalf("fichier écrasé : %s", raw)
		}
		if s.Layout(LayoutLights).Version != 0 {
			t.Fatal("lecture : défaut")
		}
	}
}

func TestLayoutStore_fusion(t *testing.T) {
	s, _ := newStore(t)
	at := func(x string) *string { return &x }
	doc := func(v int, when string, by string, dis ...string) LayoutSyncDoc {
		return LayoutSyncDoc{Order: []string{"salon"}, Disabled: append([]string{}, dis...), Version: v, UpdatedAt: at(when), UpdatedBy: by}
	}
	// version 0, sans date, date illisible : ignorés
	for _, d := range []LayoutSyncDoc{{Order: []string{}, Disabled: []string{}}, {Version: 1}, doc(1, "pas une date", "")} {
		if ch, err := s.MergeLayout(LayoutLights, d); ch || err != nil {
			t.Fatalf("%+v %v %v", d, ch, err)
		}
	}
	// le cloud adopté tel quel quand la gateway n'a rien
	if ch, _ := s.MergeLayout(LayoutLights, doc(2, "2026-11-07T10:00:00.000Z", "admin@example.org", "wc2", "inconnu", "wc2")); !ch {
		t.Fatal("adoption attendue")
	}
	if l := s.Layout(LayoutLights); !reflect.DeepEqual(l.Disabled, []string{"wc2"}) || l.Version != 2 {
		t.Fatalf("inconnus et doublons retirés : %+v", l)
	}
	if got := s.LayoutSyncPayload(LayoutLights); got.UpdatedBy != "admin@example.org" {
		t.Fatalf("%+v", got)
	}
	// plus ancien ou égal : local conservé
	if ch, _ := s.MergeLayout(LayoutLights, doc(5, "2026-11-07T09:00:00.000Z", "z")); ch {
		t.Fatal("un document plus ancien ne doit pas écraser")
	}
	if ch, _ := s.MergeLayout(LayoutLights, doc(2, "2026-11-07T10:00:00.000Z", "admin@example.org")); ch {
		t.Fatal("égalité : local conservé")
	}
	// même date, version plus grande
	if ch, _ := s.MergeLayout(LayoutLights, doc(3, "2026-11-07T10:00:00.000Z", "a")); !ch {
		t.Fatal("version supérieure")
	}
	// les volets ne bougent pas
	if s.Layout(LayoutShutters).Version != 0 {
		t.Fatal("genres indépendants")
	}
	if got := s.LayoutSyncPayload(LayoutShutters); got.Version != 0 || got.UpdatedAt != nil || len(got.Order) != 0 {
		t.Fatalf("%+v", got)
	}
	raw := mustMarshal(t, s.LayoutSyncPayload(LayoutLights))
	if !strings.Contains(string(raw), `"updated_by":"a"`) {
		t.Fatalf("la synchro porte updated_by : %s", raw)
	}
}
