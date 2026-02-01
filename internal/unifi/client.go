package unifi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/config"
	"golang.org/x/net/publicsuffix"
)

// Client handles communication with UniFi Protect API
type Client struct {
	config     config.UniFiConfig
	httpClient *http.Client
	mu         sync.RWMutex
	jar        *cookiejar.Jar
	authenticated bool
}

// NewClient creates a new UniFi Protect client
func NewClient(cfg config.UniFiConfig) *Client {
	// Create HTTP client that skips SSL verification (UDM Pro uses self-signed certs)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Create cookie jar for session management
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})

	client := &Client{
		config:        cfg,
		jar:           jar,
		authenticated: false,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
			Jar:       jar,
		},
	}

	return client
}

// authenticate performs authentication to get a session token
// Note: /api/bootstrap works directly with X-API-KEY header, so authentication
// may not be needed. This method is kept for compatibility with endpoints that require session.
func (c *Client) authenticate() error {
	if c.authenticated {
		return nil
	}

	if !c.config.Enabled {
		return fmt.Errorf("UniFi Protect is disabled")
	}

	// If API key is provided, try to use it directly (no session auth needed for /api/bootstrap)
	if c.config.APIKey != "" {
		// Test if API key works by trying /api/bootstrap
		testURL := fmt.Sprintf("%s/api/bootstrap", c.config.BaseURL)
		req, _ := http.NewRequest("GET", testURL, nil)
		req.Header.Set("X-API-Key", c.config.APIKey)
		req.Header.Set("Accept", "application/json")
		
		resp, err := c.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				// API key works directly, no session needed
				c.authenticated = true
				return nil
			}
		}
	}

	// Fallback to username/password session auth if API key alone doesn't work
	if c.config.Username != "" && c.config.Password != "" {
		authURL := fmt.Sprintf("%s/api/auth/login", c.config.BaseURL)
		authData := map[string]string{
			"username": c.config.Username,
			"password": c.config.Password,
		}
		jsonData, _ := json.Marshal(authData)
		
		req, _ := http.NewRequest("POST", authURL, bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		
		resp, err := c.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			c.authenticated = true
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		return fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	// If only API key is configured, assume it works (will be validated on first request)
	if c.config.APIKey != "" {
		c.authenticated = true
		return nil
	}

	return fmt.Errorf("no authentication method configured")
}

// addAuthHeaders adds authentication headers to the request
func (c *Client) addAuthHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("X-API-Key", c.config.APIKey)
	}
}

// GetBootstrap retrieves bootstrap data including cameras list
func (c *Client) GetBootstrap() (*BootstrapResponse, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("UniFi Protect is disabled")
	}

	// Authenticate first if not already authenticated
	if err := c.authenticate(); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Try different endpoints (start with /proxy/protect/integration/v1/cameras from API docs)
	endpoints := []string{
		"/proxy/protect/integration/v1/cameras",  // Official API endpoint from docs
		"/api/cameras",  // Fallback: direct cameras endpoint
		"/api/bootstrap",  // Fallback: bootstrap might have cameras
		"/unifi-api/protect/api/bootstrap",
	}

	var lastErr error
	for _, endpoint := range endpoints {
		bootstrapURL := fmt.Sprintf("%s%s", c.config.BaseURL, endpoint)
		req, err := http.NewRequest("GET", bootstrapURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		c.addAuthHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			// Check if response is JSON (not HTML)
			contentType := resp.Header.Get("Content-Type")
			if contentType != "" && !strings.Contains(contentType, "application/json") {
				// Read first bytes to check if it's HTML
				bodyBytes, _ := io.ReadAll(resp.Body)
				bodyStr := string(bodyBytes)
				if strings.HasPrefix(bodyStr, "<!doctype") || strings.HasPrefix(bodyStr, "<html") {
					// Try next endpoint
					lastErr = fmt.Errorf("endpoint %s returned HTML instead of JSON", endpoint)
					continue
				}
				// Try to decode anyway
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}

			// Read response body to check structure
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				lastErr = err
				continue
			}

			// Try to decode as BootstrapResponse (with cameras array)
			var bootstrap BootstrapResponse
			if err := json.Unmarshal(bodyBytes, &bootstrap); err != nil {
				lastErr = fmt.Errorf("failed to decode JSON: %w", err)
				continue
			}

			// Check if response has cameras array (BootstrapResponse format)
			if len(bootstrap.Cameras) > 0 {
				return &bootstrap, nil
			}

			// If endpoint is /proxy/protect/integration/v1/cameras or /api/cameras,
			// try to decode as direct cameras array
			if endpoint == "/proxy/protect/integration/v1/cameras" || endpoint == "/api/cameras" {
				var camerasArray []CameraData
				if err := json.Unmarshal(bodyBytes, &camerasArray); err == nil && len(camerasArray) > 0 {
					bootstrap.Cameras = camerasArray
					return &bootstrap, nil
				}
			}

			// If we got model info but no cameras, this is the wrong endpoint
			if bootstrap.Model.ID != "" {
				lastErr = fmt.Errorf("endpoint %s returned model info but no cameras", endpoint)
				continue
			}

			// Empty cameras array, try next endpoint
			lastErr = fmt.Errorf("endpoint %s returned empty cameras array", endpoint)
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized {
			// Re-authenticate and retry
			c.authenticated = false
			if err := c.authenticate(); err != nil {
				return nil, fmt.Errorf("re-authentication failed: %w", err)
			}
			// Retry this endpoint
			continue
		}

		lastErr = fmt.Errorf("endpoint %s returned status %d", endpoint, resp.StatusCode)
	}

	return nil, fmt.Errorf("all endpoints failed: %w", lastErr)
}

