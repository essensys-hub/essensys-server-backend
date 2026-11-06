package laniam

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type fakeRememberRow struct {
	userID    int
	createdAt time.Time
	expiresAt time.Time
	userAgent string
}

// fakeRememberStore simule la table lan_remember_sessions ; le partager entre deux
// SessionStore simule un redémarrage de la gateway.
type fakeRememberStore struct {
	rows      map[string]fakeRememberRow
	hashes    []string
	createErr error
	deleteErr error
}

func newFakeRememberStore() *fakeRememberStore {
	return &fakeRememberStore{rows: map[string]fakeRememberRow{}}
}

func (f *fakeRememberStore) Create(idHash string, userID int, createdAt, expiresAt time.Time, userAgent string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.hashes = append(f.hashes, idHash)
	f.rows[idHash] = fakeRememberRow{userID, createdAt, expiresAt, userAgent}
	return nil
}

func (f *fakeRememberStore) FindByHash(idHash string) (*RememberRecord, error) {
	row, ok := f.rows[idHash]
	if !ok {
		return nil, nil
	}
	return &RememberRecord{UserID: row.userID, CreatedAt: row.createdAt, ExpiresAt: row.expiresAt}, nil
}

func (f *fakeRememberStore) DeleteByHash(idHash string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.rows, idHash)
	return nil
}

func (f *fakeRememberStore) DeleteByUser(userID int) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for h, row := range f.rows {
		if row.userID == userID {
			delete(f.rows, h)
		}
	}
	return nil
}

func (f *fakeRememberStore) PurgeExpired(now time.Time) (int64, error) {
	var n int64
	for h, row := range f.rows {
		if !now.Before(row.expiresAt) {
			delete(f.rows, h)
			n++
		}
	}
	return n, nil
}

type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

var rememberT0 = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

func newRememberUsers() map[int]*models.LanUser {
	return map[int]*models.LanUser{
		1: {ID: 1, Email: "a@test.local", Role: models.LanRoleUser},
		2: {ID: 2, Email: "b@test.local", Role: models.LanRoleUser},
	}
}

// newRememberStoreAt construit un SessionStore branché sur db, comme après un (re)démarrage.
func newRememberStoreAt(clock *testClock, db *fakeRememberStore, users map[int]*models.LanUser) *SessionStore {
	s := NewSessionStore(168)
	s.SetClock(clock.now)
	s.SetRememberStore(db, func(id int) (*models.LanUser, error) { return users[id], nil })
	return s
}

func TestRememberedSessionLastsExactly30DaysAndUseNeverExtendsIt(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s := newRememberStoreAt(clock, db, users)

	sess, err := s.CreateRememberedSession(users[1], "UA")
	if err != nil {
		t.Fatal(err)
	}
	want := rememberT0.Add(30 * 24 * time.Hour)
	if !sess.ExpiresAt.Equal(want) {
		t.Fatalf("expiry %v, want %v", sess.ExpiresAt, want)
	}
	if s.RememberCookieMaxAgeSeconds() != 30*24*3600 {
		t.Fatalf("cookie max age %d", s.RememberCookieMaxAgeSeconds())
	}

	for day := 1; day <= 29; day++ {
		clock.t = rememberT0.Add(time.Duration(day) * 24 * time.Hour)
		got, ok := s.GetSession(sess.ID)
		if !ok {
			t.Fatalf("session refused on day %d", day)
		}
		if !got.ExpiresAt.Equal(want) {
			t.Fatalf("day %d: expiry moved to %v", day, got.ExpiresAt)
		}
	}
	clock.t = want.Add(-time.Minute)
	if _, ok := s.GetSession(sess.ID); !ok {
		t.Fatal("session must still be valid one minute before the end")
	}
	clock.t = want
	if _, ok := s.GetSession(sess.ID); ok {
		t.Fatal("session must be refused at day 30 even if used the day before")
	}
	if len(db.rows) != 0 {
		t.Fatal("expired remembered session must be deleted from the database")
	}
}

func TestRememberedSessionSurvivesGatewayRestart(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	before := newRememberStoreAt(clock, db, users)
	sess, err := before.CreateRememberedSession(users[1], "UA")
	if err != nil {
		t.Fatal(err)
	}

	after := newRememberStoreAt(clock, db, users) // redémarrage : mémoire vide, même base
	clock.t = rememberT0.Add(5 * 24 * time.Hour)
	got, ok := after.GetSession(sess.ID)
	if !ok || got.User == nil || got.User.ID != 1 {
		t.Fatalf("restart: session not restored (ok=%v)", ok)
	}
	if !got.ExpiresAt.Equal(rememberT0.Add(30 * 24 * time.Hour)) {
		t.Fatalf("restart: expiry changed to %v", got.ExpiresAt)
	}

	for day := 6; day <= 29; day++ {
		clock.t = rememberT0.Add(time.Duration(day) * 24 * time.Hour)
		if _, ok := after.GetSession(sess.ID); !ok {
			t.Fatalf("restored session refused on day %d", day)
		}
	}
	clock.t = rememberT0.Add(31 * 24 * time.Hour)
	if _, ok := after.GetSession(sess.ID); ok {
		t.Fatal("restored session must be refused on day 31")
	}
}

