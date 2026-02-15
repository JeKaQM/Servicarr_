package alerts

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
	"time"
)

// Manager handles email alert functionality
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

// SendEmail sends an email alert
func (m *Manager) SendEmail(subject, body string) error {
	if m.config == nil || !m.config.Enabled {
		return nil
	}

	if m.config.SMTPHost == "" || m.config.AlertEmail == "" {
		return errors.New("SMTP configuration incomplete")
	}

	from := m.config.FromEmail
	if from == "" {
		from = m.config.SMTPUser
	}

	// Create MIME message with HTML
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = m.config.AlertEmail
	headers["Subject"] = mime.QEncoding.Encode("utf-8", subject)
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg string
	for k, v := range headers {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + body

	host := strings.TrimSpace(m.config.SMTPHost)
	port := m.config.SMTPPort
	addr := fmt.Sprintf("%s:%d", host, port)

	c, err := dialSMTP(addr, host, port, m.config.SMTPSkipVerify)
	if err == nil {
		auth := smtp.PlainAuth("", m.config.SMTPUser, m.config.SMTPPassword, host)
		if ok, _ := c.Extension("AUTH"); ok && m.config.SMTPUser != "" {
			if authErr := c.Auth(auth); authErr != nil {
				_ = c.Close()
				err = authErr
			}
		}
	}

	if err == nil {
		if mailErr := c.Mail(from); mailErr != nil {
			_ = c.Close()
			err = mailErr
		}
	}

	if err == nil {
		if rcptErr := c.Rcpt(m.config.AlertEmail); rcptErr != nil {
			_ = c.Close()
			err = rcptErr
		}
	}

	if err == nil {
		w, dataErr := c.Data()
		if dataErr != nil {
			_ = c.Close()
			err = dataErr
		} else {
			_, writeErr := w.Write([]byte(msg))
			closeErr := w.Close()
			if writeErr != nil {
				err = writeErr
			} else if closeErr != nil {
				err = closeErr
			}
			_ = c.Quit()
		}
	}

	// Log email send attempt
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, database.LogCategoryEmail, "", "Failed to send email", fmt.Sprintf("to=%s, subject=%s, error=%v", m.config.AlertEmail, subject, err))
	} else {
		_ = database.InsertLog(database.LogLevelInfo, database.LogCategoryEmail, "", "Email sent successfully", fmt.Sprintf("to=%s, subject=%s", m.config.AlertEmail, subject))
	}

	return err
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

	// Slack
	if m.config.SlackEnabled && m.config.SlackWebhookURL != "" {
		go m.SendSlack(subject, statusType, serviceName, message, statusPageURL)
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

// ── Discord ─────────────────────────────────────────────────────────

// SendDiscord sends a rich embed message via Discord webhook
func (m *Manager) SendDiscord(subject, statusType, serviceName, message, statusPageURL string) {
	colorMap := map[string]int{"down": 0xef4444, "degraded": 0xeab308, "up": 0x22c55e}
	color := colorMap[statusType]

	payload := map[string]interface{}{
		"username":   "Servicarr",
		"avatar_url": "https://raw.githubusercontent.com/JeKaQM/Servicarr_/main/web/static/images/icon.png",
		"embeds": []map[string]interface{}{
			{
				"title":       subject,
				"description": strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", "**"), "</strong>", "**"),
				"color":       color,
				"fields": []map[string]interface{}{
					{"name": "Service", "value": serviceName, "inline": true},
					{"name": "Status", "value": strings.ToUpper(statusType), "inline": true},
					{"name": "Time", "value": time.Now().Format(time.RFC1123), "inline": false},
				},
				"footer": map[string]string{"text": "Servicarr Status Monitor"},
			},
		},
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(m.config.DiscordWebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Discord notification failed", err.Error())
		return
	}
	defer resp.Body.Close()
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, "Discord notification sent", fmt.Sprintf("status=%d", resp.StatusCode))
}

// ── Slack ────────────────────────────────────────────────────────────

// SendSlack sends a rich message via Slack incoming webhook
func (m *Manager) SendSlack(subject, statusType, serviceName, message, statusPageURL string) {
	colorMap := map[string]string{"down": "#ef4444", "degraded": "#eab308", "up": "#22c55e"}
	emojiMap := map[string]string{"down": ":red_circle:", "degraded": ":warning:", "up": ":white_check_mark:"}

	plainMsg := strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", "*"), "</strong>", "*")

	payload := map[string]interface{}{
		"username":   "Servicarr",
		"icon_emoji": emojiMap[statusType],
		"attachments": []map[string]interface{}{
			{
				"color": colorMap[statusType],
				"title": subject,
				"text":  plainMsg,
				"fields": []map[string]interface{}{
					{"title": "Service", "value": serviceName, "short": true},
					{"title": "Status", "value": strings.ToUpper(statusType), "short": true},
				},
				"footer": "Servicarr Status Monitor",
				"ts":     time.Now().Unix(),
			},
		},
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(m.config.SlackWebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Slack notification failed", err.Error())
		return
	}
	defer resp.Body.Close()
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, "Slack notification sent", fmt.Sprintf("status=%d", resp.StatusCode))
}

// ── Telegram ────────────────────────────────────────────────────────

// SendTelegram sends a message via Telegram Bot API
func (m *Manager) SendTelegram(subject, statusType, serviceName, message string) {
	plainMsg := strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", "<b>"), "</strong>", "</b>")
	text := fmt.Sprintf("<b>%s</b>\n\n%s\n\n🕒 %s", subject, plainMsg, time.Now().Format(time.RFC1123))

	payload := map[string]interface{}{
		"chat_id":    m.config.TelegramChatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", m.config.TelegramBotToken)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Telegram notification failed", err.Error())
		return
	}
	defer resp.Body.Close()
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, "Telegram notification sent", fmt.Sprintf("status=%d", resp.StatusCode))
}

