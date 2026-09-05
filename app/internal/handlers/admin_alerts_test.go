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

func TestNotificationConfigRejectsUnsafeURLs(t *testing.T) {
	for _, config := range []models.AlertConfig{
		{DiscordWebhookURL: "https://discord.com.evil.example/api/webhooks/123/token"},
		{DiscordWebhookURL: "https://discord.com/other-endpoint"},
		{WebhookURL: "file:///etc/passwd"},
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
