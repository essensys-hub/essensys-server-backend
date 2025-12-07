package data

import (
	"sync"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func TestMemoryStore_GetSetValue(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	// Test setting and getting a value
	store.SetValue(clientID, 100, "test-value")
	value, exists := store.GetValue(clientID, 100)

	if !exists {
		t.Error("Expected value to exist")
	}
	if value != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", value)
	}
}

func TestMemoryStore_OverwriteValue(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	// Set initial value
	store.SetValue(clientID, 100, "first-value")
	
	// Overwrite with new value
	store.SetValue(clientID, 100, "second-value")
	
	value, exists := store.GetValue(clientID, 100)
	if !exists {
		t.Error("Expected value to exist")
	}
	if value != "second-value" {
		t.Errorf("Expected 'second-value', got '%s'", value)
	}
}

func TestMemoryStore_NonExistentIndex(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	value, exists := store.GetValue(clientID, 999)
	if exists {
		t.Error("Expected value to not exist")
	}
	if value != "" {
		t.Errorf("Expected empty string, got '%s'", value)
	}
}

func TestMemoryStore_GetAllValues(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	// Set multiple values
	store.SetValue(clientID, 100, "value1")
	store.SetValue(clientID, 200, "value2")
	store.SetValue(clientID, 300, "value3")

	// Get all values
	indices := []int{100, 200, 300, 400} // 400 doesn't exist
	values := store.GetAllValues(clientID, indices)

	if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	}

	// Verify values
	expectedValues := map[int]string{
		100: "value1",
		200: "value2",
		300: "value3",
	}

	for _, kv := range values {
		if expectedValues[kv.K] != kv.V {
			t.Errorf("Expected value '%s' for index %d, got '%s'", expectedValues[kv.K], kv.K, kv.V)
		}
	}
}

func TestMemoryStore_ActionQueue_FIFO(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	// Enqueue actions
	action1 := protocol.Action{GUID: "guid-1", Params: []protocol.ExchangeKV{{K: 100, V: "1"}}}
	action2 := protocol.Action{GUID: "guid-2", Params: []protocol.ExchangeKV{{K: 200, V: "2"}}}
	action3 := protocol.Action{GUID: "guid-3", Params: []protocol.ExchangeKV{{K: 300, V: "3"}}}

	store.EnqueueAction(clientID, action1)
	store.EnqueueAction(clientID, action2)
	store.EnqueueAction(clientID, action3)

	// Dequeue all actions
	actions := store.DequeueActions(clientID)

	if len(actions) != 3 {
		t.Errorf("Expected 3 actions, got %d", len(actions))
	}

	// Verify FIFO order
	if actions[0].GUID != "guid-1" {
		t.Errorf("Expected first action to be 'guid-1', got '%s'", actions[0].GUID)
	}
	if actions[1].GUID != "guid-2" {
		t.Errorf("Expected second action to be 'guid-2', got '%s'", actions[1].GUID)
	}
	if actions[2].GUID != "guid-3" {
		t.Errorf("Expected third action to be 'guid-3', got '%s'", actions[2].GUID)
	}
}

func TestMemoryStore_AcknowledgeAction(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	// Enqueue actions
	action1 := protocol.Action{GUID: "guid-1", Params: []protocol.ExchangeKV{{K: 100, V: "1"}}}
	action2 := protocol.Action{GUID: "guid-2", Params: []protocol.ExchangeKV{{K: 200, V: "2"}}}
	action3 := protocol.Action{GUID: "guid-3", Params: []protocol.ExchangeKV{{K: 300, V: "3"}}}

	store.EnqueueAction(clientID, action1)
	store.EnqueueAction(clientID, action2)
	store.EnqueueAction(clientID, action3)

	// Acknowledge middle action
	acknowledged := store.AcknowledgeAction(clientID, "guid-2")
	if !acknowledged {
		t.Error("Expected action to be acknowledged")
	}

	// Verify remaining actions
	actions := store.DequeueActions(clientID)
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions remaining, got %d", len(actions))
	}

	// Verify correct action was removed
	for _, action := range actions {
		if action.GUID == "guid-2" {
			t.Error("Action 'guid-2' should have been removed")
		}
	}

	// Verify order is preserved
	if actions[0].GUID != "guid-1" || actions[1].GUID != "guid-3" {
		t.Error("FIFO order not preserved after acknowledgment")
	}
}

func TestMemoryStore_AcknowledgeNonExistentAction(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	// Try to acknowledge non-existent action
	acknowledged := store.AcknowledgeAction(clientID, "non-existent-guid")
	if acknowledged {
		t.Error("Expected acknowledgment to fail for non-existent action")
	}
}

func TestMemoryStore_ClientConnection(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	// Initially not connected
	if store.IsClientConnected(clientID) {
		t.Error("Expected client to not be connected initially")
	}

	// Set connected
	store.SetClientConnected(clientID, true)
	if !store.IsClientConnected(clientID) {
		t.Error("Expected client to be connected")
	}

	// Set disconnected
	store.SetClientConnected(clientID, false)
	if store.IsClientConnected(clientID) {
		t.Error("Expected client to be disconnected")
	}
}

func TestMemoryStore_ThreadSafety(t *testing.T) {
	store := NewMemoryStore()
	clientID := "test-client"

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent writes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			store.SetValue(clientID, index, "value")
		}(i)
	}

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			store.GetValue(clientID, index)
		}(i)
	}

	wg.Wait()
}

func TestMemoryStore_MultipleClients(t *testing.T) {
	store := NewMemoryStore()

	// Set values for different clients
	store.SetValue("client1", 100, "client1-value")
	store.SetValue("client2", 100, "client2-value")

	// Verify isolation
	value1, _ := store.GetValue("client1", 100)
	value2, _ := store.GetValue("client2", 100)

	if value1 != "client1-value" {
		t.Errorf("Expected 'client1-value', got '%s'", value1)
	}
	if value2 != "client2-value" {
		t.Errorf("Expected 'client2-value', got '%s'", value2)
	}
}
