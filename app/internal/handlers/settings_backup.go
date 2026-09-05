package handlers

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"status/app/internal/auth"
	"status/app/internal/buildinfo"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// DatabaseExport represents the exported database structure
type DatabaseExport struct {
	Version              string                       `json:"version"`
	ApplicationVersion   string                       `json:"application_version,omitempty"`
	ApplicationCommit    string                       `json:"application_commit,omitempty"`
	DatabaseSchema       int                          `json:"database_schema,omitempty"`
	ExportedAt           string                       `json:"exported_at"`
	AppSettings          *exportAppSettings           `json:"app_settings"`
	Services             []exportService              `json:"services"`
	AlertConfig          *exportAlertConfig           `json:"alert_config"`
	Resources            *exportResourcesConfig       `json:"resources_config"`
	Samples              []exportSample               `json:"samples"`
	MaintenanceSchedules []models.MaintenanceSchedule `json:"maintenance_schedules"`
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
	NUTHost    string `json:"nut_host"`
	UPSName    string `json:"ups_name"`
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
	UPS        bool   `json:"ups"`
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

		build := buildinfo.Current()
		export := DatabaseExport{
			Version:            "1.0",
			ApplicationVersion: build.Version,
			ApplicationCommit:  build.Commit,
			DatabaseSchema:     database.SchemaVersion,
			ExportedAt:         time.Now().UTC().Format(time.RFC3339),
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
				NUTHost:    resCfg.NUTHost,
				UPSName:    resCfg.UPSName,
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
				UPS:        resCfg.UPS,
			}
		}

		// Export all samples
		rows, err := database.DB.Query(`
			SELECT taken_at, service_key, ok, COALESCE(http_status, 0), latency_ms 
			FROM samples 
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

		if schedules, err := database.GetMaintenanceSchedules(); err == nil {
			export.MaintenanceSchedules = schedules
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
		if err := r.ParseMultipartForm(64 << 20); err != nil { // 64MB max
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid form data"})
			return
		}
		defer r.MultipartForm.RemoveAll()

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
		if export.DatabaseSchema > database.SchemaVersion {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Backup requires a newer Servicarr database schema"})
			return
		}

		// Import services (clear existing first)
		if len(export.Services) > 0 {
			_, _ = database.DB.Exec(`DELETE FROM services`)
			_, _ = database.DB.Exec(`DELETE FROM service_state`)
			_, _ = database.DB.Exec(`DELETE FROM service_status_history`)
			_, _ = database.DB.Exec(`DELETE FROM service_outage_state`)
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
				NUTHost:    export.Resources.NUTHost,
				UPSName:    export.Resources.UPSName,
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
				UPS:        export.Resources.UPS,
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

		if export.MaintenanceSchedules != nil {
			_, _ = database.DB.Exec(`DELETE FROM maintenance_schedules`)
			for i := range export.MaintenanceSchedules {
				if maintenance.ValidateSchedule(&export.MaintenanceSchedules[i]) == nil {
					_ = database.SaveMaintenanceSchedule(&export.MaintenanceSchedules[i])
				}
			}
		}
		logBackupImport(export)

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
			"status_alert_overrides",
			"service_status_history",
			"service_outage_state",
			"app_settings",
			"stat_minutely",
			"stat_hourly",
			"stat_daily",
			"heartbeats",
			"system_logs",
			"maintenance_schedules",
			"maintenance_windows",
			"incident_events",
			"ups_monitor_state",
			"app_metadata",
			"software_deployments",
		}

		for _, table := range tables {
			_, _ = database.DB.Exec(`DELETE FROM ` + table)
		}
		_ = database.EnsureSchema()
		_ = database.RecordSoftwareDeployment(buildinfo.Current())

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

func logBackupImport(export DatabaseExport) {
	sourceVersion := export.ApplicationVersion
	if sourceVersion == "" {
		sourceVersion = "unknown"
	}
	details := "backup_format=" + export.Version + ", source_version=" + sourceVersion
	if export.DatabaseSchema > 0 {
		details += ", source_database_schema=" + strconv.Itoa(export.DatabaseSchema)
	}
	if export.ApplicationCommit != "" && export.ApplicationCommit != "unknown" {
		details += ", source_commit=" + export.ApplicationCommit
	}
	_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "", "Database backup imported", details)
}
