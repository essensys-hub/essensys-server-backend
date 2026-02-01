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
// The structure may vary depending on the endpoint used
type BootstrapResponse struct {
	Cameras []CameraData `json:"cameras"`
	// Some endpoints return model info instead
	Model struct {
		ID        string `json:"id"`
		ShortName string `json:"shortName"`
		LongName  string `json:"longName"`
	} `json:"model,omitempty"`
	Images []struct {
		Size int    `json:"size"`
		URL  string `json:"url"`
	} `json:"images,omitempty"`
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