// ── Generic Webhook ─────────────────────────────────────────────────

// SendWebhook sends a JSON payload to a generic webhook URL with optional HMAC signing
func (m *Manager) SendWebhook(subject, statusType, serviceName, serviceKey, message string) {
	payload := map[string]interface{}{
		"event":        "status_change",
		"service_key":  serviceKey,
		"service_name": serviceName,
		"status":       statusType,
		"subject":      subject,
		"message":      strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", ""), "</strong>", ""),
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", m.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Webhook request failed", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Servicarr/1.0")

	// HMAC-SHA256 signature
	if m.config.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(m.config.WebhookSecret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Servicarr-Signature", "sha256="+sig)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Webhook notification failed", err.Error())
		return
	}
	defer resp.Body.Close()
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, "Webhook notification sent", fmt.Sprintf("url=%s, status=%d", m.config.WebhookURL, resp.StatusCode))
}

// CreateHTMLEmail generates a styled HTML email
func CreateHTMLEmail(subject, statusType, serviceName, serviceKey, message, statusPageURL string) string {
	// Status colors and text
	statusColors := map[string]string{
		"down":     "#ef4444",
		"degraded": "#eab308",
		"up":       "#22c55e",
	}
	statusTexts := map[string]string{
		"down":     "SERVICE DOWN",
		"degraded": "SERVICE DEGRADED",
		"up":       "SERVICE UP",
	}

	color := statusColors[statusType]
	statusText := statusTexts[statusType]

	// Default URL if not set
	if statusPageURL == "" {
		statusPageURL = "#"
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
</head>
<body style="margin:0; padding:0; background-color:#0c121c; color:#e5e7eb; font-family:'Segoe UI', Arial, Helvetica, sans-serif;">
  <div style="display:none; max-height:0; overflow:hidden; opacity:0; color:transparent;">
    %s
  </div>
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:#0c121c; padding:32px 12px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:620px; background-color:#111827; border:1px solid #1f2937; border-radius:16px; overflow:hidden; box-shadow:0 10px 30px rgba(0,0,0,0.35);">
          <tr>
            <td style="padding:28px 28px 18px 28px; border-bottom:1px solid #1f2937;">
              <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
                <tr>
                  <td>
                    <div style="font-size:18px; font-weight:700; letter-spacing:0.5px;">Servicarr</div>
                    <div style="color:#9ca3af; font-size:12px; margin-top:4px;">Service Status Monitor</div>
                  </td>
                  <td align="right">
                    <span style="display:inline-block; padding:6px 10px; border-radius:999px; background-color:rgba(34,197,94,0.16); color:#22c55e; font-size:11px; font-weight:700; letter-spacing:0.6px; text-transform:uppercase;">
                      %s
                    </span>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding:28px;">
              <div style="font-size:22px; font-weight:700; margin-bottom:10px;">%s</div>
              <div style="color:#9ca3af; font-size:13px; margin-bottom:18px;">%s</div>
              <div style="background-color:#0f172a; border:1px solid #1f2937; border-radius:12px; padding:16px; margin-bottom:22px;">
                <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
                  <tr>
                    <td style="color:#9ca3af; font-size:12px; padding-bottom:6px;">Service</td>
                    <td style="text-align:right; color:#e5e7eb; font-size:13px; font-weight:600;">%s</td>
                  </tr>
                  <tr>
                    <td style="color:#9ca3af; font-size:12px; padding-bottom:6px;">Status</td>
                    <td style="text-align:right; color:%s; font-size:13px; font-weight:700; text-transform:uppercase;">%s</td>
                  </tr>
                  <tr>
                    <td style="color:#9ca3af; font-size:12px;">Time</td>
                    <td style="text-align:right; color:#e5e7eb; font-size:13px;">%s</td>
                  </tr>
                </table>
              </div>
              <div style="text-align:center;">
                <a href="%s" style="display:inline-block; background-color:#22c55e; color:#0c121c; text-decoration:none; padding:12px 22px; border-radius:10px; font-weight:700; font-size:13px; letter-spacing:0.3px;">
                  View Status Dashboard
                </a>
              </div>
            </td>
          </tr>
          <tr>
            <td style="padding:18px 28px 26px 28px; border-top:1px solid #1f2937; color:#9ca3af; font-size:11px; line-height:1.6;">
              This is an automated alert from your Servicarr monitor. You are receiving this because alerts are enabled.
              <div style="margin-top:8px; color:#6b7280;">&#169; 2026 Servicarr</div>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, subject, message, statusText, subject, message, serviceName, color, statusText, time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"), statusPageURL)

	return html
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

func dialSMTP(addr, host string, port int, skipVerify bool) (*smtp.Client, error) {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: skipVerify,
	}

	// Implicit TLS for SMTPS (commonly port 465)
	if port == 465 {
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, host)
	}

	// Plain TCP + STARTTLS if supported
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(tlsConfig); err != nil {
			_ = c.Close()
			return nil, err
		}
	}

	return c, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
