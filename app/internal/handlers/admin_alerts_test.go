package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"status/app/internal/alerts"
	"status/app/internal/database"
	"status/app/internal/models"
)

func TestNotificationTestReportsDeliveryOutcome(t *testing.T) {
	initMaintenanceHandlerDB(t)
	for _, channel := range []string{"discord", "webhook"} {
		for _, code := range []int{http.StatusNoContent, http.StatusUnauthorized} {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
			manager := alerts.NewManager("")
			manager.SetConfig(&models.AlertConfig{DiscordWebhookURL: srv.URL, WebhookURL: srv.URL})
			req := httptest.NewRequest(http.MethodPost, "/api/admin/alerts/test-channel", strings.NewReader(`{"channel":"`+channel+`","status":"down"}`))
			w := httptest.NewRecorder()
			HandleTestNotification(manager)(w, req)
			srv.Close()
			var result struct {
				Success bool `json:"success"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if code == http.StatusNoContent {
				if w.Code != http.StatusOK || !result.Success {
					t.Fatalf("successful delivery reported failure: %d %s", w.Code, w.Body)
				}
			} else if w.Code != http.StatusBadGateway || result.Success {
				t.Fatalf("failed delivery reported success: %d %s", w.Code, w.Body)
			}
		}
	}
}

func TestNotificationConfigPersistsDiscordOptions(t *testing.T) {
	initMaintenanceHandlerDB(t)
	manager := alerts.NewManager("")
	req := httptest.NewRequest(http.MethodPost, "/api/admin/alerts/config", strings.NewReader(`{"enabled":true,"discord_enabled":true,"discord_webhook_url":"https://discord.com/api/webhooks/123/token","discord_username":"Operations","discord_silent":true,"alert_on_up":true}`))
	w := httptest.NewRecorder()
	HandleSaveAlertsConfig(manager)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save failed: %d %s", w.Code, w.Body)
	}
	config, err := database.LoadAlertConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.DiscordUsername != "Operations" || !config.DiscordSilent || !config.AlertOnUp {
		t.Fatalf("Discord options not persisted: %+v", config)
	}
	if manager.GetConfig().DiscordUsername != "Operations" {
		t.Fatal("in-memory configuration not updated")
	}
}

func TestNotificationConfigAcceptsSecretBearingWebhookURLs(t *testing.T) {
	initMaintenanceHandlerDB(t)
	manager := alerts.NewManager("")
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"signed query", "https://hooks.example.com/events?key=public&sig=secret-token", "https://hooks.example.com/events?key=public&sig=secret-token"},
		{"secret path", "  https://hooks.example.com/webhook/secret-token  ", "https://hooks.example.com/webhook/secret-token"},
		{"HTTP Basic credentials", "https://api-user:p%40ssword@hooks.example.com/events", "https://api-user:p%40ssword@hooks.example.com/events"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(models.AlertConfig{WebhookEnabled: true, WebhookURL: test.url})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/admin/alerts/config", strings.NewReader(string(payload)))
			w := httptest.NewRecorder()
			HandleSaveAlertsConfig(manager)(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("save failed: %d %s", w.Code, w.Body)
			}
			if got := manager.GetConfig().WebhookURL; got != test.want {
				t.Fatalf("saved URL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNotificationConfigAcceptsVersionedDiscordWebhookURL(t *testing.T) {
	config := models.AlertConfig{
		DiscordEnabled:    true,
		DiscordWebhookURL: "https://discord.com/api/v10/webhooks/123456/secret-token?thread_id=789",
	}
	if err := validateAlertConfig(&config); err != nil {
		t.Fatalf("valid Discord webhook rejected: %v", err)
	}
}

func TestNotificationConfigResponseRedactsSecrets(t *testing.T) {
	initMaintenanceHandlerDB(t)
	manager := alerts.NewManager("")
	manager.SetConfig(&models.AlertConfig{
		SMTPPassword:      "smtp-password-unique",
		DiscordWebhookURL: "https://discord.com/api/webhooks/123/discord-token-unique",
		TelegramBotToken:  "telegram-token-unique",
		WebhookURL:        "https://api-user:webhook-password-unique@hooks.example.com/events?sig=query-secret-unique",
		WebhookSecret:     "signing-secret-unique",
		StatusPageURL:     "https://status.example.com?token=dashboard-secret-unique",
	})
	req := httptest.NewRequest(http.MethodGet, "/api/admin/alerts/config", nil)
	w := httptest.NewRecorder()
	HandleGetAlertsConfig(manager)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get failed: %d %s", w.Code, w.Body)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	body := w.Body.String()
	for _, secret := range []string{"smtp-password-unique", "discord-token-unique", "telegram-token-unique", "webhook-password-unique", "query-secret-unique", "signing-secret-unique", "dashboard-secret-unique"} {
		if strings.Contains(body, secret) {
			t.Fatalf("configuration response exposed %q", secret)
		}
	}
	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"smtp_password", "discord_webhook_url", "telegram_bot_token", "webhook_url", "webhook_secret"} {
		if _, exposed := response[field]; exposed {
			t.Errorf("configuration response contains secret field %q", field)
		}
	}
	for _, field := range []string{"smtp_password_configured", "discord_webhook_configured", "telegram_bot_token_configured", "webhook_url_configured", "webhook_secret_configured"} {
		if response[field] != true {
			t.Errorf("%s = %v, want true", field, response[field])
		}
	}
}

func TestNotificationConfigBlankSecretsPreserveSavedValues(t *testing.T) {
	initMaintenanceHandlerDB(t)
	manager := alerts.NewManager("")
	manager.SetConfig(&models.AlertConfig{
		SMTPPassword:      "smtp-secret",
		DiscordWebhookURL: "https://discord.com/api/webhooks/123/discord-secret",
		TelegramBotToken:  "telegram-secret",
		WebhookURL:        "https://hooks.example.com/original-secret",
		WebhookSecret:     "signing-secret",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/alerts/config", strings.NewReader(`{"discord_username":"Operations"}`))
	w := httptest.NewRecorder()
	HandleSaveAlertsConfig(manager)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save failed: %d %s", w.Code, w.Body)
	}
	got := manager.GetConfig()
	if got.SMTPPassword != "smtp-secret" || got.DiscordWebhookURL != "https://discord.com/api/webhooks/123/discord-secret" || got.TelegramBotToken != "telegram-secret" || got.WebhookURL != "https://hooks.example.com/original-secret" || got.WebhookSecret != "signing-secret" {
		t.Fatalf("blank replacement fields did not preserve secrets: %+v", got)
	}

	clearReq := httptest.NewRequest(http.MethodPost, "/api/admin/alerts/config", strings.NewReader(`{"clear_webhook_secret":true}`))
	clearW := httptest.NewRecorder()
	HandleSaveAlertsConfig(manager)(clearW, clearReq)
	if clearW.Code != http.StatusOK || manager.GetConfig().WebhookSecret != "" {
		t.Fatalf("explicit clear failed: %d %+v", clearW.Code, manager.GetConfig())
	}
}

func TestDisabledLegacyProviderDoesNotBlockWebhookSave(t *testing.T) {
	initMaintenanceHandlerDB(t)
	manager := alerts.NewManager("")
	manager.SetConfig(&models.AlertConfig{DiscordWebhookURL: "http://legacy.invalid/old-discord-hook"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/alerts/config", strings.NewReader(`{"webhook_enabled":true,"webhook_url":"https://api-user:secret@hooks.example.com/events"}`))
	w := httptest.NewRecorder()
	HandleSaveAlertsConfig(manager)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unrelated legacy URL blocked save: %d %s", w.Code, w.Body)
	}
	if got := manager.GetConfig().WebhookURL; got != "https://api-user:secret@hooks.example.com/events" {
		t.Fatalf("webhook URL = %q", got)
	}
}

func TestNotificationConfigRejectsUnsafeURLs(t *testing.T) {
	for _, config := range []models.AlertConfig{
		{DiscordWebhookURL: "https://discord.com.evil.example/api/webhooks/123/token"},
		{DiscordWebhookURL: "https://discord.com/other-endpoint"},
		{DiscordWebhookURL: "https://discord.com/api/webhooks/123/token/extra"},
		{DiscordWebhookURL: "https://user:password@discord.com/api/webhooks/123/token"},
		{WebhookURL: "file:///etc/passwd"},
		{WebhookURL: "http://user:password@hooks.example.com/events"},
		{WebhookURL: "https://hooks.example.com/webhook#secret"},
		{StatusPageURL: "https://user:password@status.example.com"},
		{StatusPageURL: "https://status.example.com?token=secret"},
		{DiscordUsername: strings.Repeat("x", 81)},
		{SMTPHost: "mail.example.com", SMTPPort: 65536},
	} {
		if err := validateAlertConfig(&config); err == nil {
			t.Errorf("unsafe configuration accepted: %+v", config)
		}
	}
}
