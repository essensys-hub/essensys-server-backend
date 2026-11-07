package cloudsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

// affichage-objets-2026-11-006 : synchronisation des dispositions, un appel par genre,
// indépendants de ceux des noms.

// fakeLayoutCloud imite POST /api/gateway/{lights|shutters}/layout/sync : fusion du document
// entier (updated_at, version, updated_by) puis renvoi de l'état fusionné.
type fakeLayoutCloud struct {
	mu       sync.Mutex
	state    map[string]core.LayoutSyncDoc // clé : genre
	calls    map[string]int
	auth     string
	hold     chan struct{}
	received chan string
}

func newFakeLayoutCloud() *fakeLayoutCloud {
	return &fakeLayoutCloud{state: map[string]core.LayoutSyncDoc{}, calls: map[string]int{}, received: make(chan string, 16)}
}

func (f *fakeLayoutCloud) put(kind string, order, disabled []string, version int, at, by string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state[kind] = core.LayoutSyncDoc{Order: order, Disabled: disabled, Version: version, UpdatedAt: &at, UpdatedBy: by}
}

func (f *fakeLayoutCloud) get(kind string) core.LayoutSyncDoc {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state[kind]
}

func (f *fakeLayoutCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var kind string
	switch r.URL.Path {
	case "/api/gateway/lights/layout/sync":
		kind = "lights"
	case "/api/gateway/shutters/layout/sync":
		kind = "shutters"
	}
	if r.Method != http.MethodPost || kind == "" {
		http.NotFound(w, r)
		return
	}
	if f.hold != nil {
		<-f.hold
	}
	var in core.LayoutSyncDoc
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.calls[kind]++
	f.auth = r.Header.Get("Authorization")
	cur, ok := f.state[kind]
	if in.Version > 0 && in.UpdatedAt != nil {
		if !ok || *in.UpdatedAt > *cur.UpdatedAt || (*in.UpdatedAt == *cur.UpdatedAt && in.Version > cur.Version) {
			f.state[kind] = in
			cur = in
		}
	}
	if cur.Order == nil {
		cur.Order, cur.Disabled = []string{}, []string{}
	}
	f.mu.Unlock()
	f.received <- kind
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cur)
}

func layoutAgent(t *testing.T, cloud http.Handler) (*Agent, *core.LightNameStore, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(cloud)
	t.Cleanup(srv.Close)
	store := core.NewLightNameStore(filepath.Join(t.TempDir(), "light-names.json"))
	a := &Agent{cfg: Config{Enabled: true, HubURL: srv.URL, GatewayID: "gw-1", GatewayToken: "secret"}, client: srv.Client()}
	a.SetLightNames(store)
	a.SetShutterNames(store)
	a.SetObjectLayout(store)
	return a, store, srv
}

var tl0 = time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)

func TestLayoutSync_cloudPlusRecent(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, store, _ := layoutAgent(t, cloud)
	_, _ = store.SetLayout(core.LayoutLights, []string{}, []string{"wc1"}, false, "a@example.org", tl0)
	cloud.put("lights", []string{"salon"}, []string{"wc2"}, 2, "2026-11-07T10:00:00.000Z", "admin@example.org")
	if err := a.syncLayout(context.Background(), core.LayoutLights); err != nil {
		t.Fatal(err)
	}
	if l := store.Layout(core.LayoutLights); l.Version != 2 || !reflect.DeepEqual(l.Disabled, []string{"wc2"}) || l.Order[0] != "salon" {
		t.Fatalf("la réplique doit adopter le cloud : %+v", l)
	}
	if cloud.auth != "Bearer secret" {
		t.Fatalf("authentification de la gateway : %q", cloud.auth)
	}
}

func TestLayoutSync_gatewayPlusRecente(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, store, _ := layoutAgent(t, cloud)
	cloud.put("shutters", []string{}, []string{"voletbureau"}, 4, "2026-11-07T08:00:00.000Z", "admin@example.org")
	_, _ = store.SetLayout(core.LayoutShutters, []string{"store"}, []string{"voletstore"}, false, "a@example.org", tl0)
	if err := a.syncLayout(context.Background(), core.LayoutShutters); err != nil {
		t.Fatal(err)
	}
	if l := store.Layout(core.LayoutShutters); !reflect.DeepEqual(l.Disabled, []string{"voletstore"}) || l.Version != 1 {
		t.Fatalf("un document local plus récent ne doit pas être écrasé : %+v", l)
	}
	if d := cloud.get("shutters"); !reflect.DeepEqual(d.Disabled, []string{"voletstore"}) || d.UpdatedBy != "a@example.org" {
		t.Fatalf("le cloud doit recevoir le document local, updated_by compris : %+v", d)
	}
}

