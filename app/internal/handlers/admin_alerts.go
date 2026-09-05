package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"status/app/internal/alerts"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
	"unicode/utf16"
)

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
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var config models.AlertConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&config); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := validateAlertConfig(&config); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

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
			log.Printf("Test email failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"message": "Failed to send test email. Check your SMTP settings.",
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

// HandleTestNotification sends a test notification to a specific channel
func HandleTestNotification(alertMgr *alerts.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Channel string `json:"channel"` // discord, telegram, webhook
			Status  string `json:"status"`  // optional preview: down, degraded, up
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		config := alertMgr.GetConfig()
		if config == nil {
			http.Error(w, "alerts not configured", http.StatusBadRequest)
			return
		}

		statusType := req.Status
		if statusType == "" {
			statusType = "test"
		}
		messages := map[string]string{
			"test":     "This is a test notification from Servicarr. Your notification destination accepted delivery.",
			"down":     "Example: the service is failing health checks. Investigate its availability.",
			"degraded": "Example: the service is responding, but response time exceeds the 200 ms degradation threshold.",
			"up":       "Example: the service has recovered and is responding normally to health checks.",
		}
		message, valid := messages[statusType]
		if !valid {
			http.Error(w, "unknown test status", http.StatusBadRequest)
			return
		}
		subject := "[Test] Servicarr notification: " + statusType
		statusPageURL := alertMgr.ResolveStatusPageURL("")
		var deliveryErr error

		switch req.Channel {
		case "discord":
			if config.DiscordWebhookURL == "" {
				http.Error(w, "Discord webhook URL not configured", http.StatusBadRequest)
				return
			}
			deliveryErr = alertMgr.SendDiscord(subject, statusType, "Test Service", message, statusPageURL)
		case "telegram":
			if config.TelegramBotToken == "" || config.TelegramChatID == "" {
				http.Error(w, "Telegram bot token or chat ID not configured", http.StatusBadRequest)
				return
			}
			deliveryErr = alertMgr.SendTelegram(subject, statusType, "Test Service", message)
		case "webhook":
			if config.WebhookURL == "" {
				http.Error(w, "Webhook URL not configured", http.StatusBadRequest)
				return
			}
			deliveryErr = alertMgr.SendWebhook(subject, statusType, "Test Service", "test", message)
		default:
			http.Error(w, "unknown channel: "+req.Channel, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if deliveryErr != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": deliveryErr.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Test " + req.Channel + " notification sent"})
	}
}

func validateAlertConfig(config *models.AlertConfig) error {
	config.DiscordWebhookURL = strings.TrimSpace(config.DiscordWebhookURL)
	config.WebhookURL = strings.TrimSpace(config.WebhookURL)
	config.StatusPageURL = strings.TrimSpace(config.StatusPageURL)
	config.DiscordUsername = strings.TrimSpace(config.DiscordUsername)
	if len(utf16.Encode([]rune(config.DiscordUsername))) > 80 {
		return fmt.Errorf("Discord display name must be at most 80 characters")
	}
	if config.SMTPHost != "" && (config.SMTPPort < 1 || config.SMTPPort > 65535) {
		return fmt.Errorf("SMTP port must be between 1 and 65535")
	}
	for _, field := range []struct{ name, value string }{
		{"Discord webhook URL", config.DiscordWebhookURL},
		{"Webhook URL", config.WebhookURL},
		{"Dashboard URL", config.StatusPageURL},
	} {
		if field.value == "" {
			continue
		}
		u, err := url.Parse(field.value)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
			return fmt.Errorf("%s must use HTTP or HTTPS without embedded credentials or fragments", field.name)
		}
		if field.name == "Dashboard URL" && u.RawQuery != "" {
			return fmt.Errorf("Dashboard URL must not include query parameters")
		}
		if field.name == "Discord webhook URL" {
			host := strings.ToLower(u.Hostname())
			if u.Scheme != "https" || (host != "discord.com" && host != "discordapp.com" && host != "canary.discord.com" && host != "ptb.discord.com") || (u.Port() != "" && u.Port() != "443") || !strings.HasPrefix(u.Path, "/api/webhooks/") {
				return fmt.Errorf("Discord webhook URL must be an HTTPS Discord webhook address")
			}
		}
	}
	if config.DiscordEnabled && config.DiscordWebhookURL == "" {
		return fmt.Errorf("Discord webhook URL is required when Discord is enabled")
	}
	if config.TelegramEnabled && (strings.TrimSpace(config.TelegramBotToken) == "" || strings.TrimSpace(config.TelegramChatID) == "") {
		return fmt.Errorf("Telegram bot token and chat ID are required when Telegram is enabled")
	}
	if config.WebhookEnabled && config.WebhookURL == "" {
		return fmt.Errorf("Webhook URL is required when webhook notifications are enabled")
	}
	return nil
}
