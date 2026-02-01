package unifi

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
)

// Client handles communication with UniFi Protect API
type Client struct {
	config     config.UniFiConfig
	httpClient *http.Client
	mu         sync.RWMutex
}

// NewClient creates a new UniFi Protect client
func NewClient(cfg config.UniFiConfig) *Client {
	// Create HTTP client that skips SSL verification (UDM Pro uses self-signed certs)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		},
	}
}

// addAuthHeaders adds authentication headers to the request using API key
func (c *Client) addAuthHeaders(req *http.Request) {
	if c.config.APIKey != "" {
		// UniFi Protect API uses X-API-Key header (not Authorization Bearer)
		req.Header.Set("X-API-Key", c.config.APIKey)
		req.Header.Set("Accept", "application/json")
	}
}

// GetBootstrap retrieves bootstrap data including cameras list
func (c *Client) GetBootstrap() (*BootstrapResponse, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("UniFi Protect is disabled")
	}

	if c.config.APIKey == "" {
		return nil, fmt.Errorf("UniFi Protect API key is not configured")
	}

	// Use /unifi-api/protect/api/bootstrap endpoint for UniFi Protect API
	bootstrapURL := fmt.Sprintf("%s/unifi-api/protect/api/bootstrap", c.config.BaseURL)
	req, err := http.NewRequest("GET", bootstrapURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create bootstrap request: %w", err)
	}

	c.addAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get bootstrap: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: invalid API key")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get bootstrap: %d - %s", resp.StatusCode, string(body))
	}

	var bootstrap BootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&bootstrap); err != nil {
		return nil, fmt.Errorf("failed to decode bootstrap response: %w", err)
	}

	return &bootstrap, nil
}

// GetCameraSnapshot retrieves a snapshot image for a camera
func (c *Client) GetCameraSnapshot(cameraID string) ([]byte, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("UniFi Protect is disabled")
	}

	if c.config.APIKey == "" {
		return nil, fmt.Errorf("UniFi Protect API key is not configured")
	}

	// Use /unifi-api/protect/api/cameras/{id}/snapshot endpoint for snapshots
	snapshotURL := fmt.Sprintf("%s/unifi-api/protect/api/cameras/%s/snapshot", c.config.BaseURL, cameraID)
	req, err := http.NewRequest("GET", snapshotURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot request: %w", err)
	}

	c.addAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: invalid API key")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get snapshot: %d - %s", resp.StatusCode, string(body))
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot data: %w", err)
	}

	return imageData, nil
}

// GetCameras returns a list of cameras
func (c *Client) GetCameras() ([]Camera, error) {
	bootstrap, err := c.GetBootstrap()
	if err != nil {
		return nil, err
	}

	cameras := make([]Camera, 0, len(bootstrap.Cameras))
	for _, camData := range bootstrap.Cameras {
		camera := Camera{
			ID:          camData.ID,
			Name:        camData.Name,
			Type:        camData.Type,
			Model:       camData.Model,
			Status:      camData.State,
			LastSeen:    time.Unix(camData.LastSeen/1000, 0),
			IsRecording: camData.IsRecording,
			IsConnected: camData.IsConnected,
			Mac:         camData.Mac,
			Firmware:    camData.Firmware,
		}
		cameras = append(cameras, camera)
	}

	return cameras, nil
}
