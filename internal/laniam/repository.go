package laniam

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByEmail(email string) (*models.LanUser, error) {
	var u models.LanUser
	err := r.db.Get(&u, `SELECT * FROM lan_users WHERE LOWER(email) = LOWER($1)`, strings.TrimSpace(email))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetByID(id int) (*models.LanUser, error) {
	var u models.LanUser
	err := r.db.Get(&u, `SELECT * FROM lan_users WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) List() ([]models.LanUser, error) {
	var users []models.LanUser
	err := r.db.Select(&users, `SELECT * FROM lan_users ORDER BY email`)
	return users, err
}

func (r *UserRepository) CountActiveAdmins() (int, error) {
	var n int
	err := r.db.Get(&n, `SELECT COUNT(*) FROM lan_users WHERE role = $1 AND disabled_at IS NULL`, models.LanRoleAdmin)
	return n, err
}

func (r *UserRepository) Create(u *models.LanUser) error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	q := `INSERT INTO lan_users (email, password_hash, password_algo, role, display_name, disabled_at, created_at, updated_at)
	      VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`
	return r.db.QueryRow(q, u.Email, u.PasswordHash, u.PasswordAlgo, u.Role, u.DisplayName, u.DisabledAt, u.CreatedAt, u.UpdatedAt).Scan(&u.ID)
}

func (r *UserRepository) UpdatePassword(id int, hash, algo string) error {
	_, err := r.db.Exec(`UPDATE lan_users SET password_hash = $1, password_algo = $2, updated_at = NOW() WHERE id = $3`, hash, algo, id)
	return err
}

func (r *UserRepository) SetDisabled(id int, disabled bool) error {
	if disabled {
		_, err := r.db.Exec(`UPDATE lan_users SET disabled_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
		return err
	}
	_, err := r.db.Exec(`UPDATE lan_users SET disabled_at = NULL, updated_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *UserRepository) TouchLastLogin(id int) error {
	_, err := r.db.Exec(`UPDATE lan_users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	return err
}

func ValidateRole(role string) error {
	switch role {
	case models.LanRoleAdmin, models.LanRoleUser, models.LanRoleGuest:
		return nil
	default:
		return fmt.Errorf("invalid role: %s", role)
	}
}

func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email")
	}
	return nil
}
