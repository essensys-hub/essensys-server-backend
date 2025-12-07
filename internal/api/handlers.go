package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

// Handler contains HTTP request handlers
type Handler struct {
	actionService *core.ActionService
	statusService *core.StatusService
	store         data.Store
}

// NewHandler creates a new Handler instance
func NewHandler(actionService *core.ActionService, statusService *core.StatusService, store data.Store) *Handler {
	return &Handler{
		actionService: actionService,
		statusService: statusService,
		store:         store,
	}
}

// GetServerInfos handles GET /api/serverinfos
func (h *Handler) GetServerInfos(w http.ResponseWriter, r *http.Request) {
	// Indices requested by the server from the client
	// These are the indices the server wants the client to report in mystatus
	// 613: Lumière Escalier ON
	// 607: Lumière Escalier OFF
	// 615: Lumière SDB2 ON
	// 590: Trigger Scenario
	// Others: Various system indices
	indices := []int{613, 607, 615, 590, 349, 350, 351, 352, 363, 425, 426, 920}

	// Build response
	// isconnected: always true (client is connected if it's making this request)
	// infos: list of indices the server wants from the client
	// newversion: "no" means no firmware update available
	response := protocol.ServerInfoResponse{
		IsConnected: true,
		Infos:       indices,
		NewVersion:  "no",
	}

	// Set Content-Type header with space before semicolon (as per requirement 5.5)
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// PostMyStatus handles POST /api/mystatus
func (h *Handler) PostMyStatus(w http.ResponseWriter, r *http.Request) {
	// Get client ID from context (set by auth middleware)
	clientID, ok := middleware.GetClientID(r)
	if !ok {
		clientID = "default"
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Normalize malformed JSON
	normalizedBody, err := NormalizeJSON(body)
	if err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Parse status request
	var statusReq protocol.StatusRequest
	if err := json.Unmarshal(normalizedBody, &statusReq); err != nil {
		http.Error(w, "Failed to parse status request", http.StatusBadRequest)
		return
	}

	// Log status update (similar to server.sample.go)
	log.Printf("[GO] Status Update (Version: %s, Items: %d)", statusReq.Version, len(statusReq.EK))

	// Update status in the store
	if err := h.statusService.UpdateStatus(clientID, statusReq); err != nil {
		http.Error(w, "Failed to update status", http.StatusInternalServerError)
		return
	}

	// Set Content-Type header with space before semicolon (as per requirement 5.5)
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
}

// GetMyActions handles GET /api/myactions
func (h *Handler) GetMyActions(w http.ResponseWriter, r *http.Request) {
	// Get client ID from context (set by auth middleware)
	clientID, ok := middleware.GetClientID(r)
	if !ok {
		clientID = "default"
	}

	// Get all pending actions for the client
	actions := h.store.DequeueActions(clientID)

	// Build response with proper field ordering (_de67f before actions)
	response := protocol.ActionsResponse{
		De67f:   nil, // No alarm command for now
		Actions: actions,
	}

	// If actions is nil, ensure it's an empty array in JSON
	if response.Actions == nil {
		response.Actions = []protocol.Action{}
	}

	// Marshal to JSON for logging
	jsonBytes, _ := json.Marshal(response)
	log.Printf("[GO] Sending Actions: %s", string(jsonBytes))

	// Set Content-Type header with space before semicolon (as per requirement 5.5)
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// PostDone handles POST /api/done/{guid}
func (h *Handler) PostDone(w http.ResponseWriter, r *http.Request) {
	// Get client ID from context (set by auth middleware)
	clientID, ok := middleware.GetClientID(r)
	if !ok {
		clientID = "default"
	}

	// Extract GUID from URL path
	// The path is /api/done/{guid}, so we need to extract the last segment
	guid := r.URL.Path[len("/api/done/"):]
	if guid == "" {
		http.Error(w, "GUID is required", http.StatusBadRequest)
		return
	}

	// Acknowledge the action
	found := h.store.AcknowledgeAction(clientID, guid)
	if !found {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}

	// Log acknowledgment (like server.sample.go)
	log.Printf("[GO] Action acknowledged: %s", guid)

	// Set Content-Type header with space before semicolon (as per requirement 5.5)
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusCreated)
}

// PostAdminInject handles POST /api/admin/inject
// This endpoint allows administrators to manually inject actions into the queue
func (h *Handler) PostAdminInject(w http.ResponseWriter, r *http.Request) {
	// Get client ID from context (set by auth middleware)
	clientID, ok := middleware.GetClientID(r)
	if !ok {
		clientID = "default"
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Support both single object and array of objects
	var params []protocol.ExchangeKV

	// Try parsing as array first
	if err := json.Unmarshal(body, &params); err != nil {
		// If array fails, try single object
		var singleParam protocol.ExchangeKV
		if err2 := json.Unmarshal(body, &singleParam); err2 != nil {
			http.Error(w, "Invalid JSON: expected array or object", http.StatusBadRequest)
			return
		}
		params = []protocol.ExchangeKV{singleParam}
	}

	// Process the action using ActionService
	// This will handle complete block generation, bitwise fusion, etc.
	guid, err := h.actionService.AddAction(clientID, params)
	if err != nil {
		http.Error(w, "Failed to add action", http.StatusInternalServerError)
		return
	}

	// Build response
	response := map[string]string{
		"status": "ok",
		"guid":   guid,
	}

	// Set Content-Type header with space before semicolon
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
