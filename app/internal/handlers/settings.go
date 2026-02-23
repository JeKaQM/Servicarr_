package handlers

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"status/app/internal/auth"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HandleUpdateAppName handles app name change requests
func HandleUpdateAppName() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			AppName string `json:"app_name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request"})
			return
		}

		appName := strings.TrimSpace(req.AppName)
		if appName == "" {
			appName = "Service Status"
		}
		if len(appName) > 50 {
			appName = appName[:50]
		}

		// Load current settings
		settings, err := database.LoadAppSettings()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load settings"})
			return
		}

		// Update app name
		settings.AppName = appName
		if err := database.SaveAppSettings(settings); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save app name"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "app_name": appName})
	}
}

// HandleChangePassword handles password change requests
func HandleChangePassword(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		if req.CurrentPassword == "" || req.NewPassword == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Both current and new password are required"})
			return
		}

		if len(req.NewPassword) < 8 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "New password must be at least 8 characters"})
			return
		}

		// Load current settings
		settings, err := database.LoadAppSettings()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load settings"})
			return
		}

		// Verify current password
		if err := bcrypt.CompareHashAndPassword([]byte(settings.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Current password is incorrect"})
			return
		}

		// Hash new password
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to hash password"})
			return
		}

		// Update password in database
		settings.PasswordHash = string(newHash)
		if err := database.SaveAppSettings(settings); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save new password"})
			return
		}

		// Reload auth manager with new credentials
		authMgr.Reload(settings.Username, []byte(settings.PasswordHash), []byte(settings.AuthSecret))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}

// DatabaseExport represents the exported database structure
type DatabaseExport struct {
	Version     string                 `json:"version"`
	ExportedAt  string                 `json:"exported_at"`
	AppSettings *exportAppSettings     `json:"app_settings"`
	Services    []exportService        `json:"services"`
	AlertConfig *exportAlertConfig     `json:"alert_config"`
	Resources   *exportResourcesConfig `json:"resources_config"`
	Samples     []exportSample         `json:"samples"`
}

type exportService struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	ServiceType   string `json:"service_type"`
	Icon          string `json:"icon"`
	IconURL       string `json:"icon_url"`
	DisplayOrder  int    `json:"display_order"`
	Visible       bool   `json:"visible"`
	CheckType     string `json:"check_type"`
	CheckInterval int    `json:"check_interval"`
	Timeout       int    `json:"timeout"`
	ExpectedMin   int    `json:"expected_min"`
	ExpectedMax   int    `json:"expected_max"`
}

type exportAppSettings struct {
	Username string `json:"username"`
	// Password hash is NOT exported for security
}

type exportAlertConfig struct {
	Enabled         bool   `json:"enabled"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPUser        string `json:"smtp_user"`
	AlertEmail      string `json:"alert_email"`
	FromEmail       string `json:"from_email"`
	StatusPageURL   string `json:"status_page_url"`
	SMTPSkipVerify  bool   `json:"smtp_skip_verify"`
	AlertOnDown     bool   `json:"alert_on_down"`
	AlertOnDegraded bool   `json:"alert_on_degraded"`
	AlertOnUp       bool   `json:"alert_on_up"`
	// SMTP password is NOT exported for security
}

type exportResourcesConfig struct {
	Enabled    bool   `json:"enabled"`
	GlancesURL string `json:"glances_url"`
	CPU        bool   `json:"cpu"`
	Memory     bool   `json:"memory"`
	Network    bool   `json:"network"`
	Temp       bool   `json:"temp"`
	Storage    bool   `json:"storage"`
	Swap       bool   `json:"swap"`
	Load       bool   `json:"load"`
	GPU        bool   `json:"gpu"`
	Containers bool   `json:"containers"`
	Processes  bool   `json:"processes"`
	Uptime     bool   `json:"uptime"`
}

type exportSample struct {
	TakenAt    string `json:"taken_at"`
	ServiceKey string `json:"service_key"`
	OK         bool   `json:"ok"`
	HTTPStatus int    `json:"http_status"`
	LatencyMS  *int   `json:"latency_ms"`
}

