package handlers

import (
	"net/http"
	"status/app/internal/alerts"
	"status/app/internal/auth"
	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/resources"
	"status/app/internal/security"
	"strings"
)

// SetupRoutes configures all HTTP routes and middlewares
func SetupRoutes(authMgr *auth.Auth, alertMgr *alerts.Manager, services []*models.Service, gl *resources.Client) http.Handler {
	// Public API routes (with rate limiting)
	api := http.NewServeMux()
	api.HandleFunc("/api/check", HandleCheck(services))
	api.HandleFunc("/api/metrics", HandleMetrics())
	api.HandleFunc("/api/resources", HandleResources(gl))
	api.HandleFunc("/api/resources/config", HandleGetResourcesUIConfig())
	api.HandleFunc("/api/services", HandleGetServices) // Public services list
	api.HandleFunc("/api/services/templates", HandleGetServiceTemplates)

	// Admin API routes (with authentication)
	authAPI := http.NewServeMux()
	authAPI.HandleFunc("/api/admin/ingest-now", authMgr.RequireAuth(HandleIngestNow(services)))
	authAPI.HandleFunc("/api/admin/reset-recent", authMgr.RequireAuth(HandleResetRecent()))
	authAPI.HandleFunc("/api/admin/check", authMgr.RequireAuth(HandleAdminCheck(services)))
	authAPI.HandleFunc("/api/admin/toggle-monitoring", authMgr.RequireAuth(HandleToggleMonitoring(services)))
	authAPI.HandleFunc("/api/admin/blocks", authMgr.RequireAuth(HandleListBlocks()))
	authAPI.HandleFunc("/api/admin/unblock", authMgr.RequireAuth(HandleUnblockIP()))
	authAPI.HandleFunc("/api/admin/clear-blocks", authMgr.RequireAuth(HandleClearAllBlocks()))
	authAPI.HandleFunc("/api/admin/alerts/config", authMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			HandleGetAlertsConfig(alertMgr)(w, r)
		} else if r.Method == http.MethodPost {
			HandleSaveAlertsConfig(alertMgr)(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	authAPI.HandleFunc("/api/admin/resources/config", authMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			HandleGetResourcesUIConfig()(w, r)
		} else if r.Method == http.MethodPost {
			HandleSaveResourcesUIConfig()(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	authAPI.HandleFunc("/api/admin/alerts/test", authMgr.RequireAuth(HandleTestEmail(alertMgr)))
	authAPI.HandleFunc("/api/admin/status-alerts", authMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			HandleGetStatusAlerts()(w, r)
		case http.MethodPost:
			HandleCreateStatusAlert()(w, r)
		case http.MethodDelete:
			HandleDeleteStatusAlert()(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Service management routes (admin only)
	authAPI.HandleFunc("/api/admin/services", authMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// Add admin query param
			r.URL.RawQuery = "admin=true"
			HandleGetServices(w, r)
		case http.MethodPost:
			HandleCreateService(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	// Register specific routes before the catch-all pattern
	authAPI.HandleFunc("/api/admin/services/reorder", authMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			HandleReorderServices(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	authAPI.HandleFunc("/api/admin/services/test", func(w http.ResponseWriter, r *http.Request) {
		// Allow test connection during setup (no auth required)
		complete, _ := database.IsSetupComplete()
		if !complete {
			if r.Method == http.MethodPost {
				HandleTestServiceConnection(w, r)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		// Normal auth-required path
		authMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				HandleTestServiceConnection(w, r)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})(w, r)
	})
	authAPI.HandleFunc("/api/admin/services/", authMgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		// Handle /api/admin/services/{id} and /api/admin/services/{id}/visibility
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/services/")
		parts := strings.Split(path, "/")

		if len(parts) >= 1 && parts[0] != "" {
			// Store ID in query params for handlers to access
			q := r.URL.Query()
			q.Set("_id", parts[0])
			r.URL.RawQuery = q.Encode()
		}

		if len(parts) == 1 && parts[0] != "" && parts[0] != "reorder" && parts[0] != "test" {
			// /api/admin/services/{id}
			switch r.Method {
			case http.MethodPut:
				HandleUpdateService(w, r)
			case http.MethodDelete:
				HandleDeleteService(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		} else if len(parts) == 2 && parts[1] == "visibility" {
			// /api/admin/services/{id}/visibility
			if r.Method == http.MethodPut {
				HandleToggleServiceVisibility(w, r)
			} else {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	// Auth routes
	authAPI.HandleFunc("/api/me", HandleWhoAmI(authMgr))
	authAPI.HandleFunc("/api/login", HandleLogin(authMgr))
	authAPI.HandleFunc("/api/logout", HandleLogout(authMgr))

	// Main router
	mux := http.NewServeMux()
	
	// Setup routes (must be accessible before setup is complete)
	mux.HandleFunc("/setup", HandleSetupPage)
	mux.HandleFunc("/api/setup", HandleCompleteSetup(authMgr))
	mux.HandleFunc("/api/setup/status", HandleSetupStatus)
	mux.HandleFunc("/api/setup/service", HandleAddFirstService)
	
	mux.Handle("/api/admin/", security.RateLimit(authAPI))
	mux.Handle("/api/login", security.RateLimit(http.HandlerFunc(HandleLogin(authMgr))))
	mux.Handle("/api/logout", security.RateLimit(http.HandlerFunc(HandleLogout(authMgr))))
	mux.Handle("/api/me", security.RateLimit(http.HandlerFunc(HandleWhoAmI(authMgr))))
	mux.HandleFunc("/api/status-alerts", HandleGetStatusAlerts()) // Public, no rate limit
	mux.Handle("/api/", security.RateLimit(api))
	mux.HandleFunc("/static/", HandleStatic())
	mux.HandleFunc("/favicon.ico", HandleFavicon())
	mux.Handle("/", security.CheckIPBlock(http.HandlerFunc(HandleIndex(authMgr))))

	// Wrap everything with setup check middleware
	return SetupRequiredMiddleware(mux)
}
