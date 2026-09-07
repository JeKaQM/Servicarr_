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

// alertConfigResponse deliberately omits notification destinations and secrets.
// The configured flags let the UI distinguish an empty setting from a saved one
// without placing credentials back into form fields or browser caches.
type alertConfigResponse struct {
	Enabled                  bool   `json:"enabled"`
	SMTPHost                 string `json:"smtp_host"`
	SMTPPort                 int    `json:"smtp_port"`
	SMTPUser                 string `json:"smtp_user"`
	AlertEmail               string `json:"alert_email"`
	FromEmail                string `json:"from_email"`
	StatusPageURL            string `json:"status_page_url"`
	SMTPSkipVerify           bool   `json:"smtp_skip_verify"`
	AlertOnDown              bool   `json:"alert_on_down"`
	AlertOnDegraded          bool   `json:"alert_on_degraded"`
	AlertOnUp                bool   `json:"alert_on_up"`
	DiscordEnabled           bool   `json:"discord_enabled"`
	DiscordUsername          string `json:"discord_username"`
	DiscordSilent            bool   `json:"discord_silent"`
	TelegramChatID           string `json:"telegram_chat_id"`
	TelegramEnabled          bool   `json:"telegram_enabled"`
	WebhookEnabled           bool   `json:"webhook_enabled"`
	SMTPPasswordConfigured   bool   `json:"smtp_password_configured"`
	DiscordWebhookConfigured bool   `json:"discord_webhook_configured"`
	TelegramTokenConfigured  bool   `json:"telegram_bot_token_configured"`
	WebhookURLConfigured     bool   `json:"webhook_url_configured"`
	WebhookSecretConfigured  bool   `json:"webhook_secret_configured"`
}

type alertConfigUpdate struct {
	models.AlertConfig
	ClearSMTPPassword   bool `json:"clear_smtp_password"`
	ClearDiscordWebhook bool `json:"clear_discord_webhook_url"`
	ClearTelegramToken  bool `json:"clear_telegram_bot_token"`
	ClearWebhookURL     bool `json:"clear_webhook_url"`
	ClearWebhookSecret  bool `json:"clear_webhook_secret"`
}