func TestLayoutSync_premierContactEnvoieLeDocument(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, store, _ := layoutAgent(t, cloud)
	_, _ = store.SetLayout(core.LayoutLights, []string{"salon"}, []string{}, false, "a@example.org", tl0)
	if err := a.syncLayout(context.Background(), core.LayoutLights); err != nil {
		t.Fatal(err)
	}
	if d := cloud.get("lights"); d.Version != 1 || d.Order[0] != "salon" {
		t.Fatalf("%+v", d)
	}
}

func TestLayoutSync_defautSeSynchroniseSansEffet(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, store, _ := layoutAgent(t, cloud)
	if err := a.syncLayout(context.Background(), core.LayoutLights); err != nil {
		t.Fatal(err)
	}
	if cloud.calls["lights"] != 1 || store.Layout(core.LayoutLights).Version != 0 {
		t.Fatal("un appel est fait (pour recevoir), la réplique reste par défaut")
	}
}

func TestLayoutSync_horsLigneEtRetour(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, store, srv := layoutAgent(t, cloud)
	online := srv.URL
	a.cfg.HubURL = "https://127.0.0.1:1"
	_, _ = store.SetLayout(core.LayoutShutters, []string{}, []string{"store"}, false, "a@example.org", tl0)
	if err := a.syncLayout(context.Background(), core.LayoutShutters); err == nil {
		t.Fatal("erreur attendue hors ligne")
	}
	if l := store.Layout(core.LayoutShutters); l.Version != 1 || l.Disabled[0] != "store" {
		t.Fatalf("la réplique locale doit rester intacte : %+v", l)
	}
	a.cfg.HubURL = online
	if err := a.syncLayout(context.Background(), core.LayoutShutters); err != nil {
		t.Fatal(err)
	}
	if d := cloud.get("shutters"); d.Version != 1 || d.Disabled[0] != "store" {
		t.Fatalf("le cloud doit recevoir la modification faite hors ligne : %+v", d)
	}
}

func TestLayoutSync_conflitLePlusRecentGagnePartoutEtEntier(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, store, _ := layoutAgent(t, cloud)
	_, _ = store.SetLayout(core.LayoutLights, []string{"salon"}, []string{"wc1"}, false, "a@example.org", tl0)
	cloud.put("lights", []string{"entree"}, []string{"wc2"}, 1, "2026-11-07T09:00:01.000Z", "admin@example.org")
	_, _ = store.SetLayout(core.LayoutShutters, []string{"store"}, []string{"voletbureau"}, false, "a@example.org", tl0.Add(5*time.Second))
	cloud.put("shutters", []string{"voletstore"}, []string{"store"}, 1, "2026-11-07T09:00:00.000Z", "admin@example.org")
	a.syncLayouts(context.Background())
	l := store.Layout(core.LayoutLights)
	if l.Order[0] != "entree" || !reflect.DeepEqual(l.Disabled, []string{"wc2"}) {
		t.Fatalf("lumières : le cloud gagne, document entier : %+v", l)
	}
	d := cloud.get("shutters")
	if d.Order[0] != "store" || !reflect.DeepEqual(d.Disabled, []string{"voletbureau"}) {
		t.Fatalf("volets : la gateway gagne, document entier : %+v", d)
	}
}

