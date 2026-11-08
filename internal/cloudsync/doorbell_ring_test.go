package cloudsync

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// sonnette-plages-push-2026-11-008 : signalement d'un appui au cloud (faux cloud, serveur de
// test local ; aucun vrai cloud).

const testRingToken = "jeton-exemple-secret"

func ringAgent(srv *httptest.Server) *Agent {
	a := NewAgent(Config{Enabled: true, HubURL: srv.URL, GatewayID: "gw-exemple", GatewayToken: testRingToken, Eth0MAC: "00:00:5e:00:53:01"}, nil, nil, nil)
	a.client = srv.Client()
	return a
}

func fastRetries(t *testing.T) {
	t.Helper()
	w, d := doorbellRingWaits, doorbellRingDeadline
	doorbellRingWaits = []time.Duration{time.Millisecond, 3 * time.Millisecond}
	t.Cleanup(func() { doorbellRingWaits, doorbellRingDeadline = w, d })
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestDoorbellRing_succes(t *testing.T) {
	fastRetries(t)
	var calls atomic.Int32
	var got map[string]string
	var auth, gw string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		auth, gw = r.Header.Get("Authorization"), r.Header.Get("X-Gateway-ID")
		if r.Method != http.MethodPost || r.URL.Path != "/api/gateway/doorbell/ring" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"status":"accepted","recipients":1}`))
	}))
	defer srv.Close()
	at := time.Date(2026, 11, 8, 10, 15, 30, 123000000, time.UTC)
	ringAgent(srv).NotifyDoorbellRing("evt-1", at)
	if calls.Load() != 1 || got["ring_id"] != "evt-1" || got["at"] != "2026-11-08T10:15:30.123Z" {
		t.Fatalf("%d %v", calls.Load(), got)
	}
	if auth != "Bearer "+testRingToken || gw != "gw-exemple" {
		t.Fatal("authentification gateway attendue")
	}
}

func TestDoorbellRing_injoignableTroisTentativesSansFuite(t *testing.T) {
	fastRetries(t)
	logs := captureLogs(t)
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	ringAgent(srv).NotifyDoorbellRing("evt-2", time.Now())
	if calls.Load() != 3 {
		t.Fatalf("3 tentatives attendues, %d", calls.Load())
	}
	if strings.Contains(logs.String(), testRingToken) || strings.Contains(logs.String(), host) {
		t.Fatalf("ni jeton ni adresse dans les journaux : %s", logs.String())
	}
}

func TestDoorbellRing_reseauCoupe(t *testing.T) {
	fastRetries(t)
	logs := captureLogs(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	a := ringAgent(srv)
	host := strings.TrimPrefix(srv.URL, "https://")
	srv.Close() // cloud injoignable
	a.NotifyDoorbellRing("evt", time.Now())
	if strings.Contains(logs.String(), testRingToken) || strings.Contains(logs.String(), host) {
		t.Fatalf("ni jeton ni adresse dans les journaux : %s", logs.String())
	}
}

func TestDoorbellRing_abandonApresLeDelai(t *testing.T) {
	fastRetries(t)
	doorbellRingDeadline = 30 * time.Millisecond
	doorbellRingWaits = []time.Duration{time.Hour, time.Hour}
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	start := time.Now()
	ringAgent(srv).NotifyDoorbellRing("evt", time.Now())
	if time.Since(start) > 2*time.Second || calls.Load() != 1 {
		t.Fatalf("abandon attendu après le délai (%d appels, %s)", calls.Load(), time.Since(start))
	}
}

func TestDoorbellRing_jetonRefuseOuLimiteSansInsister(t *testing.T) {
	fastRetries(t)
	for _, code := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
		var calls atomic.Int32
		code := code
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.WriteHeader(code)
		}))
		ringAgent(srv).NotifyDoorbellRing("evt", time.Now())
		srv.Close()
		if calls.Load() != 1 {
			t.Fatalf("code %d : %d appels", code, calls.Load())
		}
	}
}

func TestDoorbellRing_reprendApresUneErreurTemporaire(t *testing.T) {
	fastRetries(t)
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	ringAgent(srv).NotifyDoorbellRing("evt", time.Now())
	if calls.Load() != 3 {
		t.Fatalf("%d", calls.Load())
	}
}

func TestDoorbellRing_cloudNonConfigureAucunAppel(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer srv.Close()
	logs := captureLogs(t)
	for _, cfg := range []Config{
		{}, // aucune configuration
		{Enabled: false, HubURL: srv.URL, GatewayID: "g", GatewayToken: "t"},
		{Enabled: true, HubURL: srv.URL, GatewayID: "g"},                                       // sans jeton
		{Enabled: true, HubURL: "http://cloud.example.org", GatewayID: "g", GatewayToken: "t"}, // pas en HTTPS
	} {
		a := NewAgent(cfg, nil, nil, nil)
		a.client = srv.Client()
		a.NotifyDoorbellRing("evt", time.Now())
	}
	if calls.Load() != 0 || logs.Len() != 0 {
		t.Fatalf("aucun appel ni message attendu : %d %q", calls.Load(), logs.String())
	}
}
