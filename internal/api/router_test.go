package api

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/core"
	"github.com/essensys-hub/essensys-server-backend/internal/data"
)

func testRouter(handler *Handler, creds map[string]string, authEnabled bool, store data.Store) http.Handler {
	return NewRouter(RouterConfig{
		Handler:            handler,
		ValidCredentials:   creds,
		AuthEnabled:        authEnabled,
		PassiveAuthCapture: true,
		Store:              store,
	})
}

func TestRouter_HealthCheck(t *testing.T) {
	// Create test dependencies
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store, nil, nil)

	// Create router with empty credentials (health check doesn't need auth)
	router := testRouter(handler, map[string]string{}, false, store)

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Execute request
	router.ServeHTTP(w, req)

	// Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

func TestRouter_AuthenticationRequired(t *testing.T) {
	t.Skip("Auth middleware is passive (captures credentials but accepts all requests); routes no longer return 401 without auth. Test asserts obsolete rejection behavior (SCRUM-9)")
	// Create test dependencies
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store, nil, nil)

	// Create router with credentials
	validCredentials := map[string]string{
		"testclient": "testpass",
	}
	router := testRouter(handler, validCredentials, true, store)

	// Test routes that require authentication
	routes := []string{
		"/api/serverinfos",
		"/api/mystatus",
		"/api/myactions",
		"/api/done/test-guid",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			// Request without auth header
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("Expected status 401 for %s without auth, got %d", route, w.Code)
			}
		})
	}
}

func TestRouter_ValidAuthentication(t *testing.T) {
	// Create test dependencies
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store, nil, nil)

	// Create router with credentials
	validCredentials := map[string]string{
		"testclient": "testpass",
	}
	router := testRouter(handler, validCredentials, true, store)

	// Create request with valid auth
	req := httptest.NewRequest(http.MethodGet, "/api/serverinfos", nil)
	
	// Add Basic Auth header
	credentials := base64.StdEncoding.EncodeToString([]byte("testclient:testpass"))
	req.Header.Set("Authorization", "Basic "+credentials)
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should not be 401
	if w.Code == http.StatusUnauthorized {
		t.Errorf("Expected successful auth, got 401")
	}
}

func TestRouter_MiddlewareChain(t *testing.T) {
	// Create test dependencies
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store, nil, nil)

	// Create router
	validCredentials := map[string]string{
		"testclient": "testpass",
	}
	router := testRouter(handler, validCredentials, true, store)

	// Create request with valid auth
	req := httptest.NewRequest(http.MethodGet, "/api/serverinfos", nil)
	credentials := base64.StdEncoding.EncodeToString([]byte("testclient:testpass"))
	req.Header.Set("Authorization", "Basic "+credentials)
	
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify the request went through the middleware chain and reached the handler
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify Content-Type header is set (by handler)
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json ;charset=UTF-8" {
		t.Errorf("Expected Content-Type 'application/json ;charset=UTF-8', got %s", contentType)
	}
}

func TestRouter_AuthenticationDisabled(t *testing.T) {
	// Create test dependencies
	store := data.NewMemoryStore()
	actionService := core.NewActionService(store)
	statusService := core.NewStatusService(store)
	handler := NewHandler(actionService, statusService, store, nil, nil)

	// Create router with authentication disabled
	router := testRouter(handler, map[string]string{}, false, store)

	// Test routes that normally require authentication
	routes := []string{
		"/api/serverinfos",
		"/api/mystatus",
		"/api/myactions",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			// Request without auth header should succeed when auth is disabled
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Should not be 401 when auth is disabled
			if w.Code == http.StatusUnauthorized {
				t.Errorf("Expected no auth required for %s when auth disabled, got 401", route)
			}
		})
	}
}
