package cloudsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

// noms-volets-2026-11-005 : synchronisation des noms des volets, indépendante de celle des lumières.

// fakeShutterCloud imite POST /api/gateway/shutters/names/sync : fusion par volet
// (updated_at, version) puis renvoi de l'état complet.
type fakeShutterCloud struct {
	mu       sync.Mutex
	state    map[string]core.ShutterNameView
	calls    int
	hold     chan struct{}
	received chan struct{}
}

func newFakeShutterCloud() *fakeShutterCloud {
	return &fakeShutterCloud{state: map[string]core.ShutterNameView{}, received: make(chan struct{}, 16)}
}

func (f *fakeShutterCloud) put(id string, name *string, version int, at string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state[id] = core.ShutterNameView{ID: id, Name: name, Version: version, UpdatedAt: &at}
}

func (f *fakeShutterCloud) get(id string) core.ShutterNameView {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state[id]
}

func (f *fakeShutterCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/api/gateway/shutters/names/sync" {
		http.NotFound(w, r)
		return
	}
	if f.hold != nil {
		<-f.hold
	}
	var in struct {
		Names []core.ShutterNameSyncItem `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.calls++
	for _, it := range in.Names {
		if it.UpdatedAt == nil || it.Version == 0 {
			continue
		}
		cur, ok := f.state[it.ID]
		if !ok || *it.UpdatedAt > *cur.UpdatedAt || (*it.UpdatedAt == *cur.UpdatedAt && it.Version > cur.Version) {
			f.state[it.ID] = core.ShutterNameView{ID: it.ID, Name: it.Name, Version: it.Version, UpdatedAt: it.UpdatedAt}
		}
	}
	shutters := make([]core.ShutterNameView, 0, 16)
	for _, info := range core.ShutterNames() {
		v := f.state[info.ID]
		v.ID, v.Default = info.ID, info.Default
		shutters = append(shutters, v)
	}
	f.mu.Unlock()
	f.received <- struct{}{}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"shutters": shutters})
}

func shutterAgent(t *testing.T, cloud http.Handler) (*Agent, *core.LightNameStore, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(cloud)
	t.Cleanup(srv.Close)
	store := core.NewLightNameStore(filepath.Join(t.TempDir(), "light-names.json"))
	a := &Agent{cfg: Config{Enabled: true, HubURL: srv.URL, GatewayID: "gw-1", GatewayToken: "secret"}, client: srv.Client()}
	a.SetLightNames(store)
	a.SetShutterNames(store)
	return a, store, srv
}

func shutterOf(s *core.LightNameStore, id string) core.ShutterNameView {
	v, _ := s.GetShutter(id)
	return v
}

func TestShutterNamesSync_cloudPlusRecent(t *testing.T) {
	cloud := newFakeShutterCloud()
	a, store, _ := shutterAgent(t, cloud)
	_, _ = store.SetShutter("volet1salon", sp("Local"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	cloud.put("volet1salon", sp("Volet baie vitrée"), 2, "2026-11-07T10:00:00.000Z")
	if err := a.syncShutterNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := shutterOf(store, "volet1salon"); v.Name == nil || *v.Name != "Volet baie vitrée" || v.Version != 2 {
		t.Fatalf("la réplique doit adopter le cloud : %+v", v)
	}
}

func TestShutterNamesSync_gatewayPlusRecente(t *testing.T) {
	cloud := newFakeShutterCloud()
	a, store, _ := shutterAgent(t, cloud)
	cloud.put("store", sp("Cloud"), 4, "2026-11-07T08:00:00.000Z")
	_, _ = store.SetShutter("store", sp("Local"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	if err := a.syncShutterNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := shutterOf(store, "store"); *v.Name != "Local" {
		t.Fatalf("un nom local plus récent ne doit pas être écrasé : %+v", v)
	}
	if v := cloud.get("store"); *v.Name != "Local" {
		t.Fatalf("le cloud doit recevoir le nom local : %+v", v)
	}
}

func TestShutterNamesSync_horsLigneEtRetour(t *testing.T) {
	cloud := newFakeShutterCloud()
	a, store, srv := shutterAgent(t, cloud)
	online := srv.URL
	a.cfg.HubURL = "https://127.0.0.1:1"
	_, _ = store.SetShutter("store", sp("Hors ligne"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	if err := a.syncShutterNames(context.Background()); err == nil {
		t.Fatal("erreur attendue hors ligne")
	}
	if v := shutterOf(store, "store"); v.Name == nil || *v.Name != "Hors ligne" || v.Version != 1 {
		t.Fatalf("la réplique locale doit rester intacte : %+v", v)
	}
	a.cfg.HubURL = online
	if err := a.syncShutterNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := cloud.get("store"); v.Name == nil || *v.Name != "Hors ligne" {
		t.Fatalf("le cloud doit recevoir la modification faite hors ligne : %+v", v)
	}
}

func TestShutterNamesSync_conflitLePlusRecentGagnePartout(t *testing.T) {
	cloud := newFakeShutterCloud()
	a, store, _ := shutterAgent(t, cloud)
	_, _ = store.SetShutter("volet1salon", sp("Gateway 1"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	cloud.put("volet1salon", sp("Cloud 1"), 1, "2026-11-07T09:00:01.000Z")
	_, _ = store.SetShutter("store", sp("Gateway 2"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 5, 0, time.UTC))
	cloud.put("store", sp("Cloud 2"), 1, "2026-11-07T09:00:00.000Z")
	if err := a.syncShutterNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *shutterOf(store, "volet1salon").Name != "Cloud 1" || *cloud.get("volet1salon").Name != "Cloud 1" {
		t.Fatal("volet1salon : le cloud doit gagner partout")
	}
	if *shutterOf(store, "store").Name != "Gateway 2" || *cloud.get("store").Name != "Gateway 2" {
		t.Fatal("store : la gateway doit gagner partout")
	}
}

func TestShutterNamesSync_remiseAZeroSePropage(t *testing.T) {
	cloud := newFakeShutterCloud()
	a, store, _ := shutterAgent(t, cloud)
	_, _ = store.SetShutter("store", sp("Nom"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	cloud.put("store", nil, 2, "2026-11-07T10:00:00.000Z")
	if err := a.syncShutterNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := shutterOf(store, "store"); v.Name != nil || v.Version != 2 {
		t.Fatalf("%+v", v)
	}
}

func TestShutterNamesSync_immediateNonBloquante(t *testing.T) {
	cloud := newFakeShutterCloud()
	cloud.hold = make(chan struct{})
	a, store, _ := shutterAgent(t, cloud)
	_, _ = store.SetShutter("store", sp("Vite"), "a@example.org", time.Now())
	done := make(chan struct{})
	go func() { a.TriggerShutterNamesSync(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TriggerShutterNamesSync ne doit pas bloquer sur un cloud lent")
	}
	close(cloud.hold)
	select {
	case <-cloud.received:
	case <-time.After(5 * time.Second):
		t.Fatal("la synchro immédiate n'a pas atteint le cloud")
	}
	if v := cloud.get("store"); v.Name == nil || *v.Name != "Vite" {
		t.Fatalf("%+v", v)
	}
}

func TestShutterNamesSync_inactifSansConfiguration(t *testing.T) {
	cloud := newFakeShutterCloud()
	a, _, _ := shutterAgent(t, cloud)
	a.cfg.Enabled = false
	a.TriggerShutterNamesSync()
	a2 := &Agent{cfg: Config{Enabled: true, HubURL: "http://insecure.example.org", GatewayID: "g", GatewayToken: "t"}}
	a2.TriggerShutterNamesSync()
	select {
	case <-cloud.received:
		t.Fatal("aucun appel attendu")
	case <-time.After(200 * time.Millisecond):
	}
}

// cloudMixte sert les lumières mais pas (encore) les volets : 404 sur la route des volets.
func TestShutterNamesSync_cloudSans404SurLesVolets(t *testing.T) {
	lights := newFakeNamesCloud()
	a, store, _ := shutterAgent(t, lights) // newFakeNamesCloud répond 404 sur la route des volets
	_, _ = store.Set("wc1", sp("Toilettes"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	_, _ = store.SetShutter("store", sp("Banne"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))

	a.syncNames(context.Background())

	if lights.get("wc1").Name == nil || *lights.get("wc1").Name != "Toilettes" {
		t.Fatal("les lumières doivent continuer de se synchroniser")
	}
	if v := shutterOf(store, "store"); v.Name == nil || *v.Name != "Banne" || v.Version != 1 {
		t.Fatalf("la réplique des volets doit rester utilisable : %+v", v)
	}
	if v, _ := store.Get("wc1"); *v.Name != "Toilettes" {
		t.Fatal("lumière perdue")
	}
}

// Un échec côté lumières ne bloque pas les volets.
func TestShutterNamesSync_echecDesLumieresNeBloquePasLesVolets(t *testing.T) {
	shutters := newFakeShutterCloud()
	a, store, _ := shutterAgent(t, shutters) // répond 404 sur les lumières
	_, _ = store.SetShutter("store", sp("Banne"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	a.syncNames(context.Background())
	if v := shutters.get("store"); v.Name == nil || *v.Name != "Banne" {
		t.Fatalf("les volets doivent se synchroniser : %+v", v)
	}
}

// Les deux synchronisations partagent le même fichier sans se marcher dessus.
func TestShutterNamesSync_lesDeuxSectionsDansLeMemeFichier(t *testing.T) {
	both := http.NewServeMux()
	lc, sc := newFakeNamesCloud(), newFakeShutterCloud()
	both.Handle("/api/gateway/lights/names/sync", lc)
	both.Handle("/api/gateway/shutters/names/sync", sc)
	a, store, _ := shutterAgent(t, both)
	lc.put("dressing", sp("WC 3"), 2, "2026-11-07T10:00:00.000Z")
	sc.put("volet1salon", sp("Volet baie vitrée"), 2, "2026-11-07T10:00:00.000Z")
	a.syncNames(context.Background())
	if v, _ := store.Get("dressing"); v.Name == nil || *v.Name != "WC 3" {
		t.Fatalf("%+v", v)
	}
	if v := shutterOf(store, "volet1salon"); v.Name == nil || *v.Name != "Volet baie vitrée" {
		t.Fatalf("%+v", v)
	}
}

// NR: NR-server-backend-31 essensys-hub/essensys-feature-lifecycle#41
// Les noms de volets se synchronisent dans les deux sens avec le cloud (le plus récent gagne,
// reprise après coupure) et un cloud qui répond 404 n'empêche pas la synchronisation des lumières.
func Test_NR_server_backend_31_synchro_des_noms_de_volets_avec_le_cloud(t *testing.T) {
	cloud := newFakeShutterCloud()
	a, store, srv := shutterAgent(t, cloud)
	cloud.put("volet1salon", sp("Du cloud"), 2, "2026-11-07T10:00:00.000Z")
	_, _ = store.SetShutter("store", sp("Locale"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	if err := a.syncShutterNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := shutterOf(store, "volet1salon"); v.Name == nil || *v.Name != "Du cloud" {
		t.Fatalf("cloud -> gateway : %+v", v)
	}
	if v := cloud.get("store"); v.Name == nil || *v.Name != "Locale" {
		t.Fatalf("gateway -> cloud : %+v", v)
	}
	// coupure puis retour
	online := srv.URL
	a.cfg.HubURL = "https://127.0.0.1:1"
	_, _ = store.SetShutter("voletbureau", sp("Hors ligne"), "a@example.org", time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC))
	if a.syncShutterNames(context.Background()) == nil {
		t.Fatal("erreur attendue hors ligne")
	}
	a.cfg.HubURL = online
	if err := a.syncShutterNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := cloud.get("voletbureau"); v.Name == nil || *v.Name != "Hors ligne" {
		t.Fatalf("reprise après coupure : %+v", v)
	}
	// 404 côté cloud : les lumières ne sont pas affectées
	lights := newFakeNamesCloud()
	a2, store2, _ := shutterAgent(t, lights)
	_, _ = store2.Set("wc1", sp("Toilettes"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	a2.syncNames(context.Background())
	if v := lights.get("wc1"); v.Name == nil || *v.Name != "Toilettes" {
		t.Fatal("les lumières doivent se synchroniser malgré le 404 des volets")
	}
}
