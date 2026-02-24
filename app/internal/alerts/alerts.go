package alerts

import (
	"database/sql"
	"fmt"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
)

// Manager handles alert notification functionality
type Manager struct {
	config        *models.AlertConfig
	statusPageURL string
}

// NewManager creates a new alerts manager
func NewManager(statusPageURL string) *Manager {
	config, _ := database.LoadAlertConfig()
	return &Manager{config: config, statusPageURL: statusPageURL}
}

// ReloadConfig reloads the alert configuration from database
func (m *Manager) ReloadConfig() error {
	config, err := database.LoadAlertConfig()
	if err != nil {
		return err
	}
	m.config = config
	return nil
}

// GetConfig returns the current alert configuration
func (m *Manager) GetConfig() *models.AlertConfig {
	return m.config
}

// GetStatusPageURL returns the configured status page URL
func (m *Manager) GetStatusPageURL() string {
	return m.statusPageURL
}

// ResolveStatusPageURL returns the status page URL from config, falling back to env or the provided fallback.
func (m *Manager) ResolveStatusPageURL(fallback string) string {
	if m.config != nil {
		if url := strings.TrimSpace(m.config.StatusPageURL); url != "" {
			return normalizeStatusPageURL(url)
		}
	}
	if url := strings.TrimSpace(m.statusPageURL); url != "" {
		return normalizeStatusPageURL(url)
	}
	return normalizeStatusPageURL(fallback)
}

// SetConfig updates the alert configuration
func (m *Manager) SetConfig(config *models.AlertConfig) {
	m.config = config
}

// CheckAndSendAlerts checks for service status changes and sends alerts across all configured channels
func (m *Manager) CheckAndSendAlerts(serviceKey, serviceName string, ok, degraded bool) {
	if m.config == nil || !m.config.Enabled {
		return
	}

	// Dependency-aware suppression: if upstream dependency is down, suppress
	svc, _ := database.GetServiceByKey(serviceKey)
	if svc != nil && svc.DependsOn != "" {
		depKeys := strings.Split(svc.DependsOn, ",")
		for _, dk := range depKeys {
			dk = strings.TrimSpace(dk)
			if dk == "" {
				continue
			}
			// Check if the upstream dependency is currently marked as down
			var depOK int
			err := database.DB.QueryRow(`SELECT ok FROM service_status_history WHERE service_key = ?`, dk).Scan(&depOK)
			if err == nil && depOK == 0 {
				_ = database.InsertLog(database.LogLevelInfo, database.LogCategoryEmail, serviceKey,
					"Alert suppressed — upstream dependency down", fmt.Sprintf("depends_on=%s", dk))
				m.updateStatusHistory(serviceKey, ok, degraded)
				return
			}
		}
	}

	// Get previous status
	var prevOK, prevDegraded int
	err := database.DB.QueryRow(`SELECT ok, degraded FROM service_status_history WHERE service_key = ?`, serviceKey).
		Scan(&prevOK, &prevDegraded)

	if err == sql.ErrNoRows {
		// First time
		if !ok && m.config.AlertOnDown {
			_ = database.InsertLog(database.LogLevelError, database.LogCategoryEmail, serviceKey, "Service went DOWN - sending alert (first status)", serviceName)
			subject := fmt.Sprintf("🔴 Service Down: %s", serviceName)
			message := fmt.Sprintf("The service <strong>%s</strong> is currently unreachable and not responding to health checks. Please investigate immediately.", serviceName)
			m.dispatchAll(subject, "down", serviceName, serviceKey, message)
		} else if ok && degraded && m.config.AlertOnDegraded {
			_ = database.InsertLog(database.LogLevelWarn, database.LogCategoryEmail, serviceKey, "Service DEGRADED - sending alert (first status)", serviceName)
			subject := fmt.Sprintf("⚠️ Service Degraded: %s", serviceName)
			message := fmt.Sprintf("The service <strong>%s</strong> is responding but experiencing high latency (over 200ms). Performance may be impacted.", serviceName)
			m.dispatchAll(subject, "degraded", serviceName, serviceKey, message)
		}

		_, _ = database.DB.Exec(`INSERT INTO service_status_history (service_key, ok, degraded, updated_at) VALUES (?, ?, ?, datetime('now'))`,
			serviceKey, boolToInt(ok), boolToInt(degraded))
		return
	}

	prevOKBool := prevOK == 1
	prevDegradedBool := prevDegraded == 1

	// Check for status changes
	if !ok && prevOKBool && m.config.AlertOnDown {
		_ = database.InsertLog(database.LogLevelError, database.LogCategoryEmail, serviceKey, "Service went DOWN - sending alert", serviceName)
		subject := fmt.Sprintf("🔴 Service Down: %s", serviceName)
		message := fmt.Sprintf("The service <strong>%s</strong> is currently unreachable and not responding to health checks. Please investigate immediately.", serviceName)
		m.dispatchAll(subject, "down", serviceName, serviceKey, message)
	} else if ok && !prevOKBool && m.config.AlertOnUp {
		_ = database.InsertLog(database.LogLevelInfo, database.LogCategoryEmail, serviceKey, "Service RECOVERED - sending alert", serviceName)
		subject := fmt.Sprintf("✅ Service Recovered: %s", serviceName)
		message := fmt.Sprintf("Great news! The service <strong>%s</strong> has recovered and is now responding normally to health checks.", serviceName)
		m.dispatchAll(subject, "up", serviceName, serviceKey, message)
	} else if ok && degraded && !prevDegradedBool && m.config.AlertOnDegraded {
		_ = database.InsertLog(database.LogLevelWarn, database.LogCategoryEmail, serviceKey, "Service DEGRADED - sending alert", serviceName)
		subject := fmt.Sprintf("⚠️ Service Degraded: %s", serviceName)
		message := fmt.Sprintf("The service <strong>%s</strong> is responding but experiencing high latency (over 200ms). Performance may be impacted.", serviceName)
		m.dispatchAll(subject, "degraded", serviceName, serviceKey, message)
	}

	// Update status history
	m.updateStatusHistory(serviceKey, ok, degraded)
}

