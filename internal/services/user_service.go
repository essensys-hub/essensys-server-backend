package services

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/auth"
	"github.com/essensys-hub/essensys-server-backend/internal/data/database"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// UserService handles user-related business logic
type UserService struct {
	userRepo      *database.UserRepository
	machineRepo   *database.MachineRepository
	cleMachineRepo *database.CleMachineRepository
}

// NewUserService creates a new UserService
func NewUserService(db *sqlx.DB) *UserService {
	return &UserService{
		userRepo:       database.NewUserRepository(db),
		machineRepo:    database.NewMachineRepository(db),
		cleMachineRepo: database.NewCleMachineRepository(db),
	}
}

// Login validates user credentials and returns the user
func (s *UserService) Login(email, password string) (*models.User, error) {
	// Get user by email
	user, err := s.userRepo.GetByEmail(email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user is valid and not obsolete
	if !user.IsValid || user.Obsolete {
		return nil, fmt.Errorf("account not valid or closed")
	}

	// Verify password (SHA1 hash)
	hashedPassword := auth.HashPassword(password)
	if user.Password != hashedPassword {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Update last access
	user.LastAccess = &time.Time{}
	*user.LastAccess = time.Now()
	if err := s.userRepo.UpdateLastAccess(user.ID); err != nil {
		// Log error but don't fail login
	}

	return user, nil
}

// Register creates a new user account
func (s *UserService) Register(user *models.User, noSerie string) error {
	// Check if email already exists
	exists, err := s.userRepo.CheckEmailExists(user.Mail)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("email already exists")
	}

	// Check if serial number is valid
	valid, err := s.CheckNoSerie(noSerie)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("invalid serial number")
	}

	// Hash password and response
	user.Password = auth.HashPassword(user.Password)
	user.Reponse = auth.HashResponse(user.Reponse)

	// Set default values
	now := time.Now()
	user.DateCreation = now
	user.IsValid = false
	user.Obsolete = false
	user.Guid = uuid.New().String()

	// Get or create machine
	machine, err := s.machineRepo.GetByNoSerie(noSerie)
	if err != nil {
		return err
	}

	if machine == nil {
		// Create new machine
		machine = &models.Machine{
			NoSerie:        noSerie,
			IsActive:       false,
			AutoriseAlarme: false,
			DateCreation:   now,
			DateModification: now,
		}
		if err := s.machineRepo.Create(machine); err != nil {
			return err
		}
	}

	user.MachineID = &machine.ID

	// Create user
	if err := s.userRepo.Create(user); err != nil {
		return err
	}

	return nil
}

// ValidateAccount validates a user account and generates activation code
func (s *UserService) ValidateAccount(guid, email string, generateCode bool) (string, error) {
	// Get user by GUID and email
	user, err := s.userRepo.GetByGuid(guid, email)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", fmt.Errorf("user not found")
	}

	// Update user
	now := time.Now()
	user.LastAccess = &now
	user.IsValid = true

	// Generate activation code if requested
	var pkey string
	if generateCode {
		pkey, err = s.GenerateActivationCode()
		if err != nil {
			return "", err
		}

		// Update machine with activation code
		if user.MachineID != nil {
			machine, err := s.machineRepo.GetByID(*user.MachineID)
			if err != nil {
				return "", err
			}
			if machine != nil {
				machine.Pkey = pkey
				machine.HashedPkey = auth.HashMD5(pkey)
				machine.IsActive = true
				machine.DateModification = now
				if err := s.machineRepo.Update(machine); err != nil {
					return "", err
				}
			}
		}

		// Update cle_machine
		if user.MachineID != nil {
			machine, _ := s.machineRepo.GetByID(*user.MachineID)
			if machine != nil {
				cleMachine, err := s.cleMachineRepo.GetByCle(machine.NoSerie)
				if err == nil && cleMachine != nil && cleMachine.MachineID == nil {
					cleMachine.MachineID = user.MachineID
					activationTime := time.Now()
					cleMachine.DateActivation = &activationTime
					s.cleMachineRepo.Update(cleMachine)
				}
			}
		}
	}

	// Update user
	if err := s.userRepo.Update(user); err != nil {
		return "", err
	}

	return pkey, nil
}

