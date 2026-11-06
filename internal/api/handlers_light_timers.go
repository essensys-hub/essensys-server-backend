package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/api/testmode"
	"github.com/essensys-hub/essensys-server-backend/internal/audit"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func writeLightTimerError(w http.ResponseWriter, code int, key string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": key})
}

const lightTimersPath = "/api/admin/lights/timers"

type lightTimerView struct {
	core.LightTimer
	Minutes *int `json:"minutes"`
}

// GetAdminLightTimers handles GET /api/admin/lights/timers.
// Returns the six light auto-off delays in minutes; null when a key was never read.
func (h *Handler) GetAdminLightTimers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientID, ok := middleware.GetClientID(r)
	if !ok {
		clientID = "default"
	}
	catalogue := core.LightTimers()
	keys := make([]int, 0, len(catalogue))
	for _, t := range catalogue {
		keys = append(keys, t.Key)
	}
	stored := make(map[int]string, len(keys))
	for _, kv := range h.store.GetAllValues(clientID, keys) {
		stored[kv.K] = kv.V
	}
	timers := make([]lightTimerView, 0, len(catalogue))
	for _, t := range catalogue {
		timers = append(timers, lightTimerView{LightTimer: t, Minutes: core.ParseLightTimerValue(stored[t.Key])})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"timers": timers})
}

// PutAdminLightTimer handles PUT /api/admin/lights/timers/{id} with {"minutes": n}.
func (h *Handler) PutAdminLightTimer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, lightTimersPath+"/")
	if _, known := core.LightTimerByID(id); !known {
		writeLightTimerError(w, http.StatusBadRequest, "unknown_light")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	var in struct {
		Minutes json.RawMessage `json:"minutes"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeLightTimerError(w, http.StatusBadRequest, "invalid_minutes")
		return
	}
	// Strict integer: a quoted string, a fraction, null or an exponent is refused.
	minutes, err := strconv.Atoi(string(bytes.TrimSpace(in.Minutes)))
	if err != nil {
		writeLightTimerError(w, http.StatusBadRequest, "invalid_minutes")
		return
	}
	param, err := core.LightTimerParam(id, minutes)
	if err != nil {
		writeLightTimerError(w, http.StatusBadRequest, "invalid_minutes")
		return
	}
	params := []protocol.ExchangeKV{param}

	clientID, ok := middleware.GetClientID(r)
	if !ok {
		clientID = "default"
	}

	if testmode.IsDryRun(r) {
		validated, err := h.actionService.ValidateParams(params)
		if err != nil {
			testmode.WriteFailed(w, err.Error())
			return
		}
		_ = validated
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "minutes": minutes, "status": "dry_run"})
		return
	}

	previousVal, hadPrevious := h.store.GetValue(clientID, param.K)
	guids, err := h.actionService.AddActions(clientID, params)
	if err != nil {
		http.Error(w, "Failed to add action", http.StatusInternalServerError)
		return
	}
	addDebugLog("Light timer %s set to %s minute(s): %v", id, param.V, guids)

	if h.audit != nil && h.audit.Enabled() {
		details := map[string]any{
			"guid":             guids[0],
			"client_id":        clientID,
			"lifecycle_status": "queued",
			"ack_status":       "pending",
			"requested_index":  param.K,
			"requested_value":  param.V,
			"light_id":         id,
		}
		if hadPrevious {
			details["previous_value"] = previousVal
		}
		if u, ok := audit.UserFromRequest(r); ok {
			details["lan_user_id"] = u.ID
			if u.DisplayName != "" {
				details["display_name"] = u.DisplayName
			}
		}
		actorID := audit.ActorIDFromRequest(r, clientID)
		_ = h.audit.EmitUserAction(r.Context(), "lan_user", actorID, fmt.Sprintf("exchange:%d", param.K), param.V, details)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "minutes": minutes, "status": "queued"})
}
