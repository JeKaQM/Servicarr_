package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"status/app/internal/alerts"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
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
		var req struct {
			Channel string `json:"channel"` // discord, telegram, webhook
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
