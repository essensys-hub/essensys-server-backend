package laniam

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

const trustedDeviceTemporaryTTL = 60 * 24 * time.Hour

type userRepository interface {
	GetByEmail(email string) (*models.LanUser, error)
	GetByID(id int) (*models.LanUser, error)
	List() ([]models.LanUser, error)
	CountActiveAdmins() (int, error)
	Create(u *models.LanUser) error
	UpdatePassword(id int, hash, algo string) error
	SetDisabled(id int, disabled bool) error
	TouchLastLogin(id int) error
}

type trustedDeviceRepository interface {
	ListByUserID(userID int) ([]models.TrustedDevice, error)
	ListAll() ([]models.TrustedDevice, error)
	ListActiveByMAC(mac string) ([]models.TrustedDevice, error)
	UpsertTemporary(userID int, macAddress, deviceLabel string, createdByUserID int, expiresAt time.Time, lastSeenAt *time.Time) (*models.TrustedDevice, error)
	CreatePermanent(userID int, macAddress, deviceLabel string, createdByUserID, approvedByAdminUserID int, lastSeenAt *time.Time) (*models.TrustedDevice, error)
	PromotePermanent(id int, approvedByAdminUserID int) error
	RevokeForUser(id, userID int) error
	RevokeByID(id int) error
	TouchLastSeen(id int, seenAt time.Time) error
	GetByID(id int) (*models.TrustedDevice, error)
}

type loginClientRepository interface {
	Upsert(userID int, macAddress, sourceIP, deviceLabel string) error
	ListByUserID(userID int) ([]models.LanLoginClient, error)
	ListRecentPairable() ([]models.LanLoginClient, error)
}

type Service struct {
	repo               userRepository
	trustedRepo        trustedDeviceRepository
	loginClientRepo    loginClientRepository
	resolver           DeviceResolver
	sessions           *SessionStore
	bootstrapTokenFile string
	now                func() time.Time
}

func NewService(repo *UserRepository, trustedRepo *TrustedDeviceRepository, loginClientRepo *LoginClientRepository, resolver DeviceResolver, sessions *SessionStore, bootstrapTokenFile string) *Service {
	if resolver == nil {
		resolver = NewNeighbourResolver()
	}
	return &Service{
		repo:               repo,
		trustedRepo:        trustedRepo,
		loginClientRepo:    loginClientRepo,
		resolver:           resolver,
		sessions:           sessions,
		bootstrapTokenFile: bootstrapTokenFile,
		now:                time.Now,
	}
}

func (s *Service) Login(email, password string) (*models.LanUser, error) {
	user, err := s.repo.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	if user.IsDisabled() {
		return nil, fmt.Errorf("account_disabled")
	}
	ok, err := VerifyPassword(user, password)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("invalid credentials")
	}
	_ = s.repo.TouchLastLogin(user.ID)
	return user, nil
}

func (s *Service) RecordLoginClient(userID int, clientIP string) {
	if s.loginClientRepo == nil || s.resolver == nil || clientIP == "" {
		return
	}
	candidate, ok := s.resolver.ResolveClientMAC(clientIP)
	if !ok {
		return
	}
	normalized, ok := NormalizeCandidate(candidate)
	if !ok {
		return
	}
	_ = s.loginClientRepo.Upsert(userID, normalized.MacAddress, normalized.SourceIP, normalized.DeviceLabel)
}

func (s *Service) AutoLogin(clientIP string) (*models.LanUser, *models.TrustedDevice, error) {
	if s.trustedRepo == nil || s.resolver == nil {
		return nil, nil, nil
	}
	candidates, err := s.resolver.ResolveCandidates(clientIP)
	if err != nil {
		return nil, nil, err
	}
	matches, err := s.matchTrustedUsers(candidates)
	if err != nil {
		return nil, nil, err
	}
	if len(matches) == 0 {
		return nil, nil, nil
	}
	if len(matches) > 1 {
		return nil, nil, fmt.Errorf("ambiguous_trusted_device")
	}
	match := matches[0]
	if match.user.IsDisabled() || !match.user.CanUseTrustedDevices() {
		return nil, nil, nil
	}
	if err := s.repo.TouchLastLogin(match.user.ID); err != nil {
		return nil, nil, err
	}
	if err := s.trustedRepo.TouchLastSeen(match.device.ID, s.now().UTC()); err != nil {
		return nil, nil, err
	}
	s.RecordLoginClient(match.user.ID, clientIP)
	return match.user, match.device, nil
}

