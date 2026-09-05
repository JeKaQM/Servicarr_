package alerts

import (
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/resources"
	"strings"
	"sync"
	"time"
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
	httpClient    *http.Client
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
	sender := &Manager{config: &config, statusPageURL: m.statusPageURL, httpClient: m.httpClient}
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

// CheckAndSendAlerts checks for service status changes and sends alerts across all configured channels.
// It reports whether at least one notification was queued for this transition.
func (m *Manager) CheckAndSendAlerts(serviceKey, serviceName string, ok, degraded bool) bool {
	config := m.GetConfig()
	if config == nil || !config.Enabled {
		return false
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
					return false
				}
				_ = database.InsertLog(database.LogLevelInfo, database.LogCategoryEmail, serviceKey,
					"Alert suppressed — upstream dependency down", fmt.Sprintf("depends_on=%s", dk))
				m.updateStatusHistory(serviceKey, ok, degraded)
				return false
			}
		}
	}

	// Treat unavailable, degraded and healthy as three distinct states.
	var prevOK, prevDegraded int
	err := database.DB.QueryRow(`SELECT ok, degraded FROM service_status_history WHERE service_key = ?`, serviceKey).
		Scan(&prevOK, &prevDegraded)
	if err != nil && err != sql.ErrNoRows {
		return false
	}
	firstStatus := err == sql.ErrNoRows
	previous := "unknown"
	if !firstStatus {
		previous = notificationStatus(prevOK == 1, prevDegraded == 1)
	}
	current := notificationStatus(ok, degraded)
	queued := false
	if current != previous {
		var subject, message string
		switch {
		case current == "down" && config.AlertOnDown:
			subject = fmt.Sprintf("🔴 Service Down: %s", serviceName)
			message = fmt.Sprintf("The service <strong>%s</strong> is failing health checks. Investigate its availability.", safeServiceName)
		case current == "degraded" && config.AlertOnDegraded:
			subject = fmt.Sprintf("⚠️ Service Degraded: %s", serviceName)
			message = fmt.Sprintf("The service <strong>%s</strong> is responding, but response time exceeds the 200 ms degradation threshold.", safeServiceName)
			if previous == "down" {
				message += " The service is reachable again, but has not fully recovered."
			}
		case current == "up" && !firstStatus && config.AlertOnUp:
			subject = fmt.Sprintf("✅ Service Recovered: %s", serviceName)
			message = fmt.Sprintf("The service <strong>%s</strong> has recovered and is responding normally to health checks.", safeServiceName)
		}
		if subject != "" {
			queued = m.dispatchAll(subject, current, serviceName, serviceKey, message)
		}
	}
	m.updateStatusHistory(serviceKey, ok, degraded)
	return queued
}

func notificationStatus(ok, degraded bool) string {
	if !ok {
		return "down"
	}
	if degraded {
		return "degraded"
	}
	return "up"
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
		ON CONFLICT(service_key) DO UPDATE SET
		updated_at=CASE WHEN ok != excluded.ok OR degraded != excluded.degraded THEN excluded.updated_at ELSE updated_at END,
		ok=excluded.ok, degraded=excluded.degraded`,
		serviceKey, boolToInt(ok), boolToInt(degraded))
}

// dispatchAll sends a notification across all enabled channels.
func (m *Manager) dispatchAll(subject, statusType, serviceName, serviceKey, message string) bool {
	configSnapshot := m.GetConfig()
	if configSnapshot == nil || !configSnapshot.Enabled {
		return false
	}
	statusPageURL := m.ResolveStatusPageURL("")
	config := *configSnapshot
	sender := &Manager{config: &config, statusPageURL: m.statusPageURL, httpClient: m.httpClient}
	queued := false

	// Email
	if config.SMTPHost != "" && config.AlertEmail != "" {
		body := CreateHTMLEmail(subject, statusType, serviceName, serviceKey, message, statusPageURL)
		m.emailQueue.enqueue(func() {
			_ = sender.SendEmail(subject, body)
		})
		queued = true
	}

	// Discord
	if config.DiscordEnabled && config.DiscordWebhookURL != "" {
		details := loadNotificationDetails(serviceKey, time.Now())
		m.discordQueue.enqueue(func() {
			_ = sender.sendDiscord(subject, statusType, serviceName, message, statusPageURL, details)
		})
		queued = true
	}

	// Telegram
	if config.TelegramEnabled && config.TelegramBotToken != "" && config.TelegramChatID != "" {
		m.telegramQueue.enqueue(func() {
			sender.SendTelegram(subject, statusType, serviceName, message)
		})
		queued = true
	}

	// Generic webhook
	if config.WebhookEnabled && config.WebhookURL != "" {
		m.webhookQueue.enqueue(func() {
			sender.SendWebhook(subject, statusType, serviceName, serviceKey, message)
		})
		queued = true
	}

	return queued
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
