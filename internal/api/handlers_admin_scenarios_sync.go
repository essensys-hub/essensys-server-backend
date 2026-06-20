package api

import (
	"encoding/json"
	"net/http"
)

// AdminScenariosSync handles GET/PUT /api/admin/scenarios/sync (LAN Settings toggle).
func (h *Handler) AdminScenariosSync(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getAdminScenariosSync(w, r)
	case http.MethodPut:
		h.putAdminScenariosSync(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) getAdminScenariosSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	if h.cloudAgent == nil {
		json.NewEncoder(w).Encode(map[string]any{
			"enabled": false,
			"found":   false,
			"message": "cloud sync agent not configured",
		})
		return
	}
	json.NewEncoder(w).Encode(h.cloudAgent.ScenariosSyncStatus())
}

func (h *Handler) putAdminScenariosSync(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if h.cloudAgent == nil {
		http.Error(w, "cloud sync agent not configured", http.StatusServiceUnavailable)
		return
	}
	if err := h.cloudAgent.SetScenariosSyncEnabled(r.Context(), body.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json ;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.cloudAgent.ScenariosSyncStatus())
}
