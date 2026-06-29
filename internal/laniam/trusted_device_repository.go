package laniam

import (
	"database/sql"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/jmoiron/sqlx"
)

type TrustedDeviceRepository struct {
	db *sqlx.DB
}

func NewTrustedDeviceRepository(db *sqlx.DB) *TrustedDeviceRepository {
	return &TrustedDeviceRepository{db: db}
}

func (r *TrustedDeviceRepository) ListByUserID(userID int) ([]models.TrustedDevice, error) {
	var devices []models.TrustedDevice
	err := r.db.Select(&devices, trustedDeviceSelect+` WHERE td.lan_user_id = $1 ORDER BY td.created_at DESC`, userID)
	return devices, err
}

func (r *TrustedDeviceRepository) ListAll() ([]models.TrustedDevice, error) {
	var devices []models.TrustedDevice
	err := r.db.Select(&devices, trustedDeviceSelect+` ORDER BY td.created_at DESC`)
	return devices, err
}

func (r *TrustedDeviceRepository) ListActiveByMAC(mac string) ([]models.TrustedDevice, error) {
	var devices []models.TrustedDevice
	err := r.db.Select(&devices, trustedDeviceSelect+` WHERE td.mac_address = $1 AND td.revoked_at IS NULL ORDER BY td.created_at DESC`, normalizeMAC(mac))
	return devices, err
}

func (r *TrustedDeviceRepository) UpsertTemporary(userID int, macAddress, deviceLabel string, createdByUserID int, expiresAt time.Time, lastSeenAt *time.Time) (*models.TrustedDevice, error) {
	mac := normalizeMAC(macAddress)
	label := strings.TrimSpace(deviceLabel)
	var device models.TrustedDevice
	query := `
INSERT INTO trusted_devices (
	lan_user_id, mac_address, device_label, trust_mode, expires_at,
	created_by_user_id, approved_by_admin_user_id, last_seen_at, revoked_at, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, NULL, NOW(), NOW())
ON CONFLICT (lan_user_id, mac_address) WHERE revoked_at IS NULL
DO UPDATE SET
	device_label = EXCLUDED.device_label,
	trust_mode = EXCLUDED.trust_mode,
	expires_at = EXCLUDED.expires_at,
	created_by_user_id = EXCLUDED.created_by_user_id,
	approved_by_admin_user_id = NULL,
	last_seen_at = COALESCE(EXCLUDED.last_seen_at, trusted_devices.last_seen_at),
	revoked_at = NULL,
	updated_at = NOW()
RETURNING id, lan_user_id, mac_address, device_label, trust_mode, expires_at,
	created_by_user_id, approved_by_admin_user_id, last_seen_at, revoked_at, created_at, updated_at`
	if err := r.db.Get(&device, query, userID, mac, label, models.TrustedDeviceModeTemporary, expiresAt, createdByUserID, lastSeenAt); err != nil {
		return nil, err
	}
	return r.GetByID(device.ID)
}

func (r *TrustedDeviceRepository) CreatePermanent(userID int, macAddress, deviceLabel string, createdByUserID, approvedByAdminUserID int, lastSeenAt *time.Time) (*models.TrustedDevice, error) {
	mac := normalizeMAC(macAddress)
	label := strings.TrimSpace(deviceLabel)
	var device models.TrustedDevice
	query := `
INSERT INTO trusted_devices (
	lan_user_id, mac_address, device_label, trust_mode, expires_at,
	created_by_user_id, approved_by_admin_user_id, last_seen_at, revoked_at, created_at, updated_at
)
VALUES ($1, $2, $3, $4, NULL, $5, $6, $7, NULL, NOW(), NOW())
ON CONFLICT (lan_user_id, mac_address) WHERE revoked_at IS NULL
DO UPDATE SET
	device_label = EXCLUDED.device_label,
	trust_mode = EXCLUDED.trust_mode,
	expires_at = NULL,
	created_by_user_id = EXCLUDED.created_by_user_id,
	approved_by_admin_user_id = EXCLUDED.approved_by_admin_user_id,
	last_seen_at = COALESCE(EXCLUDED.last_seen_at, trusted_devices.last_seen_at),
	revoked_at = NULL,
	updated_at = NOW()
RETURNING id, lan_user_id, mac_address, device_label, trust_mode, expires_at,
	created_by_user_id, approved_by_admin_user_id, last_seen_at, revoked_at, created_at, updated_at`
	if err := r.db.Get(&device, query, userID, mac, label, models.TrustedDeviceModePermanent, createdByUserID, approvedByAdminUserID, lastSeenAt); err != nil {
		return nil, err
	}
	return r.GetByID(device.ID)
}

func (r *TrustedDeviceRepository) PromotePermanent(id int, approvedByAdminUserID int) error {
	_, err := r.db.Exec(`
UPDATE trusted_devices
SET trust_mode = $2,
	expires_at = NULL,
	approved_by_admin_user_id = $3,
	updated_at = NOW()
WHERE id = $1 AND revoked_at IS NULL
`, id, models.TrustedDeviceModePermanent, approvedByAdminUserID)
	return err
}

func (r *TrustedDeviceRepository) RevokeForUser(id, userID int) error {
	_, err := r.db.Exec(`UPDATE trusted_devices SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1 AND lan_user_id = $2 AND revoked_at IS NULL`, id, userID)
	return err
}

func (r *TrustedDeviceRepository) RevokeByID(id int) error {
	_, err := r.db.Exec(`UPDATE trusted_devices SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

func (r *TrustedDeviceRepository) TouchLastSeen(id int, seenAt time.Time) error {
	_, err := r.db.Exec(`UPDATE trusted_devices SET last_seen_at = $2, updated_at = NOW() WHERE id = $1`, id, seenAt)
	return err
}

func (r *TrustedDeviceRepository) GetByID(id int) (*models.TrustedDevice, error) {
	var device models.TrustedDevice
	err := r.db.Get(&device, trustedDeviceSelect+` WHERE td.id = $1`, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &device, nil
}

const trustedDeviceSelect = `
SELECT
	td.id,
	td.lan_user_id,
	td.mac_address,
	td.device_label,
	td.trust_mode,
	td.expires_at,
	td.created_by_user_id,
	td.approved_by_admin_user_id,
	td.last_seen_at,
	td.revoked_at,
	td.created_at,
	td.updated_at,
	lu.email AS lan_user_email,
	lu.role AS lan_user_role,
	lu.display_name AS lan_user_display_name
FROM trusted_devices td
JOIN lan_users lu ON lu.id = td.lan_user_id`
