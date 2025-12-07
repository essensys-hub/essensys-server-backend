package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func TestGetServerInfos(t *testing.T) {
	// Setup
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/serverinfos", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClientIDKey, "test-client"))
	w := httptest.NewRecorder()

	// Execute
	handler.GetServerInfos(w, req)

	// Verify
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json ;charset=UTF-8" {
		t.Errorf("Expected Content-Type 'application/json ;charset=UTF-8', got '%s'", contentType)
	}

	var response protocol.ServerInfoResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response structure
	if response.Infos == nil {
		t.Error("Expected Infos to be non-nil")
	}
}

func TestPostMyStatus(t *testing.T) {
	// Setup
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	// Create request with valid JSON
	statusReq := protocol.StatusRequest{
		Version: "1.0",
		EK: []protocol.ExchangeKV{
			{K: 100, V: "test-value"},
		},
	}
	body, _ := json.Marshal(statusReq)
	req := httptest.NewRequest(http.MethodPost, "/api/mystatus", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClientIDKey, "test-client"))
	w := httptest.NewRecorder()

	// Execute
	handler.PostMyStatus(w, req)

	// Verify
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json ;charset=UTF-8" {
		t.Errorf("Expected Content-Type 'application/json ;charset=UTF-8', got '%s'", contentType)
	}

	// Verify data was stored
	value, exists := store.GetValue("test-client", 100)
	if !exists || value != "test-value" {
		t.Errorf("Expected value 'test-value', got '%s' (exists: %v)", value, exists)
	}
}

func TestPostMyStatus_MalformedJSON(t *testing.T) {
	// Setup
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	// Create request with malformed JSON (unquoted keys)
	malformedJSON := `{"version":"1.0","ek":[{k:100,v:"test"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/mystatus", bytes.NewReader([]byte(malformedJSON)))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClientIDKey, "test-client"))
	w := httptest.NewRecorder()

	// Execute
	handler.PostMyStatus(w, req)

	// Verify - should succeed after normalization
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	// Verify data was stored
	value, exists := store.GetValue("test-client", 100)
	if !exists || value != "test" {
		t.Errorf("Expected value 'test', got '%s' (exists: %v)", value, exists)
	}
}

func TestGetMyActions(t *testing.T) {
	// Setup
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	// Add an action to the queue
	action := protocol.Action{
		GUID: "test-guid-123",
		Params: []protocol.ExchangeKV{
			{K: 590, V: "1"},
			{K: 605, V: "64"},
		},
	}
	store.EnqueueAction("test-client", action)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/myactions", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClientIDKey, "test-client"))
	w := httptest.NewRecorder()

	// Execute
	handler.GetMyActions(w, req)

	// Verify
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json ;charset=UTF-8" {
		t.Errorf("Expected Content-Type 'application/json ;charset=UTF-8', got '%s'", contentType)
	}

	var response protocol.ActionsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response structure
	if len(response.Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(response.Actions))
	}

	if response.Actions[0].GUID != "test-guid-123" {
		t.Errorf("Expected GUID 'test-guid-123', got '%s'", response.Actions[0].GUID)
	}

	// Verify field ordering in JSON
	// Need to re-read the body since we already decoded it
	req2 := httptest.NewRequest(http.MethodGet, "/api/myactions", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.ClientIDKey, "test-client"))
	w2 := httptest.NewRecorder()
	handler.GetMyActions(w2, req2)
	
	jsonBytes := w2.Body.Bytes()
	jsonStr := string(jsonBytes)
	de67fPos := bytes.Index(jsonBytes, []byte(`"_de67f"`))
	actionsPos := bytes.Index(jsonBytes, []byte(`"actions"`))

	if de67fPos == -1 || actionsPos == -1 {
		t.Errorf("Expected both _de67f and actions fields in response, got: %s", jsonStr)
	} else if de67fPos > actionsPos {
		t.Errorf("Expected _de67f before actions in JSON, got: %s", jsonStr)
	}
}

func TestGetMyActions_EmptyQueue(t *testing.T) {
	// Setup
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/myactions", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClientIDKey, "test-client"))
	w := httptest.NewRecorder()

	// Execute
	handler.GetMyActions(w, req)

	// Verify
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response protocol.ActionsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify empty actions array
	if len(response.Actions) != 0 {
		t.Errorf("Expected 0 actions, got %d", len(response.Actions))
	}
}

func TestPostDone(t *testing.T) {
	// Setup
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	// Add an action to the queue
	action := protocol.Action{
		GUID: "test-guid-456",
		Params: []protocol.ExchangeKV{
			{K: 590, V: "1"},
		},
	}
	store.EnqueueAction("test-client", action)

	// Create request
	req := httptest.NewRequest(http.MethodPost, "/api/done/test-guid-456", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClientIDKey, "test-client"))
	w := httptest.NewRecorder()

	// Execute
	handler.PostDone(w, req)

	// Verify
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json ;charset=UTF-8" {
		t.Errorf("Expected Content-Type 'application/json ;charset=UTF-8', got '%s'", contentType)
	}

	// Verify action was removed
	actions := store.DequeueActions("test-client")
	if len(actions) != 0 {
		t.Errorf("Expected action to be removed, but %d actions remain", len(actions))
	}
}

func TestPostDone_NotFound(t *testing.T) {
	// Setup
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store)

	// Create request with non-existent GUID
	req := httptest.NewRequest(http.MethodPost, "/api/done/non-existent-guid", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClientIDKey, "test-client"))
	w := httptest.NewRecorder()

	// Execute
	handler.PostDone(w, req)

	// Verify
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}
