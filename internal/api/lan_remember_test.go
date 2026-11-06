package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Tests de bout en bout du login LAN avec « remember_me », sur un vrai Postgres.
// Ignorés sans ESSENSYS_TEST_DATABASE_URL ; ils travaillent dans un schéma jetable.
type rememberRig struct {
	db       *sqlx.DB
	userRepo *laniam.UserRepository
}

func newRememberRig(t *testing.T) *rememberRig {
	t.Helper()
	url := os.Getenv("ESSENSYS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ESSENSYS_TEST_DATABASE_URL non défini : test d'intégration Postgres ignoré")
	}
	db, err := sqlx.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := fmt.Sprintf("t_lanapi_%d", time.Now().UnixNano())
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		_ = db.Close()
	})
	if _, err := db.Exec(`SET search_path TO ` + schema); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"003_lan_users", "004_trusted_devices", "005_lan_login_clients", "006_lan_remember_sessions"} {
		body, err := os.ReadFile("../../migrations/" + f + ".up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	rig := &rememberRig{db: db, userRepo: laniam.NewUserRepository(db)}
	hash, algo, err := laniam.HashPassword("mot-de-passe-test")
	if err != nil {
		t.Fatal(err)
	}
	u := &models.LanUser{Email: "lan@test.local", PasswordHash: hash, PasswordAlgo: algo, Role: models.LanRoleUser}
	if err := rig.userRepo.Create(u); err != nil {
		t.Fatal(err)
	}
	return rig
}

// boot simule un (re)démarrage de la gateway : mémoire vide, même base.
func (r *rememberRig) boot(withRemember bool) (*LanIAMHandler, *laniam.SessionStore) {
	store := laniam.NewSessionStore(168)
	if withRemember {
		store.SetRememberStore(laniam.NewRememberSessionRepository(r.db), r.userRepo.GetByID)
	}
	svc := laniam.NewService(r.userRepo, laniam.NewTrustedDeviceRepository(r.db), laniam.NewLoginClientRepository(r.db), nil, store, "")
	return NewLanIAMHandler(svc, false), store
}

func (r *rememberRig) login(t *testing.T, h *LanIAMHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("User-Agent", "EssensysTest/1.0")
	w := httptest.NewRecorder()
	h.Login(w, req)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == middleware.LanSessionCookie {
			return c
		}
	}
	return nil
}

func (r *rememberRig) accepted(t *testing.T, store *laniam.SessionStore, cookie *http.Cookie) bool {
	t.Helper()
	protected := middleware.LanRequireSession(store, false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)
	return w.Code == http.StatusOK
}

func (r *rememberRig) rowCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.db.Get(&n, `SELECT COUNT(*) FROM lan_remember_sessions`); err != nil {
		t.Fatal(err)
	}
	return n
}

const loginRemember = `{"email":"lan@test.local","password":"mot-de-passe-test","remember_me":true}`
const loginPlain = `{"email":"lan@test.local","password":"mot-de-passe-test"}`

