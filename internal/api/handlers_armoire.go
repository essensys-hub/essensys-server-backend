package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/armoire"
	"github.com/essensys-hub/essensys-server-backend/internal/core"
)

// GetAdminArmoireSnapshot handles GET /api/admin/armoire/snapshot
func (h *Handler) GetAdminArmoireSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	threshold := core.DefaultArmoireOfflineThreshold
	if h.armoireCfg.OfflineThresholdSeconds > 0 {
		threshold = time.Duration(h.armoireCfg.OfflineThresholdSeconds) * time.Second
	}

	snap := armoire.BuildSnapshot(h.store, armoire.SnapshotOptions{
		ClientID:         h.armoireCfg.ClientID,
		OfflineThreshold: threshold,
	})

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(snap)
}
