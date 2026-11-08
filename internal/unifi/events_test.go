package unifi

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"github.com/gorilla/websocket"
)

// Sonnette UniFi (sonnette-unifi-2026-11-007) : sources d'événements testées avec un faux
// serveur UniFi. Clé d'API et identifiants de caméra sont des exemples.

const (
	testKey    = "cle-api-exemple-123"
	testCamera = "cam-sonnette-exemple"
	otherCam   = "cam-autre-exemple"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

type fakeConsole struct {
	srv      *httptest.Server
	mu       sync.Mutex
	conns    []*websocket.Conn
	accepted atomic.Int32
	keyOK    atomic.Bool
	lastRing atomic.Int64
	pollFail atomic.Bool
}

func newFakeConsole(t *testing.T) *fakeConsole {
	t.Helper()
	f := &fakeConsole{}
	mux := http.NewServeMux()
	mux.HandleFunc(eventsPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == testKey {
			f.keyOK.Store(true)
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		f.accepted.Add(1)
		f.mu.Lock()
		f.conns = append(f.conns, c)
		f.mu.Unlock()
	})
	mux.HandleFunc(cameraPollPath, func(w http.ResponseWriter, r *http.Request) {
		if f.pollFail.Load() {
			http.Error(w, "indisponible", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + strings.TrimPrefix(r.URL.Path, cameraPollPath) + `","lastRing":` + itoa(f.lastRing.Load()) + `}`))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (f *fakeConsole) cfg() config.UniFiConfig {
	return config.UniFiConfig{Enabled: true, BaseURL: f.srv.URL, APIKey: testKey}
}

func (f *fakeConsole) send(t *testing.T, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.conns)
		var c *websocket.Conn
		if n > 0 {
			c = f.conns[n-1]
		}
		f.mu.Unlock()
		if c != nil {
			if err := c.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("aucune connexion reçue par la console")
}

func (f *fakeConsole) dropAll() {
	f.mu.Lock()
	for _, c := range f.conns {
		_ = c.Close()
	}
	f.conns = nil
	f.mu.Unlock()
}

type collector struct {
	mu     sync.Mutex
	rings  []RingEvent
	states []bool
}

func (c *collector) ring(e RingEvent) { c.mu.Lock(); c.rings = append(c.rings, e); c.mu.Unlock() }
func (c *collector) state(b bool)     { c.mu.Lock(); c.states = append(c.states, b); c.mu.Unlock() }
func (c *collector) nRings() int      { c.mu.Lock(); defer c.mu.Unlock(); return len(c.rings) }
func (c *collector) lastState() (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.states) == 0 {
		return false, false
	}
	return c.states[len(c.states)-1], true
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("délai dépassé : %s", what)
}

func ringMsg(typ, id, device string) string {
	return `{"type":"` + typ + `","item":{"id":"` + id + `","modelKey":"event","type":"ring","device":"` + device + `","start":1760000000000}}`
}

func TestStreamSource_sonnerieDeLaCameraChoisie(t *testing.T) {
	f := newFakeConsole(t)
	src := NewStreamSource(f.cfg())
	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	defer func() { cancel(); <-done }()

	f.send(t, ringMsg("add", "ev1", testCamera))
	eventually(t, "sonnerie reçue", func() bool { return col.nRings() == 1 })
	if !f.keyOK.Load() {
		t.Fatal("la clé d'API doit être envoyée dans l'en-tête X-API-Key")
	}
	if ok, known := col.lastState(); !known || !ok {
		t.Fatal("la source doit se déclarer connectée")
	}
	if col.rings[0].CameraID != testCamera {
		t.Fatalf("%+v", col.rings[0])
	}
}

func TestStreamSource_autreCameraEtAutresEvenementsIgnores(t *testing.T) {
	f := newFakeConsole(t)
	src := NewStreamSource(f.cfg())
	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	defer func() { cancel(); <-done }()

	f.send(t, ringMsg("add", "ev1", otherCam))
	f.send(t, `{"type":"add","item":{"id":"ev2","modelKey":"event","type":"motion","device":"`+testCamera+`"}}`)
	f.send(t, `pas du json`)
	f.send(t, ringMsg("add", "ev3", testCamera)) // témoin : sert de barrière
	eventually(t, "témoin reçu", func() bool { return col.nRings() >= 1 })
	time.Sleep(50 * time.Millisecond)
	if col.nRings() != 1 {
		t.Fatalf("une seule sonnerie attendue (la caméra choisie), reçu %d", col.nRings())
	}
}

func TestStreamSource_memeEvenementAddEtUpdateUneSeuleFois(t *testing.T) {
	f := newFakeConsole(t)
	src := NewStreamSource(f.cfg())
	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	defer func() { cancel(); <-done }()

	f.send(t, ringMsg("add", "ev1", testCamera))
	f.send(t, ringMsg("update", "ev1", testCamera))
	f.send(t, ringMsg("add", "ev2", testCamera))
	eventually(t, "deux sonneries", func() bool { return col.nRings() >= 2 })
	time.Sleep(50 * time.Millisecond)
	if col.nRings() != 2 {
		t.Fatalf("2 sonneries attendues, reçu %d", col.nRings())
	}
}

func TestStreamSource_reconnexionAutomatique(t *testing.T) {
	f := newFakeConsole(t)
	src := NewStreamSource(f.cfg())
	src.Backoff = Backoff{Min: 5 * time.Millisecond, Max: 20 * time.Millisecond}
	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	defer func() { cancel(); <-done }()

	eventually(t, "première connexion", func() bool { return f.accepted.Load() == 1 })
	f.dropAll()
	eventually(t, "reconnexion", func() bool { return f.accepted.Load() == 2 })
	f.send(t, ringMsg("add", "ev9", testCamera))
	eventually(t, "sonnerie après reconnexion", func() bool { return col.nRings() == 1 })
}

func TestStreamSource_consoleInjoignableAttenteCroissante(t *testing.T) {
	f := newFakeConsole(t)
	cfg := f.cfg()
	f.srv.Close() // console éteinte
	src := NewStreamSource(cfg)
	src.Backoff = Backoff{Min: 5 * time.Millisecond, Max: 20 * time.Millisecond}
	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	eventually(t, "indisponibilité signalée", func() bool { ok, known := col.lastState(); return known && !ok })
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run doit s'arrêter à l'annulation")
	}
}

func TestBackoff_croissantPuisPlafonneEtReinitialise(t *testing.T) {
	b := Backoff{}
	var got []time.Duration
	for i := 0; i < 9; i++ {
		got = append(got, b.Next())
	}
	want := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60, 60}
	for i, w := range want {
		if got[i] != w*time.Second {
			t.Fatalf("étape %d : %s au lieu de %ds", i, got[i], w)
		}
	}
	b.Reset()
	if b.Next() != time.Second {
		t.Fatal("réinitialisation attendue")
	}
}

func TestPollSource_nouvelleSonnerieApresLaReference(t *testing.T) {
	f := newFakeConsole(t)
	f.lastRing.Store(1760000000000) // sonnerie antérieure au démarrage : pas rejouée
	src := NewPollSource(f.cfg())
	src.Interval = 5 * time.Millisecond
	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	defer func() { cancel(); <-done }()

	eventually(t, "connecté", func() bool { ok, known := col.lastState(); return known && ok })
	time.Sleep(40 * time.Millisecond)
	if col.nRings() != 0 {
		t.Fatal("la sonnerie antérieure au démarrage ne doit pas être rejouée")
	}
	f.lastRing.Store(1760000005000)
	eventually(t, "nouvelle sonnerie", func() bool { return col.nRings() == 1 })
	time.Sleep(40 * time.Millisecond)
	if col.nRings() != 1 {
		t.Fatalf("une seule sonnerie attendue, reçu %d", col.nRings())
	}
}

func TestPollSource_consoleInjoignableEtReprise(t *testing.T) {
	f := newFakeConsole(t)
	src := NewPollSource(f.cfg())
	src.Interval = 5 * time.Millisecond
	src.Backoff = Backoff{Min: 5 * time.Millisecond, Max: 20 * time.Millisecond}
	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	defer func() { cancel(); <-done }()

	eventually(t, "connecté", func() bool { ok, known := col.lastState(); return known && ok })
	f.pollFail.Store(true)
	eventually(t, "indisponible", func() bool { ok, _ := col.lastState(); return !ok })
	f.pollFail.Store(false)
	eventually(t, "reconnecté", func() bool { ok, _ := col.lastState(); return ok })
	f.lastRing.Store(1760000009000)
	eventually(t, "sonnerie après reprise", func() bool { return col.nRings() == 1 })
}

// Aucune adresse de console, clé d'API ni identifiant de caméra dans les journaux.
func TestSources_aucuneAdresseNiCleDansLesJournaux(t *testing.T) {
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	f := newFakeConsole(t)
	cfg := f.cfg()
	host := strings.TrimPrefix(cfg.BaseURL, "http://")
	f.srv.Close()

	for _, src := range []EventSource{
		func() EventSource {
			s := NewStreamSource(cfg)
			s.Backoff = Backoff{Min: time.Millisecond, Max: 2 * time.Millisecond}
			return s
		}(),
		func() EventSource {
			s := NewPollSource(cfg)
			s.Interval = time.Millisecond
			s.Backoff = Backoff{Min: time.Millisecond, Max: 2 * time.Millisecond}
			return s
		}(),
	} {
		var col collector
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
		eventually(t, "échec de connexion journalisé", func() bool { ok, known := col.lastState(); return known && !ok })
		time.Sleep(30 * time.Millisecond)
		cancel()
		<-done
	}
	out := buf.String()
	if out == "" {
		t.Fatal("un message de reconnexion est attendu")
	}
	for _, secret := range []string{host, testKey, testCamera, "127.0.0.1", "http://", "ws://"} {
		if strings.Contains(out, secret) {
			t.Fatalf("le journal contient %q :\n%s", secret, out)
		}
	}
}

func TestStreamSource_repliParInterrogationSiFluxNonExpose(t *testing.T) {
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)

	var last atomic.Int64
	last.Store(1760000000000)
	mux := http.NewServeMux() // pas de route d'événements : 404
	mux.HandleFunc(cameraPollPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"lastRing":` + itoa(last.Load()) + `}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cfg := config.UniFiConfig{Enabled: true, BaseURL: srv.URL, APIKey: testKey}
	poll := NewPollSource(cfg)
	poll.Interval = 5 * time.Millisecond
	src := NewStreamSource(cfg)
	src.Fallback = poll

	var col collector
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { src.Run(ctx, testCamera, col.ring, col.state); close(done) }()
	defer func() { cancel(); <-done }()

	eventually(t, "connecté par interrogation", func() bool { ok, known := col.lastState(); return known && ok })
	last.Store(1760000007000)
	eventually(t, "sonnerie détectée par interrogation", func() bool { return col.nRings() == 1 })
	if strings.Contains(buf.String(), srv.URL) {
		t.Fatal("l'adresse de la console ne doit pas être journalisée")
	}
}
