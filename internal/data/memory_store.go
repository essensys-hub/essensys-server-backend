package data

import (
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// Store defines the interface for data storage operations
type Store interface {
	// Exchange Table operations
	GetValue(clientID string, index int) (string, bool)
	SetValue(clientID string, index int, value string)
	GetAllValues(clientID string, indices []int) []protocol.ExchangeKV

	// Action Queue operations
	EnqueueAction(clientID string, action protocol.Action)
	DequeueActions(clientID string) []protocol.Action
	AcknowledgeAction(clientID string, guid string) bool

	// Client management
	IsClientConnected(clientID string) bool
	SetClientConnected(clientID string, connected bool)
}

// ExchangeTable is a thread-safe key-value store for exchange table data
type ExchangeTable struct {
	mu     sync.RWMutex
	values map[int]string
}

// NewExchangeTable creates a new ExchangeTable instance
func NewExchangeTable() *ExchangeTable {
	return &ExchangeTable{
		values: make(map[int]string),
	}
}

// Get retrieves a value from the exchange table
func (et *ExchangeTable) Get(index int) (string, bool) {
	et.mu.RLock()
	defer et.mu.RUnlock()
	value, exists := et.values[index]
	return value, exists
}

// Set stores a value in the exchange table
func (et *ExchangeTable) Set(index int, value string) {
	et.mu.Lock()
	defer et.mu.Unlock()
	et.values[index] = value
}

// GetAll retrieves multiple values from the exchange table
func (et *ExchangeTable) GetAll(indices []int) []protocol.ExchangeKV {
	et.mu.RLock()
	defer et.mu.RUnlock()
	
	result := make([]protocol.ExchangeKV, 0, len(indices))
	for _, index := range indices {
		if value, exists := et.values[index]; exists {
			result = append(result, protocol.ExchangeKV{
				K: index,
				V: value,
			})
		}
	}
	return result
}

// ActionQueue is a thread-safe FIFO queue for actions
type ActionQueue struct {
	mu      sync.Mutex
	actions []protocol.Action
}

// NewActionQueue creates a new ActionQueue instance
func NewActionQueue() *ActionQueue {
	return &ActionQueue{
		actions: make([]protocol.Action, 0),
	}
}

// Enqueue adds an action to the end of the queue
func (aq *ActionQueue) Enqueue(action protocol.Action) {
	aq.mu.Lock()
	defer aq.mu.Unlock()
	aq.actions = append(aq.actions, action)
}

// GetAll returns all actions in FIFO order WITHOUT removing them
// Actions remain in the queue until explicitly acknowledged via Acknowledge()
func (aq *ActionQueue) GetAll() []protocol.Action {
	aq.mu.Lock()
	defer aq.mu.Unlock()
	
	// Return a copy to prevent external modification
	result := make([]protocol.Action, len(aq.actions))
	copy(result, aq.actions)
	return result
}

// Acknowledge removes an action with the specified GUID from the queue
func (aq *ActionQueue) Acknowledge(guid string) bool {
	aq.mu.Lock()
	defer aq.mu.Unlock()
	
	for i, action := range aq.actions {
		if action.GUID == guid {
			// Remove the action by slicing
			aq.actions = append(aq.actions[:i], aq.actions[i+1:]...)
			return true
		}
	}
	return false
}

// ClientData holds all data for a single client
type ClientData struct {
	ExchangeTable *ExchangeTable
	ActionQueue   *ActionQueue
	IsConnected   bool
	LastSeen      time.Time
}

// NewClientData creates a new ClientData instance
func NewClientData() *ClientData {
	return &ClientData{
		ExchangeTable: NewExchangeTable(),
		ActionQueue:   NewActionQueue(),
		IsConnected:   false,
		LastSeen:      time.Now(),
	}
}

// MemoryStore implements Store interface with in-memory storage
type MemoryStore struct {
	mu            sync.RWMutex
	clients       map[string]*ClientData
	globalActions *ActionQueue // Global action queue shared by all clients
}

// NewMemoryStore creates a new MemoryStore instance
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		clients:       make(map[string]*ClientData),
		globalActions: NewActionQueue(), // Single global queue for all clients
	}
}

// getOrCreateClient retrieves or creates client data
func (ms *MemoryStore) getOrCreateClient(clientID string) *ClientData {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	if client, exists := ms.clients[clientID]; exists {
		return client
	}
	
	client := NewClientData()
	ms.clients[clientID] = client
	return client
}

// GetValue retrieves a value from the exchange table
func (ms *MemoryStore) GetValue(clientID string, index int) (string, bool) {
	client := ms.getOrCreateClient(clientID)
	return client.ExchangeTable.Get(index)
}

// SetValue stores a value in the exchange table
func (ms *MemoryStore) SetValue(clientID string, index int, value string) {
	client := ms.getOrCreateClient(clientID)
	client.ExchangeTable.Set(index, value)
}

// GetAllValues retrieves multiple values from the exchange table
func (ms *MemoryStore) GetAllValues(clientID string, indices []int) []protocol.ExchangeKV {
	client := ms.getOrCreateClient(clientID)
	return client.ExchangeTable.GetAll(indices)
}

// EnqueueAction adds an action to the GLOBAL queue (shared by all clients)
func (ms *MemoryStore) EnqueueAction(clientID string, action protocol.Action) {
	// Use global queue instead of per-client queue
	ms.globalActions.Enqueue(action)
}

// DequeueActions returns all pending actions from the GLOBAL queue WITHOUT removing them
// Actions are only removed when AcknowledgeAction is called with the GUID
func (ms *MemoryStore) DequeueActions(clientID string) []protocol.Action {
	// Use global queue instead of per-client queue
	return ms.globalActions.GetAll()
}

// AcknowledgeAction removes an action with the specified GUID from the GLOBAL queue
func (ms *MemoryStore) AcknowledgeAction(clientID string, guid string) bool {
	// Use global queue instead of per-client queue
	return ms.globalActions.Acknowledge(guid)
}

// IsClientConnected returns the connection status of a client
func (ms *MemoryStore) IsClientConnected(clientID string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	
	if client, exists := ms.clients[clientID]; exists {
		return client.IsConnected
	}
	return false
}

// SetClientConnected sets the connection status of a client
func (ms *MemoryStore) SetClientConnected(clientID string, connected bool) {
	client := ms.getOrCreateClient(clientID)
	
	ms.mu.Lock()
	defer ms.mu.Unlock()
	
	client.IsConnected = connected
	client.LastSeen = time.Now()
}
