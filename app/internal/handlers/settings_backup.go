package handlers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"status/app/internal/auth"
	"status/app/internal/buildinfo"
	"status/app/internal/database"
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
	CrowdSec             *exportCrowdSecConfig        `json:"crowdsec_config,omitempty"`
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
	Enabled                 bool   `json:"enabled"`
	SMTPHost                string `json:"smtp_host"`
	SMTPPort                int    `json:"smtp_port"`
	SMTPUser                string `json:"smtp_user"`
	AlertEmail              string `json:"alert_email"`
	FromEmail               string `json:"from_email"`
	StatusPageURL           string `json:"status_page_url"`
	SMTPSkipVerify          bool   `json:"smtp_skip_verify"`
	AlertOnDown             bool   `json:"alert_on_down"`
	AlertOnDegraded         bool   `json:"alert_on_degraded"`
	AlertOnUp               bool   `json:"alert_on_up"`
	AlertOnDegradedRecovery bool   `json:"alert_on_degraded_recovery"`
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

type exportCrowdSecConfig struct {
	Enabled       bool    `json:"enabled"`
	LAPIURL       string  `json:"lapi_url"`
	MachineID     string  `json:"machine_id"`
	PollIntervalS int     `json:"poll_interval_seconds"`
	TLSSkipVerify bool    `json:"tls_skip_verify"`
	MapHomeLat    float64 `json:"map_home_latitude"`
	MapHomeLng    float64 `json:"map_home_longitude"`
	// Secrets are NOT exported for security; re-enter after import.
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
				Enabled:                 alertCfg.Enabled,
				SMTPHost:                alertCfg.SMTPHost,
				SMTPPort:                alertCfg.SMTPPort,
				SMTPUser:                alertCfg.SMTPUser,
				AlertEmail:              alertCfg.AlertEmail,
				FromEmail:               alertCfg.FromEmail,
				StatusPageURL:           alertCfg.StatusPageURL,
				SMTPSkipVerify:          alertCfg.SMTPSkipVerify,
				AlertOnDown:             alertCfg.AlertOnDown,
				AlertOnDegraded:         alertCfg.AlertOnDegraded,
				AlertOnUp:               alertCfg.AlertOnUp,
				AlertOnDegradedRecovery: alertCfg.AlertOnDegradedRecovery,
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

		// Export CrowdSec config (without secrets)
		if csCfg, err := database.LoadCrowdSecConfig(); err == nil && csCfg != nil {
			export.CrowdSec = &exportCrowdSecConfig{
				Enabled:       csCfg.Enabled,
				LAPIURL:       csCfg.LAPIURL,
				MachineID:     csCfg.MachineID,
				PollIntervalS: csCfg.PollIntervalS,
				TLSSkipVerify: csCfg.TLSSkipVerify,
				MapHomeLat:    csCfg.MapHomeLat,
				MapHomeLng:    csCfg.MapHomeLng,
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

// HandleImportDatabase imports a database backup atomically.
func HandleImportDatabase() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		export, err := readBackupUpload(r)
		if err != nil {
			writeBackupError(w, err)
			return
		}
		result, err := importBackup(export)
		if err != nil {
			log.Printf("Backup import failed: %v", err)
			writeBackupError(w, err)
			return
		}
		logBackupImport(*export, result)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":           true,
			"services_imported": result.Services,
			"samples_imported":  result.Samples,
			"samples_skipped":   result.SkippedSamples,
		})
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
			"crowdsec_config",
			"crowdsec_state",
			"crowdsec_decisions",
			"crowdsec_alerts",
			"crowdsec_history_state",
			"app_metadata",
			"software_deployments",
		}

		if err := deleteAllRows(tables); err != nil {
			log.Printf("Database reset failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Reset failed; no changes were made"})
			return
		}
		if err := database.EnsureSchema(); err != nil {
			log.Printf("Warning: schema re-initialisation after reset failed: %v", err)
		}
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

func logBackupImport(export DatabaseExport, result backupImportResult) {
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
	details += fmt.Sprintf(", services=%d, samples=%d, skipped_samples=%d, schedules=%d, skipped_schedules=%d",
		result.Services, result.Samples, result.SkippedSamples, result.Schedules, result.SkippedSchedules)
	_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "", "Database backup imported", details)
}

// deleteAllRows empties the given tables in one transaction so a reset never
// leaves the database half-cleared.
func deleteAllRows(tables []string) error {
	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op once committed
	for _, table := range tables {
		// #nosec G202 -- table names are fixed literals
		if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	return tx.Commit()
}