func (s *Service) CreateUser(email, plainPassword, role, displayName string) (*models.LanUser, error) {
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}
	if err := ValidateRole(role); err != nil {
		return nil, err
	}
	if len(plainPassword) < 8 {
		return nil, fmt.Errorf("password too short")
	}
	exists, err := s.repo.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	if exists != nil {
		return nil, fmt.Errorf("email already exists")
	}
	hash, algo, err := HashPassword(plainPassword)
	if err != nil {
		return nil, err
	}
	u := &models.LanUser{
		Email:        strings.TrimSpace(email),
		PasswordHash: hash,
		PasswordAlgo: algo,
		Role:         role,
		DisplayName:  displayName,
	}
	if err := s.repo.Create(u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) BootstrapAdmin(email, plainPassword, providedToken string) (*models.LanUser, error) {
	n, err := s.repo.CountActiveAdmins()
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("admin already exists")
	}
	expected, err := os.ReadFile(s.bootstrapTokenFile)
	if err != nil {
		return nil, fmt.Errorf("bootstrap token unavailable")
	}
	if strings.TrimSpace(providedToken) != strings.TrimSpace(string(expected)) {
		return nil, fmt.Errorf("invalid bootstrap token")
	}
	u, err := s.CreateUser(email, plainPassword, models.LanRoleAdmin, "LAN Admin")
	if err != nil {
		return nil, err
	}
	_ = os.Remove(s.bootstrapTokenFile)
	return u, nil
}

func (s *Service) ChangePassword(userID int, currentPlain, newPlain string) error {
	if len(newPlain) < 8 {
		return fmt.Errorf("password too short")
	}
	user, err := s.repo.GetByID(userID)
	if err != nil || user == nil {
		return fmt.Errorf("user not found")
	}
	ok, err := VerifyPassword(user, currentPlain)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("invalid credentials")
	}
	hash, algo, err := HashPassword(newPlain)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(userID, hash, algo); err != nil {
		return err
	}
	s.sessions.DeleteSessionsForUser(userID)
	return nil
}

func (s *Service) ResetPassword(admin *models.LanUser, targetID int, newPlain string) error {
	if !admin.CanManageLanUsers() {
		return fmt.Errorf("forbidden")
	}
	if len(newPlain) < 8 {
		return fmt.Errorf("password too short")
	}
	target, err := s.repo.GetByID(targetID)
	if err != nil || target == nil {
		return fmt.Errorf("user not found")
	}
	hash, algo, err := HashPassword(newPlain)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(targetID, hash, algo); err != nil {
		return err
	}
	s.sessions.DeleteSessionsForUser(targetID)
	return nil
}

func (s *Service) DisableUser(admin *models.LanUser, targetID int) error {
	if !admin.CanManageLanUsers() {
		return fmt.Errorf("forbidden")
	}
	if admin.ID == targetID {
		return fmt.Errorf("cannot disable self")
	}
	target, err := s.repo.GetByID(targetID)
	if err != nil || target == nil {
		return fmt.Errorf("user not found")
	}
	if target.Role == models.LanRoleAdmin {
		n, err := s.repo.CountActiveAdmins()
		if err != nil {
			return err
		}
		if n <= 1 && !target.IsDisabled() {
			return fmt.Errorf("cannot disable last admin")
		}
	}
	if err := s.repo.SetDisabled(targetID, true); err != nil {
		return err
	}
	s.sessions.DeleteSessionsForUser(targetID)
	return nil
}

func (s *Service) EnableUser(admin *models.LanUser, targetID int) error {
	if !admin.CanManageLanUsers() {
		return fmt.Errorf("forbidden")
	}
	target, err := s.repo.GetByID(targetID)
	if err != nil || target == nil {
		return fmt.Errorf("user not found")
	}
	if !target.IsDisabled() {
		return nil
	}
	return s.repo.SetDisabled(targetID, false)
}

func (s *Service) ListUsers(admin *models.LanUser) ([]models.LanUser, error) {
	if !admin.CanManageLanUsers() {
		return nil, fmt.Errorf("forbidden")
	}
	return s.repo.List()
}

func (s *Service) ListTrustedDevices(user *models.LanUser) ([]models.TrustedDevice, error) {
	if !user.CanUseTrustedDevices() {
		return nil, fmt.Errorf("forbidden")
	}
	if s.trustedRepo == nil {
		return nil, nil
	}
	devices, err := s.trustedRepo.ListByUserID(user.ID)
	if err != nil {
		return nil, err
	}
	return s.filterActiveDevices(devices), nil
}

