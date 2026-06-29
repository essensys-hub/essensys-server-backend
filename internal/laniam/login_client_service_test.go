package laniam

import (
	"testing"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type stubLoginClientRepo struct {
	byUser      map[int][]models.LanLoginClient
	pairable    []models.LanLoginClient
	lastUpsert  struct {
		userID int
		mac    string
		ip     string
	}
}

func (s *stubLoginClientRepo) Upsert(userID int, macAddress, sourceIP, deviceLabel string) error {
	s.lastUpsert.userID = userID
	s.lastUpsert.mac = macAddress
	s.lastUpsert.ip = sourceIP
	return nil
}

func (s *stubLoginClientRepo) ListByUserID(userID int) ([]models.LanLoginClient, error) {
	return s.byUser[userID], nil
}

func (s *stubLoginClientRepo) ListRecentPairable() ([]models.LanLoginClient, error) {
	return s.pairable, nil
}

func TestListAdminTrustedDeviceCandidatesFromLogins(t *testing.T) {
	seen := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	svc := &Service{
		repo: &stubUserRepo{},
		loginClientRepo: &stubLoginClientRepo{
			pairable: []models.LanLoginClient{{
				LanUserID:          2,
				MacAddress:         "AA:BB:CC:DD:EE:FF",
				SourceIP:           "192.168.1.236",
				DeviceLabel:        "192.168.1.236",
				LastLoginAt:        seen,
				LanUserEmail:       "user@test.local",
				LanUserRole:        models.LanRoleUser,
				LanUserDisplayName: "Test",
			}},
		},
		sessions: NewSessionStore(168),
	}
	admin := &models.LanUser{ID: 1, Role: models.LanRoleAdmin}
	candidates, err := svc.ListAdminTrustedDeviceCandidates(admin)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(candidates) != 1 || candidates[0].LanUserEmail != "user@test.local" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestRecordLoginClientStoresMAC(t *testing.T) {
	repo := &stubLoginClientRepo{}
	svc := &Service{
		loginClientRepo: repo,
		resolver: stubResolver{candidates: []models.TrustedDeviceCandidate{{
			MacAddress:  "aa:bb:cc:dd:ee:ff",
			SourceIP:    "192.168.1.10",
			DeviceLabel: "192.168.1.10",
		}}},
	}
	svc.RecordLoginClient(7, "192.168.1.10")
	if repo.lastUpsert.userID != 7 || repo.lastUpsert.mac != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("unexpected upsert: %+v", repo.lastUpsert)
	}
}
