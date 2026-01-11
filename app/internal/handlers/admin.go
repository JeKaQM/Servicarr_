package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/alerts"
	"status/app/internal/checker"
	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/security"
	"time"
)

// HandleIngestNow forces an immediate check of all services
func HandleIngestNow(services []*models.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		for _, s := range services {
			// Skip disabled services
			if s.Disabled {
				continue
			}
			checkOK, code, ms, _ := checker.HTTPCheck(s.URL, s.Timeout, s.MinOK, s.MaxOK)

			// Update consecutive failure count
			if checkOK {
				s.ConsecutiveFailures = 0
			} else {
				s.ConsecutiveFailures++
			}

			// Service is only DOWN after 2 consecutive failures
			ok := checkOK || s.ConsecutiveFailures < 2
			database.InsertSample(now, s.Key, ok, code, ms)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"saved": true, "t": now})
	}
}

// HandleResetRecent clears recent failure incidents
func HandleResetRecent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := database.DB.Exec(`DELETE FROM samples WHERE ok=0 AND taken_at >= datetime('now','-24 hours')`)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted_recent_incidents": true})
	}
}

// HandleAdminCheck performs a forced check on a specific service
func HandleAdminCheck(services []*models.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Service string `json:"service"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		s := checker.FindServiceByKey(services, req.Service)
		if s == nil {
			http.Error(w, "unknown service", http.StatusNotFound)
			return
		}
		if s.Disabled {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(models.LiveResult{Label: s.Label, OK: false, Status: 0, Degraded: false})
			return
		}

		now := time.Now().UTC()
		checkOK, code, ms, _ := checker.HTTPCheck(s.URL, s.Timeout, s.MinOK, s.MaxOK)

		// Update consecutive failure count
		if checkOK {
			s.ConsecutiveFailures = 0
		} else {
			s.ConsecutiveFailures++
		}

		// Service is only DOWN after 2 consecutive failures
		ok := checkOK || s.ConsecutiveFailures < 2
		database.InsertSample(now, s.Key, ok, code, ms)

		degraded := ok && ms != nil && *ms > 200
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.LiveResult{Label: s.Label, OK: ok, Status: code, MS: ms, Degraded: degraded})
	}
}

// HandleToggleMonitoring enables or disables monitoring for a service
func HandleToggleMonitoring(services []*models.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Service string `json:"service"`
			Enable  bool   `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		s := checker.FindServiceByKey(services, req.Service)
		if s == nil {
			http.Error(w, "unknown service", http.StatusNotFound)
			return
		}

		s.Disabled = !req.Enable
		if err := database.SetServiceDisabledState(req.Service, s.Disabled); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service": s.Key,
			"enabled": !s.Disabled,
		})
	}
}

// HandleListBlocks returns all blocked IPs
func HandleListBlocks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		blocks, err := security.ListBlockedIPs()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"blocks": blocks,
		})
	}
}

// HandleUnblockIP removes a block for a specific IP
func HandleUnblockIP() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := security.ClearIPBlock(req.IP); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"unblocked": req.IP,
		})
	}
}

// HandleClearAllBlocks removes all IP blocks
func HandleClearAllBlocks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		affected, err := security.ClearAllIPBlocks()
		if err != nil {
			http.Error(w, "Failed to clear IP blocks", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message": fmt.Sprintf("Successfully cleared %d IP blocks", affected),
			"cleared": affected,
		})
	}
}

// HandleGetAlertsConfig retrieves alert configuration
func HandleGetAlertsConfig(alertMgr *alerts.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := alertMgr.GetConfig()
		if config == nil {
			// Return default config
			config = &models.AlertConfig{
				Enabled:         false,
				SMTPPort:        587,
				AlertOnDown:     true,
				AlertOnDegraded: true,
				AlertOnUp:       false,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(config)
	}
}

// HandleSaveAlertsConfig saves alert configuration
func HandleSaveAlertsConfig(alertMgr *alerts.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var config models.AlertConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := database.SaveAlertConfig(&config); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		// Update in-memory config
		alertMgr.SetConfig(&config)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Configuration saved successfully",
		})
	}
}

// HandleTestEmail sends a test email
func HandleTestEmail(alertMgr *alerts.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config := alertMgr.GetConfig()
		if config == nil || !config.Enabled {
			http.Error(w, "alerts not configured or disabled", http.StatusBadRequest)
			return
		}

		subject := "Test Alert from Servicarr"
		body := alerts.CreateHTMLEmail(
			subject,
			"up",
			"Test Service",
			"test",
			"This is a test email from your Servicarr monitoring system. If you received this, your email configuration is working correctly!",
			alertMgr.GetStatusPageURL(),
		)

		err := alertMgr.SendEmail(subject, body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("Failed to send test email: %v", err),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Test email sent successfully to " + config.AlertEmail,
		})
	}
}

// HandleGetStatusAlerts returns all status alerts
func HandleGetStatusAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.DB.Query(`SELECT id, service_key, message, level, created_at FROM status_alerts ORDER BY created_at DESC`)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		alerts := []models.StatusAlert{}
		for rows.Next() {
			var a models.StatusAlert
			var serviceKey sql.NullString
			if err := rows.Scan(&a.ID, &serviceKey, &a.Message, &a.Level, &a.CreatedAt); err != nil {
				continue
			}
			if serviceKey.Valid {
				a.ServiceKey = serviceKey.String
			}
			alerts = append(alerts, a)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(alerts)
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
		if req.Message == "" {
			http.Error(w, "message required", http.StatusBadRequest)
			return
		}
		if req.Level == "" {
			req.Level = "info"
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