func TestLayoutSync_immediateNonBloquantePourLeBonGenre(t *testing.T) {
	cloud := newFakeLayoutCloud()
	cloud.hold = make(chan struct{})
	a, store, _ := layoutAgent(t, cloud)
	_, _ = store.SetLayout(core.LayoutShutters, []string{}, []string{"store"}, false, "a@example.org", time.Now())
	done := make(chan struct{})
	go func() { a.TriggerShuttersLayoutSync(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TriggerShuttersLayoutSync ne doit pas bloquer sur un cloud lent")
	}
	close(cloud.hold)
	select {
	case k := <-cloud.received:
		if k != "shutters" {
			t.Fatalf("seul le genre modifié est synchronisé : %s", k)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la synchro immédiate n'a pas atteint le cloud")
	}
	if d := cloud.get("shutters"); d.Disabled[0] != "store" {
		t.Fatalf("%+v", d)
	}
}

func TestLayoutSync_inactifSansConfiguration(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, _, _ := layoutAgent(t, cloud)
	a.cfg.Enabled = false
	a.TriggerLightsLayoutSync()
	a.TriggerShuttersLayoutSync()
	a2 := &Agent{cfg: Config{Enabled: true, HubURL: "http://insecure.example.org", GatewayID: "g", GatewayToken: "t"}}
	a2.TriggerLightsLayoutSync()
	select {
	case <-cloud.received:
		t.Fatal("aucun appel attendu")
	case <-time.After(200 * time.Millisecond):
	}
}

// Un cloud qui répond 404 sur les dispositions (route pas encore déployée) ne perturbe ni les
// noms ni la réplique locale.
func TestLayoutSync_cloudSans404SurLesDispositions(t *testing.T) {
	mux := http.NewServeMux()
	lc, sc := newFakeNamesCloud(), newFakeShutterCloud()
	mux.Handle("/api/gateway/lights/names/sync", lc)
	mux.Handle("/api/gateway/shutters/names/sync", sc)
	a, store, _ := layoutAgent(t, mux)
	_, _ = store.Set("wc1", sp("Toilettes"), "a@example.org", tl0)
	_, _ = store.SetShutter("store", sp("Banne"), "a@example.org", tl0)
	_, _ = store.SetLayout(core.LayoutLights, []string{}, []string{"wc2"}, false, "a@example.org", tl0)
	a.syncNames(context.Background())
	a.syncLayouts(context.Background())
	if lc.get("wc1").Name == nil || sc.get("store").Name == nil {
		t.Fatal("les noms doivent continuer de se synchroniser")
	}
	if l := store.Layout(core.LayoutLights); l.Version != 1 || l.Disabled[0] != "wc2" {
		t.Fatalf("la réplique des dispositions doit rester utilisable : %+v", l)
	}
	if err := a.syncLayout(context.Background(), core.LayoutLights); err == nil {
		t.Fatal("le 404 est remonté comme erreur (journalisée), sans effet")
	}
}

// Un échec d'un genre ne bloque pas l'autre ; un échec des noms ne bloque pas les dispositions.
func TestLayoutSync_echecIndependantParGenre(t *testing.T) {
	cloud := newFakeLayoutCloud()
	mux := http.NewServeMux()
	mux.Handle("/api/gateway/shutters/layout/sync", cloud) // lumières : 404
	a, store, _ := layoutAgent(t, mux)
	_, _ = store.SetLayout(core.LayoutLights, []string{}, []string{"wc2"}, false, "a@example.org", tl0)
	_, _ = store.SetLayout(core.LayoutShutters, []string{}, []string{"store"}, false, "a@example.org", tl0)
	a.syncNames(context.Background()) // 404 partout côté noms
	a.syncLayouts(context.Background())
	if d := cloud.get("shutters"); d.Disabled[0] != "store" {
		t.Fatalf("les volets doivent se synchroniser malgré l'échec des lumières : %+v", d)
	}
	if l := store.Layout(core.LayoutLights); l.Disabled[0] != "wc2" {
		t.Fatalf("%+v", l)
	}
}

// Noms et dispositions partagent le même fichier sans se marcher dessus.
func TestLayoutSync_lesQuatreSynchrosDansLeMemeFichier(t *testing.T) {
	mux := http.NewServeMux()
	lc, sc, lay := newFakeNamesCloud(), newFakeShutterCloud(), newFakeLayoutCloud()
	mux.Handle("/api/gateway/lights/names/sync", lc)
	mux.Handle("/api/gateway/shutters/names/sync", sc)
	mux.Handle("/api/gateway/lights/layout/sync", lay)
	mux.Handle("/api/gateway/shutters/layout/sync", lay)
	a, store, _ := layoutAgent(t, mux)
	lc.put("dressing", sp("WC 3"), 2, "2026-11-07T10:00:00.000Z")
	sc.put("volet1salon", sp("Volet baie vitrée"), 2, "2026-11-07T10:00:00.000Z")
	lay.put("lights", []string{"salon"}, []string{"wc2"}, 3, "2026-11-07T10:00:00.000Z", "admin@example.org")
	lay.put("shutters", []string{}, []string{"store"}, 1, "2026-11-07T10:00:00.000Z", "admin@example.org")
	a.syncNames(context.Background())
	a.syncLayouts(context.Background())
	if v, _ := store.Get("dressing"); v.Name == nil || *v.Name != "WC 3" {
		t.Fatalf("%+v", v)
	}
	if v, _ := store.GetShutter("volet1salon"); v.Name == nil {
		t.Fatalf("%+v", v)
	}
	if store.Layout(core.LayoutLights).Version != 3 || store.Layout(core.LayoutShutters).Disabled[0] != "store" {
		t.Fatal("dispositions non adoptées")
	}
}

// NR: NR-server-backend-33 essensys-hub/essensys-feature-lifecycle#55
// Les dispositions se synchronisent dans les deux sens avec le cloud (le document le plus
// récent gagne en entier, reprise après coupure), indépendamment par genre, sans toucher aux
// noms de lumières et de volets (#41, #52) et malgré un cloud qui répond 404.
func Test_NR_server_backend_33_synchro_des_dispositions_avec_le_cloud(t *testing.T) {
	cloud := newFakeLayoutCloud()
	a, store, srv := layoutAgent(t, cloud)
	_, _ = store.Set("dressing", sp("WC 3"), "a@example.org", tl0)
	_, _ = store.SetShutter("store", sp("Banne"), "a@example.org", tl0)
	cloud.put("lights", []string{"salon"}, []string{"wc2"}, 2, "2026-11-07T10:00:00.000Z", "admin@example.org")
	_, _ = store.SetLayout(core.LayoutShutters, []string{}, []string{"store"}, false, "a@example.org", tl0)
	a.syncLayouts(context.Background())
	if l := store.Layout(core.LayoutLights); l.Version != 2 || l.Disabled[0] != "wc2" {
		t.Fatalf("cloud -> gateway : %+v", l)
	}
	if d := cloud.get("shutters"); d.Disabled[0] != "store" {
		t.Fatalf("gateway -> cloud : %+v", d)
	}
	// coupure puis retour
	online := srv.URL
	a.cfg.HubURL = "https://127.0.0.1:1"
	_, _ = store.SetLayout(core.LayoutLights, []string{"entree"}, []string{"wc1"}, false, "a@example.org", tl0.Add(2*time.Hour))
	if a.syncLayout(context.Background(), core.LayoutLights) == nil {
		t.Fatal("erreur attendue hors ligne")
	}
	a.cfg.HubURL = online
	a.syncLayouts(context.Background())
	if d := cloud.get("lights"); d.Order[0] != "entree" || d.Disabled[0] != "wc1" {
		t.Fatalf("reprise après coupure : %+v", d)
	}
	// 404 côté cloud : noms et réplique locale non affectés
	a2, store2, _ := layoutAgent(t, http.NotFoundHandler())
	_, _ = store2.Set("wc1", sp("Toilettes"), "a@example.org", tl0)
	_, _ = store2.SetLayout(core.LayoutLights, []string{}, []string{"wc2"}, false, "a@example.org", tl0)
	a2.syncLayouts(context.Background())
	if v, _ := store2.Get("wc1"); v.Name == nil || *v.Name != "Toilettes" {
		t.Fatal("les noms de lumières doivent rester inchangés")
	}
	if l := store2.Layout(core.LayoutLights); l.Version != 1 || l.Disabled[0] != "wc2" {
		t.Fatalf("la réplique doit rester utilisable : %+v", l)
	}
	// les noms n'ont pas bougé après toutes ces synchros
	if v, _ := store.Get("dressing"); v.Name == nil || *v.Name != "WC 3" || v.Version != 1 {
		t.Fatalf("noms de lumières (#41) modifiés : %+v", v)
	}
	if v, _ := store.GetShutter("store"); v.Name == nil || *v.Name != "Banne" || v.Version != 1 {
		t.Fatalf("noms de volets (#52) modifiés : %+v", v)
	}
}
