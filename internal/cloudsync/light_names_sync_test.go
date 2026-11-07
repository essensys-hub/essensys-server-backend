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

// fakeNamesCloud imite POST /api/gateway/lights/names/sync : fusion par lumière
// (updated_at, version) puis renvoi de l'état complet.
type fakeNamesCloud struct {
	mu       sync.Mutex
	state    map[string]core.LightNameView
	calls    int
	auth     string
	gateID   string
	hold     chan struct{} // si non nil, la requête attend
	received chan struct{}
}

func newFakeNamesCloud() *fakeNamesCloud {
	return &fakeNamesCloud{state: map[string]core.LightNameView{}, received: make(chan struct{}, 16)}
}

func (f *fakeNamesCloud) put(id string, name *string, version int, at string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state[id] = core.LightNameView{ID: id, Name: name, Version: version, UpdatedAt: &at}
}

func (f *fakeNamesCloud) get(id string) core.LightNameView {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state[id]
}

func (f *fakeNamesCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/api/gateway/lights/names/sync" {
		http.NotFound(w, r)
		return
	}
	if f.hold != nil {
		<-f.hold
	}
	var in struct {
		Names []core.LightNameSyncItem `json:"names"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.calls++
	f.auth = r.Header.Get("Authorization")
	f.gateID = r.Header.Get("X-Gateway-ID")
	for _, it := range in.Names {
		if it.UpdatedAt == nil || it.Version == 0 {
			continue
		}
		cur, ok := f.state[it.ID]
		if !ok || *it.UpdatedAt > *cur.UpdatedAt || (*it.UpdatedAt == *cur.UpdatedAt && it.Version > cur.Version) {
			f.state[it.ID] = core.LightNameView{ID: it.ID, Name: it.Name, Version: it.Version, UpdatedAt: it.UpdatedAt}
		}
	}
	lights := make([]core.LightNameView, 0, 33)
	for _, info := range core.LightNames() {
		v := f.state[info.ID]
		v.ID, v.Default = info.ID, info.Default
		lights = append(lights, v)
	}
	f.mu.Unlock()
	f.received <- struct{}{}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lights": lights})
}

func namesAgent(t *testing.T, cloud http.Handler) (*Agent, *core.LightNameStore, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(cloud)
	t.Cleanup(srv.Close)
	store := core.NewLightNameStore(filepath.Join(t.TempDir(), "light-names.json"))
	a := &Agent{cfg: Config{Enabled: true, HubURL: srv.URL, GatewayID: "gw-1", GatewayToken: "secret"}, client: srv.Client()}
	a.SetLightNames(store)
	return a, store, srv
}

func sp(s string) *string { return &s }

func nameOf(s *core.LightNameStore, id string) core.LightNameView {
	v, _ := s.Get(id)
	return v
}

func TestLightNamesSync_cloudPlusRecent(t *testing.T) {
	cloud := newFakeNamesCloud()
	a, store, _ := namesAgent(t, cloud)
	_, _ = store.Set("dressing", sp("Local"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	cloud.put("dressing", sp("WC 3"), 2, "2026-11-07T10:00:00.000Z")

	if err := a.syncLightNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := nameOf(store, "dressing")
	if v.Name == nil || *v.Name != "WC 3" || v.Version != 2 {
		t.Fatalf("la réplique doit adopter le cloud : %+v", v)
	}
	if cloud.auth != "Bearer secret" || cloud.gateID != "gw-1" {
		t.Fatalf("en-têtes passerelle : %q %q", cloud.auth, cloud.gateID)
	}
}

func TestLightNamesSync_gatewayPlusRecente(t *testing.T) {
	cloud := newFakeNamesCloud()
	a, store, _ := namesAgent(t, cloud)
	cloud.put("wc1", sp("Cloud"), 4, "2026-11-07T08:00:00.000Z")
	_, _ = store.Set("wc1", sp("Local"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))

	if err := a.syncLightNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := nameOf(store, "wc1"); *v.Name != "Local" {
		t.Fatalf("un nom local plus récent ne doit pas être écrasé : %+v", v)
	}
	if v := cloud.get("wc1"); *v.Name != "Local" {
		t.Fatalf("le cloud doit recevoir le nom local : %+v", v)
	}
}

func TestLightNamesSync_horsLigneEtRetour(t *testing.T) {
	cloud := newFakeNamesCloud()
	a, store, srv := namesAgent(t, cloud)
	online := srv.URL
	a.cfg.HubURL = "https://127.0.0.1:1" // injoignable
	_, _ = store.Set("wc1", sp("Hors ligne"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))

	if err := a.syncLightNames(context.Background()); err == nil {
		t.Fatal("erreur attendue hors ligne")
	}
	if v := nameOf(store, "wc1"); v.Name == nil || *v.Name != "Hors ligne" || v.Version != 1 {
		t.Fatalf("la réplique locale doit rester intacte : %+v", v)
	}

	a.cfg.HubURL = online // la connexion revient
	if err := a.syncLightNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := cloud.get("wc1"); v.Name == nil || *v.Name != "Hors ligne" {
		t.Fatalf("le cloud doit recevoir la modification faite hors ligne : %+v", v)
	}
}

func TestLightNamesSync_erreursHTTPConserventLaReplique(t *testing.T) {
	status := http.StatusUnauthorized
	a, store, _ := namesAgent(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	_, _ = store.Set("wc1", sp("A"), "a@example.org", time.Now())
	for _, st := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		status = st
		if err := a.syncLightNames(context.Background()); err == nil {
			t.Fatalf("erreur attendue pour %d", st)
		}
	}
	if v := nameOf(store, "wc1"); *v.Name != "A" {
		t.Fatal("réplique modifiée")
	}
}

func TestLightNamesSync_conflitLePlusRecentGagnePartout(t *testing.T) {
	cloud := newFakeNamesCloud()
	a, store, _ := namesAgent(t, cloud)
	// Même lumière modifiée des deux côtés : le cloud est plus récent pour wc1, la gateway pour wc2.
	_, _ = store.Set("wc1", sp("Gateway 1"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	cloud.put("wc1", sp("Cloud 1"), 1, "2026-11-07T09:00:01.000Z")
	_, _ = store.Set("wc2", sp("Gateway 2"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 5, 0, time.UTC))
	cloud.put("wc2", sp("Cloud 2"), 1, "2026-11-07T09:00:00.000Z")

	if err := a.syncLightNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *nameOf(store, "wc1").Name != "Cloud 1" || *cloud.get("wc1").Name != "Cloud 1" {
		t.Fatal("wc1 : le cloud doit gagner partout")
	}
	if *nameOf(store, "wc2").Name != "Gateway 2" || *cloud.get("wc2").Name != "Gateway 2" {
		t.Fatal("wc2 : la gateway doit gagner partout")
	}
}

func TestLightNamesSync_remiseAZeroSePropage(t *testing.T) {
	cloud := newFakeNamesCloud()
	a, store, _ := namesAgent(t, cloud)
	_, _ = store.Set("wc1", sp("Nom"), "a@example.org", time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))
	cloud.put("wc1", nil, 2, "2026-11-07T10:00:00.000Z")
	if err := a.syncLightNames(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := nameOf(store, "wc1"); v.Name != nil || v.Version != 2 {
		t.Fatalf("%+v", v)
	}
}

func TestLightNamesSync_immediateNonBloquante(t *testing.T) {
	cloud := newFakeNamesCloud()
	cloud.hold = make(chan struct{})
	a, store, _ := namesAgent(t, cloud)
	_, _ = store.Set("wc1", sp("Vite"), "a@example.org", time.Now())

	done := make(chan struct{})
	go func() { a.TriggerLightNamesSync(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TriggerLightNamesSync ne doit pas bloquer sur un cloud lent")
	}
	close(cloud.hold)
	select {
	case <-cloud.received:
	case <-time.After(5 * time.Second):
		t.Fatal("la synchro immédiate n'a pas atteint le cloud")
	}
	if v := cloud.get("wc1"); v.Name == nil || *v.Name != "Vite" {
		t.Fatalf("%+v", v)
	}
}

func TestLightNamesSync_inactifSansConfiguration(t *testing.T) {
	cloud := newFakeNamesCloud()
	a, _, _ := namesAgent(t, cloud)
	a.cfg.Enabled = false
	a.TriggerLightNamesSync()
	a2 := &Agent{cfg: Config{Enabled: true, HubURL: "http://insecure.example.org", GatewayID: "g", GatewayToken: "t"}}
	a2.TriggerLightNamesSync() // pas de magasin, pas de HTTPS : ne fait rien, sans panique
	select {
	case <-cloud.received:
		t.Fatal("aucun appel attendu")
	case <-time.After(200 * time.Millisecond):
	}
}