// GetCameraSnapshot retrieves a snapshot image for a camera
func (c *Client) GetCameraSnapshot(cameraID string) ([]byte, error) {
	if !c.config.Enabled {
		return nil, fmt.Errorf("UniFi Protect is disabled")
	}

	// Authenticate first if not already authenticated
	if err := c.authenticate(); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Try different snapshot endpoints (start with /proxy/protect/integration/v1/cameras from API docs)
	endpoints := []string{
		fmt.Sprintf("/proxy/protect/integration/v1/cameras/%s/snapshot", cameraID),  // Official API endpoint from docs
		fmt.Sprintf("/api/cameras/%s/snapshot", cameraID),  // Fallback: direct cameras endpoint
		fmt.Sprintf("/unifi-api/protect/api/cameras/%s/snapshot", cameraID),
		fmt.Sprintf("/proxy/protect/api/cameras/%s/snapshot", cameraID),
	}

	var lastErr error
	for _, endpoint := range endpoints {
		snapshotURL := fmt.Sprintf("%s%s", c.config.BaseURL, endpoint)
		req, err := http.NewRequest("GET", snapshotURL, nil)
		if err != nil {
			lastErr = err
			continue
		}

		c.addAuthHeaders(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			// Re-authenticate and retry
			c.authenticated = false
			if err := c.authenticate(); err != nil {
				return nil, fmt.Errorf("re-authentication failed: %w", err)
			}
			continue
		}

		if resp.StatusCode == http.StatusOK {
			imageData, err := io.ReadAll(resp.Body)
			if err != nil {
				lastErr = err
				continue
			}
			// Verify it's an image (starts with image magic bytes)
			if len(imageData) > 4 && (imageData[0] == 0xFF && imageData[1] == 0xD8) {
				return imageData, nil
			}
			lastErr = fmt.Errorf("endpoint %s did not return valid image data", endpoint)
			continue
		}

		lastErr = fmt.Errorf("endpoint %s returned status %d", endpoint, resp.StatusCode)
	}

	return nil, fmt.Errorf("all snapshot endpoints failed: %w", lastErr)
}

// GetCameras returns a list of cameras
func (c *Client) GetCameras() ([]Camera, error) {
	bootstrap, err := c.GetBootstrap()
	if err != nil {
		return nil, err
	}

	cameras := make([]Camera, 0, len(bootstrap.Cameras))
	for _, camData := range bootstrap.Cameras {
		// Map State to Status (CONNECTED -> online, DISCONNECTED -> offline)
		status := "offline"
		if camData.State == "CONNECTED" {
			status = "online"
		}
		// Use IsConnected if available, otherwise infer from State
		isConnected := camData.IsConnected
		if !isConnected && camData.State == "CONNECTED" {
			isConnected = true
		}

		// Handle LastSeen (may be 0 if not provided)
		var lastSeen time.Time
		if camData.LastSeen > 0 {
			lastSeen = time.Unix(camData.LastSeen/1000, 0)
		} else {
			lastSeen = time.Now() // Use current time as fallback
		}

		camera := Camera{
			ID:          camData.ID,
			Name:        camData.Name,
			Type:        camData.Type
			if camData.Type == "" {
				camera.Type = camData.ModelKey // Use modelKey as fallback
			}
			Model:       camData.Model
			if camData.Model == "" {
				camera.Model = camData.ModelKey // Use modelKey as fallback
			}
			Status:      status,
			LastSeen:    lastSeen,
			IsRecording: camData.IsRecording,
			IsConnected: isConnected,
			Mac:         camData.Mac,
			Firmware:    camData.Firmware,
		}
		cameras = append(cameras, camera)
	}

	return cameras, nil
}
