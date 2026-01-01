package data

import (
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// Store defines the interface for data storage operations
// This interface is implemented by both MemoryStore (legacy) and DatabaseStore (new)
// It maintains compatibility with the legacy IoT client protocol
type Store interface {
	// Exchange Table operations (for legacy IoT client protocol)
	GetValue(clientID string, index int) (string, bool)
	SetValue(clientID string, index int, value string)
	GetAllValues(clientID string, indices []int) []protocol.ExchangeKV

	// Action Queue operations (for legacy IoT client protocol)
	EnqueueAction(clientID string, action protocol.Action)
	DequeueActions(clientID string) []protocol.Action
	AcknowledgeAction(clientID string, guid string) bool

	// Client management (for legacy IoT client protocol)
	IsClientConnected(clientID string) bool
	SetClientConnected(clientID string, connected bool)
}

// DatabaseStoreInterface extends Store with database-specific operations
// This interface is for the new web endpoints that need full database access
// Note: The actual DatabaseStore struct implements Store, and repositories
// provide the extended database operations
type DatabaseStoreInterface interface {
	Store // Inherit all Store methods for legacy compatibility

	// User operations
	GetUserByID(id int) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByGuid(guid, email string) (*models.User, error)
	CreateUser(user *models.User) error
	UpdateUser(user *models.User) error
	DeleteUser(id int) error

	// Machine operations
	GetMachineByID(id int) (*models.Machine, error)
	GetMachineByNoSerie(noSerie string) (*models.Machine, error)
	CreateMachine(machine *models.Machine) error
	UpdateMachine(machine *models.Machine) error

	// Action operations (database-backed)
	CreateAction(action *models.Action, indexes []models.ActionIndex) error
	GetActionByGUID(guid string) (*models.Action, error)
	GetPendingActionsByMachineID(machineID int) ([]*models.Action, error)
	MarkActionDone(guid string) error

	// State operations
	CreateState(state *models.State, indexes []models.StateIndex) error
	GetLatestStateByMachineID(machineID int) (*models.State, error)
	GetStatesByMachineIDAfter(machineID int, after time.Time) ([]*models.State, error)
}