// HandleExportDatabase exports the database as JSON
func HandleExportDatabase() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		export := DatabaseExport{
			Version:    "1.0",
			ExportedAt: time.Now().UTC().Format(time.RFC3339),
		}

		// Export app settings (without sensitive data)
		if settings, err := database.LoadAppSettings(); err == nil && settings != nil {
			export.AppSettings = &exportAppSettings{
				Username: settings.Username,
			}
		}

		// Export services
		if services, err := database.GetAllServices(); err == nil {
			export.Services = make([]exportService, 0, len(services))
			for _, s := range services {
				export.Services = append(export.Services, exportService{
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
				})
			}
		}

		// Export alert config (without SMTP password)
		if alertCfg, err := database.LoadAlertConfig(); err == nil && alertCfg != nil {
			export.AlertConfig = &exportAlertConfig{
				Enabled:         alertCfg.Enabled,
				SMTPHost:        alertCfg.SMTPHost,
				SMTPPort:        alertCfg.SMTPPort,
				SMTPUser:        alertCfg.SMTPUser,
				AlertEmail:      alertCfg.AlertEmail,
				FromEmail:       alertCfg.FromEmail,
				StatusPageURL:   alertCfg.StatusPageURL,
				SMTPSkipVerify:  alertCfg.SMTPSkipVerify,
				AlertOnDown:     alertCfg.AlertOnDown,
				AlertOnDegraded: alertCfg.AlertOnDegraded,
				AlertOnUp:       alertCfg.AlertOnUp,
			}
		}

		// Export resources config
		if resCfg, err := database.LoadResourcesUIConfig(); err == nil && resCfg != nil {
			export.Resources = &exportResourcesConfig{
				Enabled:    resCfg.Enabled,
				GlancesURL: resCfg.GlancesURL,
				CPU:        resCfg.CPU,
				Memory:     resCfg.Memory,
				Network:    resCfg.Network,
				Temp:       resCfg.Temp,
				Storage:    resCfg.Storage,
				Swap:       resCfg.Swap,
				Load:       resCfg.Load,
				GPU:        resCfg.GPU,
				Containers: resCfg.Containers,
				Processes:  resCfg.Processes,
				Uptime:     resCfg.Uptime,
			}
		}

		// Export recent samples (last 7 days)
		rows, err := database.DB.Query(`
			SELECT taken_at, service_key, ok, COALESCE(http_status, 0), latency_ms 
			FROM samples 
			WHERE taken_at >= datetime('now', '-7 days')
			ORDER BY taken_at DESC`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sample exportSample
				var ok int
				var latencyMS *int
				if err := rows.Scan(&sample.TakenAt, &sample.ServiceKey, &ok, &sample.HTTPStatus, &latencyMS); err == nil {
					sample.OK = ok == 1
					sample.LatencyMS = latencyMS
					export.Samples = append(export.Samples, sample)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=servicarr-backup.json")
		_ = json.NewEncoder(w).Encode(export)
	}
}

// HandleImportDatabase imports a database backup
func HandleImportDatabase() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse multipart form
		if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid form data"})
			return
		}

		file, _, err := r.FormFile("backup")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "No backup file provided"})
			return
		}
		defer file.Close()

		// Read and parse JSON
		data, err := io.ReadAll(file)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read file"})
			return
		}

		var export DatabaseExport
		if err := json.Unmarshal(data, &export); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid backup format"})
			return
		}

		// Validate export
		if export.Version == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid backup file: missing version"})
			return
		}

		// Import services (clear existing first)
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
			_, _ = database.DB.Exec(`DELETE FROM samples WHERE taken_at >= datetime('now', '-7 days')`)
			for _, s := range export.Samples {
				ok := 0
				if s.OK {
					ok = 1
				}
				_, _ = database.DB.Exec(`INSERT INTO samples (taken_at, service_key, ok, http_status, latency_ms) VALUES (?, ?, ?, ?, ?)`,
					s.TakenAt, s.ServiceKey, ok, s.HTTPStatus, s.LatencyMS)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "services_imported": len(export.Services)})
	}
}

// HandleResetDatabase resets the database to initial state
func HandleResetDatabase(authMgr *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Password string `json:"password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request"})
			return
		}

		if req.Password == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Password is required"})
			return
		}

		// Verify password
		settings, err := database.LoadAppSettings()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to verify password"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(settings.PasswordHash), []byte(req.Password)); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Incorrect password"})
			return
		}

		// Delete all data
		tables := []string{
			"services",
			"samples",
			"ip_blocks",
			"ip_whitelist",
			"ip_blacklist",
			"service_state",
			"alert_config",
			"resources_ui_config",
			"status_alerts",
			"service_status_history",
			"app_settings",
			"stat_minutely",
			"stat_hourly",
			"stat_daily",
			"heartbeats",
			"system_logs",
		}

		for _, table := range tables {
			_, _ = database.DB.Exec(`DELETE FROM ` + table)
		}

		// Generate a fresh random temporary secret
		tempSecret := make([]byte, 32)
		if _, err := rand.Read(tempSecret); err != nil {
			tempSecret = []byte("reset-fallback")
		}

		// Reset auth manager
		authMgr.Reload("", []byte{}, tempSecret)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Database reset complete"})
	}
}
