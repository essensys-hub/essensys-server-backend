package unifi

import "time"

// Camera represents a UniFi Protect camera
type Camera struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Model       string    `json:"model"`
	Status      string    `json:"status"` // "online", "offline", etc.
	LastSeen    time.Time `json:"last_seen"`
	IsRecording bool      `json:"is_recording"`
	IsConnected bool      `json:"is_connected"`
	Mac         string    `json:"mac"`
	Firmware    string    `json:"firmware"`
}

// BootstrapResponse represents the UniFi Protect bootstrap API response
type BootstrapResponse struct {
	Cameras []CameraData `json:"cameras"`
}

// CameraData represents camera data from bootstrap API
type CameraData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Model       string `json:"model"`
	State       string `json:"state"`
	LastSeen    int64  `json:"lastSeen"`
	IsRecording bool   `json:"isRecording"`
	IsConnected bool   `json:"isConnected"`
	Mac         string `json:"mac"`
	Firmware    string `json:"firmwareVersion"`
}

