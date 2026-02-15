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
	"status/app/internal/monitor"
	"status/app/internal/security"
	"status/app/internal/stats"
	"strings"
	"time"
)

// HandleIngestNow forces an immediate check of all services
func HandleIngestNow(tracker *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		dbServices, err := database.GetAllServices()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		for _, sc := range dbServices {
			// Skip disabled services
			disabled, _ := database.GetServiceDisabledState(sc.Key)
			if disabled {
				continue
			}

			timeout := time.Duration(sc.Timeout) * time.Second
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			checkOK, code, ms, errMsg := checker.Check(checker.CheckOptions{
				URL:         sc.URL,
				Timeout:     timeout,
				ExpectedMin: sc.ExpectedMin,
				ExpectedMax: sc.ExpectedMax,
				CheckType:   sc.CheckType,
				ServiceType: sc.ServiceType,
				APIToken:    sc.APIToken,
			})

			failures := tracker.Update(sc.Key, checkOK)
			ok := checkOK || failures < 2

			stats.RecordHeartbeat(sc.Key, ok, ms, code, errMsg)
			database.InsertSample(now, sc.Key, ok, code, ms)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"saved": true, "t": now})
	}
}

// HandleResetRecent clears recent failure incidents
func HandleResetRecent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		if _, err := database.DB.Exec(`DELETE FROM heartbeats WHERE status = 0 AND time >= ?`, cutoff); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if _, err := database.DB.Exec(`DELETE FROM samples WHERE ok = 0 AND taken_at >= ?`, cutoff); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted_recent_incidents": true})
	}
}

// HandleAdminCheck performs a forced check on a specific service
func HandleAdminCheck(tracker *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Service string `json:"service"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		sc, err := database.GetServiceByKey(req.Service)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if sc == nil {
			http.Error(w, "unknown service", http.StatusNotFound)
			return
		}

		disabled, _ := database.GetServiceDisabledState(req.Service)
		if disabled {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(models.LiveResult{Label: sc.Name, OK: false, Status: 0, Degraded: false, Disabled: true, CheckType: sc.CheckType})
			return
		}

		now := time.Now().UTC()
		timeout := time.Duration(sc.Timeout) * time.Second
		if timeout == 0 {
			timeout = 5 * time.Second
		}

		checkOK, code, ms, errMsg := checker.Check(checker.CheckOptions{
			URL:         sc.URL,
			Timeout:     timeout,
			ExpectedMin: sc.ExpectedMin,
			ExpectedMax: sc.ExpectedMax,
			CheckType:   sc.CheckType,
			ServiceType: sc.ServiceType,
			APIToken:    sc.APIToken,
		})

		failures := tracker.Update(sc.Key, checkOK)
		ok := checkOK || failures < 2
		stats.RecordHeartbeat(sc.Key, ok, ms, code, errMsg)
		database.InsertSample(now, sc.Key, ok, code, ms)

		degraded := ok && ms != nil && *ms > 200
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.LiveResult{Label: sc.Name, OK: ok, Status: code, MS: ms, Degraded: degraded, CheckType: sc.CheckType})
	}
}

// HandleToggleMonitoring enables or disables monitoring for a service
func HandleToggleMonitoring(tracker *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Service string `json:"service"`
			Enable  bool   `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		sc, err := database.GetServiceByKey(req.Service)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if sc == nil {
			http.Error(w, "unknown service", http.StatusNotFound)
			return
		}

		disabled := !req.Enable
		if err := database.SetServiceDisabledState(req.Service, disabled); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if disabled {
			tracker.Reset(req.Service)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service": sc.Key,
			"enabled": !disabled,
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

// HandleListWhitelist returns all whitelisted IPs
func HandleListWhitelist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := security.ListWhitelist()
		if err != nil {
			http.Error(w, "Failed to load whitelist", http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"whitelist": list,
		})
	}
}

// HandleAddToWhitelist adds an IP to the whitelist
func HandleAddToWhitelist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP   string `json:"ip"`
			Note string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := security.AddToWhitelist(req.IP, req.Note); err != nil {
			http.Error(w, "Failed to add to whitelist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"ip": req.IP,
		})
	}
}

// HandleRemoveFromWhitelist removes an IP from the whitelist
func HandleRemoveFromWhitelist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := security.RemoveFromWhitelist(req.IP); err != nil {
			http.Error(w, "Failed to remove from whitelist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"ip": req.IP,
		})
	}
}

