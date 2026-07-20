package alerts

import (
	"database/sql"
	"fmt"
	"html"
	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/resources"
	"strings"
	"sync"
)

type dispatchQueue struct {
	mu   sync.Mutex
	tail chan struct{}
}

func (q *dispatchQueue) enqueue(send func()) {
	q.mu.Lock()
	previous := q.tail
	done := make(chan struct{})
	q.tail = done
	q.mu.Unlock()

	go func() {
		if previous != nil {
			<-previous
		}
		defer close(done)
		send()
	}()
}

// Manager handles alert notification functionality
type Manager struct {
	configMu      sync.RWMutex
	config        *models.AlertConfig
	statusPageURL string
	emailQueue    dispatchQueue
	discordQueue  dispatchQueue
	telegramQueue dispatchQueue
	webhookQueue  dispatchQueue
}

// NewManager creates a new alerts manager
func NewManager(statusPageURL string) *Manager {
	config, _ := database.LoadAlertConfig()
	manager := &Manager{statusPageURL: statusPageURL}
	manager.SetConfig(config)
	return manager
}

// ReloadConfig reloads the alert configuration from database
func (m *Manager) ReloadConfig() error {
	config, err := database.LoadAlertConfig()
	if err != nil {
		return err
	}
	m.SetConfig(config)
	return nil
}

// GetConfig returns a snapshot of the current alert configuration.
func (m *Manager) GetConfig() *models.AlertConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	config := *m.config
	return &config
}

// GetStatusPageURL returns the configured status page URL
func (m *Manager) GetStatusPageURL() string {
	return m.statusPageURL
}

