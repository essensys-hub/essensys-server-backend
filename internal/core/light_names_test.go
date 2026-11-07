package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func strp(s string) *string { return &s }

func TestLightNames_catalogue33(t *testing.T) {
	want := "terrasse entree escalier deg1 deg2 pieceserv ann1 ann2 salon sam cuisine sdb1 sdb2 wc1 wc2 bureau gdchamb ptchamb1 ptchamb2 ptchamb3 dressing " +
		"isalonind isalonind2 icuisine isdb1 isdb2 igdchamb1 igdchamb2 iptchamb1 iptchamb2 iptchamb22 iptchamb3 idressing"
	var got []string
	for _, l := range LightNames() {
		got = append(got, l.ID)
		if l.Default == "" {
			t.Fatalf("%s sans nom d'origine", l.ID)
		}
	}
	if len(got) != 33 || strings.Join(got, " ") != want {
		t.Fatalf("catalogue inattendu (%d) : %v", len(got), got)
	}
	if d, _ := LightNameDefault("sam"); d != "Salle à Manger" {
		t.Fatalf("nom d'origine de sam : %q", d)
	}
	if d, _ := LightNameDefault("iptchamb2"); d != "Petite Chambre 1 (chevet 2)" {
		t.Fatalf("nom d'origine de iptchamb2 : %q", d)
	}
}

func TestValidateLightName(t *testing.T) {
	ok := []struct{ in, out string }{
		{"WC 3", "WC 3"}, {"  Salon  ", "Salon"}, {strings.Repeat("é", 40), strings.Repeat("é", 40)}, {"Cave à vin 🍷", "Cave à vin 🍷"},
	}
	for _, c := range ok {
		got, err := ValidateLightName(c.in)
		if err != nil || got == nil || *got != c.out {
			t.Fatalf("%q : %v %v", c.in, got, err)
		}
	}
	for _, in := range []string{"", "   "} {
		if got, err := ValidateLightName(in); err != nil || got != nil {
			t.Fatalf("%q doit remettre à zéro : %v %v", in, got, err)
		}
	}
	for _, in := range []string{strings.Repeat("a", 41), "<b>x</b>", "a>b", "a\x00b", "a\nb", "a\x7fb", "tab\tx"} {
		if _, err := ValidateLightName(in); err == nil {
			t.Fatalf("%q doit être refusé", in)
		}
	}
}

func newStore(t *testing.T) (*LightNameStore, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "light-names.json")
	return NewLightNameStore(p), p
}

var t0 = time.Date(2026, 11, 7, 10, 15, 30, 123000000, time.UTC)

func TestLightNameStore_relectureApresRedemarrage(t *testing.T) {
	s, p := newStore(t)
	v, err := s.Set("dressing", strp("WC 3"), "nicolas@example.org", t0)
	if err != nil || v.Version != 1 || v.UpdatedAt == nil || *v.UpdatedAt != "2026-11-07T10:15:30.123Z" {
		t.Fatalf("%+v %v", v, err)
	}
	v, _ = s.Set("dressing", strp("WC 4"), "nicolas@example.org", t0.Add(time.Second))
	if v.Version != 2 {
		t.Fatalf("version %d", v.Version)
	}
	s2 := NewLightNameStore(p)
	for _, l := range s2.List() {
		if l.ID == "dressing" && (l.Name == nil || *l.Name != "WC 4" || l.Version != 2 || l.Default != "Dressing") {
			t.Fatalf("%+v", l)
		}
		if l.ID == "salon" && (l.Name != nil || l.Version != 0 || l.UpdatedAt != nil) {
			t.Fatalf("salon doit rester vierge : %+v", l)
		}
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), `"version":1`) || !strings.Contains(string(raw), `"updated_by":"nicolas@example.org"`) {
		t.Fatalf("format du fichier : %s", raw)
	}
}