func TestLanLoginWithRememberMeOpens30DaySessionAndSurvivesRestart(t *testing.T) {
	rig := newRememberRig(t)
	h, _ := rig.boot(true)

	w := rig.login(t, h, loginRemember)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	cookie := sessionCookie(w)
	if cookie == nil || cookie.MaxAge != 30*24*3600 {
		t.Fatalf("cookie must last 30 days, got %+v", cookie)
	}
	var body struct {
		RememberExpiresAt string `json:"remember_expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	exp, err := time.Parse(time.RFC3339, body.RememberExpiresAt)
	if err != nil {
		t.Fatalf("remember_expires_at %q: %v", body.RememberExpiresAt, err)
	}
	if d := time.Until(exp) - 30*24*time.Hour; d > time.Minute || d < -time.Minute {
		t.Fatalf("remember_expires_at %v is not 30 days from now", exp)
	}
	var stored string
	if err := rig.db.Get(&stored, `SELECT t::text FROM lan_remember_sessions t`); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, cookie.Value) {
		t.Fatal("the session id is stored in clear")
	}

	_, restarted := rig.boot(true)
	if !rig.accepted(t, restarted, cookie) {
		t.Fatal("the remembered session must survive a gateway restart")
	}
}

func TestLanLoginWithoutRememberMeIsUnchanged(t *testing.T) {
	rig := newRememberRig(t)
	h, _ := rig.boot(true)

	w := rig.login(t, h, loginPlain)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
	if c := sessionCookie(w); c == nil || c.MaxAge != 168*3600 {
		t.Fatalf("plain cookie must keep its 7 days, got %+v", c)
	}
	if strings.Contains(w.Body.String(), "remember_expires_at") {
		t.Fatal("no remember_expires_at without the option")
	}
	if rig.rowCount(t) != 0 {
		t.Fatal("a plain session must not be written to the database")
	}
	cookie := sessionCookie(w)
	_, restarted := rig.boot(true)
	if rig.accepted(t, restarted, cookie) {
		t.Fatal("a plain session is lost at restart, as before")
	}
}

func TestLanLoginFallsBackToPlainSessionWhenRememberIsNotAvailable(t *testing.T) {
	rig := newRememberRig(t)
	h, _ := rig.boot(false)

	w := rig.login(t, h, loginRemember)
	if w.Code != http.StatusOK {
		t.Fatalf("login must succeed without error: %d", w.Code)
	}
	if c := sessionCookie(w); c == nil || c.MaxAge != 168*3600 {
		t.Fatalf("fallback cookie must be the plain one, got %+v", c)
	}
	if strings.Contains(w.Body.String(), "remember_expires_at") {
		t.Fatal("no remember_expires_at when the server cannot remember")
	}
}

func TestLanLoginFailsWhenTheRememberedSessionCannotBeStored(t *testing.T) {
	rig := newRememberRig(t)
	h, _ := rig.boot(true)
	if _, err := rig.db.Exec(`DROP TABLE lan_remember_sessions`); err != nil {
		t.Fatal(err)
	}

	w := rig.login(t, h, loginRemember)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("login must fail when the session cannot be stored, got %d", w.Code)
	}
	if sessionCookie(w) != nil {
		t.Fatal("no cookie may be issued for a session that would not survive")
	}
}

func TestLanLogoutRevokesTheRememberedSessionEvenAfterRestart(t *testing.T) {
	rig := newRememberRig(t)
	h, _ := rig.boot(true)
	cookie := sessionCookie(rig.login(t, h, loginRemember))

	h2, store2 := rig.boot(true)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	h2.Logout(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout: %d", w.Code)
	}
	if rig.rowCount(t) != 0 {
		t.Fatal("logout must delete the database row")
	}
	_, store3 := rig.boot(true)
	if rig.accepted(t, store2, cookie) || rig.accepted(t, store3, cookie) {
		t.Fatal("the cookie must be refused after logout")
	}
}

func TestLanPasswordChangeRevokesRememberedSessions(t *testing.T) {
	rig := newRememberRig(t)
	h, _ := rig.boot(true)
	cookie := sessionCookie(rig.login(t, h, loginRemember))

	user, err := rig.userRepo.GetByEmail("lan@test.local")
	if err != nil || user == nil {
		t.Fatal(err)
	}
	h2, _ := rig.boot(true)
	if err := h2.svc.ChangePassword(user.ID, "mot-de-passe-test", "nouveau-mot-de-passe"); err != nil {
		t.Fatal(err)
	}
	_, restarted := rig.boot(true)
	if rig.accepted(t, restarted, cookie) {
		t.Fatal("the remembered session must be refused after the password change")
	}
	if rig.rowCount(t) != 0 {
		t.Fatal("the row must be deleted with the password change")
	}
}
