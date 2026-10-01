package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
)

// maxBackupBytes bounds an uploaded backup file. A year of history for a
// typical install is well below this; the global request cap is slightly larger.
const maxBackupBytes = 64 << 20

// backupImportResult summarises what a restore wrote.
type backupImportResult struct {
	Services         int
	Samples          int
	SkippedSamples   int
	Schedules        int
	SkippedSchedules int
}

// backupUploadError is a validation failure that is safe to show to the client.
type backupUploadError struct {
	status  int
	message string
}

func (e *backupUploadError) Error() string { return e.message }

// readBackupUpload extracts the "backup" multipart file and validates the export envelope.
func readBackupUpload(r *http.Request) (*DatabaseExport, error) {
	if err := r.ParseMultipartForm(maxBackupBytes); err != nil {
		return nil, &backupUploadError{http.StatusBadRequest, "Invalid form data"}
	}
	defer r.MultipartForm.RemoveAll()

	file, _, err := r.FormFile("backup")
	if err != nil {
		return nil, &backupUploadError{http.StatusBadRequest, "No backup file provided"}
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxBackupBytes+1))
	if err != nil {
		return nil, &backupUploadError{http.StatusBadRequest, "Failed to read file"}
	}
	if len(data) > maxBackupBytes {
		return nil, &backupUploadError{http.StatusRequestEntityTooLarge, "Backup file is too large"}
	}

	var export DatabaseExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, &backupUploadError{http.StatusBadRequest, "Invalid backup format"}
	}
	if export.Version == "" {
		return nil, &backupUploadError{http.StatusBadRequest, "Invalid backup file: missing version"}
	}
	if export.DatabaseSchema > database.SchemaVersion {
		return nil, &backupUploadError{http.StatusBadRequest, "Backup requires a newer Servicarr database schema"}
	}
	return &export, nil
}

// writeBackupError reports a failed upload or restore as JSON.
func writeBackupError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "Import failed; no changes were made"
	var uploadErr *backupUploadError
	if errors.As(err, &uploadErr) {
		status, message = uploadErr.status, uploadErr.message
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// importBackup restores a portable backup in a single transaction: either
// every section is applied or the database is left exactly as it was.
// Everything inside must go through tx; the pool has one connection, so a
// call that uses database.DB directly would block until the import finished.
func importBackup(export *DatabaseExport) (backupImportResult, error) {
	var result backupImportResult
	tx, err := database.DB.Begin()
	if err != nil {
		return result, fmt.Errorf("begin import: %w", err)
	}
	defer tx.Rollback() // no-op once committed

	if len(export.Services) > 0 {
		for _, table := range []string{
			"services", "service_state", "service_status_history", "service_outage_state",
			"stat_minutely", "stat_hourly", "stat_daily", "heartbeats",
		} {
			// #nosec G202 -- table names are fixed literals
			if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
				return result, fmt.Errorf("clear %s: %w", table, err)
			}
		}
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
			if _, err := database.CreateServiceWith(tx, svc); err != nil {
				return result, fmt.Errorf("import service %q: %w", s.Key, err)
			}
			result.Services++
		}
	}

	if export.AlertConfig != nil {
		alertCfg := &models.AlertConfig{
			Enabled:                 export.AlertConfig.Enabled,
			SMTPHost:                export.AlertConfig.SMTPHost,
			SMTPPort:                export.AlertConfig.SMTPPort,
			SMTPUser:                export.AlertConfig.SMTPUser,
			AlertEmail:              export.AlertConfig.AlertEmail,
			FromEmail:               export.AlertConfig.FromEmail,
			StatusPageURL:           export.AlertConfig.StatusPageURL,
			SMTPSkipVerify:          export.AlertConfig.SMTPSkipVerify,
			AlertOnDown:             export.AlertConfig.AlertOnDown,
			AlertOnDegraded:         export.AlertConfig.AlertOnDegraded,
			AlertOnUp:               export.AlertConfig.AlertOnUp,
			AlertOnDegradedRecovery: export.AlertConfig.AlertOnDegradedRecovery,
		}
		if err := database.SaveAlertConfigWith(tx, alertCfg); err != nil {
			return result, fmt.Errorf("import alert config: %w", err)
		}
	}

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
		if err := database.SaveResourcesUIConfigWith(tx, resCfg); err != nil {
			return result, fmt.Errorf("import resources config: %w", err)
		}
	}

	// Secrets never travel in backups, so an imported CrowdSec config stays
	// disabled until credentials are re-entered.
	if export.CrowdSec != nil {
		csCfg := &models.CrowdSecConfig{
			Enabled:       false,
			LAPIURL:       export.CrowdSec.LAPIURL,
			MachineID:     export.CrowdSec.MachineID,
			PollIntervalS: export.CrowdSec.PollIntervalS,
			TLSSkipVerify: export.CrowdSec.TLSSkipVerify,
			MapHomeLat:    export.CrowdSec.MapHomeLat,
			MapHomeLng:    export.CrowdSec.MapHomeLng,
		}
		if csCfg.PollIntervalS < 10 {
			csCfg.PollIntervalS = 30
		}
		if err := database.SaveCrowdSecConfigWith(tx, csCfg); err != nil {
			return result, fmt.Errorf("import CrowdSec config: %w", err)
		}
	}

	if len(export.Samples) > 0 {
		if _, err := tx.Exec(`DELETE FROM samples`); err != nil {
			return result, fmt.Errorf("clear samples: %w", err)
		}
		stmt, err := tx.Prepare(`INSERT INTO samples (taken_at, service_key, ok, http_status, latency_ms) VALUES (?, ?, ?, ?, ?)`)
		if err != nil {
			return result, fmt.Errorf("prepare samples: %w", err)
		}
		defer stmt.Close()
		for _, s := range export.Samples {
			takenAt, valid := normalizeSampleTime(s.TakenAt)
			if !valid || strings.TrimSpace(s.ServiceKey) == "" {
				result.SkippedSamples++
				continue
			}
			ok := 0
			if s.OK {
				ok = 1
			}
			if _, err := stmt.Exec(takenAt, s.ServiceKey, ok, s.HTTPStatus, s.LatencyMS); err != nil {
				return result, fmt.Errorf("import samples: %w", err)
			}
			result.Samples++
		}
	}

	if export.MaintenanceSchedules != nil {
		if _, err := tx.Exec(`DELETE FROM maintenance_schedules`); err != nil {
			return result, fmt.Errorf("clear maintenance schedules: %w", err)
		}
		for i := range export.MaintenanceSchedules {
			schedule := &export.MaintenanceSchedules[i]
			if maintenance.ValidateSchedule(schedule) != nil {
				result.SkippedSchedules++
				continue
			}
			if err := database.SaveMaintenanceScheduleWith(tx, schedule); err != nil {
				return result, fmt.Errorf("import maintenance schedule %q: %w", schedule.ID, err)
			}
			result.Schedules++
		}
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit import: %w", err)
	}
	return result, nil
}

// normalizeSampleTime converts a backup timestamp to the UTC RFC3339 form the
// scheduler writes. Range queries compare taken_at as text, so mixed formats
// (offsets, fractional seconds) would silently fall outside their windows.
func normalizeSampleTime(raw string) (string, bool) {
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	return t.UTC().Format(time.RFC3339), true
}