func TestLightNameStore_remiseAZeroIncrementeLaVersion(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.Set("wc1", strp("Toilettes"), "a@example.org", t0)
	v, err := s.Set("wc1", nil, "a@example.org", t0.Add(time.Second))
	if err != nil || v.Name != nil || v.Version != 2 || v.UpdatedAt == nil {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestLightNameStore_ecritureAtomique(t *testing.T) {
	s, p := newStore(t)
	_, _ = s.Set("wc1", strp("A"), "a@example.org", t0)
	_, _ = s.Set("wc2", strp("B"), "a@example.org", t0)
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("fichier temporaire résiduel : %v", entries)
	}
}

func TestLightNameStore_stockageIndisponible(t *testing.T) {
	for name, s := range map[string]*LightNameStore{
		"chemin vide":    NewLightNameStore(""),
		"dossier absent": NewLightNameStore(filepath.Join(t.TempDir(), "absent", "x.json")),
	} {
		if len(s.List()) != 33 {
			t.Fatalf("%s : lecture", name)
		}
		for _, l := range s.List() {
			if l.Name != nil || l.Default == "" {
				t.Fatalf("%s : %+v", name, l)
			}
		}
		if _, err := s.Set("wc1", strp("A"), "a@example.org", t0); err != ErrLightNamesUnavailable {
			t.Fatalf("%s : %v", name, err)
		}
	}
}

func TestLightNameStore_fichierCorrompuNonEcrase(t *testing.T) {
	s, p := newStore(t)
	_ = os.WriteFile(p, []byte("{pas du json"), 0o644)
	if len(s.List()) != 33 {
		t.Fatal("lecture")
	}
	if _, err := s.Set("wc1", strp("A"), "a@example.org", t0); err != ErrLightNamesUnavailable {
		t.Fatalf("%v", err)
	}
	if raw, _ := os.ReadFile(p); string(raw) != "{pas du json" {
		t.Fatal("le fichier corrompu ne doit pas être écrasé")
	}
}

func TestLightNameStore_identifiantInconnu(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Set("nimporte", strp("A"), "a@example.org", t0); err != ErrUnknownLight {
		t.Fatalf("%v", err)
	}
}

func remote(id string, name *string, version int, at string) LightNameView {
	v := LightNameView{ID: id, Name: name, Version: version}
	if at != "" {
		v.UpdatedAt = &at
	}
	return v
}

func TestLightNameStore_fusionCloudPlusRecent(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.Set("wc1", strp("Local"), "a@example.org", t0)
	changed, err := s.Merge([]LightNameView{remote("wc1", strp("Cloud"), 2, "2026-11-07T10:20:00.000Z")})
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	for _, l := range s.List() {
		if l.ID == "wc1" && (*l.Name != "Cloud" || l.Version != 2) {
			t.Fatalf("%+v", l)
		}
	}
}

func TestLightNameStore_fusionGatewayPlusRecenteConservee(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.Set("wc1", strp("Local"), "a@example.org", t0.Add(time.Hour))
	changed, _ := s.Merge([]LightNameView{remote("wc1", strp("Cloud"), 5, "2026-11-07T10:20:00.000Z")})
	if changed {
		t.Fatal("un nom local plus récent ne doit pas être écrasé")
	}
	for _, l := range s.List() {
		if l.ID == "wc1" && *l.Name != "Local" {
			t.Fatalf("%+v", l)
		}
	}
}

func TestLightNameStore_fusionEgaliteEtVersion(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.Set("wc1", strp("Local"), "a@example.org", t0)
	// même instant, version plus grande : le cloud gagne
	changed, _ := s.Merge([]LightNameView{remote("wc1", strp("Cloud"), 2, "2026-11-07T10:15:30.123Z")})
	if !changed {
		t.Fatal("version plus grande à instant égal doit gagner")
	}
	// égalité complète : rien ne change
	changed, _ = s.Merge([]LightNameView{remote("wc1", strp("Cloud"), 2, "2026-11-07T10:15:30.123Z")})
	if changed {
		t.Fatal("égalité : pas de changement")
	}
}

func TestLightNameStore_fusionRemiseAZeroEtIgnores(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.Set("wc1", strp("Local"), "a@example.org", t0)
	changed, _ := s.Merge([]LightNameView{
		remote("wc1", nil, 2, "2026-11-07T11:00:00.000Z"),           // remise à zéro propagée
		remote("inconnu", strp("X"), 1, "2026-11-07T11:00:00.000Z"), // ignoré
		remote("wc2", strp("<b>"), 1, "2026-11-07T11:00:00.000Z"),   // invalide, ignoré
		remote("wc3", nil, 0, ""),                                   // vierge, ignoré
	})
	if !changed {
		t.Fatal("changement attendu")
	}
	for _, l := range s.List() {
		if l.ID == "wc1" && (l.Name != nil || l.Version != 2) {
			t.Fatalf("%+v", l)
		}
		if l.ID == "wc2" && l.Name != nil {
			t.Fatal("nom invalide adopté")
		}
	}
}

func TestLightNameStore_syncPayloadEnvoieLes33(t *testing.T) {
	s, _ := newStore(t)
	_, _ = s.Set("wc1", strp("A"), "a@example.org", t0)
	items := s.SyncPayload()
	if len(items) != 33 || items[13].ID != "wc1" || items[13].Name == nil || items[13].Version != 1 || items[0].Version != 0 || items[0].UpdatedAt != nil {
		t.Fatalf("%d %+v", len(items), items[13])
	}
}

// NR: NR-server-backend-28 essensys-hub/essensys-feature-lifecycle#41
// Le catalogue des 33 lumières et la règle du nom (1 à 40 caractères, ni contrôle ni
// balise) doivent rester identiques à ceux du cloud et des clients.
func Test_NR_server_backend_28_catalogue_et_regle_du_nom_des_lumieres(t *testing.T) {
	if len(LightNames()) != 33 {
		t.Fatalf("%d lumières", len(LightNames()))
	}
	if _, err := ValidateLightName(strings.Repeat("a", 40)); err != nil {
		t.Fatal("40 caractères doit être accepté")
	}
	if _, err := ValidateLightName(strings.Repeat("a", 41)); err == nil {
		t.Fatal("41 caractères doit être refusé")
	}
	if _, err := ValidateLightName("<script>"); err == nil {
		t.Fatal("balise refusée")
	}
}
