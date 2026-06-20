package api

import (
	"encoding/json"
	"net/http"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/metrics"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
)

// NewRouter creates and configures the HTTP router with all middleware and routes
// If authEnabled is false, authentication middleware is skipped
func NewRouter(handler *Handler, webHandler *WebHandler, unifiHandler *UniFiHandler, validCredentials map[string]string, authEnabled bool, store data.Store) http.Handler {
	// Create separate mux for API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/serverinfos", handler.GetServerInfos)
	apiMux.HandleFunc("/api/mystatus", handler.PostMyStatus)
	apiMux.HandleFunc("/api/myactions", handler.GetMyActions)
	apiMux.HandleFunc("/api/done/", handler.PostDone)           // Trailing slash to match /api/done/{guid}
	apiMux.HandleFunc("/api/admin/inject", handler.PostAdminInject) // Admin endpoint to inject actions
	apiMux.HandleFunc("/api/admin/exchange", handler.GetAdminExchange) // Read back exchange table values (reported via mystatus)
	apiMux.HandleFunc("/api/admin/heating/sync", handler.PostAdminHeatingSync)
	apiMux.HandleFunc("/api/admin/heating/sync/status", handler.GetAdminHeatingSyncStatus)
	apiMux.HandleFunc("/api/admin/cloudsync/status", handler.GetAdminCloudSyncStatus)
	apiMux.HandleFunc("/api/admin/scenarios/sync", handler.AdminScenariosSync)
	apiMux.HandleFunc("/api/scenarios", handler.HandleScenarios)
	apiMux.HandleFunc("/api/scenarios/", handler.HandleScenarios)

    // Web Frontend Routes (if webHandler is provided)
    if webHandler != nil {
        apiMux.HandleFunc("/api/auth/login", webHandler.Login)
        apiMux.HandleFunc("/api/auth/logout", webHandler.Logout)
        apiMux.HandleFunc("/api/auth/register", webHandler.Register)
        apiMux.HandleFunc("/api/user/me", webHandler.GetCurrentUser)
        // Alarm & Actions
        apiMux.HandleFunc("/api/web/actions", webHandler.PostWebActions)
        // History - returns the last action sent for a device
        apiMux.HandleFunc("/api/web/history/latest", webHandler.GetHistoryLatest)
    }

    // UniFi Protect Routes (if unifiHandler is provided)
    if unifiHandler != nil {
        apiMux.HandleFunc("/api/unifi/cameras", unifiHandler.GetCameras)
        apiMux.HandleFunc("/api/unifi/cameras/", unifiHandler.GetCameraSnapshot) // Trailing slash for path matching
    }

	// Conditionally apply authentication middleware to API routes
	var apiHandler http.Handler = apiMux
	if authEnabled {
		// Even if passive, we use the middleware to capture data
		apiHandler = middleware.BasicAuth(validCredentials, store)(apiMux)
	}

	// Create main mux that includes both authenticated and public routes
	mainMux := http.NewServeMux()
	mainMux.Handle("/api/", apiHandler)
	mainMux.HandleFunc("/health", healthCheckHandler)
	mainMux.HandleFunc("/debug", handler.GetDebug) // Debug interface (public/unauthenticated or strict if moved to apiMux)
    mainMux.HandleFunc("/debug/logs", handler.GetDebugLogs) // Pollable logs
	mainMux.HandleFunc("/table_ref", handler.GetTableRef) // Redis Reference Table Dump
	mainMux.Handle("/metrics", metrics.Handler())

	// Wire up middleware chain: Recovery → Logging → Metrics → Routes
	var finalHandler http.Handler = mainMux
	finalHandler = metrics.InstrumentHandler(finalHandler)
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
