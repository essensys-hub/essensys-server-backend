package api

import (
	"encoding/json"
	"net/http"

	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
)

// NewRouter creates and configures the HTTP router with all middleware and routes
// If authEnabled is false, authentication middleware is skipped
func NewRouter(handler *Handler, validCredentials map[string]string, authEnabled bool) http.Handler {
	// Create separate mux for API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/serverinfos", handler.GetServerInfos)
	apiMux.HandleFunc("/api/mystatus", handler.PostMyStatus)
	apiMux.HandleFunc("/api/myactions", handler.GetMyActions)
	apiMux.HandleFunc("/api/done/", handler.PostDone)           // Trailing slash to match /api/done/{guid}
	apiMux.HandleFunc("/api/admin/inject", handler.PostAdminInject) // Admin endpoint to inject actions

	// Conditionally apply authentication middleware to API routes
	var apiHandler http.Handler = apiMux
	if authEnabled {
		apiHandler = middleware.BasicAuth(validCredentials)(apiMux)
	}

	// Create main mux that includes both authenticated and public routes
	mainMux := http.NewServeMux()
	mainMux.Handle("/api/", apiHandler)
	mainMux.HandleFunc("/health", healthCheckHandler)

	// Wire up middleware chain: Recovery → Logging → Routes
	// The chain is applied in reverse order (innermost to outermost)
	var finalHandler http.Handler = mainMux
	finalHandler = middleware.RequestLogger(finalHandler)
	finalHandler = middleware.Recovery(finalHandler)

	return finalHandler
}

// healthCheckHandler handles GET /health
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	// Only allow GET method
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return simple health check response
	response := map[string]string{
		"status": "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