// UpdateUser updates user information
func (s *UserService) UpdateUser(user *models.User, currentPassword, newPassword, question, response, currentResponse string) error {
	// Get existing user
	existing, err := s.userRepo.GetByID(user.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("user not found")
	}

	// Verify current password if changing password
	if newPassword != "" {
		if currentPassword == "" {
			return fmt.Errorf("current password required")
		}
		hashedCurrent := auth.HashPassword(currentPassword)
		if existing.Password != hashedCurrent {
			return fmt.Errorf("invalid current password")
		}
		existing.Password = auth.HashPassword(newPassword)
	}

	// Verify current response if changing question/response
	if question != "" && response != "" {
		if currentResponse == "" {
			return fmt.Errorf("current response required")
		}
		hashedCurrentResponse := auth.HashResponse(currentResponse)
		if existing.Reponse != hashedCurrentResponse {
			return fmt.Errorf("invalid current response")
		}
		existing.Question = question
		existing.Reponse = auth.HashResponse(response)
	}

	// Update other fields
	existing.Nom = user.Nom
	existing.Prenom = user.Prenom
	existing.Adr1 = user.Adr1
	existing.Adr2 = user.Adr2
	existing.Cp = user.Cp
	existing.Ville = user.Ville
	existing.Phone = user.Phone
	existing.SendInfos = user.SendInfos

	return s.userRepo.Update(existing)
}

// CloseAccount closes a user account
func (s *UserService) CloseAccount(userID int) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// Soft delete user
	if err := s.userRepo.Delete(userID); err != nil {
		return err
	}

	// Clear machine keys if machine exists
	if user.MachineID != nil {
		machine, err := s.machineRepo.GetByID(*user.MachineID)
		if err == nil && machine != nil {
			machine.Pkey = ""
			machine.HashedPkey = ""
			machine.IsActive = false
			machine.DateModification = time.Now()
			s.machineRepo.Update(machine)

			// Clear cle_machine association
			cleMachine, err := s.cleMachineRepo.GetByCle(machine.NoSerie)
			if err == nil && cleMachine != nil {
				cleMachine.MachineID = nil
				cleMachine.DateActivation = nil
				s.cleMachineRepo.Update(cleMachine)
			}
		}
	}

	return nil
}

// TestQuestion verifies a security question response
func (s *UserService) TestQuestion(userID int, response string) (bool, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, fmt.Errorf("user not found")
	}

	hashedResponse := auth.HashResponse(response)
	return user.Reponse == hashedResponse, nil
}

// ForgotPassword generates a new password and sends it by email
func (s *UserService) ForgotPassword(email string) (string, error) {
	user, err := s.userRepo.GetByEmail(email)
	if err != nil {
		return "", err
	}
	if user == nil || user.Obsolete {
		return "", fmt.Errorf("user not found")
	}

	// Generate new password (8 digits, like legacy)
	newPassword := s.GenerateRandomCode(8)
	user.Password = auth.HashPassword(newPassword)

	if err := s.userRepo.Update(user); err != nil {
		return "", err
	}

	// TODO: Send email with new password
	// For now, return the password (in production, send by email only)
	return newPassword, nil
}

// CheckEmailExists checks if an email is already registered
func (s *UserService) CheckEmailExists(email string) (bool, error) {
	return s.userRepo.CheckEmailExists(email)
}

// CheckNoSerie checks if a serial number is valid and available
func (s *UserService) CheckNoSerie(noSerie string) (bool, error) {
	return s.userRepo.CheckNoSerieExists(noSerie)
}

// GenerateActivationCode generates a unique 32-character activation code
func (s *UserService) GenerateActivationCode() (string, error) {
	// Generate code until unique (like legacy CodeHelper.GenerateCode)
	for {
		code := s.GenerateRandomCode(32)
		
		// Check if code is unique (not used by any machine)
		machine, err := s.machineRepo.GetByNoSerie(code)
		if err != nil {
			return "", err
		}
		if machine == nil {
			return code, nil
		}
		// Code exists, generate another one
	}
}

// GenerateRandomCode generates a random numeric code of specified length
func (s *UserService) GenerateRandomCode(length int) string {
	const digits = "0123456789"
	b := make([]byte, length)
	for i := range b {
		// Use crypto/rand for secure random numbers
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			// Fallback (should not happen)
			b[i] = '0'
			continue
		}
		b[i] = digits[n.Int64()]
	}
	return string(b)
}

