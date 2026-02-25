package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"status/app/internal/auth"
	"status/app/internal/database"
	"status/app/internal/models"

	"golang.org/x/crypto/bcrypt"
)

// SetupState tracks setup wizard progress
type SetupState struct {
	NeedsSetup  bool `json:"needs_setup"`
	HasServices bool `json:"has_services"`
}

// HandleSetupPage serves the setup wizard page
func HandleSetupPage(w http.ResponseWriter, r *http.Request) {
	// Check if setup is already complete
	complete, err := database.IsSetupComplete()
	if err == nil && complete {
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
	complete, _ := database.IsSetupComplete()
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
		complete, _ := database.IsSetupComplete()
		if complete {
			w.Header().Set("Content-Type", "application/json")
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

		if req.Password == "" || len(req.Password) < 8 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   "Password must be at least 8 characters",
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
		authMgr.Reload(req.Username, passwordHash, authSecretBytes)
		log.Printf("Setup complete - auth credentials loaded for user: %s", req.Username)

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
	complete, _ := database.IsSetupComplete()
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
		complete, _ := database.IsSetupComplete()
		if complete {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Setup already complete"})
			return
		}

		// Parse multipart form
		if err := r.ParseMultipartForm(64 << 20); err != nil { // 64MB max
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid form data"})
			return
		}

		file, _, err := r.FormFile("backup")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "No backup file provided"})
			return
		}
		defer file.Close()

		// Read and parse JSON
		data, err := io.ReadAll(file)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read file"})
			return
		}

		var export DatabaseExport
		if err := json.Unmarshal(data, &export); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid backup format"})
			return
		}

		// Validate export
		if export.Version == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid backup file: missing version"})
			return
		}

		// Import services
		if len(export.Services) > 0 {
			_, _ = database.DB.Exec(`DELETE FROM services`)
			_, _ = database.DB.Exec(`DELETE FROM service_state`)
			_, _ = database.DB.Exec(`DELETE FROM service_status_history`)
			_, _ = database.DB.Exec(`DELETE FROM stat_minutely`)
			_, _ = database.DB.Exec(`DELETE FROM stat_hourly`)
			_, _ = database.DB.Exec(`DELETE FROM stat_daily`)
			_, _ = database.DB.Exec(`DELETE FROM heartbeats`)
			for _, s := range export.Services {
				svc := &models.ServiceConfig{
					Key:           s.Key,
					Name:          s.Name,
					URL:           s.URL,
					ServiceType:   s.ServiceType,
					Icon:          s.Icon,
					IconURL:       s.IconURL,
					DisplayOrder:  s.DisplayOrder,
					Visible:       s.Visible,
					CheckType:     s.CheckType,
					CheckInterval: s.CheckInterval,
					Timeout:       s.Timeout,
					ExpectedMin:   s.ExpectedMin,
					ExpectedMax:   s.ExpectedMax,
				}
				_, _ = database.CreateService(svc)
			}
		}

		// Import alert config
		if export.AlertConfig != nil {
			alertCfg := &models.AlertConfig{
				Enabled:         export.AlertConfig.Enabled,
				SMTPHost:        export.AlertConfig.SMTPHost,
				SMTPPort:        export.AlertConfig.SMTPPort,
				SMTPUser:        export.AlertConfig.SMTPUser,
				AlertEmail:      export.AlertConfig.AlertEmail,
				FromEmail:       export.AlertConfig.FromEmail,
				StatusPageURL:   export.AlertConfig.StatusPageURL,
				SMTPSkipVerify:  export.AlertConfig.SMTPSkipVerify,
				AlertOnDown:     export.AlertConfig.AlertOnDown,
				AlertOnDegraded: export.AlertConfig.AlertOnDegraded,
				AlertOnUp:       export.AlertConfig.AlertOnUp,
			}
			_ = database.SaveAlertConfig(alertCfg)
		}

		// Import resources config
		if export.Resources != nil {
			resCfg := &models.ResourcesUIConfig{
				Enabled:    export.Resources.Enabled,
				GlancesURL: export.Resources.GlancesURL,
				CPU:        export.Resources.CPU,
				Memory:     export.Resources.Memory,
				Network:    export.Resources.Network,
				Temp:       export.Resources.Temp,
				Storage:    export.Resources.Storage,
				Swap:       export.Resources.Swap,
				Load:       export.Resources.Load,
				GPU:        export.Resources.GPU,
				Containers: export.Resources.Containers,
				Processes:  export.Resources.Processes,
				Uptime:     export.Resources.Uptime,
			}
			_ = database.SaveResourcesUIConfig(resCfg)
		}

		// Import samples
		if len(export.Samples) > 0 {
			_, _ = database.DB.Exec(`DELETE FROM samples`)
			for _, s := range export.Samples {
				ok := 0
				if s.OK {
					ok = 1
				}
				_, _ = database.DB.Exec(`INSERT INTO samples (taken_at, service_key, ok, http_status, latency_ms) VALUES (?, ?, ?, ?, ?)`,
					s.TakenAt, s.ServiceKey, ok, s.HTTPStatus, s.LatencyMS)
			}
		}

		// Now we need credentials - prompt user to create them
		// But for import, we'll require them in a separate step or use the backup username
		// For now, redirect to main setup to create credentials
		// Mark as NOT complete so user still needs to create credentials

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":           true,
			"services_imported": len(export.Services),
			"needs_credentials": true,
			"message":           "Backup restored. Please create admin credentials.",
		})
	}
}

// SetupRequiredMiddleware redirects to setup if not configured
func SetupRequiredMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow setup routes through
		if r.URL.Path == "/setup" || r.URL.Path == "/api/setup" ||
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
		complete, _ := database.IsSetupComplete()

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
