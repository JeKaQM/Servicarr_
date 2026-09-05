package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"status/app/internal/resources"
	"strings"
	"time"
)

const serviceRecoveryBannerDuration = 24 * time.Hour

// HandleGetStatusAlerts returns manual and currently generated status alerts.
func HandleGetStatusAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alerts, err := getAdminStatusAlerts(time.Now())
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(alerts)
	}
}

// HandleGetPublicStatusAlerts includes active scheduled maintenance banners.
func HandleGetPublicStatusAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alerts, err := getPublicStatusAlerts(time.Now())
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(alerts)
	}
}

func getPublicStatusAlerts(now time.Time) ([]models.StatusAlert, error) {
	return getEffectiveStatusAlerts(now, false)
}

func getAdminStatusAlerts(now time.Time) ([]models.StatusAlert, error) {
	return getEffectiveStatusAlerts(now, true)
}

func getEffectiveStatusAlerts(now time.Time, includeHidden bool) ([]models.StatusAlert, error) {
	manual, err := getStatusAlerts()
	if err != nil {
		return nil, err
	}
	windows, _ := maintenance.Current(now)
	alerts := make([]models.StatusAlert, 0, len(windows)+len(manual)+2)
	monitoringSuppressed := false
	for _, window := range windows {
		endsAt := ""
		if !window.EndsAt.IsZero() {
			endsAt = window.EndsAt.UTC().Format(time.RFC3339)
		}
		if window.Schedule.SuppressMonitoring {
			monitoringSuppressed = true
		}
		alerts = append(alerts, models.StatusAlert{
			ID:        "scheduled:" + window.Schedule.ID,
			Message:   window.Schedule.Message,
			Level:     window.Schedule.Level,
			CreatedAt: window.StartsAt.UTC().Format(time.RFC3339),
			Scheduled: true,
			EndsAt:    endsAt,
			Source:    "scheduled",
			Editable:  true,
		})
	}
	if !monitoringSuppressed {
		automatic, err := getAutomaticServiceStatusAlert(now)
		if err != nil {
			return nil, err
		}
		if automatic != nil {
			alerts = append(alerts, *automatic)
		}
	}
	upsAlert, err := getAutomaticUPSStatusAlert()
	if err != nil {
		return nil, err
	}
	if upsAlert != nil {
		alerts = append(alerts, *upsAlert)
	}
	alerts = append(alerts, manual...)
	return applyStatusAlertOverrides(alerts, includeHidden)
}

func getAutomaticServiceStatusAlert(now time.Time) (*models.StatusAlert, error) {
	states, err := database.GetVisibleServiceOutageStates()
	if err != nil {
		return nil, err
	}
	now = now.UTC()

	down := make([]database.ServiceOutageState, 0)
	alertSent := false
	var outageStarted time.Time
	for _, state := range states {
		if !state.IsDown {
			continue
		}
		down = append(down, state)
		alertSent = alertSent || state.AlertSent
		started := state.UpdatedAt
		if state.DownSince != nil {
			started = *state.DownSince
		}
		if outageStarted.IsZero() || started.Before(outageStarted) {
			outageStarted = started
		}
	}

	if len(down) > 0 {
		message := ""
		if len(down) == 1 {
			message = fmt.Sprintf("Critical outage: %s is currently unavailable.", down[0].ServiceName)
		} else {
			message = fmt.Sprintf("Critical outage: %d services are currently unavailable.", len(down))
		}
		if alertSent {
			message += " An alert has been sent and the outage is being investigated."
		} else {
			message += " The outage has been detected and is being investigated."
		}
		return &models.StatusAlert{
			ID:        "automatic:critical-outage",
			Message:   message,
			Level:     "error",
			CreatedAt: outageStarted.UTC().Format(time.RFC3339),
			Automatic: true,
			Kind:      "critical_outage",
			Source:    "automatic",
			Editable:  true,
		}, nil
	}

	var latestRestored time.Time
	for _, state := range states {
		if state.IsDown || state.RestoredAt == nil {
			continue
		}
		restored := state.RestoredAt.UTC()
		if !now.Before(restored.Add(serviceRecoveryBannerDuration)) {
			continue
		}
		if restored.After(latestRestored) {
			latestRestored = restored
		}
	}
	if latestRestored.IsZero() {
		return nil, nil
	}

	return &models.StatusAlert{
		ID:        "automatic:services-restored",
		Message:   "Services have been restored. Performance is being closely monitored for 24 hours.",
		Level:     "info",
		CreatedAt: latestRestored.Format(time.RFC3339),
		Automatic: true,
		Kind:      "services_restored",
		EndsAt:    latestRestored.Add(serviceRecoveryBannerDuration).Format(time.RFC3339),
		Source:    "automatic",
		Editable:  true,
	}, nil
}

