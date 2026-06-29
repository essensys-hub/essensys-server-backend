package models

import "time"

const (
	TrustedDeviceModeTemporary = "temporary"
	TrustedDeviceModePermanent = "permanent"
)

type TrustedDevice struct {
	ID                    int        `db:"id" json:"id"`
	LanUserID             int        `db:"lan_user_id" json:"lan_user_id"`
	MacAddress            string     `db:"mac_address" json:"mac_address"`
	DeviceLabel           string     `db:"device_label" json:"device_label"`
	TrustMode             string     `db:"trust_mode" json:"trust_mode"`
	ExpiresAt             *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	CreatedByUserID       *int       `db:"created_by_user_id" json:"created_by_user_id,omitempty"`
	ApprovedByAdminUserID *int       `db:"approved_by_admin_user_id" json:"approved_by_admin_user_id,omitempty"`
	LastSeenAt            *time.Time `db:"last_seen_at" json:"last_seen_at,omitempty"`
	RevokedAt             *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	CreatedAt             time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time  `db:"updated_at" json:"updated_at"`

	LanUserEmail       string `db:"lan_user_email" json:"lan_user_email,omitempty"`
	LanUserRole        string `db:"lan_user_role" json:"lan_user_role,omitempty"`
	LanUserDisplayName string `db:"lan_user_display_name" json:"lan_user_display_name,omitempty"`
}

type TrustedDeviceCandidate struct {
	MacAddress  string     `json:"mac_address"`
	DeviceLabel string     `json:"device_label"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	SourceIP    string     `json:"source_ip,omitempty"`
	LanUserID          *int   `json:"lan_user_id,omitempty"`
	LanUserEmail       string `json:"lan_user_email,omitempty"`
	LanUserRole        string `json:"lan_user_role,omitempty"`
	LanUserDisplayName string `json:"lan_user_display_name,omitempty"`
}

func (d *TrustedDevice) IsRevoked() bool {
	return d.RevokedAt != nil
}

func (d *TrustedDevice) IsExpired(now time.Time) bool {
	return d.ExpiresAt != nil && !d.ExpiresAt.After(now)
}

func (d *TrustedDevice) IsActive(now time.Time) bool {
	return !d.IsRevoked() && !d.IsExpired(now)
}
