package laniam

import (
	"fmt"

	"github.com/essensys-hub/essensys-server-backend/internal/auth"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

func HashPassword(plain string) (hash string, algo string, err error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", "", err
	}
	return string(b), models.PasswordAlgoBcrypt, nil
}

func VerifyPassword(user *models.LanUser, plain string) (bool, error) {
	switch user.PasswordAlgo {
	case models.PasswordAlgoBcrypt, "":
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(plain))
		return err == nil, nil
	case models.PasswordAlgoSHA1Legacy:
		return auth.HashPassword(plain) == user.PasswordHash, nil
	default:
		return false, fmt.Errorf("unsupported password_algo: %s", user.PasswordAlgo)
	}
}
