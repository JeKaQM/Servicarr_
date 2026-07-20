package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"strings"
	"time"
)

const serviceRecoveryBannerDuration = 24 * time.Hour

// HandleGetStatusAlerts returns all status alerts
func HandleGetStatusAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alerts, err := getStatusAlerts()
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
	manual, err := getStatusAlerts()
	if err != nil {
		return nil, err
	}
	windows, _ := maintenance.Current(now)
	alerts := make([]models.StatusAlert, 0, len(windows)+len(manual)+1)
	monitoringSuppressed := false
	for _, window := range windows {
		if window.Schedule.SuppressMonitoring {
			monitoringSuppressed = true
		}
		alerts = append(alerts, models.StatusAlert{
			ID:        "scheduled:" + window.Schedule.ID,
			Message:   window.Schedule.Message,
			Level:     window.Schedule.Level,
			CreatedAt: window.StartsAt.UTC().Format(time.RFC3339),
			Scheduled: true,
			EndsAt:    window.EndsAt.UTC().Format(time.RFC3339),
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
	return append(alerts, manual...), nil
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
	}, nil
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
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
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

		_, err := database.DB.Exec(`DELETE FROM status_alerts WHERE id = ?`, id)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}
