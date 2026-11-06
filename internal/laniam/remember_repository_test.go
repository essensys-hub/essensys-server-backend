package laniam

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Test d'intégration Postgres : ignoré sans ESSENSYS_TEST_DATABASE_URL.
// Il travaille dans un schéma jetable et applique les vraies migrations.
func newRememberTestDB(t *testing.T) *sqlx.DB {
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
	schema := fmt.Sprintf("t_remember_%d", time.Now().UnixNano())
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
	for _, f := range []string{"003_lan_users.up.sql", "006_lan_remember_sessions.up.sql"} {
		body, err := os.ReadFile("../../migrations/" + f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	return db
}

func insertRememberTestUser(t *testing.T, db *sqlx.DB, email string) int {
	t.Helper()
	var id int
	if err := db.QueryRow(`INSERT INTO lan_users (email, password_hash) VALUES ($1, 'x') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRememberMigrationIsIdempotentAndReversible(t *testing.T) {
	db := newRememberTestDB(t)
	up, err := os.ReadFile("../../migrations/006_lan_remember_sessions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("../../migrations/006_lan_remember_sessions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatalf("up rejoué : %v", err)
	}
	if _, err := db.Exec(string(down)); err != nil {
		t.Fatalf("down : %v", err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatalf("up après down : %v", err)
	}
}

func TestRememberRepositoryStoresOnlyTheHash(t *testing.T) {
	db := newRememberTestDB(t)
	repo := NewRememberSessionRepository(db)
	uid := insertRememberTestUser(t, db, "a@test.local")
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	const sessionID = "session-id-en-clair-jamais-stocke"

	if err := repo.Create(HashSessionID(sessionID), uid, now, now.Add(RememberTTL), "UA/1.0"); err != nil {
		t.Fatal(err)
	}
	var row string
	if err := db.QueryRow(`SELECT t::text FROM lan_remember_sessions t`).Scan(&row); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(row, sessionID) {
		t.Fatalf("l'identifiant de session est en clair en base : %s", row)
	}
	if !strings.Contains(row, HashSessionID(sessionID)) {
		t.Fatalf("l'empreinte est absente de la ligne : %s", row)
	}

	rec, err := repo.FindByHash(HashSessionID(sessionID))
	if err != nil || rec == nil {
		t.Fatalf("FindByHash : rec=%v err=%v", rec, err)
	}
	if rec.UserID != uid || !rec.ExpiresAt.Equal(now.Add(RememberTTL)) {
		t.Fatalf("ligne relue inattendue : %+v", rec)
	}
	if rec, err := repo.FindByHash(HashSessionID("autre")); err != nil || rec != nil {
		t.Fatalf("identifiant inconnu : rec=%v err=%v", rec, err)
	}
}

func TestRememberRepositoryDeletePurgeAndCascade(t *testing.T) {
	db := newRememberTestDB(t)
	repo := NewRememberSessionRepository(db)
	u1 := insertRememberTestUser(t, db, "a@test.local")
	u2 := insertRememberTestUser(t, db, "b@test.local")
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	mustCreate := func(id string, uid int, created time.Time) {
		t.Helper()
		if err := repo.Create(HashSessionID(id), uid, created, created.Add(RememberTTL), ""); err != nil {
			t.Fatal(err)
		}
	}
	mustCreate("s1", u1, now)
	mustCreate("s2", u1, now)
	mustCreate("s3", u2, now)
	mustCreate("vieille", u2, now.Add(-RememberTTL-time.Hour))

	if n, err := repo.PurgeExpired(now); err != nil || n != 1 {
		t.Fatalf("PurgeExpired : n=%d err=%v", n, err)
	}
	if err := repo.DeleteByHash(HashSessionID("s1")); err != nil {
		t.Fatal(err)
	}
	if rec, _ := repo.FindByHash(HashSessionID("s1")); rec != nil {
		t.Fatal("s1 aurait dû être supprimée")
	}
	if err := repo.DeleteByUser(u1); err != nil {
		t.Fatal(err)
	}
	if rec, _ := repo.FindByHash(HashSessionID("s2")); rec != nil {
		t.Fatal("s2 aurait dû être supprimée avec l'utilisateur")
	}
	if rec, _ := repo.FindByHash(HashSessionID("s3")); rec == nil {
		t.Fatal("s3 appartient à un autre utilisateur et doit rester")
	}

	if _, err := db.Exec(`DELETE FROM lan_users WHERE id = $1`, u2); err != nil {
		t.Fatal(err)
	}
	if rec, _ := repo.FindByHash(HashSessionID("s3")); rec != nil {
		t.Fatal("la suppression de l'utilisateur doit supprimer ses sessions mémorisées")
	}
}
