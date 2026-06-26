package models

import "time"

const (
	LanRoleAdmin = "lan_admin"
	LanRoleUser  = "lan_user"
	LanRoleGuest = "lan_guest"

	PasswordAlgoBcrypt     = "bcrypt"
	PasswordAlgoSHA1Legacy = "sha1_legacy"
)

// LanUser is a local gateway IAM account (OpenSpec 2026-06.017).
type LanUser struct {
	ID           int        `db:"id" json:"id"`
	Email        string     `db:"email" json:"email"`
	PasswordHash string     `db:"password_hash" json:"-"`
	PasswordAlgo string     `db:"password_algo" json:"-"`
	Role         string     `db:"role" json:"role"`
	DisplayName  string     `db:"display_name" json:"display_name"`
	DisabledAt   *time.Time `db:"disabled_at" json:"disabled_at,omitempty"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	LastLoginAt  *time.Time `db:"last_login_at" json:"last_login_at,omitempty"`
}

func (u *LanUser) IsDisabled() bool {
	return u.DisabledAt != nil
}

func (u *LanUser) CanManageLanUsers() bool {
	return u.Role == LanRoleAdmin
}

func (u *LanUser) CanPilotDomotics() bool {
	switch u.Role {
	case LanRoleAdmin, LanRoleUser, LanRoleGuest:
		return true
	default:
		return false
	}
}
