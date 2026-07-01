package data

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/data/database"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
	"github.com/jmoiron/sqlx"
)

// DatabaseStore implements Store interface using PostgreSQL
// It maintains compatibility with the legacy IoT client protocol
// while persisting data to the database
type DatabaseStore struct {
	db *sqlx.DB

	// Repositories
	userRepo      *database.UserRepository
	machineRepo   *database.MachineRepository
	actionRepo    *database.ActionRepository
	stateRepo     *database.StateRepository
	dataIndexRepo *database.DataIndexRepository

	// Cache for clientID -> machineID mapping (for legacy protocol compatibility)
	// clientID is typically "default" or a machine identifier from Basic Auth
	mu          sync.RWMutex
	clientCache map[string]int // clientID -> machineID

	lastPollMu         sync.RWMutex
	lastPollByClient   map[string]time.Time
	lastPolledClientID string
}

// NewDatabaseStore creates a new DatabaseStore instance
func NewDatabaseStore(db *sqlx.DB) *DatabaseStore {
	return &DatabaseStore{
		db:            db,
		userRepo:      database.NewUserRepository(db),
		machineRepo:   database.NewMachineRepository(db),
		actionRepo:    database.NewActionRepository(db),
		stateRepo:     database.NewStateRepository(db),
		dataIndexRepo: database.NewDataIndexRepository(db),
		clientCache:     make(map[string]int),
		lastPollByClient: make(map[string]time.Time),
	}
}

// getMachineIDByClientID resolves clientID to machineID
// For legacy protocol, clientID might be "default" or a machine identifier
func (ds *DatabaseStore) getMachineIDByClientID(clientID string) (int, error) {
	ds.mu.RLock()
	if machineID, exists := ds.clientCache[clientID]; exists {
		ds.mu.RUnlock()
		return machineID, nil
	}
	ds.mu.RUnlock()

	// Try to find machine by no_serie (clientID might be the serial number)
	machine, err := ds.machineRepo.GetByNoSerie(clientID)
	if err != nil {
		return 0, err
	}

	if machine != nil {
		ds.mu.Lock()
		ds.clientCache[clientID] = machine.ID
		ds.mu.Unlock()
		return machine.ID, nil
	}

	// If not found, return 0 (will need to handle this case)
	return 0, fmt.Errorf("machine not found for clientID: %s", clientID)
}

// GetValue retrieves a value from the exchange table (legacy protocol compatibility)
func (ds *DatabaseStore) GetValue(clientID string, index int) (string, bool) {
	machineID, err := ds.getMachineIDByClientID(clientID)
	if err != nil {
		return "", false
	}

	// Get latest state for this machine
	state, err := ds.stateRepo.GetLatestByMachineID(machineID)
	if err != nil || state == nil {
		return "", false
	}

	// Get indexes for this state
	indexes, err := ds.stateRepo.GetIndexesByStateID(state.ID)
	if err != nil {
		return "", false
	}

	// Find the index key
	indexKey := fmt.Sprintf("%d", index)
	for _, si := range indexes {
		// Get the data index to compare keys
		di, err := ds.dataIndexRepo.GetByID(si.IndexID)
		if err != nil || di == nil {
			continue
		}
		if di.IndexKey == indexKey {
			return si.Value, true
		}
	}

	return "", false
}

