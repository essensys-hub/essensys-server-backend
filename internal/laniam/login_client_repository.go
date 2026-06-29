package laniam

import (
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

type LoginClientRepository struct {
	db *sqlx.DB
}

func NewLoginClientRepository(db *sqlx.DB) *LoginClientRepository {
	return &LoginClientRepository{db: db}
}

func (r *LoginClientRepository) Upsert(userID int, macAddress, sourceIP, deviceLabel string) error {
	_, err := r.db.Exec(`
INSERT INTO lan_login_clients (lan_user_id, mac_address, source_ip, device_label, last_login_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (lan_user_id, mac_address) DO UPDATE SET
    source_ip = EXCLUDED.source_ip,
    device_label = EXCLUDED.device_label,
    last_login_at = NOW(),
    updated_at = NOW()
`, userID, macAddress, sourceIP, deviceLabel)
	return err
}

func (r *LoginClientRepository) ListByUserID(userID int) ([]models.LanLoginClient, error) {
	var rows []models.LanLoginClient
	err := r.db.Select(&rows, `
SELECT id, lan_user_id, mac_address, source_ip, device_label, last_login_at, created_at, updated_at
FROM lan_login_clients
WHERE lan_user_id = $1
ORDER BY last_login_at DESC
LIMIT 50
`, userID)
	return rows, err
}

func (r *LoginClientRepository) ListRecentPairable() ([]models.LanLoginClient, error) {
	var rows []models.LanLoginClient
	err := r.db.Select(&rows, `
SELECT
    lc.id,
    lc.lan_user_id,
    lc.mac_address,
    lc.source_ip,
    lc.device_label,
    lc.last_login_at,
    lc.created_at,
    lc.updated_at,
    u.email AS lan_user_email,
    u.role AS lan_user_role,
    u.display_name AS lan_user_display_name
FROM lan_login_clients lc
JOIN lan_users u ON u.id = lc.lan_user_id
WHERE u.disabled_at IS NULL
  AND LOWER(u.email) <> LOWER('admin@essensys.local')
ORDER BY lc.last_login_at DESC
LIMIT 100
`)
	return rows, err
}