func getAutomaticUPSStatusAlert() (*models.StatusAlert, error) {
	config, err := database.LoadResourcesUIConfig()
	if err != nil {
		return nil, err
	}
	if config == nil || !config.Enabled || !config.UPS {
		return nil, nil
	}
	nutAddress := resources.NormalizeNUTAddress(config.NUTHost)
	upsName := strings.TrimSpace(config.UPSName)
	if nutAddress == "" || upsName == "" {
		return nil, nil
	}
	expectedSource := nutAddress + "/" + upsName
	state, err := database.GetUPSPowerState()
	if err != nil || state == nil || state.PowerPresent || state.Source != expectedSource {
		return nil, err
	}
	message := "Mains power lost. The monitored system is running on UPS battery."
	if state.LossNotified {
		message += " A power-loss notification has been sent."
	}
	return &models.StatusAlert{
		ID:        "automatic:ups-line-loss",
		Message:   message,
		Level:     "warning",
		CreatedAt: state.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Automatic: true,
		Kind:      "ups_line_loss",
		Source:    "automatic",
		Editable:  true,
	}, nil
}

func applyStatusAlertOverrides(alerts []models.StatusAlert, includeHidden bool) ([]models.StatusAlert, error) {
	overrides, err := database.GetStatusAlertOverrides()
	if err != nil {
		return nil, err
	}
	result := make([]models.StatusAlert, 0, len(alerts))
	for _, alert := range alerts {
		if alert.Source != "manual" {
			key := database.StatusAlertOverrideKey(alert.ID, alert.CreatedAt)
			if override, exists := overrides[key]; exists {
				if override.Message != nil {
					alert.Message = *override.Message
				}
				if override.Level != nil {
					alert.Level = *override.Level
				}
				alert.Hidden = override.Hidden
			}
		}
		if alert.Hidden && !includeHidden {
			continue
		}
		result = append(result, alert)
	}
	return result, nil
}