func TestRememberedSessionRefusedAfterRestartPastDay30(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	sess, _ := newRememberStoreAt(clock, db, users).CreateRememberedSession(users[1], "UA")

	clock.t = rememberT0.Add(30 * 24 * time.Hour)
	if _, ok := newRememberStoreAt(clock, db, users).GetSession(sess.ID); ok {
		t.Fatal("restart at day 30: session must be refused")
	}
	if len(db.rows) != 0 {
		t.Fatal("expired row must be removed")
	}
}

func TestPlainSessionKeepsItsMemoryOnlySevenDayBehaviour(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s := newRememberStoreAt(clock, db, users)

	sess, err := s.CreateSession(users[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(db.rows) != 0 {
		t.Fatal("a plain session must not be written to the database")
	}
	if s.CookieMaxAgeSeconds() != 168*3600 {
		t.Fatalf("plain cookie max age changed: %d", s.CookieMaxAgeSeconds())
	}
	clock.t = rememberT0.Add(6 * 24 * time.Hour)
	got, ok := s.GetSession(sess.ID)
	if !ok {
		t.Fatal("plain session refused within its 7 days")
	}
	// Comportement existant : slidingMax == ttl, l'expiration reste plafonnée à création + 7 jours.
	if !got.ExpiresAt.Equal(rememberT0.Add(168 * time.Hour)) {
		t.Fatalf("plain session expiry changed to %v", got.ExpiresAt)
	}
	clock.t = rememberT0.Add(8 * 24 * time.Hour)
	if _, ok := s.GetSession(sess.ID); ok {
		t.Fatal("plain session must still end after 7 days")
	}

	if _, ok := newRememberStoreAt(clock, db, users).GetSession(sess.ID); ok {
		t.Fatal("a plain session is lost at restart, as before")
	}
}

func TestRememberedSessionRequiresPersistence(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s := newRememberStoreAt(clock, db, users)

	db.createErr = errors.New("db down")
	if sess, err := s.CreateRememberedSession(users[1], "UA"); err == nil || sess != nil {
		t.Fatal("login must fail when the session cannot be persisted")
	}
	if len(s.sessions) != 0 {
		t.Fatal("no session must be kept in memory after a failed insert")
	}

	plain := NewSessionStore(168)
	if plain.RememberEnabled() {
		t.Fatal("remember must be disabled without a store")
	}
	if _, err := plain.CreateRememberedSession(users[1], "UA"); !errors.Is(err, ErrRememberUnavailable) {
		t.Fatalf("want ErrRememberUnavailable, got %v", err)
	}
}

func TestDeleteSessionRevokesMemoryAndDatabase(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s := newRememberStoreAt(clock, db, users)
	sess, _ := s.CreateRememberedSession(users[1], "UA")

	if err := s.DeleteSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetSession(sess.ID); ok {
		t.Fatal("deleted session still valid in memory")
	}
	if _, ok := newRememberStoreAt(clock, db, users).GetSession(sess.ID); ok {
		t.Fatal("deleted session valid again after restart")
	}

	sess2, _ := s.CreateRememberedSession(users[1], "UA")
	db.deleteErr = errors.New("db down")
	if err := s.DeleteSession(sess2.ID); err == nil {
		t.Fatal("a failed revocation must be reported, not hidden")
	}
}

func TestDeleteSessionAfterRestartRevokesNotYetLoadedSession(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	sess, _ := newRememberStoreAt(clock, db, users).CreateRememberedSession(users[1], "UA")

	restarted := newRememberStoreAt(clock, db, users)
	if err := restarted.DeleteSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	if len(db.rows) != 0 {
		t.Fatal("logout right after a restart must still delete the row")
	}
}

func TestDeleteSessionsForUserRevokesAllRememberedSessionsOfThatUserOnly(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s := newRememberStoreAt(clock, db, users)
	a1, _ := s.CreateRememberedSession(users[1], "phone")
	a2, _ := s.CreateRememberedSession(users[1], "tablet")
	b1, _ := s.CreateRememberedSession(users[2], "phone")

	if err := s.DeleteSessionsForUser(1); err != nil {
		t.Fatal(err)
	}
	after := newRememberStoreAt(clock, db, users)
	for _, id := range []string{a1.ID, a2.ID} {
		if _, ok := s.GetSession(id); ok {
			t.Fatal("session of the user still valid in memory")
		}
		if _, ok := after.GetSession(id); ok {
			t.Fatal("session of the user still valid after restart")
		}
	}
	if _, ok := after.GetSession(b1.ID); !ok {
		t.Fatal("another user's session must survive")
	}
}

func TestRestoreRefusesDisabledOrDeletedUser(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s1, _ := newRememberStoreAt(clock, db, users).CreateRememberedSession(users[1], "UA")
	s2, _ := newRememberStoreAt(clock, db, users).CreateRememberedSession(users[2], "UA")

	disabledAt := rememberT0
	users[1].DisabledAt = &disabledAt
	delete(users, 2)

	restarted := newRememberStoreAt(clock, db, users)
	if _, ok := restarted.GetSession(s1.ID); ok {
		t.Fatal("disabled user's session must be refused")
	}
	if _, ok := restarted.GetSession(s2.ID); ok {
		t.Fatal("deleted user's session must be refused")
	}
	if len(db.rows) != 0 {
		t.Fatal("rows of refused users must be removed")
	}
}

func TestOnlyTheHashOfTheSessionIDIsStored(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	sess, _ := newRememberStoreAt(clock, db, users).CreateRememberedSession(users[1], "UA")

	if len(db.hashes) != 1 || db.hashes[0] != HashSessionID(sess.ID) {
		t.Fatalf("stored %v, want the SHA-256 of the session id", db.hashes)
	}
	if len(db.hashes[0]) != 64 || strings.Contains(db.hashes[0], sess.ID) || db.hashes[0] == sess.ID {
		t.Fatal("the session id must never be stored in clear")
	}
}

func TestPurgeRemovesOnlyExpiredRememberedSessions(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s := newRememberStoreAt(clock, db, users)
	_, _ = s.CreateRememberedSession(users[1], "old")
	clock.t = rememberT0.Add(20 * 24 * time.Hour)
	fresh, _ := s.CreateRememberedSession(users[1], "fresh")

	clock.t = rememberT0.Add(31 * 24 * time.Hour)
	n, err := s.PurgeExpiredRemembered()
	if err != nil || n != 1 {
		t.Fatalf("purged %d (err %v), want 1", n, err)
	}
	if _, ok := db.rows[HashSessionID(fresh.ID)]; !ok {
		t.Fatal("the unexpired session must remain")
	}
}

func TestUserAgentIsTruncatedByCharacters(t *testing.T) {
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := newRememberUsers()
	s := newRememberStoreAt(clock, db, users)
	long := strings.Repeat("é", 400)
	sess, _ := s.CreateRememberedSession(users[1], long)

	got := db.rows[HashSessionID(sess.ID)].userAgent
	if r := []rune(got); len(r) != 255 || string(r) != strings.Repeat("é", 255) {
		t.Fatalf("user agent kept %d chars", len([]rune(got)))
	}
}

// NR: NR-server-backend-25 essensys-hub/essensys-server-backend#14
// session-30-jours-2026-11-001: a remembered session (30 days, kept in the database)
// is refused as soon as its user changes password — even after a gateway restart —
// and neither the session id nor its hash ever reaches the logs.
func Test_NR_server_backend_25_session_memorisee_refusee_apres_changement_de_mot_de_passe(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)

	hash, algo, err := HashPassword("ancien-mot-de-passe")
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{rememberT0}
	db := newFakeRememberStore()
	users := map[int]*models.LanUser{
		1: {ID: 1, Email: "a@test.local", Role: models.LanRoleUser, PasswordHash: hash, PasswordAlgo: algo},
	}
	sessions := newRememberStoreAt(clock, db, users)
	svc := &Service{repo: &stubUserRepo{byID: users}, sessions: sessions, now: clock.now}

	phone, _ := sessions.CreateRememberedSession(users[1], "phone")
	tablet, _ := sessions.CreateRememberedSession(users[1], "tablet")
	clock.t = rememberT0.Add(5 * 24 * time.Hour)
	if _, ok := sessions.GetSession(phone.ID); !ok {
		t.Fatal("precondition: the remembered session must be valid before the change")
	}

	if err := svc.ChangePassword(1, "ancien-mot-de-passe", "nouveau-mot-de-passe"); err != nil {
		t.Fatal(err)
	}

	restarted := newRememberStoreAt(clock, db, users)
	for name, store := range map[string]*SessionStore{"same process": sessions, "after restart": restarted} {
		for _, id := range []string{phone.ID, tablet.ID} {
			if _, ok := store.GetSession(id); ok {
				t.Fatalf("%s: remembered session still valid after the password change", name)
			}
		}
	}

	db.deleteErr = errors.New("connection refused")
	svc2 := &Service{repo: &stubUserRepo{byID: users}, sessions: sessions, now: clock.now}
	_ = svc2.ChangePassword(1, "ancien-mot-de-passe", "autre-mot-de-passe")
	_ = sessions.DeleteSession(phone.ID)

	for _, secret := range []string{phone.ID, tablet.ID, HashSessionID(phone.ID), HashSessionID(tablet.ID)} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("a session id or its hash appeared in the logs: %q", logs.String())
		}
	}
}
