package laniam

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/jmoiron/sqlx"
)

// RememberRecord est la ligne d'une session mémorisée relue en base.
type RememberRecord struct {
	UserID    int
	CreatedAt time.Time
	ExpiresAt time.Time
}

// RememberStore persiste les sessions « Se souvenir de moi ». Il ne reçoit jamais
// l'identifiant de session, seulement son empreinte (voir HashSessionID).
type RememberStore interface {
	Create(idHash string, userID int, createdAt, expiresAt time.Time, userAgent string) error
	FindByHash(idHash string) (*RememberRecord, error)
	DeleteByHash(idHash string) error
	DeleteByUser(userID int) error
	PurgeExpired(now time.Time) (int64, error)
}

// HashSessionID retourne l'empreinte hexadécimale SHA-256 d'un identifiant de session.
func HashSessionID(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])
}

type RememberSessionRepository struct {
	db *sqlx.DB
}

func NewRememberSessionRepository(db *sqlx.DB) *RememberSessionRepository {
	return &RememberSessionRepository{db: db}
}

func (r *RememberSessionRepository) Create(idHash string, userID int, createdAt, expiresAt time.Time, userAgent string) error {
	_, err := r.db.Exec(
		`INSERT INTO lan_remember_sessions (id_hash, lan_user_id, created_at, expires_at, user_agent)
		 VALUES ($1, $2, $3, $4, $5)`,
		idHash, userID, createdAt, expiresAt, userAgent)
	return err
}

func (r *RememberSessionRepository) FindByHash(idHash string) (*RememberRecord, error) {
	var rec RememberRecord
	err := r.db.QueryRow(
		`SELECT lan_user_id, created_at, expires_at FROM lan_remember_sessions WHERE id_hash = $1`,
		idHash).Scan(&rec.UserID, &rec.CreatedAt, &rec.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *RememberSessionRepository) DeleteByHash(idHash string) error {
	_, err := r.db.Exec(`DELETE FROM lan_remember_sessions WHERE id_hash = $1`, idHash)
	return err
}

func (r *RememberSessionRepository) DeleteByUser(userID int) error {
	_, err := r.db.Exec(`DELETE FROM lan_remember_sessions WHERE lan_user_id = $1`, userID)
	return err
}

func (r *RememberSessionRepository) PurgeExpired(now time.Time) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM lan_remember_sessions WHERE expires_at <= $1`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
