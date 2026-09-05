package alerts

import (
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/smtp"
	"status/app/internal/database"
	"strconv"
	"time"
)

// SendEmail sends an email alert
func (m *Manager) SendEmail(subject, body string) error {
	config := m.GetConfig()
	if config == nil || !config.Enabled {
		return nil
	}

	if config.SMTPHost == "" || config.AlertEmail == "" {
		return errors.New("SMTP configuration incomplete")
	}

	from := config.FromEmail
	if from == "" {
		from = config.SMTPUser
	}

	// Create MIME message with HTML
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = config.AlertEmail
	headers["Subject"] = mime.QEncoding.Encode("utf-8", subject)
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msg string
	for k, v := range headers {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + body

	host := config.SMTPHost
	port := config.SMTPPort
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	c, err := dialSMTP(addr, host, port, config.SMTPSkipVerify)
	if err == nil {
		defer c.Close()
		auth := smtp.PlainAuth("", config.SMTPUser, config.SMTPPassword, host)
		if ok, _ := c.Extension("AUTH"); ok && config.SMTPUser != "" {
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
		if rcptErr := c.Rcpt(config.AlertEmail); rcptErr != nil {
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
		_ = database.InsertLog(database.LogLevelError, database.LogCategoryEmail, "", "Failed to send email", fmt.Sprintf("to=%s, subject=%s, error=%v", config.AlertEmail, subject, err))
	} else {
		_ = database.InsertLog(database.LogLevelInfo, database.LogCategoryEmail, "", "Email sent successfully", fmt.Sprintf("to=%s, subject=%s", config.AlertEmail, subject))
	}

	return err
}

// CreateHTMLEmail generates a styled HTML email
func CreateHTMLEmail(subject, statusType, serviceName, serviceKey, message, statusPageURL string) string {
	// Status colors and text
	statusColors := map[string]string{
		"down":       "#ef4444",
		"degraded":   "#eab308",
		"up":         "#22c55e",
		"power_lost": "#f59e0b",
	}
	statusTexts := map[string]string{
		"down":       "SERVICE DOWN",
		"degraded":   "SERVICE DEGRADED",
		"up":         "SERVICE UP",
		"power_lost": "POWER LOST",
	}

	color := statusColors[statusType]
	statusText := statusTexts[statusType]
	if color == "" {
		color = "#9ca3af"
	}
	if statusText == "" {
		statusText = "STATUS UPDATE"
	}

	// Default URL if not set
	if statusPageURL == "" {
		statusPageURL = "#"
	}
	safeSubject := html.EscapeString(subject)
	safeServiceName := html.EscapeString(serviceName)
	safeStatusPageURL := html.EscapeString(statusPageURL)

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
                    <span style="display:inline-block; padding:6px 10px; border-radius:999px; background-color:#1f2937; color:%s; font-size:11px; font-weight:700; letter-spacing:0.6px; text-transform:uppercase;">
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
</html>`, safeSubject, message, color, statusText, safeSubject, message, safeServiceName, color, statusText, time.Now().Format("Monday, January 2, 2006 at 3:04 PM MST"), safeStatusPageURL)

	return html
}

func dialSMTP(addr, host string, port int, skipVerify bool) (*smtp.Client, error) {
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: skipVerify,
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	// Implicit TLS for SMTPS (commonly port 465)
	if port == 465 {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if err != nil {
			return nil, err
		}
		if err := conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
			_ = conn.Close()
			return nil, err
		}
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
		}
		return client, err
	}

	// Plain TCP + STARTTLS if supported
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		_ = conn.Close()
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
