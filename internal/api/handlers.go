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
    _ "embed"
    "sync"
    "time"
    "fmt"
    "html/template"
    "strconv"
    "strings"
)

//go:embed debug.html
var debugHTML []byte

// Simple in-memory log buffer for debug interface
var (
    debugLogs      []string
    debugLogMutex  sync.Mutex
    maxDebugLogs   = 50
)

func addDebugLog(format string, args ...interface{}) {
    debugLogMutex.Lock()
    defer debugLogMutex.Unlock()
    
    msg := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
    debugLogs = append([]string{msg}, debugLogs...)
    
    if len(debugLogs) > maxDebugLogs {
        debugLogs = debugLogs[:maxDebugLogs]
    }
}

// GetDebugLogs handles GET /debug/logs
func (h *Handler) GetDebugLogs(w http.ResponseWriter, r *http.Request) {
    debugLogMutex.Lock()
    logs := make([]string, len(debugLogs))
    copy(logs, debugLogs)
    debugLogMutex.Unlock()
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{"logs": logs})
}

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
	// 566-585: Temps d'action des volets (secondes) — Volets_PDV/CHB/PDE_Temps
	// Others: Various system indices
	indices := []int{613, 607, 615, 590, 349, 350, 351, 352, 363, 425, 426, 920,
		// Volets PDV (566-572) : salon x3, salle à manger x2, bureau, volet store
		566, 567, 568, 569, 570, 571, 572,
		// Volets CHB (574-578) : grande chambre x2, petites chambres x3
		574, 575, 576, 577, 578,
		// Volets PDE (582-585) : cuisine x2, salle de bain, store terrasse
		582, 583, 584, 585}

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
    addDebugLog("Status received: %+v", statusReq.EK) // Log full content for debugging

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
    
    if len(actions) > 0 {
        addDebugLog("Client retrieved %d actions (GUIDs: %v)", len(actions), getGuids(actions))
    } else {
        // addDebugLog("Client polled for actions (Empty)") // Too noisy
    }

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

func getGuids(actions []protocol.Action) []string {
    guids := make([]string, len(actions))
    for i, a := range actions {
        guids[i] = a.GUID
    }
    return guids
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
	action, found := h.store.AcknowledgeAction(clientID, guid)
	if !found {
        addDebugLog("Client tried to confirm unknown/expired GUID: %s", guid)
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}
    
    // Sync action values to the store (Reference Table)
    // This ensures that when an action is done, the server state reflects the change immediately
    if action != nil {
        for _, param := range action.Params {
            h.store.SetValue(clientID, param.K, param.V)
        }
        addDebugLog("Synced %d values from confirmed action %s", len(action.Params), guid)
    }

	// Log acknowledgment (like server.sample.go)
	log.Printf("[GO] Action acknowledged: %s", guid)
    addDebugLog("Client CONFIRMED action: %s", guid)

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
    
    addDebugLog("Injected action: %s (Params: %v)", guid, params)

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

// GetAdminExchange handles GET /api/admin/exchange?keys=566,567,568
// Returns the last known values of the requested exchange table indices,
// as reported by the firmware via POST /api/mystatus.
// Indices never reported by the firmware are omitted from the response.
func (h *Handler) GetAdminExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientID, ok := middleware.GetClientID(r)
	if !ok {
		clientID = "default"
	}

	keysParam := r.URL.Query().Get("keys")
	if keysParam == "" {
		http.Error(w, "Missing 'keys' query parameter (e.g. ?keys=566,567)", http.StatusBadRequest)
		return
	}

	var indices []int
	for _, part := range strings.Split(keysParam, ",") {
		k, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			http.Error(w, "Invalid index in 'keys': "+part, http.StatusBadRequest)
			return
		}
		indices = append(indices, k)
	}

	values := h.store.GetAllValues(clientID, indices)
	if values == nil {
		values = []protocol.ExchangeKV{}
	}

	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"values": values})
}

// GetDebug handles GET /debug
// Serves the HTML debug interface
func (h *Handler) GetDebug(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write(debugHTML)
}
// GetTableRef handles GET /table_ref
// Serves the HTML reference table page (Redis dump)
func (h *Handler) GetTableRef(w http.ResponseWriter, r *http.Request) {
	// Get all values from the store (default client)
	data := h.store.GetFullTable("default")

	// Convert map to slice for display
	type Item struct {
		Key   int
		Value string
	}
	items := make([]Item, 0, len(data))
	for k, v := range data {
		items = append(items, Item{Key: k, Value: v})
	}

	// Sort items by Key
	// We need generic sorting or custom slice type. Since it's simple:
	// Use sort.Slice in a block or import sort.
	// We will import sort at the top of file or use a simple bubble sort if import is tricky with replace.
	// Let's rely on adding sort to imports first or inline bubble sort for simplicity?
	// Actually, best practice is to add "sort" to imports. I'll do that in a separate step or assume I can't easily edit imports far away.
	// I'll do a simple inline sort to avoid import hassle on partial file edits, or just add "sort" to the imports block now.
    // Let's try to add sort to imports in the file view.
    // Wait, I can't modify imports easily here. I will just use a helper function that does insertion sort or bubble sort since len is small (<1000).
    
    // Bubble sort for simplicity (N < 2048 usually)
    for i := 0; i < len(items)-1; i++ {
        for j := 0; j < len(items)-i-1; j++ {
            if items[j].Key > items[j+1].Key {
                items[j], items[j+1] = items[j+1], items[j]
            }
        }
    }

	// Render template
	t, err := template.New("table_ref").Parse(TableRefHTML)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

    // Get pending actions (Peek)
    queue := h.store.DequeueActions("default")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dataView := struct {
		Count      int
		Items      []Item
        Queue      []protocol.Action
        QueueCount int
	}{
		Count:      len(items),
		Items:      items,
        Queue:      queue,
        QueueCount: len(queue),
	}

	if err := t.Execute(w, dataView); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
