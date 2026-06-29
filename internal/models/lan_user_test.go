package models

import "testing"

func TestCanUseTrustedDevices(t *testing.T) {
	tests := []struct {
		email string
		role  string
		want  bool
	}{
		{BootstrapLanAdminEmail, LanRoleAdmin, false},
		{"nicolas@rineau.eu", LanRoleAdmin, true},
		{"user@test.local", LanRoleUser, true},
		{"guest@test.local", LanRoleGuest, true},
	}
	for _, tc := range tests {
		u := &LanUser{Email: tc.email, Role: tc.role}
		if got := u.CanUseTrustedDevices(); got != tc.want {
			t.Fatalf("%s (%s): got %v want %v", tc.email, tc.role, got, tc.want)
		}
	}
}
