package core

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func TestStatusService_UpdateStatus(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewStatusService(store)
	clientID := "test-client"

	// Create a status request with some exchange values
	status := protocol.StatusRequest{
		Version: "1.0",
		EK: []protocol.ExchangeKV{
			{K: 100, V: "value1"},
			{K: 200, V: "value2"},
			{K: 300, V: "value3"},
		},
	}

	// Update status
	err := service.UpdateStatus(clientID, status)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify values were stored in exchange table
	value1, exists := store.GetValue(clientID, 100)
	if !exists || value1 != "value1" {
		t.Errorf("Expected value1 at index 100, got %v (exists: %v)", value1, exists)
	}

	value2, exists := store.GetValue(clientID, 200)
	if !exists || value2 != "value2" {
		t.Errorf("Expected value2 at index 200, got %v (exists: %v)", value2, exists)
	}

	value3, exists := store.GetValue(clientID, 300)
	if !exists || value3 != "value3" {
		t.Errorf("Expected value3 at index 300, got %v (exists: %v)", value3, exists)
	}

	// Verify client is marked as connected
	if !store.IsClientConnected(clientID) {
		t.Error("Expected client to be marked as connected")
	}
}

func TestStatusService_UpdateStatus_Overwrite(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewStatusService(store)
	clientID := "test-client"

	// First update
	status1 := protocol.StatusRequest{
		Version: "1.0",
		EK: []protocol.ExchangeKV{
			{K: 100, V: "initial"},
		},
	}
	service.UpdateStatus(clientID, status1)

	// Second update with same index
	status2 := protocol.StatusRequest{
		Version: "1.0",
		EK: []protocol.ExchangeKV{
			{K: 100, V: "updated"},
		},
	}
	service.UpdateStatus(clientID, status2)

	// Verify value was overwritten
	value, exists := store.GetValue(clientID, 100)
	if !exists || value != "updated" {
		t.Errorf("Expected 'updated' at index 100, got %v (exists: %v)", value, exists)
	}
}

func TestStatusService_GetRequestedIndices(t *testing.T) {
	store := data.NewMemoryStore()
	service := NewStatusService(store)
	clientID := "test-client"

	// Get requested indices
	indices := service.GetRequestedIndices(clientID)

	// For now, should return empty slice
	if len(indices) != 0 {
		t.Errorf("Expected empty slice, got %v", indices)
	}
}
