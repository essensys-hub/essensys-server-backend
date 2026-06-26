package laniam_test

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

func TestHashAndVerifyPasswordBcrypt(t *testing.T) {
	hash, algo, err := laniam.HashPassword("secretpass")
	if err != nil {
		t.Fatal(err)
	}
	if algo != models.PasswordAlgoBcrypt {
		t.Fatalf("expected bcrypt, got %s", algo)
	}
	user := &models.LanUser{PasswordHash: hash, PasswordAlgo: algo}
	ok, err := laniam.VerifyPassword(user, "secretpass")
	if err != nil || !ok {
		t.Fatalf("verify failed: ok=%v err=%v", ok, err)
	}
	ok, err = laniam.VerifyPassword(user, "wrong")
	if err != nil || ok {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestSessionStoreTTL168h(t *testing.T) {
	store := laniam.NewSessionStore(168)
	if store.CookieMaxAgeSeconds() != 168*3600 {
		t.Fatalf("unexpected max age %d", store.CookieMaxAgeSeconds())
	}
}
