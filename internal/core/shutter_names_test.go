package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// noms-volets-2026-11-005 : noms personnalisés des volets et stores, même fichier que les lumières.

func TestShutterNames_catalogue16(t *testing.T) {
	want := "volet1salon volet2salon volet3salon volet1salleamanger volet2salleamanger volet1cuisine volet2cuisine voletsdb " +
		"volet1gdchamb volet2gdchamb volet1ptchamb volet2ptchamb volet3ptchamb voletbureau voletstore store"
	var got []string
	for _, s := range ShutterNames() {
		got = append(got, s.ID)
		if s.Default == "" {
			t.Fatalf("%s sans nom d'origine", s.ID)
		}
	}
	if len(got) != 16 || strings.Join(got, " ") != want {
		t.Fatalf("catalogue inattendu (%d) : %v", len(got), got)
	}
	if d, _ := ShutterNameDefault("voletstore"); d != `Volet "Store"` {
		t.Fatalf("voletstore garde ses guillemets droits : %q", d)
	}
	if d, _ := ShutterNameDefault("store"); d != "Store (banne)" {
		t.Fatalf("store : %q", d)
	}
	if d, _ := ShutterNameDefault("volet1salleamanger"); d != "Volet 1 Salle à Manger" {
		t.Fatalf("%q", d)
	}
	if _, ok := ShutterNameDefault("dressing"); ok {
		t.Fatal("un identifiant de lumière n'est pas un volet")
	}
	if _, ok := LightNameDefault("volet1salon"); ok {
		t.Fatal("un identifiant de volet n'est pas une lumière")
	}
}

func TestValidateShutterName(t *testing.T) {
	if n, err := ValidateShutterName("  Volet baie vitrée "); err != nil || n == nil || *n != "Volet baie vitrée" {
		t.Fatalf("%v %v", n, err)
	}
	if n, err := ValidateShutterName(`Store "nord"`); err != nil || *n != `Store "nord"` {
		t.Fatal("les guillemets sont permis")
	}
	if n, err := ValidateShutterName("   "); err != nil || n != nil {
		t.Fatal("vide = remise à zéro")
	}
	if _, err := ValidateShutterName(strings.Repeat("é", 40)); err != nil {
		t.Fatal("40 caractères accepté")
	}
	for _, bad := range []string{strings.Repeat("a", 41), "<b>", "a>b", "a\nb", "a\x7fb", "a\x00b"} {
		if _, err := ValidateShutterName(bad); err != ErrInvalidLightName {
			t.Fatalf("%q devrait être refusé", bad)
		}
	}
}

func readRaw(t *testing.T, p string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]json.RawMessage
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("%s : %v", raw, err)
	}
	return d
}

func TestShutterNameStore_listeParDefaut(t *testing.T) {
	s, _ := newStore(t)
	list := s.ShutterList()
	if len(list) != 16 || list[0].ID != "volet1salon" || list[0].Name != nil || list[0].Version != 0 || list[0].UpdatedAt != nil || list[0].Default != "Volet 1 Salon" {
		t.Fatalf("%+v", list[0])
	}
}

func TestShutterNameStore_relectureApresRedemarrage(t *testing.T) {
	s, p := newStore(t)
	v, err := s.SetShutter("volet1salon", strp("Volet baie vitrée"), "nicolas@example.org", t0)
	if err != nil || v.Version != 1 || v.UpdatedAt == nil || *v.UpdatedAt != "2026-11-07T10:15:30.123Z" {
		t.Fatalf("%+v %v", v, err)
	}
	again := NewLightNameStore(p)
	got, _ := again.GetShutter("volet1salon")
	if got.Name == nil || *got.Name != "Volet baie vitrée" || got.Version != 1 {
		t.Fatalf("%+v", got)
	}
	// remise à zéro : version incrémentée, nom nul
	v, _ = again.SetShutter("volet1salon", nil, "nicolas@example.org", t0.Add(time.Second))
	if v.Name != nil || v.Version != 2 {
		t.Fatalf("%+v", v)
	}
	if _, err := again.SetShutter("dressing", strp("X"), "a@example.org", t0); err != ErrUnknownShutter {
		t.Fatalf("un identifiant de lumière doit être refusé : %v", err)
	}
}

