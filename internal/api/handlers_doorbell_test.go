package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/essensys-hub/essensys-server-backend/internal/unifi"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// sonnette-unifi-2026-11-007 : routes de la sonnette (gateway). Adresses, clés et identifiants
// de caméra sont des exemples ; aucune armoire réelle (table d'échange en mémoire).

const (
	camOK    = "cam-sonnette-exemple"
	camOther = "cam-autre-exemple"
)

type doorbellRig struct {
	router  http.Handler
	cookies map[string]*http.Cookie
	path    string
	store   data.Store
	names   *core.LightNameStore
	svc     *core.DoorbellService
	h       *DoorbellHandler

	consoleDown atomic.Bool
	snapshots   atomic.Int32
	changed     atomic.Int32
}

type memWriter struct {
	mu     sync.Mutex
	blocks [][]protocol.ExchangeKV
}

func (m *memWriter) Write(p []protocol.ExchangeKV) error {
	m.mu.Lock()
	m.blocks = append(m.blocks, p)
	m.mu.Unlock()
	return nil
}

func newDoorbellRig(t *testing.T, iamEnabled, withStorage bool) *doorbellRig {
	t.Helper()
	store := data.NewMemoryStore()
	rig := &doorbellRig{store: store, cookies: map[string]*http.Cookie{}}
	if withStorage {
		rig.path = filepath.Join(t.TempDir(), "light-names.json")
	}
	rig.names = core.NewLightNameStore(rig.path)
	as := core.NewActionService(store)
	// L'écrivain de la séquence est celui de production (file d'actions en mémoire) : les tests
	// vérifient que la file reste vide.
	rig.svc = core.NewDoorbellService(rig.names, core.ExchangeDoorbellWriter{Actions: as, Store: store}, core.ExchangeLightState{Store: store})
	rig.svc.Sleep = func(time.Duration) {}
	t.Cleanup(rig.svc.Close)
	rig.h = NewDoorbellHandler(rig.svc, rig.names)
	rig.h.Cameras = func() ([]unifi.Camera, error) {
		if rig.consoleDown.Load() {
			return nil, errors.New("console injoignable")
		}
		return []unifi.Camera{{ID: camOK, Name: "Sonnette"}, {ID: camOther, Name: "Jardin"}}, nil
	}
	rig.h.Snapshot = func(string) ([]byte, error) {
		rig.snapshots.Add(1)
		if rig.consoleDown.Load() {
			return nil, errors.New("console injoignable")
		}
		return []byte("\xff\xd8\xff\xe0JPEG-EXEMPLE"), nil
	}
	rig.h.OnSettingsChanged = func() { rig.changed.Add(1) }
	rig.h.StreamInterval = 5 * time.Millisecond

	h := NewHandler(as, core.NewStatusService(store), store, nil, nil, config.ArmoireConfig{})
	sessions := laniam.NewSessionStore(1)
	for i, role := range []string{models.LanRoleAdmin, models.LanRoleUser} {
		sess, err := sessions.CreateSession(&models.LanUser{ID: i + 1, Email: role + "@example.org", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		rig.cookies[role] = &http.Cookie{Name: middleware.LanSessionCookie, Value: sess.ID}
	}
	rig.router = NewRouter(RouterConfig{Handler: h, DoorbellHandler: rig.h, LanSessionStore: sessions, LanIAMEnabled: iamEnabled, Store: store})
	return rig
}

func (r *doorbellRig) do(method, path, body, role string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if c, ok := r.cookies[role]; ok {
		req.AddCookie(c)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	r.router.ServeHTTP(rec, req)
	return rec
}

func (r *doorbellRig) save(t *testing.T, body string) {
	t.Helper()
	if rec := r.do(http.MethodPut, "/api/admin/doorbell/settings", body, models.LanRoleAdmin); rec.Code != http.StatusOK {
		t.Fatalf("PUT : %d %s", rec.Code, rec.Body.String())
	}
}

func (r *doorbellRig) status(t *testing.T, role string) map[string]any {
	t.Helper()
	rec := r.do(http.MethodGet, "/api/doorbell/status", "", role)
	if rec.Code != http.StatusOK {
		t.Fatalf("status : %d %s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (r *doorbellRig) queueEmpty() bool {
	return len(r.store.DequeueActions("default")) == 0
}

const goodSettings = `{"enabled":true,"camera_id":"` + camOK + `","lights":["salon","entree"],"blinks":3}`

// --- Réglages ---------------------------------------------------------------------------------

func TestDoorbell_reglagesParDefaut(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rec := rig.do(http.MethodGet, "/api/admin/doorbell/settings", "", models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["enabled"] != false || m["camera_id"] != "" || m["blinks"] != float64(5) || m["version"] != float64(0) || m["updated_at"] != nil || len(m["lights"].([]any)) != 0 {
		t.Fatalf("%s", rec.Body.String())
	}
	// sans stockage : valeurs par défaut aussi
	rig = newDoorbellRig(t, true, false)
	if rig.do(http.MethodGet, "/api/admin/doorbell/settings", "", models.LanRoleAdmin).Code != http.StatusOK {
		t.Fatal("lecture sans stockage : valeurs par défaut")
	}
}

func TestDoorbell_enregistrementEtRelectureApresRedemarrage(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", goodSettings, models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["status"] != "saved" || m["version"] != float64(1) || m["blinks"] != float64(3) || m["enabled"] != true || m["camera_id"] != camOK || m["updated_at"] == nil {
		t.Fatalf("%s", rec.Body.String())
	}
	if rig.changed.Load() != 1 {
		t.Fatal("l'écoute doit être reconfigurée après un enregistrement")
	}
	restarted := newDoorbellRig(t, true, true)
	restarted.names = core.NewLightNameStore(rig.path) // même fichier, nouveau magasin
	got := restarted.names.Doorbell()
	if !got.Enabled || got.CameraID != camOK || got.Blinks != 3 || len(got.Lights) != 2 || got.Version != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestDoorbell_valeursRefusees(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	cases := []struct{ body, key string }{
		{`{"enabled":true,"camera_id":"` + camOK + `","lights":["nimporte"],"blinks":3}`, "unknown_light"},
		{`{"enabled":true,"camera_id":"` + camOK + `","lights":["volet1salon"],"blinks":3}`, "unknown_light"},
		{`{"enabled":true,"camera_id":"` + camOK + `","lights":["salon"],"blinks":0}`, "invalid_blinks"},
		{`{"enabled":true,"camera_id":"` + camOK + `","lights":["salon"],"blinks":11}`, "invalid_blinks"},
		{`{"enabled":true,"camera_id":"` + camOK + `","lights":["salon"]}`, "invalid_blinks"},
		{`{"enabled":true,"camera_id":"camera-inconnue","lights":["salon"],"blinks":3}`, "unknown_camera"},
		{`{"enabled":true,"camera_id":"","lights":["salon"],"blinks":3}`, "unknown_camera"},
		{`pas du json`, "invalid_settings"},
		{`{"enabled":true,"camera_id":"` + camOK + `","blinks":3}`, "invalid_settings"},
	}
	for _, c := range cases {
		rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", c.body, models.LanRoleAdmin)
		if rec.Code != http.StatusBadRequest || errKey(t, rec) != c.key {
			t.Fatalf("%s : %d %s (attendu %s)", c.body, rec.Code, rec.Body.String(), c.key)
		}
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("rien ne doit être écrit sur une valeur refusée")
	}
	// désactivée et sans caméra : acceptée
	rig.save(t, `{"enabled":false,"camera_id":"","lights":[],"blinks":5}`)
}

func TestDoorbell_droits(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/admin/doorbell/settings", goodSettings},
		{http.MethodGet, "/api/admin/doorbell/settings", ""},
		{http.MethodPost, "/api/admin/doorbell/test", ""},
	} {
		if rec := rig.do(c.method, c.path, c.body, models.LanRoleUser); rec.Code != http.StatusForbidden {
			t.Fatalf("lan_user %s %s : %d", c.method, c.path, rec.Code)
		}
		if rec := rig.do(c.method, c.path, c.body, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("sans session %s %s : %d", c.method, c.path, rec.Code)
		}
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("aucun fichier écrit par un refus")
	}
}

func TestDoorbell_stockageIndisponible(t *testing.T) {
	rig := newDoorbellRig(t, true, false)
	rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", goodSettings, models.LanRoleAdmin)
	if rec.Code != http.StatusServiceUnavailable || errKey(t, rec) != "names_unavailable" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestDoorbell_consoleInjoignablePourValiderLaCamera(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rig.consoleDown.Store(true)
	rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", goodSettings, models.LanRoleAdmin)
	if rec.Code != http.StatusServiceUnavailable || errKey(t, rec) != "doorbell_unavailable" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rig.h.Cameras = nil // UniFi désactivé
	rec = rig.do(http.MethodPut, "/api/admin/doorbell/settings", goodSettings, models.LanRoleAdmin)
	if rec.Code != http.StatusServiceUnavailable || errKey(t, rec) != "doorbell_unavailable" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestDoorbell_autresSectionsPreservees(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	now := time.Now()
	n := "Salon TV"
	_, _ = rig.names.Set("salon", &n, "admin@example.org", now)
	_, _ = rig.names.SetShutter("store", &n, "admin@example.org", now)
	_, _ = rig.names.SetLayout(core.LayoutLights, []string{"salon"}, []string{"wc2"}, false, "admin@example.org", now)
	_, _ = rig.names.SetLayout(core.LayoutShutters, []string{"store"}, []string{}, false, "admin@example.org", now)
	before := readSections(t, rig.path)
	rig.save(t, goodSettings)
	after := readSections(t, rig.path)
	for _, k := range []string{"lights", "shutters", "lights_layout", "shutters_layout"} {
		if before[k] == "" || before[k] != after[k] {
			t.Fatalf("section %q modifiée ou absente", k)
		}
	}
	if after["doorbell"] == "" {
		t.Fatal("section doorbell absente")
	}
}

func readSections(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for k, v := range raw {
		out[k] = string(v)
	}
	return out
}

func TestDoorbell_reglagesASec(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rec := rig.do(http.MethodPut, "/api/admin/doorbell/settings", goodSettings, models.LanRoleAdmin, "X-Essensys-Test-Mode", "dry-run")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"dry_run"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(rig.path); err == nil {
		t.Fatal("rien d'écrit à sec")
	}
}

// --- État ---------------------------------------------------------------------------------------

func TestDoorbell_etatUtilisateurAuthentifieSansSessionEtIAMDesactive(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	for _, role := range []string{models.LanRoleUser, models.LanRoleAdmin} {
		st := rig.status(t, role)
		if st["enabled"] != false || st["available"] != false || st["ringing"] != false || st["ringing_until"] != nil || st["last_ring_at"] != nil {
			t.Fatalf("%s : %+v", role, st)
		}
	}
	if rig.do(http.MethodGet, "/api/doorbell/status", "", "").Code != http.StatusUnauthorized {
		t.Fatal("sans session : 401")
	}
	if rig.do(http.MethodGet, "/api/doorbell/stream", "", "").Code != http.StatusUnauthorized {
		t.Fatal("flux sans session : 401")
	}
	open := newDoorbellRig(t, false, true)
	if open.do(http.MethodGet, "/api/doorbell/status", "", "").Code != http.StatusOK {
		t.Fatal("IAM désactivé : ouvert")
	}
}

func TestDoorbell_sonnerieVisibleDansLEtat(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rig.save(t, `{"enabled":true,"camera_id":"`+camOK+`","lights":[],"blinks":3}`)
	rig.svc.SetAvailable(true)
	rig.svc.HandleRing(unifi.RingEvent{CameraID: camOK})
	st := rig.status(t, models.LanRoleUser)
	if st["enabled"] != true || st["available"] != true || st["ringing"] != true || st["last_ring_at"] == nil || st["ringing_until"] == nil {
		t.Fatalf("%+v", st)
	}
	rig.svc.HandleRing(unifi.RingEvent{CameraID: camOther}) // autre caméra : sans effet sur le dernier événement
	if rig.status(t, models.LanRoleUser)["last_ring_at"] != st["last_ring_at"] {
		t.Fatal("une autre caméra ne change rien")
	}
}

// --- Flux ---------------------------------------------------------------------------------------

func TestDoorbell_fluxInstantanes(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rig.save(t, goodSettings)
	srv := httptest.NewServer(rig.router)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/doorbell/stream", nil)
	req.AddCookie(rig.cookies[models.LanRoleUser])
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal(resp.StatusCode)
	}
	mt, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/x-mixed-replace" {
		t.Fatalf("%q %v", resp.Header.Get("Content-Type"), err)
	}
	mr := multipart.NewReader(bufio.NewReader(resp.Body), params["boundary"])
	for i := 0; i < 2; i++ {
		part, err := mr.NextPart()
		if err != nil {
			t.Fatalf("image %d : %v", i, err)
		}
		b, _ := io.ReadAll(part)
		if part.Header.Get("Content-Type") != "image/jpeg" || !strings.HasPrefix(string(b), "\xff\xd8") {
			t.Fatalf("image %d invalide", i)
		}
	}
	if rig.snapshots.Load() < 2 {
		t.Fatal("un instantané par tour attendu")
	}
}

func TestDoorbell_fluxConsoleInjoignable(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rig.save(t, goodSettings)
	rig.consoleDown.Store(true)
	rec := rig.do(http.MethodGet, "/api/doorbell/stream", "", models.LanRoleUser)
	if rec.Code != http.StatusServiceUnavailable || errKey(t, rec) != "doorbell_unavailable" {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "injoignable") || strings.Contains(body, camOK) {
		t.Fatalf("la réponse ne doit rien révéler de la console : %s", body)
	}
	// le reste de la gateway fonctionne
	if rig.do(http.MethodGet, "/api/doorbell/status", "", models.LanRoleUser).Code != http.StatusOK {
		t.Fatal("l'état reste disponible")
	}
	// fonction désactivée ou UniFi absent : même réponse
	rig.consoleDown.Store(false)
	rig.save(t, `{"enabled":false,"camera_id":"","lights":[],"blinks":5}`)
	if rig.do(http.MethodGet, "/api/doorbell/stream", "", models.LanRoleUser).Code != http.StatusServiceUnavailable {
		t.Fatal("désactivé : 503")
	}
	rig.h.Snapshot = nil
	if rig.do(http.MethodGet, "/api/doorbell/stream", "", models.LanRoleUser).Code != http.StatusServiceUnavailable {
		t.Fatal("UniFi désactivé : 503")
	}
}

// --- Essai --------------------------------------------------------------------------------------

func TestDoorbell_essaiASecAucuneEcritureDansLArmoire(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rig.save(t, goodSettings)
	rec := rig.do(http.MethodPost, "/api/admin/doorbell/test", "", models.LanRoleAdmin, "X-Essensys-Test-Mode", "dry-run")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["status"] != "dry_run" || m["ringing"] != true || m["lights"] != float64(2) {
		t.Fatalf("%+v", m)
	}
	rig.svc.WaitIdle()
	if !rig.queueEmpty() {
		t.Fatal("aucun ordre ne doit être écrit à sec")
	}
	if rig.status(t, models.LanRoleUser)["ringing"] != true {
		t.Fatal("ringing=true attendu après l'essai")
	}
}

func TestDoorbell_essaiReelEcritDansLaFile(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	rig.save(t, goodSettings)
	rec := rig.do(http.MethodPost, "/api/admin/doorbell/test", "", models.LanRoleAdmin)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"started"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	rig.svc.WaitIdle()
	if rig.queueEmpty() {
		t.Fatal("un essai sans dry-run écrit les ordres de clignotement (file en mémoire)")
	}
}

// NR: NR-server-backend-34 essensys-hub/essensys-feature-lifecycle#58
// La sonnette UniFi : réglages et essai réservés à lan_admin, état et flux lisibles par tout
// utilisateur authentifié, stockage dans la section « doorbell » sans toucher aux autres
// sections, aucun ordre écrit dans l'armoire à sec, aucun endpoint legacy modifié, aucune clé
// ni adresse de console exposée.
func Test_NR_server_backend_34_sonnette_unifi_reglages_et_essai_reserves_a_lan_admin(t *testing.T) {
	rig := newDoorbellRig(t, true, true)
	name := "Salon TV"
	_, _ = rig.names.Set("salon", &name, "admin@example.org", time.Now())
	_, _ = rig.names.SetLayout(core.LayoutLights, []string{"salon"}, []string{}, false, "admin@example.org", time.Now())
	before := readSections(t, rig.path)

	// droits
	if rig.do(http.MethodPut, "/api/admin/doorbell/settings", goodSettings, models.LanRoleUser).Code != http.StatusForbidden ||
		rig.do(http.MethodPost, "/api/admin/doorbell/test", "", models.LanRoleUser).Code != http.StatusForbidden {
		t.Fatal("lan_user doit être refusé")
	}
	rig.save(t, goodSettings)
	for _, p := range []string{"/api/doorbell/status", "/api/doorbell/stream"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		req.AddCookie(rig.cookies[models.LanRoleUser])
		rec := httptest.NewRecorder()
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		rig.router.ServeHTTP(rec, req.WithContext(ctx))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s : lan_user doit être accepté (%d)", p, rec.Code)
		}
	}

	// autres sections intactes
	after := readSections(t, rig.path)
	for _, k := range []string{"lights", "lights_layout"} {
		if before[k] != after[k] {
			t.Fatalf("section %q modifiée", k)
		}
	}

	// essai à sec : aucune écriture dans l'armoire, état ringing
	if rig.do(http.MethodPost, "/api/admin/doorbell/test", "", models.LanRoleAdmin, "X-Essensys-Test-Mode", "dry-run").Code != http.StatusOK {
		t.Fatal("essai à sec")
	}
	rig.svc.WaitIdle()
	if !rig.queueEmpty() {
		t.Fatal("aucune écriture dans l'armoire")
	}

	// jamais de clé ni d'adresse dans les réponses
	for _, body := range []string{
		rig.do(http.MethodGet, "/api/doorbell/status", "", models.LanRoleUser).Body.String(),
		rig.do(http.MethodGet, "/api/admin/doorbell/settings", "", models.LanRoleAdmin).Body.String(),
	} {
		for _, secret := range []string{"api_key", "base_url", "X-API-Key", "http://", "https://"} {
			if strings.Contains(body, secret) {
				t.Fatalf("réponse : %q ne doit pas contenir %q", body, secret)
			}
		}
	}

	// legacy IoT : les routes de l'armoire ne connaissent pas la sonnette
	for _, p := range []string{"/api/serverinfos", "/api/mystatus", "/api/myactions", "/api/done/x"} {
		if rec := rig.do(http.MethodPost, p, "", ""); strings.Contains(rec.Body.String(), "doorbell") {
			t.Fatalf("%s ne doit pas évoquer la sonnette", p)
		}
	}
}
