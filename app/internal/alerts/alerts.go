package alerts

import (
	"database/sql"
	"errors"
	"fmt"
	"net/smtp"
	"status/app/internal/database"
	"status/app/internal/models"
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
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg string
	for k, v := range headers {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + body

	auth := smtp.PlainAuth("", m.config.SMTPUser, m.config.SMTPPassword, m.config.SMTPHost)
	addr := fmt.Sprintf("%s:%d", m.config.SMTPHost, m.config.SMTPPort)

	return smtp.SendMail(addr, auth, from, []string{m.config.AlertEmail}, []byte(msg))
}

// CheckAndSendAlerts checks for service status changes and sends alerts
func (m *Manager) CheckAndSendAlerts(serviceKey, serviceName string, ok, degraded bool) {
	if m.config == nil || !m.config.Enabled {
		return
	}

	// Get previous status
	var prevOK, prevDegraded int
	err := database.DB.QueryRow(`SELECT ok, degraded FROM service_status_history WHERE service_key = ?`, serviceKey).
		Scan(&prevOK, &prevDegraded)

	if err == sql.ErrNoRows {
		// First time - just save current status
		_, _ = database.DB.Exec(`INSERT INTO service_status_history (service_key, ok, degraded, updated_at) VALUES (?, ?, ?, datetime('now'))`,
			serviceKey, boolToInt(ok), boolToInt(degraded))
		return
	}

	prevOKBool := prevOK == 1
	prevDegradedBool := prevDegraded == 1

	// Check for status changes
	if !ok && prevOKBool && m.config.AlertOnDown {
		// Service went down
		subject := fmt.Sprintf("🔴 Service Down: %s", serviceName)
		message := fmt.Sprintf("The service <strong>%s</strong> is currently unreachable and not responding to health checks. Please investigate immediately.", serviceName)
		body := CreateHTMLEmail(subject, "down", serviceName, serviceKey, message, m.statusPageURL)
		go m.SendEmail(subject, body)
	} else if ok && !prevOKBool && m.config.AlertOnUp {
		// Service came back up
		subject := fmt.Sprintf("✅ Service Recovered: %s", serviceName)
		message := fmt.Sprintf("Great news! The service <strong>%s</strong> has recovered and is now responding normally to health checks.", serviceName)
		body := CreateHTMLEmail(subject, "up", serviceName, serviceKey, message, m.statusPageURL)
		go m.SendEmail(subject, body)
	} else if ok && degraded && !prevDegradedBool && m.config.AlertOnDegraded {
		// Service became degraded
		subject := fmt.Sprintf("⚠️ Service Degraded: %s", serviceName)
		message := fmt.Sprintf("The service <strong>%s</strong> is responding but experiencing high latency (over 200ms). Performance may be impacted.", serviceName)
		body := CreateHTMLEmail(subject, "degraded", serviceName, serviceKey, message, m.statusPageURL)
		go m.SendEmail(subject, body)
	}

	// Update status history
	_, _ = database.DB.Exec(`INSERT INTO service_status_history (service_key, ok, degraded, updated_at) VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(service_key) DO UPDATE SET ok=?, degraded=?, updated_at=datetime('now')`,
		serviceKey, boolToInt(ok), boolToInt(degraded), boolToInt(ok), boolToInt(degraded))
}

// CreateHTMLEmail generates a styled HTML email
func CreateHTMLEmail(subject, statusType, serviceName, serviceKey, message, statusPageURL string) string {
	// Status colors and text
	statusColors := map[string]string{
		"down":     "#ef4444",
		"degraded": "#eab308",
		"up":       "#16a34a",
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
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #f3f4f6;">
    <table width="100%%" cellpadding="0" cellspacing="0" style="background-color: #f3f4f6; padding: 40px 0;">
        <tr>
            <td align="center">
                <table width="600" cellpadding="0" cellspacing="0" style="background-color: #ffffff; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); overflow: hidden;">
                    <!-- Header -->
                    <tr>
                        <td style="background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); padding: 40px 30px; text-align: center;">
                            <h1 style="margin: 0; color: #ffffff; font-size: 32px; font-weight: 700; letter-spacing: 1px;">Servicarr</h1>
                            <p style="margin: 8px 0 0 0; color: #e0e7ff; font-size: 14px;">Service Status Monitor</p>
                        </td>
                    </tr>
                    
                    <!-- Status Banner -->
                    <tr>
                        <td style="background-color: %s; padding: 20px 30px; text-align: center;">
                            <h2 style="margin: 0; color: #ffffff; font-size: 20px; font-weight: 600; text-transform: uppercase;">%s - %s</h2>
                        </td>
                    </tr>
                    
                    <!-- Content -->
                    <tr>
                        <td style="padding: 40px 30px;">
                            <p style="margin: 0 0 20px 0; color: #374151; font-size: 16px; line-height: 1.6;">
                                %s
                            </p>
                            
                            <!-- Details Box -->
                            <table width="100%%" cellpadding="0" cellspacing="0" style="background-color: #f9fafb; border-radius: 6px; border: 1px solid #e5e7eb; margin-top: 20px;">
                                <tr>
                                    <td style="padding: 20px;">
                                        <table width="100%%" cellpadding="8" cellspacing="0">
                                            <tr>
                                                <td style="color: #6b7280; font-size: 14px; width: 120px;">Service:</td>
                                                <td style="color: #111827; font-size: 14px; font-weight: 600;">%s</td>
                                            </tr>
                            <tr>
                                                <td style="color: #6b7280; font-size: 14px;">Status:</td>
                                                <td style="color: %s; font-size: 14px; font-weight: 600; text-transform: uppercase;">%s</td>
                                            </tr>
                                            <tr>
                                                <td style="color: #6b7280; font-size: 14px;">Time:</td>
                                                <td style="color: #111827; font-size: 14px;">%s</td>
                                            </tr>
                        </table>
                                    </td>
                                </tr>
                            </table>
                            
                            <!-- Action Button -->
                            <table width="100%%" cellpadding="0" cellspacing="0" style="margin-top: 30px;">
                                <tr>
                                    <td align="center">
                                        <a href="%s" style="display: inline-block; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: #ffffff; text-decoration: none; padding: 14px 32px; border-radius: 6px; font-weight: 600; font-size: 14px;">
                                            View Status Dashboard
                                        </a>
                                    </td>
                                </tr>
                            </table>
                        </td>
                    </tr>
                    
                    <!-- Footer -->
                    <tr>
                        <td style="background-color: #f9fafb; padding: 30px; text-align: center; border-top: 1px solid #e5e7eb;">
                            <p style="margin: 0; color: #6b7280; font-size: 12px; line-height: 1.6;">
                                This is an automated alert from your service monitoring system.<br>
                                You are receiving this because you have alerts enabled.
                            </p>
                            <p style="margin: 12px 0 0 0; color: #9ca3af; font-size: 11px;">
                                © 2025 Servicarr • Automated Service Monitor
                            </p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>`, subject, color, statusText, serviceName, message, serviceName, color, statusText, time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"), statusPageURL)

	return html
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
