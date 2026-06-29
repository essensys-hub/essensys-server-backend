package models

import "time"

// LanLoginClient records a successful LAN login observed from a client MAC.
type LanLoginClient struct {
	ID           int       `db:"id" json:"id"`
	LanUserID    int       `db:"lan_user_id" json:"lan_user_id"`
	MacAddress   string    `db:"mac_address" json:"mac_address"`
	SourceIP     string    `db:"source_ip" json:"source_ip"`
	DeviceLabel  string    `db:"device_label" json:"device_label"`
	LastLoginAt  time.Time `db:"last_login_at" json:"last_login_at"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at" json:"updated_at"`
	LanUserEmail string    `db:"lan_user_email" json:"lan_user_email,omitempty"`
	LanUserRole  string    `db:"lan_user_role" json:"lan_user_role,omitempty"`
	LanUserDisplayName string `db:"lan_user_display_name" json:"lan_user_display_name,omitempty"`
}
