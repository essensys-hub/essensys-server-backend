package laniam

import (
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type stubUserRepo struct {
	byID map[int]*models.LanUser
}

func (s *stubUserRepo) GetByEmail(string) (*models.LanUser, error) { return nil, nil }
func (s *stubUserRepo) List() ([]models.LanUser, error)            { return nil, nil }
func (s *stubUserRepo) CountActiveAdmins() (int, error)            { return 0, nil }
func (s *stubUserRepo) Create(*models.LanUser) error               { return nil }
func (s *stubUserRepo) UpdatePassword(int, string, string) error   { return nil }
func (s *stubUserRepo) SetDisabled(int, bool) error                { return nil }
func (s *stubUserRepo) TouchLastLogin(int) error                   { return nil }
func (s *stubUserRepo) GetByID(id int) (*models.LanUser, error)    { return s.byID[id], nil }

type stubTrustedRepo struct {
	devicesByUser map[int][]models.TrustedDevice
	devicesByMAC  map[string][]models.TrustedDevice
	upserted      *models.TrustedDevice
	touchedID     int
}

func (s *stubTrustedRepo) ListByUserID(userID int) ([]models.TrustedDevice, error) {
	return s.devicesByUser[userID], nil
}
func (s *stubTrustedRepo) ListAll() ([]models.TrustedDevice, error) { return nil, nil }
func (s *stubTrustedRepo) ListActiveByMAC(mac string) ([]models.TrustedDevice, error) {
	return s.devicesByMAC[mac], nil
}
func (s *stubTrustedRepo) UpsertTemporary(userID int, macAddress, deviceLabel string, createdByUserID int, expiresAt time.Time, lastSeenAt *time.Time) (*models.TrustedDevice, error) {
	device := &models.TrustedDevice{ID: 10, LanUserID: userID, MacAddress: macAddress, DeviceLabel: deviceLabel, TrustMode: models.TrustedDeviceModeTemporary, ExpiresAt: &expiresAt}
	s.upserted = device
	return device, nil
}
func (s *stubTrustedRepo) CreatePermanent(int, string, string, int, int, *time.Time) (*models.TrustedDevice, error) {
	return nil, nil
}
func (s *stubTrustedRepo) PromotePermanent(int, int) error { return nil }
func (s *stubTrustedRepo) RevokeForUser(int, int) error    { return nil }
func (s *stubTrustedRepo) RevokeByID(int) error            { return nil }
func (s *stubTrustedRepo) TouchLastSeen(id int, _ time.Time) error {
	s.touchedID = id
	return nil
}
func (s *stubTrustedRepo) GetByID(int) (*models.TrustedDevice, error) { return nil, nil }

type stubResolver struct {
	candidates []models.TrustedDeviceCandidate
}

func (s stubResolver) ResolveCandidates(string) ([]models.TrustedDeviceCandidate, error) {
	return s.candidates, nil
}

func (s stubResolver) ResolveClientMAC(string) (models.TrustedDeviceCandidate, bool) {
	if len(s.candidates) == 0 {
		return models.TrustedDeviceCandidate{}, false
	}
	return s.candidates[0], true
}

func TestTrustCurrentDeviceSetsTemporary60DayExpiry(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	repo := &stubTrustedRepo{}
	svc := &Service{
		repo:        &stubUserRepo{},
		trustedRepo: repo,
		resolver:    stubResolver{},
		sessions:    NewSessionStore(168),
		now:         func() time.Time { return now },
	}
	user := &models.LanUser{ID: 7, Role: models.LanRoleUser}

	device, err := svc.TrustCurrentDevice(user, models.TrustedDeviceCandidate{MacAddress: "aa:bb:cc:dd:ee:ff", DeviceLabel: "Tablette salon"})
	if err != nil {
		t.Fatalf("TrustCurrentDevice error: %v", err)
	}
	if device == nil || repo.upserted == nil {
		t.Fatalf("expected upserted trusted device")
	}
	want := now.Add(60 * 24 * time.Hour)
	if repo.upserted.ExpiresAt == nil || !repo.upserted.ExpiresAt.Equal(want) {
		t.Fatalf("unexpected expiry: got %v want %v", repo.upserted.ExpiresAt, want)
	}
}

func TestAutoLoginRejectsAmbiguousMAC(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	repo := &stubUserRepo{byID: map[int]*models.LanUser{
		1: {ID: 1, Role: models.LanRoleUser, Email: "a@test.local"},
		2: {ID: 2, Role: models.LanRoleGuest, Email: "b@test.local"},
	}}
	trustedRepo := &stubTrustedRepo{devicesByMAC: map[string][]models.TrustedDevice{
		"AA:BB:CC:DD:EE:FF": {
			{ID: 11, LanUserID: 1, MacAddress: "AA:BB:CC:DD:EE:FF", TrustMode: models.TrustedDeviceModeTemporary, ExpiresAt: ptrTime(now.Add(time.Hour))},
			{ID: 12, LanUserID: 2, MacAddress: "AA:BB:CC:DD:EE:FF", TrustMode: models.TrustedDeviceModePermanent},
		},
	}}
	svc := &Service{
		repo:        repo,
		trustedRepo: trustedRepo,
		resolver: stubResolver{candidates: []models.TrustedDeviceCandidate{{
			MacAddress: "aa:bb:cc:dd:ee:ff",
			SourceIP:   "192.168.0.42",
		}}},
		sessions: NewSessionStore(168),
		now:      func() time.Time { return now },
	}

	user, _, err := svc.AutoLogin("192.168.0.42")
	if err == nil || err.Error() != "ambiguous_trusted_device" {
		t.Fatalf("expected ambiguous_trusted_device, got user=%v err=%v", user, err)
	}
}

func TestAutoLoginIgnoresBootstrapAdminTrustedDevices(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	repo := &stubUserRepo{byID: map[int]*models.LanUser{
		1: {ID: 1, Role: models.LanRoleAdmin, Email: models.BootstrapLanAdminEmail},
	}}
	trustedRepo := &stubTrustedRepo{devicesByMAC: map[string][]models.TrustedDevice{
		"AA:BB:CC:DD:EE:FF": {
			{ID: 11, LanUserID: 1, MacAddress: "AA:BB:CC:DD:EE:FF", TrustMode: models.TrustedDeviceModePermanent},
		},
	}}
	svc := &Service{
		repo:        repo,
		trustedRepo: trustedRepo,
		resolver:    stubResolver{candidates: []models.TrustedDeviceCandidate{{MacAddress: "AA:BB:CC:DD:EE:FF"}}},
		sessions:    NewSessionStore(168),
		now:         func() time.Time { return now },
	}

	user, device, err := svc.AutoLogin("192.168.0.42")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if user != nil || device != nil {
		t.Fatalf("expected admin trusted device to be ignored, got user=%v device=%v", user, device)
	}
}

func ptrTime(v time.Time) *time.Time { return &v }
