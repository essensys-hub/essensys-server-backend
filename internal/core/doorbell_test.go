package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/unifi"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// sonnette-unifi-2026-11-007 : réglages (section « doorbell ») et service de la sonnette.

const testDoorbellCam = "cam-sonnette-exemple"

func doorbellStore(t *testing.T) (*LightNameStore, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "light-names.json")
	return NewLightNameStore(p), p
}

// --- Réglages ---------------------------------------------------------------------------------

func TestDoorbellSettings_defauts(t *testing.T) {
	st, _ := doorbellStore(t)
	got := st.Doorbell()
	if got.Enabled || got.CameraID != "" || len(got.Lights) != 0 || got.Blinks != 5 || got.Version != 0 || got.UpdatedAt != nil {
		t.Fatalf("%+v", got)
	}
	if def := NewLightNameStore("").Doorbell(); def.Blinks != 5 || def.Enabled {
		t.Fatalf("sans stockage : %+v", def)
	}
}

func TestDoorbellSettings_ecritureEtRelectureApresRedemarrage(t *testing.T) {
	st, p := doorbellStore(t)
	at := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	saved, err := st.SetDoorbell(true, testDoorbellCam, []string{"salon", "entree"}, 3, "admin@example.org", at)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version != 1 || saved.Blinks != 3 || saved.UpdatedAt == nil || *saved.UpdatedAt != "2026-11-08T10:00:00.000Z" {
		t.Fatalf("%+v", saved)
	}
	again := NewLightNameStore(p).Doorbell() // « redémarrage » : nouveau magasin, même fichier
	if !reflect.DeepEqual(again, saved) {
		t.Fatalf("relecture : %+v au lieu de %+v", again, saved)
	}
	next, _ := st.SetDoorbell(false, testDoorbellCam, []string{}, 5, "admin@example.org", at)
	if next.Version != 2 || next.Enabled {
		t.Fatalf("%+v", next)
	}
}

func TestDoorbellSettings_validation(t *testing.T) {
	if err := ValidateDoorbellLights([]string{"salon", "nimporte"}); err != ErrUnknownLight {
		t.Fatalf("%v", err)
	}
	if err := ValidateDoorbellLights([]string{"volet1salon"}); err != ErrUnknownLight {
		t.Fatalf("un volet n'est pas une lumière : %v", err)
	}
	if err := ValidateDoorbellLights([]string{"salon", "salon"}); err != ErrUnknownLight {
		t.Fatalf("doublon : %v", err)
	}
	for _, n := range []int{0, 11, -1} {
		if ValidateDoorbellBlinks(n) != ErrInvalidBlinks {
			t.Fatalf("blinks=%d", n)
		}
	}
	for _, n := range []int{1, 5, 10} {
		if ValidateDoorbellBlinks(n) != nil {
			t.Fatalf("blinks=%d", n)
		}
	}
	st, _ := doorbellStore(t)
	if _, err := st.SetDoorbell(true, "c", []string{"nimporte"}, 3, "x", time.Now()); err != ErrUnknownLight {
		t.Fatalf("%v", err)
	}
	if _, err := st.SetDoorbell(true, "c", []string{}, 0, "x", time.Now()); err != ErrInvalidBlinks {
		t.Fatalf("%v", err)
	}
}

func TestDoorbellSettings_stockageIndisponibleEtSectionIllisible(t *testing.T) {
	if _, err := NewLightNameStore("").SetDoorbell(true, "c", []string{}, 3, "x", time.Now()); err != ErrLightNamesUnavailable {
		t.Fatalf("%v", err)
	}
	st, p := doorbellStore(t)
	raw := `{"version":1,"doorbell":"pas un objet"}`
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetDoorbell(true, "c", []string{}, 3, "x", time.Now()); err != ErrLightNamesUnavailable {
		t.Fatalf("section illisible : %v", err)
	}
	if got, _ := os.ReadFile(p); string(got) != raw {
		t.Fatal("une section illisible ne doit pas être écrasée")
	}
}

