package middleware

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuth_MissingAuthHeader(t *testing.T) {
	validCredentials := map[string]string{
		"client1": "pass1",
	}

	handler := BasicAuth(validCredentials)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestBasicAuth_InvalidAuthHeader(t *testing.T) {
	validCredentials := map[string]string{
		"client1": "pass1",
	}

	handler := BasicAuth(validCredentials)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token123")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestBasicAuth_InvalidBase64(t *testing.T) {
	validCredentials := map[string]string{
		"client1": "pass1",
	}

	handler := BasicAuth(validCredentials)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic invalid!!!base64")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestBasicAuth_InvalidCredentials(t *testing.T) {
	validCredentials := map[string]string{
		"client1": "pass1",
	}

	handler := BasicAuth(validCredentials)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create invalid credentials
	credentials := "client1:wrongpass"
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

func TestBasicAuth_ValidCredentials(t *testing.T) {
	validCredentials := map[string]string{
		"client1": "pass1",
	}

	var capturedClientID string
	handler := BasicAuth(validCredentials)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract clientID from context
		if clientID, ok := r.Context().Value(ClientIDKey).(string); ok {
			capturedClientID = clientID
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Create valid credentials
	credentials := "client1:pass1"
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if capturedClientID != "client1" {
		t.Errorf("Expected clientID 'client1', got '%s'", capturedClientID)
	}
}

func TestBasicAuth_MultipleClients(t *testing.T) {
	validCredentials := map[string]string{
		"client1": "pass1",
		"client2": "pass2",
		"client3": "pass3",
	}

	testCases := []struct {
		username string
		password string
		expected int
	}{
		{"client1", "pass1", http.StatusOK},
		{"client2", "pass2", http.StatusOK},
		{"client3", "pass3", http.StatusOK},
		{"client1", "wrongpass", http.StatusUnauthorized},
		{"unknown", "pass1", http.StatusUnauthorized},
	}

	for _, tc := range testCases {
		handler := BasicAuth(validCredentials)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		credentials := tc.username + ":" + tc.password
		encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Basic "+encoded)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != tc.expected {
			t.Errorf("For %s:%s, expected status %d, got %d", tc.username, tc.password, tc.expected, w.Code)
		}
	}
}

func TestBasicAuth_MalformedCredentials(t *testing.T) {
	validCredentials := map[string]string{
		"client1": "pass1",
	}

	handler := BasicAuth(validCredentials)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Credentials without colon
	credentials := "client1pass1"
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic "+encoded)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}