// ResolveStatusPageURL returns the status page URL from config, falling back to env or the provided fallback.
func (m *Manager) ResolveStatusPageURL(fallback string) string {
	if config := m.GetConfig(); config != nil {
		if url := strings.TrimSpace(config.StatusPageURL); url != "" {
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
	m.configMu.Lock()
	defer m.configMu.Unlock()
	if config == nil {
		m.config = nil
		return
	}
	copy := *config
	m.config = &copy
}

// NotifyUPSLineLost queues an email when the UPS confirms that mains power is absent.
// It returns true only when email delivery was configured and the message was queued.
func (m *Manager) NotifyUPSLineLost(info *resources.UPSInfo) bool {
	configSnapshot := m.GetConfig()
	if configSnapshot == nil || !configSnapshot.Enabled || configSnapshot.SMTPHost == "" || configSnapshot.AlertEmail == "" {
		return false
	}

	config := *configSnapshot
	sender := &Manager{config: &config, statusPageURL: m.statusPageURL}
	name := "UPS"
	if info != nil && strings.TrimSpace(info.Model) != "" {
		name = cleanNotificationName(info.Model, name)
	}
	subject := "UPS Power Lost: " + name
	message := "Mains power has been lost. The monitored system is currently running on UPS battery."
	if info != nil && info.BatteryChargePercent != nil {
		message += fmt.Sprintf("<br><br><strong>Battery charge:</strong> %.0f%%", *info.BatteryChargePercent)
	}
	if info != nil && info.BatteryRuntimeSeconds != nil {
		runtimeMinutes := *info.BatteryRuntimeSeconds / 60
		message += fmt.Sprintf("<br><strong>Estimated runtime:</strong> %.0f minutes", runtimeMinutes)
	}

	body := CreateHTMLEmail(subject, "power_lost", name, "ups", message, m.ResolveStatusPageURL(""))
	m.emailQueue.enqueue(func() {
		_ = sender.SendEmail(subject, body)
	})
	return true
}

// CheckAndSendAlerts checks for service status changes and sends alerts across all configured channels
func (m *Manager) CheckAndSendAlerts(serviceKey, serviceName string, ok, degraded bool) {
	config := m.GetConfig()
	if config == nil || !config.Enabled {
		return
	}
	serviceName = cleanNotificationName(serviceName, serviceKey)
	safeServiceName := html.EscapeString(serviceName)

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
				if !m.statusChanged(serviceKey, ok, degraded) {
					m.updateStatusHistory(serviceKey, ok, degraded)
					return
				}
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
		if !ok && config.AlertOnDown {
			_ = database.InsertLog(database.LogLevelError, database.LogCategoryEmail, serviceKey, "Service went DOWN - sending alert (first status)", serviceName)
			subject := fmt.Sprintf("🔴 Service Down: %s", serviceName)
			message := fmt.Sprintf("The service <strong>%s</strong> is currently unreachable and not responding to health checks. Please investigate immediately.", safeServiceName)
			m.dispatchAll(subject, "down", serviceName, serviceKey, message)
		} else if ok && degraded && config.AlertOnDegraded {
			_ = database.InsertLog(database.LogLevelWarn, database.LogCategoryEmail, serviceKey, "Service DEGRADED - sending alert (first status)", serviceName)
			subject := fmt.Sprintf("⚠️ Service Degraded: %s", serviceName)
			message := fmt.Sprintf("The service <strong>%s</strong> is responding but experiencing high latency (over 200ms). Performance may be impacted.", safeServiceName)
			m.dispatchAll(subject, "degraded", serviceName, serviceKey, message)
		}

		_, _ = database.DB.Exec(`INSERT INTO service_status_history (service_key, ok, degraded, updated_at) VALUES (?, ?, ?, datetime('now'))`,
			serviceKey, boolToInt(ok), boolToInt(degraded))
		return
	}

	prevOKBool := prevOK == 1
	prevDegradedBool := prevDegraded == 1

	// Check for status changes
	if !ok && prevOKBool && config.AlertOnDown {
		_ = database.InsertLog(database.LogLevelError, database.LogCategoryEmail, serviceKey, "Service went DOWN - sending alert", serviceName)
		subject := fmt.Sprintf("🔴 Service Down: %s", serviceName)
		message := fmt.Sprintf("The service <strong>%s</strong> is currently unreachable and not responding to health checks. Please investigate immediately.", safeServiceName)
		m.dispatchAll(subject, "down", serviceName, serviceKey, message)
	} else if ok && !prevOKBool && config.AlertOnUp {
		_ = database.InsertLog(database.LogLevelInfo, database.LogCategoryEmail, serviceKey, "Service RECOVERED - sending alert", serviceName)
		subject := fmt.Sprintf("✅ Service Recovered: %s", serviceName)
		message := fmt.Sprintf("Great news! The service <strong>%s</strong> has recovered and is now responding normally to health checks.", safeServiceName)
		m.dispatchAll(subject, "up", serviceName, serviceKey, message)
	} else if ok && degraded && !prevDegradedBool && config.AlertOnDegraded {
		_ = database.InsertLog(database.LogLevelWarn, database.LogCategoryEmail, serviceKey, "Service DEGRADED - sending alert", serviceName)
		subject := fmt.Sprintf("⚠️ Service Degraded: %s", serviceName)
		message := fmt.Sprintf("The service <strong>%s</strong> is responding but experiencing high latency (over 200ms). Performance may be impacted.", safeServiceName)
		m.dispatchAll(subject, "degraded", serviceName, serviceKey, message)
	}

	// Update status history
	m.updateStatusHistory(serviceKey, ok, degraded)
}

func (m *Manager) statusChanged(serviceKey string, ok, degraded bool) bool {
	var prevOK, prevDegraded int
	err := database.DB.QueryRow(`SELECT ok, degraded FROM service_status_history WHERE service_key = ?`, serviceKey).
		Scan(&prevOK, &prevDegraded)
	if err == sql.ErrNoRows {
		return true
	}
	if err != nil {
		return true
	}
	return prevOK != boolToInt(ok) || prevDegraded != boolToInt(degraded)
}

// updateStatusHistory persists the current status for comparison on next check
func (m *Manager) updateStatusHistory(serviceKey string, ok, degraded bool) {
	_, _ = database.DB.Exec(`INSERT INTO service_status_history (service_key, ok, degraded, updated_at) VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(service_key) DO UPDATE SET ok=?, degraded=?, updated_at=datetime('now')`,
		serviceKey, boolToInt(ok), boolToInt(degraded), boolToInt(ok), boolToInt(degraded))
}

// dispatchAll sends a notification across all enabled channels
func (m *Manager) dispatchAll(subject, statusType, serviceName, serviceKey, message string) {
	configSnapshot := m.GetConfig()
	if configSnapshot == nil || !configSnapshot.Enabled {
		return
	}
	statusPageURL := m.ResolveStatusPageURL("")
	config := *configSnapshot
	sender := &Manager{config: &config, statusPageURL: m.statusPageURL}

	// Email
	if config.SMTPHost != "" && config.AlertEmail != "" {
		body := CreateHTMLEmail(subject, statusType, serviceName, serviceKey, message, statusPageURL)
		m.emailQueue.enqueue(func() {
			_ = sender.SendEmail(subject, body)
		})
	}

	// Discord
	if config.DiscordEnabled && config.DiscordWebhookURL != "" {
		m.discordQueue.enqueue(func() {
			sender.SendDiscord(subject, statusType, serviceName, message, statusPageURL)
		})
	}

	// Telegram
	if config.TelegramEnabled && config.TelegramBotToken != "" && config.TelegramChatID != "" {
		m.telegramQueue.enqueue(func() {
			sender.SendTelegram(subject, statusType, serviceName, message)
		})
	}

	// Generic webhook
	if config.WebhookEnabled && config.WebhookURL != "" {
		m.webhookQueue.enqueue(func() {
			sender.SendWebhook(subject, statusType, serviceName, serviceKey, message)
		})
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

func cleanNotificationName(value, fallback string) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return fallback
	}
	return value
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