// SetValue stores a value in the exchange table (legacy protocol compatibility)
// This creates or updates a state entry
func (ds *DatabaseStore) SetValue(clientID string, index int, value string) {
	machineID, err := ds.getMachineIDByClientID(clientID)
	if err != nil {
		return // Silently fail for legacy compatibility
	}

	// Get or create the latest state
	state, err := ds.stateRepo.GetLatestByMachineID(machineID)
	if err != nil || state == nil {
		// Create new state
		state = &models.State{
			MachineID: machineID,
			Completed: false,
			StateDate: time.Now(),
		}
		// We'll create it with the index below
	}

	// Get or create the data index
	dataIndex, err := ds.dataIndexRepo.GetOrCreateByKey(fmt.Sprintf("%d", index))
	if err != nil {
		return
	}

	// Get existing indexes for this state
	var stateIndexes []models.StateIndex
	if state.ID > 0 {
		stateIndexes, _ = ds.stateRepo.GetIndexesByStateID(state.ID)
	}

	// Update or add the index value
	found := false
	for i := range stateIndexes {
		if stateIndexes[i].IndexID == dataIndex.ID {
			stateIndexes[i].Value = value
			stateIndexes[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}
	if !found {
		stateIndexes = append(stateIndexes, models.StateIndex{
			StateID:   state.ID,
			IndexID:   dataIndex.ID,
			Value:     value,
			UpdatedAt: time.Now(),
		})
	}

	// Create or update state
	if state.ID == 0 {
		// Create new state
		err = ds.stateRepo.Create(state, stateIndexes)
	} else {
		// Update existing state indexes
		// For simplicity, we'll create a new state entry (historical tracking)
		// In production, you might want to update in place
		newState := &models.State{
			MachineID: machineID,
			Completed: true,
			StateDate: time.Now(),
		}
		err = ds.stateRepo.Create(newState, stateIndexes)
	}
	if err != nil {
		// Log error but don't fail for legacy compatibility
		return
	}
}

// GetAllValues retrieves multiple values from the exchange table
func (ds *DatabaseStore) GetAllValues(clientID string, indices []int) []protocol.ExchangeKV {
	machineID, err := ds.getMachineIDByClientID(clientID)
	if err != nil {
		return []protocol.ExchangeKV{}
	}

	// Get latest state
	state, err := ds.stateRepo.GetLatestByMachineID(machineID)
	if err != nil || state == nil {
		return []protocol.ExchangeKV{}
	}

	// Get all indexes for this state
	stateIndexes, err := ds.stateRepo.GetIndexesByStateID(state.ID)
	if err != nil {
		return []protocol.ExchangeKV{}
	}

	// Build a map of index_key -> value
	valueMap := make(map[string]string)
	for _, si := range stateIndexes {
		di, err := ds.dataIndexRepo.GetByID(si.IndexID)
		if err != nil || di == nil {
			continue
		}
		valueMap[di.IndexKey] = si.Value
	}

	// Build result
	result := make([]protocol.ExchangeKV, 0)
	for _, index := range indices {
		indexKey := fmt.Sprintf("%d", index)
		if value, exists := valueMap[indexKey]; exists {
			result = append(result, protocol.ExchangeKV{
				K: index,
				V: value,
			})
		}
	}

	return result
}

// GetFullTable retrieves all values from the exchange table for a client
func (ds *DatabaseStore) GetFullTable(clientID string) map[int]string {
	machineID, err := ds.getMachineIDByClientID(clientID)
	if err != nil {
		return map[int]string{}
	}

	// Get latest state
	state, err := ds.stateRepo.GetLatestByMachineID(machineID)
	if err != nil || state == nil {
		return map[int]string{}
	}

	// Get all indexes for this state
	stateIndexes, err := ds.stateRepo.GetIndexesByStateID(state.ID)
	if err != nil {
		return map[int]string{}
	}

	result := make(map[int]string)
	for _, si := range stateIndexes {
		di, err := ds.dataIndexRepo.GetByID(si.IndexID)
		if err != nil || di == nil {
			continue
		}
		if index, err := strconv.Atoi(di.IndexKey); err == nil {
			result[index] = si.Value
		}
	}

	return result
}

// EnqueueAction adds an action to the queue (legacy protocol compatibility)
func (ds *DatabaseStore) EnqueueAction(clientID string, action protocol.Action) {
	machineID, err := ds.getMachineIDByClientID(clientID)
	if err != nil {
		log.Printf("[STORE] EnqueueAction failed: clientID=%s, error=%v", clientID, err)
		return // Silently fail for legacy compatibility
	}
	log.Printf("[STORE] EnqueueAction: clientID=%s, machineID=%d, guid=%s, params=%d", 
		clientID, machineID, action.GUID, len(action.Params))

	// Convert protocol.Action to models.Action
	dbAction := &models.Action{
		MachineID:   machineID,
		Guid:        action.GUID,
		ActionType:  "LEGACY", // Default type for legacy actions
		ActionInfo:  "",
		IsDone:      false,
		DateCreation: time.Now(),
	}

	// Convert params to ActionIndex
	actionIndexes := make([]models.ActionIndex, 0, len(action.Params))
	for _, param := range action.Params {
		// Get or create data index
		dataIndex, err := ds.dataIndexRepo.GetOrCreateByKey(fmt.Sprintf("%d", param.K))
		if err != nil {
			continue
		}

		actionIndexes = append(actionIndexes, models.ActionIndex{
			IndexID: dataIndex.ID,
			Value:   param.V,
		})
	}

	// Create action in database
	err = ds.actionRepo.Create(dbAction, actionIndexes)
	if err != nil {
		// Log error but don't fail for legacy compatibility
		return
	}
}

// DequeueActions returns all pending actions (legacy protocol compatibility)
func (ds *DatabaseStore) DequeueActions(clientID string) []protocol.Action {
	machineID, err := ds.getMachineIDByClientID(clientID)
	if err != nil {
		log.Printf("[STORE] DequeueActions failed: clientID=%s, error=%v", clientID, err)
		return []protocol.Action{}
	}

	// Get pending actions from database
	dbActions, err := ds.actionRepo.GetPendingByMachineID(machineID)
	if err != nil {
		log.Printf("[STORE] DequeueActions GetPendingByMachineID failed: machineID=%d, error=%v", machineID, err)
		return []protocol.Action{}
	}
	
	log.Printf("[STORE] DequeueActions: clientID=%s, machineID=%d, found %d actions", 
		clientID, machineID, len(dbActions))

	// Convert to protocol.Action
	result := make([]protocol.Action, 0, len(dbActions))
	for _, dbAction := range dbActions {
		// Get indexes for this action
		actionIndexes, err := ds.actionRepo.GetIndexesByActionID(dbAction.ID)
		if err != nil {
			continue
		}

		// Convert to ExchangeKV
		params := make([]protocol.ExchangeKV, 0, len(actionIndexes))
		for _, ai := range actionIndexes {
			// Get data index to get the key
			di, err := ds.dataIndexRepo.GetByID(ai.IndexID)
			if err != nil || di == nil {
				continue
			}

			// Parse index key to int
			var k int
			fmt.Sscanf(di.IndexKey, "%d", &k)

			params = append(params, protocol.ExchangeKV{
				K: k,
				V: ai.Value,
			})
		}

		result = append(result, protocol.Action{
			GUID:   dbAction.Guid,
			Params: params,
		})
	}

	return result
}

// AcknowledgeAction removes an action from the queue
func (ds *DatabaseStore) AcknowledgeAction(clientID string, guid string) (*protocol.Action, bool) {
	err := ds.actionRepo.MarkDone(guid)
    if err != nil {
        return nil, false
    }
    // Ideally we should return the action, but for now return nil as it's not strictly used by result
	return nil, true 
}

// IsClientConnected checks if a client is connected
func (ds *DatabaseStore) IsClientConnected(clientID string) bool {
	machineID, err := ds.getMachineIDByClientID(clientID)
	if err != nil {
		return false
	}

	// Check if there's a recent state (within last 5 minutes)
	recentTime := time.Now().Add(-5 * time.Minute)
	hasRefreshed, err := ds.stateRepo.HasRefreshed(machineID, recentTime)
	if err != nil {
		return false
	}

	return hasRefreshed
}

// SetClientConnected sets the connection status
func (ds *DatabaseStore) SetClientConnected(clientID string, connected bool) {
	// For database store, connection status is derived from state freshness
	// This method is kept for interface compatibility
	// The actual connection status is determined by IsClientConnected
}

func (ds *DatabaseStore) RecordClientPoll(clientID string, at time.Time) {
	ds.lastPollMu.Lock()
	defer ds.lastPollMu.Unlock()
	if ds.lastPollByClient == nil {
		ds.lastPollByClient = make(map[string]time.Time)
	}
	ds.lastPollByClient[clientID] = at
	ds.lastPolledClientID = clientID
}

func (ds *DatabaseStore) GetClientLastPoll(clientID string) (time.Time, bool) {
	ds.lastPollMu.RLock()
	defer ds.lastPollMu.RUnlock()
	t, ok := ds.lastPollByClient[clientID]
	return t, ok
}

func (ds *DatabaseStore) GetLastPolledClientID() (string, bool) {
	ds.lastPollMu.RLock()
	defer ds.lastPollMu.RUnlock()
	if ds.lastPolledClientID == "" {
		return "", false
	}
	return ds.lastPolledClientID, true
}

// SetAuthInfo stores auth info
func (ds *DatabaseStore) SetAuthInfo(clientID, ip, auth, version string) {
	// Not implemented for DatabaseStore yet
}

// GetAuthInfo retrieves auth info
func (ds *DatabaseStore) GetAuthInfo(clientID string) (string, string, string, bool) {
	return "", "", "", false
}