func TestDoorbellSettings_toutesLesSectionsCoexistent(t *testing.T) {
	st, p := doorbellStore(t)
	now := time.Now()
	name := "Salon TV"
	if _, err := st.Set("salon", &name, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetShutter("store", &name, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetLayout(LayoutLights, []string{"salon"}, []string{"wc2"}, false, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetLayout(LayoutShutters, []string{"store"}, []string{}, false, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	// section inconnue ajoutée à la main
	var doc map[string]json.RawMessage
	b, _ := os.ReadFile(p)
	_ = json.Unmarshal(b, &doc)
	doc["autre"] = json.RawMessage(`{"garde":true}`)
	b, _ = json.Marshal(doc)
	_ = os.WriteFile(p, b, 0o644)

	if _, err := st.SetDoorbell(true, testDoorbellCam, []string{"salon"}, 4, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	var got map[string]json.RawMessage
	_ = json.Unmarshal(after, &got)
	for _, k := range []string{"version", "lights", "shutters", "lights_layout", "shutters_layout", "autre", "doorbell"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("section %q perdue : %s", k, after)
		}
	}
	for _, k := range []string{"lights", "shutters", "lights_layout", "shutters_layout", "autre"} {
		if string(got[k]) != string(doc[k]) {
			t.Fatalf("section %q modifiée : %s au lieu de %s", k, got[k], doc[k])
		}
	}
	// et l'écriture d'une autre section préserve la sonnette
	if _, err := st.SetLayout(LayoutLights, []string{}, []string{}, true, "admin@example.org", now); err != nil {
		t.Fatal(err)
	}
	if st.Doorbell().Blinks != 4 || !st.Doorbell().Enabled {
		t.Fatal("la section doorbell doit survivre aux autres écritures")
	}
}

func TestLightCommands_toutLeCatalogueEstCommandable(t *testing.T) {
	for _, id := range LayoutLights.Catalogue() {
		c, ok := lightCommands[id]
		if !ok {
			t.Fatalf("lumière %q sans commande", id)
		}
		if c.On < 611 || c.On > 616 || c.Off != c.On-6 || c.Mask == 0 {
			t.Fatalf("%q : %+v", id, c)
		}
	}
	if len(lightCommands) != len(LayoutLights.Catalogue()) {
		t.Fatal("commandes hors catalogue")
	}
}

// --- Service ----------------------------------------------------------------------------------

type fakeWriter struct {
	mu     sync.Mutex
	blocks [][]protocol.ExchangeKV
	fail   bool
}

func (f *fakeWriter) Write(params []protocol.ExchangeKV) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return ErrDoorbellWrite
	}
	f.blocks = append(f.blocks, append([]protocol.ExchangeKV{}, params...))
	return nil
}

func (f *fakeWriter) all() [][]protocol.ExchangeKV {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]protocol.ExchangeKV{}, f.blocks...)
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type rig struct {
	svc *DoorbellService
	w   *fakeWriter
	clk *clock
	st  *LightNameStore
}

func newDoorbellRig(t *testing.T, lights []string, blinks int) *rig {
	t.Helper()
	st, _ := doorbellStore(t)
	if _, err := st.SetDoorbell(true, testDoorbellCam, lights, blinks, "admin@example.org", time.Now()); err != nil {
		t.Fatal(err)
	}
	w := &fakeWriter{}
	svc := NewDoorbellService(st, w)
	clk := &clock{t: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	svc.Now = clk.now
	svc.Sleep = func(time.Duration) {} // pas d'attente réelle
	t.Cleanup(svc.Close)
	return &rig{svc: svc, w: w, clk: clk, st: st}
}

func kv(k int, v string) protocol.ExchangeKV { return protocol.ExchangeKV{K: k, V: v} }

func TestDoorbell_sequenceCompleteFinitAllumee(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon", "entree"}, 2)
	res := r.svc.Ring(false)
	if !res.Sequence || res.Lights != 2 {
		t.Fatalf("%+v", res)
	}
	r.svc.WaitIdle()
	on := []protocol.ExchangeKV{kv(611, "1"), kv(612, "128")}
	off := []protocol.ExchangeKV{kv(605, "1"), kv(606, "128")}
	// 2 cycles éteint/allumé ; le dernier ordre allume
	want := [][]protocol.ExchangeKV{off, on, off, on}
	if got := r.w.all(); !reflect.DeepEqual(got, want) {
		t.Fatalf("blocs écrits :\n%v\nattendu :\n%v", got, want)
	}
	for _, b := range r.w.all() {
		for _, p := range b {
			if p.K < 605 || p.K > 616 {
				t.Fatalf("indice hors lumières : %d", p.K)
			}
		}
	}
}

func TestDoorbell_masquesCumulesParIndice(t *testing.T) {
	// terrasse 4, deg1 1, deg2 2 partagent 616/610
	r := newDoorbellRig(t, []string{"terrasse", "deg1", "deg2"}, 1)
	r.svc.Ring(false)
	r.svc.WaitIdle()
	got := r.w.all()
	if len(got) != 2 || !reflect.DeepEqual(got[0], []protocol.ExchangeKV{kv(610, "7")}) || !reflect.DeepEqual(got[1], []protocol.ExchangeKV{kv(616, "7")}) {
		t.Fatalf("%v", got)
	}
}

func TestDoorbell_rafaleUneSeuleSequence(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon"}, 1)
	r.svc.Ring(false)
	r.svc.WaitIdle()
	n := len(r.w.all())
	// trois pressions en 10 s : une seule séquence jouée
	for i := 0; i < 3; i++ {
		r.clk.add(3 * time.Second)
		if res := r.svc.Ring(false); res.Sequence {
			t.Fatalf("pression %d : pas de nouvelle séquence dans les 30 s", i)
		}
	}
	r.svc.WaitIdle()
	if len(r.w.all()) != n {
		t.Fatalf("ordres supplémentaires écrits : %d au lieu de %d", len(r.w.all()), n)
	}
	// la vidéo, elle, se réaffiche : ringing reste vrai
	if !r.svc.Status().Ringing {
		t.Fatal("ringing attendu")
	}
	// au-delà de 30 s une nouvelle séquence repart
	r.clk.add(31 * time.Second)
	if !r.svc.Ring(false).Sequence {
		t.Fatal("nouvelle séquence attendue après 30 s")
	}
	r.svc.WaitIdle()
}

func TestDoorbell_pressionPendantLaSequenceIgnoree(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon"}, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	r.svc.Sleep = func(time.Duration) { once.Do(func() { close(started) }); <-release }
	r.svc.Ring(false)
	<-started
	r.clk.add(40 * time.Second) // même hors fenêtre de 30 s, la séquence tourne encore
	if r.svc.Ring(false).Sequence {
		t.Fatal("une pression pendant une séquence est ignorée")
	}
	close(release)
	r.svc.WaitIdle()
}

func TestDoorbell_commandeManuellePendantLaSequence(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon", "entree"}, 2)
	calls := 0
	r.svc.Sleep = func(time.Duration) {
		calls++
		if calls == 2 { // pendant le clignotement, l'utilisateur éteint le salon à la main
			r.svc.NoteCommand([]protocol.ExchangeKV{kv(606, "128")})
		}
	}
	r.svc.Ring(false)
	r.svc.WaitIdle()
	blocks := r.w.all()
	// après la commande manuelle : plus aucun ordre pour le salon (612/606), entrée finit allumée
	for _, b := range blocks[2:] {
		for _, p := range b {
			if p.K == 612 || p.K == 606 {
				t.Fatalf("le salon commandé à la main ne doit plus recevoir d'ordre : %v", blocks)
			}
		}
	}
	last := blocks[len(blocks)-1]
	if !reflect.DeepEqual(last, []protocol.ExchangeKV{kv(611, "1")}) {
		t.Fatalf("dernier ordre : allumer l'entrée seule : %v", last)
	}
}

func TestDoorbell_commandeManuelleHorsSequenceOuAutreLumiere(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon"}, 1)
	r.svc.NoteCommand([]protocol.ExchangeKV{kv(606, "128")}) // hors séquence : sans effet
	calls := 0
	r.svc.Sleep = func(time.Duration) {
		calls++
		if calls == 1 {
			r.svc.NoteCommand([]protocol.ExchangeKV{kv(606, "64")})  // autre bit (sam), autre lumière
			r.svc.NoteCommand([]protocol.ExchangeKV{kv(617, "128")}) // volet, autre indice
		}
	}
	r.svc.Ring(false)
	r.svc.WaitIdle()
	if blocks := r.w.all(); len(blocks) != 2 { // off, on
		t.Fatalf("%v", blocks)
	}
}

func TestDoorbell_aucuneLumiereChoisie(t *testing.T) {
	r := newDoorbellRig(t, []string{}, 3)
	res := r.svc.Ring(false)
	r.svc.WaitIdle()
	if res.Sequence || len(r.w.all()) != 0 {
		t.Fatal("aucun ordre ne doit être écrit")
	}
	st := r.svc.Status()
	if !st.Ringing || st.LastRingAt == nil {
		t.Fatalf("la vidéo doit être signalée : %+v", st)
	}
}

func TestDoorbell_etatRingingSoixanteSecondes(t *testing.T) {
	r := newDoorbellRig(t, []string{}, 3)
	if r.svc.Status().Ringing {
		t.Fatal("pas de sonnerie au départ")
	}
	r.svc.Ring(false)
	st := r.svc.Status()
	if !st.Ringing || st.RingingUntil == nil || *st.RingingUntil != "2026-11-08T12:01:00.000Z" || *st.LastRingAt != "2026-11-08T12:00:00.000Z" {
		t.Fatalf("%+v", st)
	}
	r.clk.add(59 * time.Second)
	if !r.svc.Status().Ringing {
		t.Fatal("encore vrai à 59 s")
	}
	r.clk.add(2 * time.Second)
	if r.svc.Status().Ringing {
		t.Fatal("faux après 60 s")
	}
	if r.svc.Status().LastRingAt == nil {
		t.Fatal("last_ring_at est conservé")
	}
}

func TestDoorbell_essaiASecAucuneEcriture(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon", "entree"}, 3)
	res := r.svc.Ring(true)
	r.svc.WaitIdle()
	if !res.DryRun || !res.Sequence || res.Lights != 2 {
		t.Fatalf("%+v", res)
	}
	if len(r.w.all()) != 0 {
		t.Fatal("aucune écriture à sec")
	}
	if !r.svc.Status().Ringing {
		t.Fatal("ringing=true attendu")
	}
	// un essai à sec n'arme pas l'anti-rafale d'une vraie sonnerie
	if !r.svc.Ring(false).Sequence {
		t.Fatal("la vraie sonnerie doit pouvoir clignoter")
	}
	r.svc.WaitIdle()
}

func TestDoorbell_ecritureEnEchecArreteLaSequence(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon"}, 3)
	r.w.fail = true
	r.svc.Ring(false)
	r.svc.WaitIdle() // ne bloque pas
	if len(r.w.all()) != 0 {
		t.Fatal("rien d'écrit")
	}
	if !r.svc.Status().Ringing {
		t.Fatal("la sonnerie reste signalée")
	}
}

func TestDoorbell_HandleRingFiltreCameraEtActivation(t *testing.T) {
	r := newDoorbellRig(t, []string{"salon"}, 1)
	r.svc.HandleRing(unifi.RingEvent{CameraID: "autre-camera"})
	r.svc.WaitIdle()
	if r.svc.Status().Ringing || len(r.w.all()) != 0 {
		t.Fatal("une autre caméra ne déclenche rien")
	}
	r.svc.HandleRing(unifi.RingEvent{CameraID: testDoorbellCam})
	r.svc.WaitIdle()
	if !r.svc.Status().Ringing || len(r.w.all()) == 0 {
		t.Fatal("la caméra choisie déclenche")
	}
	// fonction désactivée
	r2 := newDoorbellRig(t, []string{"salon"}, 1)
	if _, err := r2.st.SetDoorbell(false, testDoorbellCam, []string{"salon"}, 1, "x", time.Now()); err != nil {
		t.Fatal(err)
	}
	r2.svc.HandleRing(unifi.RingEvent{CameraID: testDoorbellCam})
	if r2.svc.Status().Ringing {
		t.Fatal("désactivé : rien ne se passe")
	}
}

func TestDoorbell_disponibiliteSeulementSiActive(t *testing.T) {
	r := newDoorbellRig(t, []string{}, 1)
	if r.svc.Status().Available {
		t.Fatal("injoignable au départ")
	}
	r.svc.SetAvailable(true)
	if !r.svc.Status().Available || !r.svc.Status().Enabled {
		t.Fatal("disponible")
	}
	r.svc.SetAvailable(false)
	if r.svc.Status().Available {
		t.Fatal("console perdue")
	}
	off := NewDoorbellService(NewLightNameStore(""), nil)
	off.SetAvailable(true)
	if s := off.Status(); s.Available || s.Enabled {
		t.Fatalf("désactivé : %+v", s)
	}
}

func TestDoorbell_ObservateurDeCommandesDeLActionService(t *testing.T) {
	store := data.NewMemoryStore()
	as := NewActionService(store)
	r := newDoorbellRig(t, []string{"salon"}, 1)
	r.svc.mu.Lock()
	r.svc.running, r.svc.seqLights, r.svc.excluded = true, map[string]bool{"salon": true}, map[string]bool{}
	r.svc.mu.Unlock()
	as.SetCommandObserver(r.svc.NoteCommand)

	if _, err := as.AddActionsUnobserved("c1", []protocol.ExchangeKV{kv(606, "128")}); err != nil {
		t.Fatal(err)
	}
	if len(r.svc.active([]string{"salon"})) != 1 {
		t.Fatal("les ordres de la séquence ne l'excluent pas")
	}
	if _, err := as.AddActions("c1", []protocol.ExchangeKV{kv(606, "128")}); err != nil {
		t.Fatal(err)
	}
	if len(r.svc.active([]string{"salon"})) != 0 {
		t.Fatal("un ordre utilisateur exclut la lumière")
	}
	r.svc.mu.Lock()
	r.svc.running = false
	r.svc.mu.Unlock()
}

func TestExchangeDoorbellWriter_miseEnFile(t *testing.T) {
	store := data.NewMemoryStore()
	as := NewActionService(store)
	w := ExchangeDoorbellWriter{Actions: as, Store: store}
	if err := w.Write([]protocol.ExchangeKV{kv(612, "128")}); err != nil {
		t.Fatal(err)
	}
	acts := store.DequeueActions("default")
	if len(acts) != 1 {
		t.Fatalf("%d actions", len(acts))
	}
	got := map[int]string{}
	for _, p := range acts[0].Params {
		got[p.K] = p.V
	}
	if got[612] != "128" || got[605] != "0" || got[622] != "0" {
		t.Fatalf("bloc complet 605-622 attendu : %v", got)
	}
}

// --- Superviseur ------------------------------------------------------------------------------

type fakeSource struct {
	mu      sync.Mutex
	cameras []string
	running int
}

func (f *fakeSource) Run(ctx context.Context, cameraID string, onRing func(unifi.RingEvent), onState func(bool)) {
	f.mu.Lock()
	f.cameras = append(f.cameras, cameraID)
	f.running++
	f.mu.Unlock()
	onState(true)
	<-ctx.Done()
	f.mu.Lock()
	f.running--
	f.mu.Unlock()
}

func (f *fakeSource) snapshot() ([]string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.cameras...), f.running
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("délai dépassé : %s", what)
}

func TestDoorbellSupervisor_suitLesReglages(t *testing.T) {
	st, _ := doorbellStore(t)
	svc := NewDoorbellService(st, nil)
	src := &fakeSource{}
	sup := NewDoorbellSupervisor(svc, src)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sup.Start(ctx)
	if cams, _ := src.snapshot(); len(cams) != 0 {
		t.Fatal("désactivé : aucune écoute")
	}
	_, _ = st.SetDoorbell(true, "cam-a", []string{}, 3, "x", time.Now())
	sup.Reconfigure()
	waitFor(t, "écoute de cam-a", func() bool { c, r := src.snapshot(); return len(c) == 1 && r == 1 })
	waitFor(t, "disponible", func() bool { return svc.Status().Available })
	sup.Reconfigure() // sans changement : pas de redémarrage
	_, _ = st.SetDoorbell(true, "cam-b", []string{}, 3, "x", time.Now())
	sup.Reconfigure()
	waitFor(t, "écoute de cam-b", func() bool { c, r := src.snapshot(); return len(c) == 2 && c[1] == "cam-b" && r == 1 })
	_, _ = st.SetDoorbell(false, "cam-b", []string{}, 3, "x", time.Now())
	sup.Reconfigure()
	waitFor(t, "arrêt", func() bool { _, r := src.snapshot(); return r == 0 })
	sup.Stop()
}

func TestDoorbellSupervisor_UniFiDesactiveAucuneEcoute(t *testing.T) {
	st, _ := doorbellStore(t)
	_, _ = st.SetDoorbell(true, "cam-a", []string{}, 3, "x", time.Now())
	svc := NewDoorbellService(st, nil)
	sup := NewDoorbellSupervisor(svc, nil) // pas de source : UniFi désactivé
	sup.Start(context.Background())
	sup.Reconfigure()
	sup.Stop()
	if svc.Status().Available {
		t.Fatal("available=false sans UniFi")
	}
}
