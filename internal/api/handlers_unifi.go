package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/essensys-hub/essensys-server-backend/internal/unifi"
)

// UniFiHandler handles UniFi Protect API requests
type UniFiHandler struct {
	unifiClient *unifi.Client
}

// NewUniFiHandler creates a new UniFi handler
func NewUniFiHandler(unifiClient *unifi.Client) *UniFiHandler {
	return &UniFiHandler{
		unifiClient: unifiClient,
	}
}

// GetCameras handles GET /api/unifi/cameras
func (h *UniFiHandler) GetCameras(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.unifiClient == nil {
		http.Error(w, "UniFi Protect is not configured", http.StatusServiceUnavailable)
		return
	}

	cameras, err := h.unifiClient.GetCameras()
	if err != nil {
		log.Printf("Failed to get cameras: %v", err)
		http.Error(w, "Failed to retrieve cameras", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cameras": cameras,
	})
}

// GetCameraSnapshot handles GET /api/unifi/cameras/:id/snapshot
func (h *UniFiHandler) GetCameraSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.unifiClient == nil {
		http.Error(w, "UniFi Protect is not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract camera ID from URL path: /api/unifi/cameras/{id}/snapshot
	// Path format: /api/unifi/cameras/{id}/snapshot
	path := strings.TrimPrefix(r.URL.Path, "/api/unifi/cameras/")
	if path == "" || path == r.URL.Path {
		http.Error(w, "Camera ID is required", http.StatusBadRequest)
		return
	}

	// Remove /snapshot suffix if present
	path = strings.TrimSuffix(path, "/snapshot")
	
	// Handle query parameters (e.g., ?t=timestamp)
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	cameraID := path
	if cameraID == "" {
		http.Error(w, "Camera ID is required", http.StatusBadRequest)
		return
	}

	snapshot, err := h.unifiClient.GetCameraSnapshot(cameraID)
	if err != nil {
		log.Printf("Failed to get snapshot for camera %s: %v", cameraID, err)
		http.Error(w, "Failed to retrieve snapshot", http.StatusInternalServerError)
		return
	}

	// Set appropriate headers for image
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(http.StatusOK)
	w.Write(snapshot)
}
