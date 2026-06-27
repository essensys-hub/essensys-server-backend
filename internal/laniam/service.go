package laniam

import (
	"fmt"
	"os"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type Service struct {
	repo               *UserRepository
	sessions           *SessionStore
	bootstrapTokenFile string
}

func NewService(repo *UserRepository, sessions *SessionStore, bootstrapTokenFile string) *Service {
	return &Service{
		repo:               repo,
		sessions:           sessions,
		bootstrapTokenFile: bootstrapTokenFile,
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

func (s *Service) Repo() *UserRepository { return s.repo }
func (s *Service) Sessions() *SessionStore { return s.sessions }
