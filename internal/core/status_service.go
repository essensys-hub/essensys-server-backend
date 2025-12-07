package core

import (
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// StatusService handles client status updates and exchange table operations
type StatusService struct {
	store data.Store
}

// NewStatusService creates a new StatusService instance
func NewStatusService(store data.Store) *StatusService {
	return &StatusService{
		store: store,
	}
}

// UpdateStatus processes status updates from client and stores them in the exchange table
func (s *StatusService) UpdateStatus(clientID string, status protocol.StatusRequest) error {
	// Store each key-value pair in the exchange table
	for _, kv := range status.EK {
		s.store.SetValue(clientID, kv.K, kv.V)
	}
	
	// Mark client as connected
	s.store.SetClientConnected(clientID, true)
	
	return nil
}

// GetRequestedIndices returns indices the server wants from client
// This can be used to request specific indices from the client in future implementations
func (s *StatusService) GetRequestedIndices(clientID string) []int {
	// For now, return an empty slice
	// In future implementations, this could return specific indices the server needs
	return []int{}
}