// Le fichier produit par #41 (section « lights » seule) est relu, puis complété sans toucher « lights ».
func TestShutterNameStore_fichierDeLumieresRelutEtComplete(t *testing.T) {
	s, p := newStore(t)
	legacy := `{"version":1,"lights":{"dressing":{"name":"WC 3","version":2,"updated_at":"2026-11-07T10:15:30.123Z","updated_by":"nicolas@example.org"}}}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	list := s.ShutterList()
	if len(list) != 16 || list[0].Name != nil {
		t.Fatal("les 16 noms d'origine attendus")
	}
	if _, err := s.SetShutter("store", strp("Store terrasse"), "a@example.org", t0); err != nil {
		t.Fatal(err)
	}
	d := readRaw(t, p)
	if _, ok := d["shutters"]; !ok {
		t.Fatal("section shutters ajoutée")
	}
	var lights map[string]lightNameEntry
	_ = json.Unmarshal(d["lights"], &lights)
	if e := lights["dressing"]; e.Name == nil || *e.Name != "WC 3" || e.Version != 2 || e.UpdatedBy != "nicolas@example.org" {
		t.Fatalf("lights altérée : %+v", e)
	}
	if string(d["version"]) != "1" {
		t.Fatalf("version du fichier : %s", d["version"])
	}
}

// Écrire un volet puis une lumière (et inversement) ne perd rien, ni une section inconnue.
func TestNamesFile_ecrituresCroiseesPreserventTout(t *testing.T) {
	s, p := newStore(t)
	seed := `{"version":1,"lights":{},"zones":{"a":{"x":1}},"note":"ajout manuel"}`
	if err := os.WriteFile(p, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetShutter("volet1salon", strp("Volet baie vitrée"), "a@example.org", t0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set("dressing", strp("WC 3"), "a@example.org", t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetShutter("store", nil, "a@example.org", t0.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set("wc1", strp("Toilettes"), "a@example.org", t0.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	// relecture complète après « redémarrage »
	again := NewLightNameStore(p)
	if v, _ := again.GetShutter("volet1salon"); v.Name == nil || *v.Name != "Volet baie vitrée" || v.Version != 1 {
		t.Fatalf("volet perdu : %+v", v)
	}
	if v, _ := again.GetShutter("store"); v.Name != nil || v.Version != 1 {
		t.Fatalf("remise à zéro du store perdue : %+v", v)
	}
	if v, _ := again.Get("dressing"); v.Name == nil || *v.Name != "WC 3" {
		t.Fatalf("lumière perdue : %+v", v)
	}
	if v, _ := again.Get("wc1"); v.Name == nil || *v.Name != "Toilettes" {
		t.Fatalf("lumière perdue : %+v", v)
	}
	d := readRaw(t, p)
	if string(d["zones"]) != `{"a":{"x":1}}` || string(d["note"]) != `"ajout manuel"` {
		t.Fatalf("sections inconnues perdues : %s", d)
	}
}

func TestShutterNameStore_stockageIndisponible(t *testing.T) {
	for name, s := range map[string]*LightNameStore{
		"chemin vide":    NewLightNameStore(""),
		"dossier absent": NewLightNameStore(filepath.Join(t.TempDir(), "absent", "x.json")),
	} {
		list := s.ShutterList()
		if len(list) != 16 {
			t.Fatalf("%s : lecture", name)
		}
		for _, v := range list {
			if v.Name != nil || v.Default == "" {
				t.Fatalf("%s : %+v", name, v)
			}
		}
		if _, err := s.SetShutter("store", strp("A"), "a@example.org", t0); err != ErrLightNamesUnavailable {
			t.Fatalf("%s : %v", name, err)
		}
	}
	var nilStore *LightNameStore
	if len(nilStore.ShutterList()) != 16 {
		t.Fatal("magasin nil : noms d'origine")
	}
}

func TestShutterNameStore_fichierCorrompuNonEcrase(t *testing.T) {
	s, p := newStore(t)
	_ = os.WriteFile(p, []byte("{pas du json"), 0o644)
	if len(s.ShutterList()) != 16 {
		t.Fatal("lecture")
	}
	if _, err := s.SetShutter("store", strp("A"), "a@example.org", t0); err != ErrLightNamesUnavailable {
		t.Fatalf("%v", err)
	}
	if raw, _ := os.ReadFile(p); string(raw) != "{pas du json" {
		t.Fatal("le fichier corrompu ne doit pas être écrasé")
	}
}

func sremote(id string, name *string, version int, at string) ShutterNameView {
	return ShutterNameView(remote(id, name, version, at))
}

func TestShutterNameStore_fusion(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.SetShutter("volet1salon", strp("Local"), "a@example.org", t0)
	// cloud plus récent
	changed, err := s.MergeShutters([]ShutterNameView{sremote("volet1salon", strp("Cloud"), 2, "2026-11-07T10:20:00.000Z")})
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	if v, _ := s.GetShutter("volet1salon"); *v.Name != "Cloud" || v.Version != 2 {
		t.Fatalf("%+v", v)
	}
	// gateway plus récente conservée
	changed, _ = s.MergeShutters([]ShutterNameView{sremote("volet1salon", strp("Vieux"), 9, "2026-11-07T10:00:00.000Z")})
	if changed {
		t.Fatal("un nom cloud plus ancien ne doit pas écraser")
	}
	// égalité parfaite : rien ne change
	if changed, _ = s.MergeShutters([]ShutterNameView{sremote("volet1salon", strp("Cloud"), 2, "2026-11-07T10:20:00.000Z")}); changed {
		t.Fatal("égalité : pas de réécriture")
	}
	// remise à zéro propagée, entrées inconnues / vierges / invalides ignorées
	changed, _ = s.MergeShutters([]ShutterNameView{
		sremote("volet1salon", nil, 3, "2026-11-07T10:30:00.000Z"),
		sremote("dressing", strp("X"), 1, "2026-11-07T10:30:00.000Z"),
		sremote("store", nil, 0, ""),
		sremote("voletbureau", strp("<b>"), 1, "2026-11-07T10:30:00.000Z"),
	})
	if !changed {
		t.Fatal("changement attendu")
	}
	if v, _ := s.GetShutter("volet1salon"); v.Name != nil || v.Version != 3 {
		t.Fatalf("%+v", v)
	}
	if v, _ := s.GetShutter("voletbureau"); v.Name != nil {
		t.Fatal("nom invalide adopté")
	}
	if v, _ := s.Get("dressing"); v.Name != nil {
		t.Fatal("un identifiant de lumière ne doit rien créer")
	}
}

func TestShutterNameStore_syncPayloadEnvoieLes16(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.SetShutter("voletsdb", strp("A"), "a@example.org", t0)
	items := s.ShutterSyncPayload()
	if len(items) != 16 || items[7].ID != "voletsdb" || items[7].Name == nil || items[7].Version != 1 || items[0].Version != 0 || items[0].UpdatedAt != nil {
		t.Fatalf("%d %+v", len(items), items[7])
	}
}

// La fusion des volets ne touche pas la section des lumières (et inversement).
func TestShutterNameStore_fusionNeToucheQueLesVolets(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.Set("wc1", strp("Toilettes"), "a@example.org", t0)
	_, _ = s.MergeShutters([]ShutterNameView{sremote("store", strp("Banne"), 1, "2026-11-07T11:00:00.000Z")})
	_, _ = s.Merge([]LightNameView{remote("wc2", strp("WC bis"), 1, "2026-11-07T11:00:00.000Z")})
	if v, _ := s.Get("wc1"); v.Name == nil || *v.Name != "Toilettes" {
		t.Fatal("lumière perdue")
	}
	if v, _ := s.GetShutter("store"); v.Name == nil || *v.Name != "Banne" {
		t.Fatal("volet perdu après la fusion des lumières")
	}
}