// HandleListBlacklist returns all blacklisted IPs
func HandleListBlacklist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := security.ListBlacklist()
		if err != nil {
			http.Error(w, "Failed to load blacklist", http.StatusInternalServerError)
			return
		}
		if list == nil {
			list = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"blacklist": list,
		})
	}
}

// HandleAddToBlacklist adds an IP to the blacklist
func HandleAddToBlacklist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP        string `json:"ip"`
			Note      string `json:"note"`
			Permanent bool   `json:"permanent"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := security.AddToBlacklist(req.IP, req.Note, req.Permanent); err != nil {
			http.Error(w, "Failed to add to blacklist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":        true,
			"ip":        req.IP,
			"permanent": req.Permanent,
		})
	}
}

// HandleRemoveFromBlacklist removes an IP from the blacklist
func HandleRemoveFromBlacklist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IP string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := security.RemoveFromBlacklist(req.IP); err != nil {
			http.Error(w, "Failed to remove from blacklist", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"ip": req.IP,
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
		statusURL := alertMgr.ResolveStatusPageURL(inferRequestBaseURL(r))
		body := alerts.CreateHTMLEmail(
			subject,
			"up",
			"Test Service",
			"test",
			"This is a test email from your Servicarr monitoring system. If you received this, your email configuration is working correctly!",
			statusURL,
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

func inferRequestBaseURL(r *http.Request) string {
	host := r.Host
	if xfHost := r.Header.Get("X-Forwarded-Host"); xfHost != "" {
		host = strings.TrimSpace(strings.Split(xfHost, ",")[0])
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto != "" {
		proto = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	if host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", proto, host)
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

// HandleGetLogs returns system logs with optional filtering
func HandleGetLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
			if limit > 500 {
				limit = 500
			}
		}

		offset := 0
		if o := r.URL.Query().Get("offset"); o != "" {
			fmt.Sscanf(o, "%d", &offset)
		}

		level := r.URL.Query().Get("level")
		category := r.URL.Query().Get("category")
		service := r.URL.Query().Get("service")

		logs, err := database.GetLogs(limit, level, category, service, offset)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if logs == nil {
			logs = []models.LogEntry{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"logs": logs,
		})
	}
}

// HandleGetLogStats returns log statistics
func HandleGetLogStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := database.GetLogStats()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}
}

// HandleClearLogs clears logs older than specified days
func HandleClearLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Days int `json:"days"` // 0 means clear all
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Default to clearing all if no body
			req.Days = 0
		}

		if err := database.ClearLogs(req.Days); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		_ = database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "", "Logs cleared", fmt.Sprintf("Cleared logs older than %d days", req.Days))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}

// ── Test Notification Channels ──────────────────────────────────────

// HandleTestNotification sends a test notification to a specific channel
func HandleTestNotification(alertMgr *alerts.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Channel string `json:"channel"` // discord, slack, telegram, webhook
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		config := alertMgr.GetConfig()
		if config == nil {
			http.Error(w, "alerts not configured", http.StatusBadRequest)
			return
		}

		subject := "🔔 Test Notification from Servicarr"
		statusPageURL := alertMgr.ResolveStatusPageURL("")

		switch req.Channel {
		case "discord":
			if config.DiscordWebhookURL == "" {
				http.Error(w, "Discord webhook URL not configured", http.StatusBadRequest)
				return
			}
			alertMgr.SendDiscord(subject, "up", "Test Service", "This is a test notification from Servicarr. If you see this, Discord notifications are working!", statusPageURL)
		case "slack":
			if config.SlackWebhookURL == "" {
				http.Error(w, "Slack webhook URL not configured", http.StatusBadRequest)
				return
			}
			alertMgr.SendSlack(subject, "up", "Test Service", "This is a test notification from Servicarr. If you see this, Slack notifications are working!", statusPageURL)
		case "telegram":
			if config.TelegramBotToken == "" || config.TelegramChatID == "" {
				http.Error(w, "Telegram bot token or chat ID not configured", http.StatusBadRequest)
				return
			}
			alertMgr.SendTelegram(subject, "up", "Test Service", "This is a test notification from Servicarr. If you see this, Telegram notifications are working!")
		case "webhook":
			if config.WebhookURL == "" {
				http.Error(w, "Webhook URL not configured", http.StatusBadRequest)
				return
			}
			alertMgr.SendWebhook(subject, "up", "Test Service", "test", "This is a test notification from Servicarr.")
		default:
			http.Error(w, "unknown channel: "+req.Channel, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Test " + req.Channel + " notification sent"})
	}
}