func getStatusAlerts() ([]models.StatusAlert, error) {
	rows, err := database.DB.Query(`SELECT id, service_key, message, level, created_at FROM status_alerts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := make([]models.StatusAlert, 0)
	for rows.Next() {
		var alert models.StatusAlert
		var serviceKey sql.NullString
		if err := rows.Scan(&alert.ID, &serviceKey, &alert.Message, &alert.Level, &alert.CreatedAt); err != nil {
			return nil, err
		}
		if serviceKey.Valid {
			alert.ServiceKey = serviceKey.String
		}
		alert.Source = "manual"
		alert.Editable = true
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

// HandleUpdateStatusAlert updates a manual banner or overrides one generated
// banner occurrence.
func HandleUpdateStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID           string `json:"id"`
			OccurrenceAt string `json:"occurrence_at"`
			ServiceKey   string `json:"service_key"`
			Message      string `json:"message"`
			Level        string `json:"level"`
			Hidden       *bool  `json:"hidden"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.ID = strings.TrimSpace(req.ID)
		if req.ID == "" {
			req.ID = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if req.ID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		var manualCount int
		if err := database.DB.QueryRow(`SELECT COUNT(*) FROM status_alerts WHERE id = ?`, req.ID).Scan(&manualCount); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if manualCount > 0 {
			message, level, ok := validateStatusAlertValues(req.Message, req.Level)
			if !ok {
				http.Error(w, "invalid banner", http.StatusBadRequest)
				return
			}
			var serviceKey any
			if strings.TrimSpace(req.ServiceKey) != "" {
				serviceKey = strings.TrimSpace(req.ServiceKey)
			}
			if _, err := database.DB.Exec(`UPDATE status_alerts SET service_key = ?, message = ?, level = ? WHERE id = ?`,
				serviceKey, message, level, req.ID); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeStatusAlertSuccess(w, "updated")
			return
		}

		alert, err := findGeneratedStatusAlert(time.Now(), req.ID, req.OccurrenceAt)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if alert == nil {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		hidden := alert.Hidden
		if req.Hidden != nil {
			hidden = *req.Hidden
		}
		override := database.StatusAlertOverride{
			AlertID: alert.ID, OccurrenceAt: alert.CreatedAt, Hidden: hidden,
		}
		if strings.TrimSpace(req.Message) != "" || strings.TrimSpace(req.Level) != "" {
			message, level, ok := validateStatusAlertValues(req.Message, req.Level)
			if !ok {
				http.Error(w, "invalid banner", http.StatusBadRequest)
				return
			}
			override.Message = &message
			override.Level = &level
		}
		if err := database.SaveStatusAlertOverride(override); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		writeStatusAlertSuccess(w, "updated")
	}
}

// HandleCreateStatusAlert creates a new alert
func HandleCreateStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ServiceKey string `json:"service_key"`
			Message    string `json:"message"`
			Level      string `json:"level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.Message = strings.TrimSpace(req.Message)
		req.Level = strings.ToLower(strings.TrimSpace(req.Level))
		if req.Message == "" {
			http.Error(w, "message required", http.StatusBadRequest)
			return
		}
		if len(req.Message) > 500 {
			http.Error(w, "message is too long", http.StatusBadRequest)
			return
		}
		if req.Level == "" {
			req.Level = "info"
		}
		if req.Level != "info" && req.Level != "warning" && req.Level != "error" {
			http.Error(w, "invalid level", http.StatusBadRequest)
			return
		}

		id := fmt.Sprintf("alert_%d", time.Now().UnixNano())
		now := time.Now().UTC().Format(time.RFC3339)

		var serviceKey interface{}
		if req.ServiceKey != "" {
			serviceKey = req.ServiceKey
		}

		_, err := database.DB.Exec(`INSERT INTO status_alerts (id, service_key, message, level, created_at) VALUES (?, ?, ?, ?, ?)`,
			id, serviceKey, req.Message, req.Level, now)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "id": id})
	}
}

// HandleDeleteStatusAlert deletes an alert by ID
func HandleDeleteStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		result, err := database.DB.Exec(`DELETE FROM status_alerts WHERE id = ?`, id)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if affected, _ := result.RowsAffected(); affected == 0 {
			alert, findErr := findGeneratedStatusAlert(time.Now(), id, r.URL.Query().Get("occurrence_at"))
			if findErr != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if alert == nil {
				http.Error(w, "alert not found", http.StatusNotFound)
				return
			}
			if err := database.SaveStatusAlertOverride(database.StatusAlertOverride{
				AlertID: alert.ID, OccurrenceAt: alert.CreatedAt, Hidden: true,
			}); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
		}
		writeStatusAlertSuccess(w, "deleted")
	}
}

func validateStatusAlertValues(message, level string) (string, string, bool) {
	message = strings.TrimSpace(message)
	level = strings.ToLower(strings.TrimSpace(level))
	if message == "" || len(message) > 500 {
		return "", "", false
	}
	if level == "" {
		level = "info"
	}
	if level != "info" && level != "warning" && level != "error" {
		return "", "", false
	}
	return message, level, true
}

func findGeneratedStatusAlert(now time.Time, id, occurrenceAt string) (*models.StatusAlert, error) {
	alerts, err := getAdminStatusAlerts(now)
	if err != nil {
		return nil, err
	}
	for index := range alerts {
		alert := &alerts[index]
		if alert.Source == "manual" || alert.ID != id {
			continue
		}
		if occurrenceAt == "" || alert.CreatedAt == occurrenceAt {
			return alert, nil
		}
	}
	return nil, nil
}

func writeStatusAlertSuccess(w http.ResponseWriter, action string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "action": action})
}
