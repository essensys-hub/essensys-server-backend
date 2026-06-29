package api

import (
	"encoding/json"
	"net/http"

	"github.com/essensys-hub/essensys-server-backend/internal/data"
	"github.com/essensys-hub/essensys-server-backend/internal/laniam"
	"github.com/essensys-hub/essensys-server-backend/internal/metrics"
	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

// RouterConfig groups HTTP router dependencies.
type RouterConfig struct {
	Handler            *Handler
	WebHandler         *WebHandler
	UniFiHandler       *UniFiHandler
	LanIAMHandler      *LanIAMHandler
	LanSessionStore    *laniam.SessionStore
	LanIAMEnabled      bool
	LanIAMReady        bool
	SecureCookie       bool
	ValidCredentials   map[string]string
	AuthEnabled        bool
	PassiveAuthCapture bool
	Store              data.Store
}

// NewRouter creates and configures the HTTP router with all middleware and routes.
func NewRouter(cfg RouterConfig) http.Handler {
	apiMux := http.NewServeMux()

	legacyIoT := func(pattern string, fn http.HandlerFunc) {
		apiMux.HandleFunc(pattern, fn)
	}

	var protect func(http.Handler) http.Handler
	if cfg.LanIAMEnabled && cfg.LanSessionStore != nil {
		protect = middleware.LanRequireSession(cfg.LanSessionStore, cfg.SecureCookie)
	} else {
		protect = func(h http.Handler) http.Handler { return h }
	}

	adminOnly := func(h http.Handler) http.Handler {
		if cfg.LanIAMEnabled && cfg.LanSessionStore != nil {
			return protect(middleware.LanRequireRole(models.LanRoleAdmin)(h))
		}
		return h
	}

	legacyIoT("/api/serverinfos", cfg.Handler.GetServerInfos)
	legacyIoT("/api/mystatus", cfg.Handler.PostMyStatus)
	legacyIoT("/api/myactions", cfg.Handler.GetMyActions)
	legacyIoT("/api/done/", cfg.Handler.PostDone)

	apiMux.Handle("/api/admin/inject", protect(http.HandlerFunc(cfg.Handler.PostAdminInject)))
	apiMux.Handle("/api/admin/exchange", protect(http.HandlerFunc(cfg.Handler.GetAdminExchange)))
	apiMux.Handle("/api/admin/heating/sync", protect(http.HandlerFunc(cfg.Handler.PostAdminHeatingSync)))
	apiMux.Handle("/api/admin/heating/sync/status", protect(http.HandlerFunc(cfg.Handler.GetAdminHeatingSyncStatus)))
	apiMux.Handle("/api/admin/cloudsync/status", protect(http.HandlerFunc(cfg.Handler.GetAdminCloudSyncStatus)))
	apiMux.Handle("/api/admin/scenarios/sync", protect(http.HandlerFunc(cfg.Handler.AdminScenariosSync)))
	apiMux.Handle("/api/scenarios", protect(http.HandlerFunc(cfg.Handler.HandleScenarios)))
	apiMux.Handle("/api/scenarios/", protect(http.HandlerFunc(cfg.Handler.HandleScenarios)))

	if cfg.LanIAMHandler != nil {
		apiMux.HandleFunc("/api/auth/login", cfg.LanIAMHandler.Login)
		apiMux.HandleFunc("/api/auth/auto-login", cfg.LanIAMHandler.AutoLogin)
		apiMux.HandleFunc("/api/auth/logout", cfg.LanIAMHandler.Logout)
		apiMux.Handle("/api/user/me", protect(http.HandlerFunc(cfg.LanIAMHandler.Me)))
		apiMux.Handle("/api/user/me/password", protect(http.HandlerFunc(cfg.LanIAMHandler.ChangePassword)))
		apiMux.Handle("/api/user/me/trusted-devices", protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				cfg.LanIAMHandler.ListTrustedDevices(w, r)
			case http.MethodPost:
				cfg.LanIAMHandler.CreateTrustedDevice(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})))
		apiMux.Handle("/api/user/me/trusted-devices/candidates", protect(http.HandlerFunc(cfg.LanIAMHandler.ListTrustedDeviceCandidates)))
		apiMux.Handle("/api/user/me/trusted-devices/", protect(http.HandlerFunc(cfg.LanIAMHandler.DeleteTrustedDevice)))
		apiMux.Handle("/api/admin/lan-users/bootstrap", http.HandlerFunc(cfg.LanIAMHandler.Bootstrap))
		apiMux.Handle("/api/admin/lan-users", adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				cfg.LanIAMHandler.ListUsers(w, r)
			case http.MethodPost:
				cfg.LanIAMHandler.CreateUser(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})))
		apiMux.Handle("/api/admin/lan-users/", adminOnly(http.HandlerFunc(cfg.LanIAMHandler.HandleLanUserSubresource)))
		apiMux.Handle("/api/admin/trusted-devices", adminOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				cfg.LanIAMHandler.ListAdminTrustedDevices(w, r)
			case http.MethodPost:
				cfg.LanIAMHandler.CreateAdminTrustedDevice(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})))
		apiMux.Handle("/api/admin/trusted-devices/candidates", adminOnly(http.HandlerFunc(cfg.LanIAMHandler.ListAdminTrustedDeviceCandidates)))
		apiMux.Handle("/api/admin/trusted-devices/", adminOnly(http.HandlerFunc(cfg.LanIAMHandler.HandleAdminTrustedDeviceSubresource)))
		apiMux.HandleFunc("/api/auth/register", cfg.LanIAMHandler.RegisterClosed)
	} else if cfg.WebHandler != nil {
		apiMux.HandleFunc("/api/auth/login", cfg.WebHandler.Login)
		apiMux.HandleFunc("/api/auth/logout", cfg.WebHandler.Logout)
		apiMux.HandleFunc("/api/auth/register", cfg.WebHandler.Register)
		apiMux.HandleFunc("/api/user/me", cfg.WebHandler.GetCurrentUser)
		apiMux.HandleFunc("/api/web/actions", cfg.WebHandler.PostWebActions)
		apiMux.HandleFunc("/api/web/history/latest", cfg.WebHandler.GetHistoryLatest)
	}

	if cfg.UniFiHandler != nil {
		apiMux.Handle("/api/unifi/cameras", protect(http.HandlerFunc(cfg.UniFiHandler.GetCameras)))
		apiMux.Handle("/api/unifi/cameras/", protect(http.HandlerFunc(cfg.UniFiHandler.GetCameraSnapshot)))
	}

	var apiHandler http.Handler = apiMux
	if cfg.AuthEnabled && !cfg.LanIAMEnabled {
		apiHandler = middleware.BasicAuth(cfg.ValidCredentials, cfg.Store, cfg.PassiveAuthCapture)(apiMux)
	}

	mainMux := http.NewServeMux()
	mainMux.Handle("/api/", apiHandler)
	mainMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthCheckHandler(w, r, cfg.LanIAMEnabled, cfg.LanIAMReady)
	})
	mainMux.HandleFunc("/debug", cfg.Handler.GetDebug)
	mainMux.HandleFunc("/debug/logs", cfg.Handler.GetDebugLogs)
	mainMux.HandleFunc("/table_ref", cfg.Handler.GetTableRef)
	mainMux.Handle("/metrics", metrics.Handler())

	finalHandler := metrics.InstrumentHandler(mainMux)
	finalHandler = middleware.RequestLogger(finalHandler)
	finalHandler = middleware.Recovery(finalHandler)
	return finalHandler
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request, lanIAMEnabled, lanIAMReady bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	response := map[string]interface{}{
		"status":          "ok",
		"lan_iam_enabled": lanIAMEnabled,
		"auth_ready":      !lanIAMEnabled || lanIAMReady,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}