// updateStatusHistory persists the current status for comparison on next check
func (m *Manager) updateStatusHistory(serviceKey string, ok, degraded bool) {
	_, _ = database.DB.Exec(`INSERT INTO service_status_history (service_key, ok, degraded, updated_at) VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(service_key) DO UPDATE SET ok=?, degraded=?, updated_at=datetime('now')`,
		serviceKey, boolToInt(ok), boolToInt(degraded), boolToInt(ok), boolToInt(degraded))
}

// dispatchAll sends a notification across all enabled channels
func (m *Manager) dispatchAll(subject, statusType, serviceName, serviceKey, message string) {
	statusPageURL := m.ResolveStatusPageURL("")

	// Email
	if m.config.SMTPHost != "" && m.config.AlertEmail != "" {
		body := CreateHTMLEmail(subject, statusType, serviceName, serviceKey, message, statusPageURL)
		go m.SendEmail(subject, body)
	}

	// Discord
	if m.config.DiscordEnabled && m.config.DiscordWebhookURL != "" {
		go m.SendDiscord(subject, statusType, serviceName, message, statusPageURL)
	}

	// Telegram
	if m.config.TelegramEnabled && m.config.TelegramBotToken != "" && m.config.TelegramChatID != "" {
		go m.SendTelegram(subject, statusType, serviceName, message)
	}

	// Generic webhook
	if m.config.WebhookEnabled && m.config.WebhookURL != "" {
		go m.SendWebhook(subject, statusType, serviceName, serviceKey, message)
	}
}

func normalizeStatusPageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "http://" + raw
	}
	return raw
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
