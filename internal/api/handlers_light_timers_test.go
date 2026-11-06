package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type timersRig struct {
	store   data.Store
	router  http.Handler
	cookies map[string]*http.Cookie
}

func newTimersRig(t *testing.T, iamEnabled bool) *timersRig {
	t.Helper()
	store := data.NewMemoryStore()
	h := NewHandler(core.NewActionService(store), core.NewStatusService(store), store, nil, nil, config.ArmoireConfig{})
	sessions := laniam.NewSessionStore(1)
	rig := &timersRig{store: store, cookies: map[string]*http.Cookie{}}
	for i, role := range []string{models.LanRoleAdmin, models.LanRoleUser} {
		sess, err := sessions.CreateSession(&models.LanUser{ID: i + 1, Email: role + "@test.local", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		rig.cookies[role] = &http.Cookie{Name: middleware.LanSessionCookie, Value: sess.ID}
	}
	rig.router = NewRouter(RouterConfig{
		Handler:         h,
		LanSessionStore: sessions,
		LanIAMEnabled:   iamEnabled,
		Store:           store,
	})
	return rig
}

func (r *timersRig) do(method, path, body, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if c, ok := r.cookies[role]; ok {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.router.ServeHTTP(rec, req)
	return rec
}

// queued counts pending actions (the memory store does not drain on read).
func (r *timersRig) queued() int { return len(r.store.DequeueActions("default")) }

func errKey(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return m["error"]
}

func TestLightTimers_get(t *testing.T) {
	rig := newTimersRig(t, true)
	rig.store.SetValue("default", 521, "60")
	rig.store.SetValue("default", 556, "0")

	rec := rig.do(http.MethodGet, "/api/admin/lights/timers", "", models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var resp struct {
		Timers []struct {
			ID      string `json:"id"`
			Label   string `json:"label"`
			Key     int    `json:"key"`
			Minutes *int   `json:"minutes"`
		} `json:"timers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	ids := []string{"dressing", "wc1", "wc2", "pieceserv", "ann1", "ann2"}
	keys := []int{521, 555, 556, 557, 561, 562}
	if len(resp.Timers) != 6 {
		t.Fatalf("got %d timers", len(resp.Timers))
	}
	for i, tm := range resp.Timers {
		if tm.ID != ids[i] || tm.Key != keys[i] || tm.Label == "" {
			t.Fatalf("entry %d: %+v", i, tm)
		}
	}
	if resp.Timers[3].Label != "Service" {
		t.Fatalf("label %q", resp.Timers[3].Label)
	}
	if resp.Timers[0].Minutes == nil || *resp.Timers[0].Minutes != 60 {
		t.Fatal("dressing must be 60")
	}
	if resp.Timers[2].Minutes == nil || *resp.Timers[2].Minutes != 0 {
		t.Fatal("wc2 stored 0 must read 0")
	}
	if !strings.Contains(rec.Body.String(), `"id":"wc1","label":"WC 1","key":555,"minutes":null`) {
		t.Fatalf("never read key must be null, body %s", rec.Body.String())
	}
}

func TestLightTimers_putMatrix(t *testing.T) {
	rig := newTimersRig(t, true)

	rec := rig.do(http.MethodPut, "/api/admin/lights/timers/dressing", `{"minutes":15}`, models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body.String())
	}
	var ok map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &ok)
	if ok["id"] != "dressing" || ok["minutes"] != float64(15) || ok["status"] != "queued" {
		t.Fatalf("response %+v", ok)
	}
	actions := rig.store.DequeueActions("default")
	if len(actions) != 1 || len(actions[0].Params) != 1 || actions[0].Params[0].K != 521 || actions[0].Params[0].V != "15" {
		t.Fatalf("expected the single action 521=15, got %+v", actions)
	}

	rec = rig.do(http.MethodPut, "/api/admin/lights/timers/dressing", `{"minutes":15}`, models.LanRoleUser)
	if rec.Code != http.StatusForbidden || errKey(t, rec) != "forbidden" || rig.queued() != 1 {
		t.Fatalf("lan_user: %d %s", rec.Code, rec.Body.String())
	}
	rec = rig.do(http.MethodPut, "/api/admin/lights/timers/dressing", `{"minutes":15}`, "")
	if rec.Code != http.StatusUnauthorized || rig.queued() != 1 {
		t.Fatalf("no session: %d", rec.Code)
	}
	for _, role := range []string{models.LanRoleUser, ""} {
		rec = rig.do(http.MethodGet, "/api/admin/lights/timers", "", role)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET role %q: %d", role, rec.Code)
		}
	}
}

func TestLightTimers_putValidation(t *testing.T) {
	rig := newTimersRig(t, true)
	for _, body := range []string{`{"minutes":256}`, `{"minutes":-1}`, `{"minutes":7.5}`, `{"minutes":"15"}`, `{"minutes":null}`, `{}`, `nope`} {
		rec := rig.do(http.MethodPut, "/api/admin/lights/timers/wc1", body, models.LanRoleAdmin)
		if rec.Code != http.StatusBadRequest || errKey(t, rec) != "invalid_minutes" {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	rec := rig.do(http.MethodPut, "/api/admin/lights/timers/salon", `{"minutes":10}`, models.LanRoleAdmin)
	if rec.Code != http.StatusBadRequest || errKey(t, rec) != "unknown_light" {
		t.Fatalf("salon: %d %s", rec.Code, rec.Body.String())
	}
	for _, body := range []string{`{"minutes":0}`, `{"minutes":255}`} {
		if rec := rig.do(http.MethodPut, "/api/admin/lights/timers/wc1", body, models.LanRoleAdmin); rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", body, rec.Code)
		}
	}
	if n := rig.queued(); n != 2 {
		t.Fatalf("expected only the two valid writes queued, got %d", n)
	}
}

func TestLightTimers_iamDisabledOpenLikeOtherRoutes(t *testing.T) {
	rig := newTimersRig(t, false)
	if rec := rig.do(http.MethodGet, "/api/admin/lights/timers", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET: %d", rec.Code)
	}
	if rec := rig.do(http.MethodPut, "/api/admin/lights/timers/ann2", `{"minutes":7}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d", rec.Code)
	}
	if rig.queued() != 1 {
		t.Fatal("expected one action")
	}
}

func TestLightTimers_putDryRun(t *testing.T) {
	rig := newTimersRig(t, true)
	rec := rig.do(http.MethodPut, "/api/admin/lights/timers/ann1?test_mode=dry_run", `{"minutes":30}`, models.LanRoleAdmin)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "dry_run" || resp["id"] != "ann1" || resp["minutes"] != float64(30) {
		t.Fatalf("resp %+v", resp)
	}
	if rig.queued() != 0 {
		t.Fatal("dry-run must not enqueue")
	}
	// Dry-run still validates and still requires the role.
	if rec := rig.do(http.MethodPut, "/api/admin/lights/timers/ann1?test_mode=dry_run", `{"minutes":300}`, models.LanRoleAdmin); rec.Code != http.StatusBadRequest {
		t.Fatalf("dry-run out of range: %d", rec.Code)
	}
	if rec := rig.do(http.MethodPut, "/api/admin/lights/timers/ann1?test_mode=dry_run", `{"minutes":30}`, models.LanRoleUser); rec.Code != http.StatusForbidden {
		t.Fatalf("dry-run lan_user: %d", rec.Code)
	}
}

// NR: NR-server-backend-27 essensys-hub/essensys-server-backend#17
// lumieres-temporisees-2026-11-003: writing a light timer is reserved to
// lan_admin (403 for lan_user, 401 without session) and never queues an
// action when refused or out of range.
func Test_NR_server_backend_27_temporisation_lumiere_reservee_a_lan_admin(t *testing.T) {
	rig := newTimersRig(t, true)
	path := "/api/admin/lights/timers/pieceserv"
	if rec := rig.do(http.MethodPut, path, `{"minutes":30}`, models.LanRoleUser); rec.Code != http.StatusForbidden {
		t.Fatalf("lan_user: %d", rec.Code)
	}
	if rec := rig.do(http.MethodPut, path, `{"minutes":30}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if rec := rig.do(http.MethodPut, path, `{"minutes":256}`, models.LanRoleAdmin); rec.Code != http.StatusBadRequest {
		t.Fatalf("out of range: %d", rec.Code)
	}
	if rig.queued() != 0 {
		t.Fatal("no action may be queued by a refused write")
	}
	if rec := rig.do(http.MethodPut, path, `{"minutes":30}`, models.LanRoleAdmin); rec.Code != http.StatusOK {
		t.Fatalf("lan_admin: %d", rec.Code)
	}
	if acts := rig.store.DequeueActions("default"); len(acts) != 1 || acts[0].Params[0].K != 557 || acts[0].Params[0].V != "30" {
		t.Fatalf("expected 557=30, got %+v", acts)
	}
}
