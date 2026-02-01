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
func (c *Client) authenticate() error {
	if c.authenticated {
		return nil
	}

	if !c.config.Enabled {
		return fmt.Errorf("UniFi Protect is disabled")
	}

	// Try API key authentication first (if username/password not provided)
	if c.config.APIKey != "" && c.config.Username == "" {
		// Some UniFi versions accept API key directly in auth endpoint
		authURL := fmt.Sprintf("%s/api/auth/login", c.config.BaseURL)
		authData := map[string]string{
			"apiKey": c.config.APIKey,
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
	}

	// Fallback to username/password if API key alone doesn't work
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

	// Try different endpoints
	endpoints := []string{
		"/api/bootstrap",
		"/unifi-api/protect/api/bootstrap",
		"/proxy/protect/api/bootstrap",
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

			var bootstrap BootstrapResponse
			if err := json.NewDecoder(resp.Body).Decode(&bootstrap); err != nil {
				lastErr = err
				continue
			}
			return &bootstrap, nil
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

	// Authenticate first if not already authenticated
	if err := c.authenticate(); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Try different snapshot endpoints
	endpoints := []string{
		fmt.Sprintf("/api/cameras/%s/snapshot", cameraID),
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