func (s *Service) ListTrustedDeviceCandidates(user *models.LanUser, clientIP string) ([]models.TrustedDeviceCandidate, error) {
	if !user.CanUseTrustedDevices() {
		return nil, fmt.Errorf("forbidden")
	}
	if s.loginClientRepo != nil {
		rows, err := s.loginClientRepo.ListByUserID(user.ID)
		if err != nil {
			return nil, err
		}
		return loginClientsToCandidates(rows), nil
	}
	return s.candidatesForClientIP(clientIP)
}

func (s *Service) TrustCurrentDevice(user *models.LanUser, candidate models.TrustedDeviceCandidate) (*models.TrustedDevice, error) {
	if !user.CanUseTrustedDevices() {
		return nil, fmt.Errorf("forbidden")
	}
	if s.trustedRepo == nil {
		return nil, fmt.Errorf("trusted devices unavailable")
	}
	normalized, ok := NormalizeCandidate(candidate)
	if !ok {
		return nil, fmt.Errorf("invalid_mac_address")
	}
	expiresAt := s.now().UTC().Add(trustedDeviceTemporaryTTL)
	return s.trustedRepo.UpsertTemporary(user.ID, normalized.MacAddress, normalized.DeviceLabel, user.ID, expiresAt, normalized.LastSeenAt)
}

func (s *Service) RevokeTrustedDevice(user *models.LanUser, deviceID int) error {
	if !user.CanUseTrustedDevices() {
		return fmt.Errorf("forbidden")
	}
	if s.trustedRepo == nil {
		return fmt.Errorf("trusted devices unavailable")
	}
	return s.trustedRepo.RevokeForUser(deviceID, user.ID)
}

func (s *Service) ListAdminTrustedDevices(admin *models.LanUser) ([]models.TrustedDevice, error) {
	if !admin.CanManageLanUsers() {
		return nil, fmt.Errorf("forbidden")
	}
	if s.trustedRepo == nil {
		return nil, nil
	}
	devices, err := s.trustedRepo.ListAll()
	if err != nil {
		return nil, err
	}
	return s.filterActiveDevices(devices), nil
}

func (s *Service) ListAdminTrustedDeviceCandidates(admin *models.LanUser) ([]models.TrustedDeviceCandidate, error) {
	if !admin.CanManageLanUsers() {
		return nil, fmt.Errorf("forbidden")
	}
	if s.loginClientRepo != nil {
		rows, err := s.loginClientRepo.ListRecentPairable()
		if err != nil {
			return nil, err
		}
		return loginClientsToCandidates(rows), nil
	}
	return nil, nil
}

func (s *Service) CreateAdminTrustedDevice(admin *models.LanUser, targetUserID int, candidate models.TrustedDeviceCandidate) (*models.TrustedDevice, error) {
	if !admin.CanManageLanUsers() {
		return nil, fmt.Errorf("forbidden")
	}
	if s.trustedRepo == nil {
		return nil, fmt.Errorf("trusted devices unavailable")
	}
	target, err := s.repo.GetByID(targetUserID)
	if err != nil || target == nil {
		return nil, fmt.Errorf("user not found")
	}
	if target == nil || !target.CanUseTrustedDevices() {
		return nil, fmt.Errorf("admin_trusted_device_forbidden")
	}
	normalized, ok := NormalizeCandidate(candidate)
	if !ok {
		return nil, fmt.Errorf("invalid_mac_address")
	}
	return s.trustedRepo.CreatePermanent(target.ID, normalized.MacAddress, normalized.DeviceLabel, admin.ID, admin.ID, normalized.LastSeenAt)
}

func (s *Service) PromoteTrustedDevice(admin *models.LanUser, deviceID int) error {
	if !admin.CanManageLanUsers() {
		return fmt.Errorf("forbidden")
	}
	if s.trustedRepo == nil {
		return fmt.Errorf("trusted devices unavailable")
	}
	device, err := s.trustedRepo.GetByID(deviceID)
	if err != nil || device == nil {
		return fmt.Errorf("trusted_device_not_found")
	}
	target, err := s.repo.GetByID(device.LanUserID)
	if err != nil {
		return err
	}
	if target == nil || !target.CanUseTrustedDevices() {
		return fmt.Errorf("admin_trusted_device_forbidden")
	}
	return s.trustedRepo.PromotePermanent(deviceID, admin.ID)
}