type alertURLValidation struct {
	discord   bool
	webhook   bool
	dashboard bool
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

		response := alertConfigResponse{
			Enabled:                  config.Enabled,
			SMTPHost:                 config.SMTPHost,
			SMTPPort:                 config.SMTPPort,
			SMTPUser:                 config.SMTPUser,
			AlertEmail:               config.AlertEmail,
			FromEmail:                config.FromEmail,
			StatusPageURL:            config.StatusPageURL,
			SMTPSkipVerify:           config.SMTPSkipVerify,
			AlertOnDown:              config.AlertOnDown,
			AlertOnDegraded:          config.AlertOnDegraded,
			AlertOnUp:                config.AlertOnUp,
			DiscordEnabled:           config.DiscordEnabled,
			DiscordUsername:          config.DiscordUsername,
			DiscordSilent:            config.DiscordSilent,
			TelegramChatID:           config.TelegramChatID,
			TelegramEnabled:          config.TelegramEnabled,
			WebhookEnabled:           config.WebhookEnabled,
			SMTPPasswordConfigured:   config.SMTPPassword != "",
			DiscordWebhookConfigured: config.DiscordWebhookURL != "",
			TelegramTokenConfigured:  config.TelegramBotToken != "",
			WebhookURLConfigured:     config.WebhookURL != "",
			WebhookSecretConfigured:  config.WebhookSecret != "",
		}
		if u, err := parseHTTPURL("Dashboard URL", response.StatusPageURL); err != nil || u.User != nil || u.RawQuery != "" {
			response.StatusPageURL = ""
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(response)
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

		var update alertConfigUpdate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&update); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		config := update.AlertConfig
		previous := alertMgr.GetConfig()
		mergeAlertConfigSecrets(&config, previous, update)

		if err := validateAlertConfigFields(&config, changedAlertURLs(&config, previous)); err != nil {
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
	return validateAlertConfigFields(config, alertURLValidation{discord: true, webhook: true, dashboard: true})
}

func mergeAlertConfigSecrets(config *models.AlertConfig, previous *models.AlertConfig, update alertConfigUpdate) {
	if previous == nil {
		previous = &models.AlertConfig{}
	}
	preserveSecret := func(value *string, oldValue string, clear bool) {
		if clear {
			*value = ""
		} else if *value == "" {
			*value = oldValue
		}
	}
	preserveURL := func(value *string, oldValue string, clear bool) {
		if clear {
			*value = ""
		} else if strings.TrimSpace(*value) == "" {
			*value = oldValue
		}
	}

	preserveSecret(&config.SMTPPassword, previous.SMTPPassword, update.ClearSMTPPassword)
	preserveURL(&config.DiscordWebhookURL, previous.DiscordWebhookURL, update.ClearDiscordWebhook)
	preserveSecret(&config.TelegramBotToken, previous.TelegramBotToken, update.ClearTelegramToken)
	preserveURL(&config.WebhookURL, previous.WebhookURL, update.ClearWebhookURL)
	preserveSecret(&config.WebhookSecret, previous.WebhookSecret, update.ClearWebhookSecret)
}

func changedAlertURLs(config, previous *models.AlertConfig) alertURLValidation {
	if previous == nil {
		return alertURLValidation{discord: true, webhook: true, dashboard: true}
	}
	return alertURLValidation{
		discord:   strings.TrimSpace(config.DiscordWebhookURL) != strings.TrimSpace(previous.DiscordWebhookURL) || (config.DiscordEnabled && !previous.DiscordEnabled),
		webhook:   strings.TrimSpace(config.WebhookURL) != strings.TrimSpace(previous.WebhookURL) || (config.WebhookEnabled && !previous.WebhookEnabled),
		dashboard: strings.TrimSpace(config.StatusPageURL) != strings.TrimSpace(previous.StatusPageURL),
	}
}

func validateAlertConfigFields(config *models.AlertConfig, validateURL alertURLValidation) error {
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
	if validateURL.webhook && config.WebhookURL != "" {
		// Generic providers commonly carry credentials in URL user-info, the
		// path, or a signed query string. Delivery converts user-info to Basic
		// Auth so credentials never remain in the outbound request URL.
		u, err := parseHTTPURL("Webhook URL", config.WebhookURL)
		if err != nil {
			return err
		}
		if u.User != nil && u.Scheme != "https" {
			return fmt.Errorf("Webhook URL must use HTTPS when it includes credentials")
		}
	}
	if validateURL.dashboard && config.StatusPageURL != "" {
		u, err := parseHTTPURL("Dashboard URL", config.StatusPageURL)
		if err != nil {
			return err
		}
		if u.User != nil {
			return fmt.Errorf("Dashboard URL must not include credentials")
		}
		if u.RawQuery != "" {
			return fmt.Errorf("Dashboard URL must not include query parameters")
		}
	}
	if validateURL.discord && config.DiscordWebhookURL != "" {
		u, err := parseHTTPURL("Discord webhook URL", config.DiscordWebhookURL)
		if err != nil {
			return err
		}
		host := strings.ToLower(u.Hostname())
		if u.User != nil || u.Scheme != "https" || (host != "discord.com" && host != "discordapp.com" && host != "canary.discord.com" && host != "ptb.discord.com") || (u.Port() != "" && u.Port() != "443") || !isDiscordWebhookPath(u.Path) {
			return fmt.Errorf("Discord webhook URL must be a complete HTTPS URL copied from Discord")
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

func parseHTTPURL(name, value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("%s must be a complete HTTP or HTTPS URL", name)
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("%s must not include a # fragment because fragments are not sent to webhook servers", name)
	}
	return u, nil
}

func isDiscordWebhookPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 4 {
		return parts[0] == "api" && parts[1] == "webhooks" && parts[2] != "" && parts[3] != ""
	}
	if len(parts) != 5 || parts[0] != "api" || parts[2] != "webhooks" || parts[3] == "" || parts[4] == "" {
		return false
	}
	version := parts[1]
	if len(version) < 2 || version[0] != 'v' {
		return false
	}
	for _, digit := range version[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
