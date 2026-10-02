package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"status/app/internal/auth"
	"status/app/internal/crypto"
	"status/app/internal/database"
	"status/app/internal/models"

	"golang.org/x/crypto/bcrypt"
)

// SetupState tracks setup wizard progress
type SetupState struct {
	NeedsSetup  bool `json:"needs_setup"`
	HasServices bool `json:"has_services"`
}

// Serialize unauthenticated setup mutations with completion so an in-flight
// setup request cannot overwrite configuration after credentials are created.
var setupMu sync.RWMutex

// HandleSetupPage serves the setup wizard page
func HandleSetupPage(w http.ResponseWriter, r *http.Request) {
	// Check if setup is already complete
	complete, err := database.IsSetupComplete()
	if err != nil {
		http.Error(w, "setup state unavailable", http.StatusServiceUnavailable)
		return
	}
	if complete {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	tmplPath := filepath.Join("web", "templates", "setup.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		log.Printf("Setup template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, nil); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// HandleSetupStatus returns the current setup state
func HandleSetupStatus(w http.ResponseWriter, r *http.Request) {
	complete, err := database.IsSetupComplete()
	if err != nil {
		http.Error(w, "setup state unavailable", http.StatusServiceUnavailable)
		return
	}
	serviceCount, _ := database.GetServiceCount()

	state := SetupState{
		NeedsSetup:  !complete,
		HasServices: serviceCount > 0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// HandleCompleteSetup processes the setup form submission
func HandleCompleteSetup(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check if setup is already complete
		setupMu.Lock()
		defer setupMu.Unlock()
		complete, err := database.IsSetupComplete()
		if err != nil {
			http.Error(w, "setup state unavailable", http.StatusServiceUnavailable)
			return
		}
		if complete {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Setup already completed",
			})
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Invalid request body",
			})
			return
		}

		// Validate inputs
		if req.Username == "" || len(req.Username) < 3 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Username must be at least 3 characters",
			})
			return
		}

		if len(req.Password) < 8 || len(req.Password) > 72 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Password must be between 8 and 72 bytes",
			})
			return
		}

		// Hash password with bcrypt
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Failed to hash password",
			})
			return
		}

		// Generate a random auth secret (32 bytes = 256 bits)
		authSecretBytes := make([]byte, 32)
		if _, err := rand.Read(authSecretBytes); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Failed to generate auth secret",
			})
			return
		}
		authSecret := base64.StdEncoding.EncodeToString(authSecretBytes)

		// Save settings to database
		settings := &models.AppSettings{
			SetupComplete: true,
			Username:      req.Username,
			PasswordHash:  string(passwordHash),
			AuthSecret:    authSecret,
		}

		if err := database.SaveAppSettings(settings); err != nil {
			log.Printf("Setup: failed to save settings: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Failed to save settings",
			})
			return
		}

		// Reload auth manager with new credentials
		authMgr.Reload(req.Username, passwordHash, []byte(authSecret))
		log.Printf("Setup complete - auth credentials loaded for user: %s", req.Username)

		// Initialize encryption key for API token encryption at rest
		crypto.SetKey([]byte(authSecret))

		// Check if we need to create a dummy service
		serviceCount, _ := database.GetServiceCount()
		if serviceCount == 0 {
			createDummyService()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "Setup completed successfully",
		})
	}
}

// createDummyService creates a demo service for first-time users
func createDummyService() {
	dummyService := &models.ServiceConfig{
		Key:           "demo-service",
		Name:          "Demo Service",
		URL:           "http://localhost",
		ServiceType:   "custom",
		Icon:          "custom",
		DisplayOrder:  -1,
		Visible:       true,
		CheckType:     "always_up",
		CheckInterval: 60,
		Timeout:       5,
		ExpectedMin:   200,
		ExpectedMax:   299,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	database.CreateService(dummyService)
}

// HandleAddFirstService adds a service during setup
func HandleAddFirstService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Only allow during initial setup
	setupMu.Lock()
	defer setupMu.Unlock()
	complete, err := database.IsSetupComplete()
	if err != nil {
		http.Error(w, "setup state unavailable", http.StatusServiceUnavailable)
		return
	}
	if complete {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "Setup already completed",
		})
		return
	}

	var req struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		ServiceType string `json:"service_type"`
		APIToken    string `json:"api_token"`
		IconURL     string `json:"icon_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	if req.Name == "" || req.URL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "Name and URL are required",
		})
		return
	}

	// Generate key from name
	key := generateServiceKey(req.Name)

	service := &models.ServiceConfig{
		Key:           key,
		Name:          req.Name,
		URL:           req.URL,
		ServiceType:   req.ServiceType,
		Icon:          req.ServiceType,
		IconURL:       req.IconURL,
		APIToken:      req.APIToken,
		DisplayOrder:  -1,
		Visible:       true,
		CheckType:     "http",
		CheckInterval: 60,
		Timeout:       5,
		ExpectedMin:   200,
		ExpectedMax:   399,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	if _, err := database.CreateService(service); err != nil {
		log.Printf("Setup: failed to create service: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "Failed to create service",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Service added successfully",
	})
}

// HandleSetupImport handles importing a backup during setup
func HandleSetupImport(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Only allow import if setup is not complete
		setupMu.Lock()
		defer setupMu.Unlock()
		complete, err := database.IsSetupComplete()
		if err != nil {
			http.Error(w, "setup state unavailable", http.StatusServiceUnavailable)
			return
		}
		if complete {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Setup already complete"})
			return
		}

		export, err := readBackupUpload(r)
		if err != nil {
			writeBackupError(w, err)
			return
		}
		result, err := importBackup(export)
		if err != nil {
			log.Printf("Setup backup import failed: %v", err)
			writeBackupError(w, err)
			return
		}
		logBackupImport(*export, result)

		// Now we need credentials - prompt user to create them
		// But for import, we'll require them in a separate step or use the backup username
		// For now, redirect to main setup to create credentials
		// Mark as NOT complete so user still needs to create credentials

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":           true,
			"services_imported": result.Services,
			"needs_credentials": true,
			"message":           "Backup restored. Please create admin credentials.",
		})
	}
}

// SetupRequiredMiddleware redirects to setup if not configured
func SetupRequiredMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow setup routes through
		if r.URL.Path == "/healthz" || r.URL.Path == "/setup" || r.URL.Path == "/api/setup" ||
			r.URL.Path == "/api/setup/status" || r.URL.Path == "/api/setup/service" ||
			r.URL.Path == "/api/setup/import" ||
			r.URL.Path == "/api/admin/services/test" { // Allow test connection during setup
			next.ServeHTTP(w, r)
			return
		}

		// Allow static files
		if len(r.URL.Path) > 8 && r.URL.Path[:8] == "/static/" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if setup is complete - ONLY check the database flag
		// Don't consider existing services as setup complete (they could be migrated from env)
		complete, err := database.IsSetupComplete()
		if err != nil {
			http.Error(w, "setup state unavailable", http.StatusServiceUnavailable)
			return
		}

		if complete {
			next.ServeHTTP(w, r)
			return
		}

		// Redirect to setup page
		if r.URL.Path != "/" && r.Header.Get("Accept") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"error":          "Setup required",
				"setup_required": true,
			})
			return
		}
		http.Redirect(w, r, "/setup", http.StatusFound)
	})
}