func (s *Service) RevokeTrustedDeviceAdmin(admin *models.LanUser, deviceID int) error {
	if !admin.CanManageLanUsers() {
		return fmt.Errorf("forbidden")
	}
	if s.trustedRepo == nil {
		return fmt.Errorf("trusted devices unavailable")
	}
	return s.trustedRepo.RevokeByID(deviceID)
}

func (s *Service) Repo() userRepository    { return s.repo }
func (s *Service) Sessions() *SessionStore { return s.sessions }

func (s *Service) resolveCandidates(clientIP string) ([]models.TrustedDeviceCandidate, error) {
	if s.resolver == nil {
		return nil, nil
	}
	candidates, err := s.resolver.ResolveCandidates(clientIP)
	if err != nil {
		return nil, err
	}
	unique := map[string]models.TrustedDeviceCandidate{}
	for _, candidate := range candidates {
		normalized, ok := NormalizeCandidate(candidate)
		if !ok {
			continue
		}
		unique[normalized.MacAddress] = normalized
	}
	ordered := make([]models.TrustedDeviceCandidate, 0, len(unique))
	for _, candidate := range unique {
		ordered = append(ordered, candidate)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		leftCurrent := clientIP != "" && ordered[i].SourceIP == clientIP
		rightCurrent := clientIP != "" && ordered[j].SourceIP == clientIP
		if leftCurrent != rightCurrent {
			return leftCurrent
		}
		return ordered[i].MacAddress < ordered[j].MacAddress
	})
	return ordered, nil
}

type trustedMatch struct {
	user   *models.LanUser
	device *models.TrustedDevice
}

func (s *Service) matchTrustedUsers(candidates []models.TrustedDeviceCandidate) ([]trustedMatch, error) {
	if s.trustedRepo == nil {
		return nil, nil
	}
	now := s.now().UTC()
	byUser := map[int]trustedMatch{}
	for _, candidate := range candidates {
		normalized, ok := NormalizeCandidate(candidate)
		if !ok {
			continue
		}
		devices, err := s.trustedRepo.ListActiveByMAC(normalized.MacAddress)
		if err != nil {
			return nil, err
		}
		for _, device := range devices {
			if !device.IsActive(now) {
				continue
			}
			user, err := s.repo.GetByID(device.LanUserID)
			if err != nil {
				return nil, err
			}
			if user == nil || user.IsDisabled() || !user.CanUseTrustedDevices() {
				continue
			}
			if _, exists := byUser[user.ID]; !exists {
				deviceCopy := device
				byUser[user.ID] = trustedMatch{user: user, device: &deviceCopy}
			}
		}
	}
	matches := make([]trustedMatch, 0, len(byUser))
	for _, match := range byUser {
		matches = append(matches, match)
	}
	return matches, nil
}

func (s *Service) filterActiveDevices(devices []models.TrustedDevice) []models.TrustedDevice {
	now := s.now().UTC()
	filtered := make([]models.TrustedDevice, 0, len(devices))
	for _, device := range devices {
		if device.IsActive(now) {
			filtered = append(filtered, device)
		}
	}
	return filtered
}

func loginClientsToCandidates(rows []models.LanLoginClient) []models.TrustedDeviceCandidate {
	out := make([]models.TrustedDeviceCandidate, 0, len(rows))
	for _, row := range rows {
		if row.MacAddress == "" {
			continue
		}
		seen := row.LastLoginAt
		userID := row.LanUserID
		out = append(out, models.TrustedDeviceCandidate{
			MacAddress:         row.MacAddress,
			DeviceLabel:        row.DeviceLabel,
			SourceIP:           row.SourceIP,
			LastSeenAt:         &seen,
			LanUserID:          &userID,
			LanUserEmail:       row.LanUserEmail,
			LanUserRole:        row.LanUserRole,
			LanUserDisplayName: row.LanUserDisplayName,
		})
	}
	return out
}

func (s *Service) candidatesForClientIP(clientIP string) ([]models.TrustedDeviceCandidate, error) {
	if clientIP == "" || s.resolver == nil {
		return nil, nil
	}
	candidate, ok := s.resolver.ResolveClientMAC(clientIP)
	if !ok {
		return nil, nil
	}
	return []models.TrustedDeviceCandidate{candidate}, nil
}
