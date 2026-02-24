package alerts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
	"testing"
)

// initTestDB sets up an in-memory SQLite database for testing.
func initTestDB(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
}

// --------------- normalizeStatusPageURL tests ---------------

func TestNormalizeStatusPageURL_Empty(t *testing.T) {
	if got := normalizeStatusPageURL(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeStatusPageURL_WithHTTP(t *testing.T) {
	got := normalizeStatusPageURL("http://example.com")
	if got != "http://example.com" {
		t.Errorf("got %q, want http://example.com", got)
	}
}

func TestNormalizeStatusPageURL_WithHTTPS(t *testing.T) {
	got := normalizeStatusPageURL("https://example.com")
	if got != "https://example.com" {
		t.Errorf("got %q, want https://example.com", got)
	}
}

func TestNormalizeStatusPageURL_NoProtocol(t *testing.T) {
	got := normalizeStatusPageURL("example.com")
	if got != "http://example.com" {
		t.Errorf("got %q, want http://example.com", got)
	}
}

func TestNormalizeStatusPageURL_WithWhitespace(t *testing.T) {
	got := normalizeStatusPageURL("  http://example.com  ")
	if got != "http://example.com" {
		t.Errorf("got %q, want http://example.com", got)
	}
}

func TestNormalizeStatusPageURL_OnlySpaces(t *testing.T) {
	got := normalizeStatusPageURL("   ")
	if got != "" {
		t.Errorf("expected empty for whitespace-only, got %q", got)
	}
}

// --------------- boolToInt tests ---------------

func TestBoolToInt_True(t *testing.T) {
	if got := boolToInt(true); got != 1 {
		t.Errorf("expected 1 for true, got %d", got)
	}
}

func TestBoolToInt_False(t *testing.T) {
	if got := boolToInt(false); got != 0 {
		t.Errorf("expected 0 for false, got %d", got)
	}
}

// --------------- Manager basic tests ---------------

func TestManager_GetConfig_Nil(t *testing.T) {
	m := &Manager{}
	if m.GetConfig() != nil {
		t.Error("expected nil config")
	}
}

func TestManager_GetConfig_Set(t *testing.T) {
	cfg := &models.AlertConfig{Enabled: true, SMTPHost: "smtp.test.com"}
	m := &Manager{config: cfg}
	got := m.GetConfig()
	if got != cfg {
		t.Error("GetConfig should return the same pointer")
	}
	if !got.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestManager_SetConfig(t *testing.T) {
	m := &Manager{}
	cfg := &models.AlertConfig{Enabled: true}
	m.SetConfig(cfg)
	if m.config != cfg {
		t.Error("SetConfig should update internal config")
	}
}

func TestManager_GetStatusPageURL(t *testing.T) {
	m := &Manager{statusPageURL: "http://status.example.com"}
	if got := m.GetStatusPageURL(); got != "http://status.example.com" {
		t.Errorf("got %q", got)
	}
}

func TestManager_GetStatusPageURL_Empty(t *testing.T) {
	m := &Manager{}
	if got := m.GetStatusPageURL(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --------------- ResolveStatusPageURL tests ---------------

func TestResolveStatusPageURL_FromConfig(t *testing.T) {
	m := &Manager{
		config:        &models.AlertConfig{StatusPageURL: "https://config-url.com"},
		statusPageURL: "https://env-url.com",
	}
	got := m.ResolveStatusPageURL("https://fallback.com")
	if got != "https://config-url.com" {
		t.Errorf("got %q, should prefer config URL", got)
	}
}

func TestResolveStatusPageURL_FromEnv(t *testing.T) {
	m := &Manager{
		config:        &models.AlertConfig{StatusPageURL: ""},
		statusPageURL: "https://env-url.com",
	}
	got := m.ResolveStatusPageURL("https://fallback.com")
	if got != "https://env-url.com" {
		t.Errorf("got %q, should fall back to env URL", got)
	}
}

func TestResolveStatusPageURL_FromFallback(t *testing.T) {
	m := &Manager{
		config:        &models.AlertConfig{StatusPageURL: ""},
		statusPageURL: "",
	}
	got := m.ResolveStatusPageURL("https://fallback.com")
	if got != "https://fallback.com" {
		t.Errorf("got %q, should fall back to fallback URL", got)
	}
}

func TestResolveStatusPageURL_NilConfig(t *testing.T) {
	m := &Manager{
		config:        nil,
		statusPageURL: "https://env-url.com",
	}
	got := m.ResolveStatusPageURL("https://fallback.com")
	if got != "https://env-url.com" {
		t.Errorf("got %q, should use env URL when config is nil", got)
	}
}

func TestResolveStatusPageURL_AllEmpty(t *testing.T) {
	m := &Manager{config: &models.AlertConfig{}, statusPageURL: ""}
	got := m.ResolveStatusPageURL("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestResolveStatusPageURL_NormalizesNoProtocol(t *testing.T) {
	m := &Manager{
		config: &models.AlertConfig{StatusPageURL: "mysite.com"},
	}
	got := m.ResolveStatusPageURL("")
	if got != "http://mysite.com" {
		t.Errorf("got %q, should have added http://", got)
	}
}

// --------------- CreateHTMLEmail tests ---------------

func TestCreateHTMLEmail_ContainsSubject(t *testing.T) {
	html := CreateHTMLEmail("🔴 Service Down: Test", "down", "TestService", "test-svc", "Service is down", "http://example.com")
	if !strings.Contains(html, "🔴 Service Down: Test") {
		t.Error("HTML should contain subject")
	}
}

func TestCreateHTMLEmail_ContainsServiceName(t *testing.T) {
	html := CreateHTMLEmail("Test", "down", "MyService", "my-svc", "msg", "http://example.com")
	if !strings.Contains(html, "MyService") {
		t.Error("HTML should contain service name")
	}
}

func TestCreateHTMLEmail_StatusDown_Color(t *testing.T) {
	html := CreateHTMLEmail("Test", "down", "Svc", "svc", "msg", "")
	if !strings.Contains(html, "#ef4444") {
		t.Error("down status should use red color #ef4444")
	}
	if !strings.Contains(html, "SERVICE DOWN") {
		t.Error("should contain SERVICE DOWN text")
	}
}

func TestCreateHTMLEmail_StatusDegraded_Color(t *testing.T) {
	html := CreateHTMLEmail("Test", "degraded", "Svc", "svc", "msg", "")
	if !strings.Contains(html, "#eab308") {
		t.Error("degraded status should use yellow color #eab308")
	}
	if !strings.Contains(html, "SERVICE DEGRADED") {
		t.Error("should contain SERVICE DEGRADED text")
	}
}

func TestCreateHTMLEmail_StatusUp_Color(t *testing.T) {
	html := CreateHTMLEmail("Test", "up", "Svc", "svc", "msg", "")
	if !strings.Contains(html, "#22c55e") {
		t.Error("up status should use green color #22c55e")
	}
	if !strings.Contains(html, "SERVICE UP") {
		t.Error("should contain SERVICE UP text")
	}
}

func TestCreateHTMLEmail_StatusPageURL(t *testing.T) {
	html := CreateHTMLEmail("Test", "up", "Svc", "svc", "msg", "https://status.test.com")
	if !strings.Contains(html, "https://status.test.com") {
		t.Error("HTML should contain the status page URL")
	}
}

func TestCreateHTMLEmail_EmptyStatusPageURL(t *testing.T) {
	html := CreateHTMLEmail("Test", "up", "Svc", "svc", "msg", "")
	// Should use "#" as fallback
	if !strings.Contains(html, `href="#"`) {
		t.Error("empty URL should default to '#'")
	}
}

func TestCreateHTMLEmail_HTMLStructure(t *testing.T) {
	html := CreateHTMLEmail("Test", "down", "Svc", "svc", "msg", "")
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("should contain DOCTYPE")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("should contain closing html tag")
	}
	if !strings.Contains(html, "Servicarr") {
		t.Error("should contain Servicarr brand")
	}
}

// --------------- NewManager with DB tests ---------------

func TestNewManager_NoAlertConfig(t *testing.T) {
	initTestDB(t)
	m := NewManager("http://status.local")
	if m == nil {
		t.Fatal("NewManager should not return nil")
	}
	if m.config != nil {
		t.Error("config should be nil when no alert_config row exists")
	}
	if m.statusPageURL != "http://status.local" {
		t.Errorf("statusPageURL = %q", m.statusPageURL)
	}
}

func TestNewManager_WithAlertConfig(t *testing.T) {
	initTestDB(t)
	cfg := &models.AlertConfig{
		Enabled:  true,
		SMTPHost: "smtp.test.com",
		SMTPPort: 587,
	}
	if err := database.SaveAlertConfig(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	m := NewManager("http://status.local")
	if m.config == nil {
		t.Fatal("config should be loaded from DB")
	}
	if !m.config.Enabled {
		t.Error("expected enabled=true")
	}
	if m.config.SMTPHost != "smtp.test.com" {
		t.Errorf("smtp_host = %q", m.config.SMTPHost)
	}
}

// --------------- ReloadConfig tests ---------------

func TestReloadConfig(t *testing.T) {
	initTestDB(t)
	m := NewManager("")
	if m.config != nil {
		t.Error("should start with nil config")
	}

	// Save a config
	cfg := &models.AlertConfig{Enabled: true, AlertEmail: "test@test.com"}
	database.SaveAlertConfig(cfg)

	if err := m.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig error: %v", err)
	}
	if m.config == nil {
		t.Fatal("config should be loaded after reload")
	}
	if m.config.AlertEmail != "test@test.com" {
		t.Errorf("alert_email = %q", m.config.AlertEmail)
	}
}

// --------------- CheckAndSendAlerts tests ---------------

func TestCheckAndSendAlerts_DisabledConfig(t *testing.T) {
	initTestDB(t)
	m := &Manager{config: &models.AlertConfig{Enabled: false}}
	// Should do nothing, not panic
	m.CheckAndSendAlerts("svc1", "Svc One", false, false)
}

func TestCheckAndSendAlerts_NilConfig(t *testing.T) {
	initTestDB(t)
	m := &Manager{config: nil}
	// Should do nothing, not panic
	m.CheckAndSendAlerts("svc1", "Svc One", false, false)
}

func TestCheckAndSendAlerts_FirstTimeDown(t *testing.T) {
	initTestDB(t)
	// Use a webhook server to verify an alert is dispatched
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			Enabled:        true,
			AlertOnDown:    true,
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
		},
	}

	m.CheckAndSendAlerts("test-svc", "Test Service", false, false)
	// dispatchAll sends via goroutine, give it time
	waitForCondition(t, func() bool { return received }, "webhook should have been called for first-time down")
}

func TestCheckAndSendAlerts_FirstTimeDegraded(t *testing.T) {
	initTestDB(t)
	var received bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			Enabled:        true,
			AlertOnDegraded: true,
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
		},
	}

	m.CheckAndSendAlerts("deg-svc", "Degraded Service", true, true)
	waitForCondition(t, func() bool { return received }, "webhook should have been called for first-time degraded")
}

func TestCheckAndSendAlerts_StatusChangeDownToUp(t *testing.T) {
	initTestDB(t)
	var callCount int
	var lastPayload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &lastPayload)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			Enabled:        true,
			AlertOnDown:    true,
			AlertOnUp:      true,
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
		},
	}

	// First: service goes down
	m.CheckAndSendAlerts("transition-svc", "Transition Svc", false, false)
	waitForCondition(t, func() bool { return callCount >= 1 }, "down alert should fire")

	// Second: service recovers
	m.CheckAndSendAlerts("transition-svc", "Transition Svc", true, false)
	waitForCondition(t, func() bool { return callCount >= 2 }, "recovery alert should fire")

	if lastPayload["status"] != "up" {
		t.Errorf("last webhook status = %v, want 'up'", lastPayload["status"])
	}
}

func TestCheckAndSendAlerts_NoAlertWhenNoChange(t *testing.T) {
	initTestDB(t)
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			Enabled:        true,
			AlertOnDown:    true,
			AlertOnUp:      true,
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
		},
	}

	// First call: down → triggers alert
	m.CheckAndSendAlerts("stable-svc", "Stable Svc", false, false)
	waitForCondition(t, func() bool { return callCount >= 1 }, "initial down alert")

	beforeCount := callCount
	// Second call: still down → no change → NO alert
	m.CheckAndSendAlerts("stable-svc", "Stable Svc", false, false)
	// Wait briefly to make sure no extra call happens
	waitBriefly()
	if callCount != beforeCount {
		t.Errorf("should not send alert when status unchanged, got %d extra calls", callCount-beforeCount)
	}
}

func TestCheckAndSendAlerts_DependencySuppression(t *testing.T) {
	initTestDB(t)

	// Create upstream and downstream services
	upstream := &models.ServiceConfig{
		Key: "upstream-svc", Name: "Upstream", URL: "http://upstream",
		ServiceType: "custom", CheckType: "http", CheckInterval: 60, Timeout: 5,
		ExpectedMin: 200, ExpectedMax: 399, Visible: true, DisplayOrder: 0,
	}
	database.CreateService(upstream)

	downstream := &models.ServiceConfig{
		Key: "downstream-svc", Name: "Downstream", URL: "http://downstream",
		ServiceType: "custom", CheckType: "http", CheckInterval: 60, Timeout: 5,
		ExpectedMin: 200, ExpectedMax: 399, Visible: true, DisplayOrder: 1,
		DependsOn: "upstream-svc",
	}
	database.CreateService(downstream)

	// Mark upstream as down in status history
	database.DB.Exec(`INSERT INTO service_status_history (service_key, ok, degraded, updated_at) VALUES ('upstream-svc', 0, 0, datetime('now'))`)

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			Enabled:        true,
			AlertOnDown:    true,
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
		},
	}

	// Downstream goes down, but upstream is already down → suppress
	m.CheckAndSendAlerts("downstream-svc", "Downstream", false, false)
	waitBriefly()
	if callCount != 0 {
		t.Errorf("alert should be suppressed when upstream dependency is down, got %d calls", callCount)
	}
}

// --------------- SendDiscord tests ---------------

func TestSendDiscord(t *testing.T) {
	initTestDB(t)
	var receivedPayload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			DiscordEnabled:    true,
			DiscordWebhookURL: srv.URL,
		},
	}

	m.SendDiscord("🔴 Down: Nginx", "down", "Nginx", "Service is unreachable", "http://status.test")

	if receivedPayload["username"] != "Servicarr" {
		t.Errorf("username = %v, want Servicarr", receivedPayload["username"])
	}
	embeds := receivedPayload["embeds"].([]interface{})
	if len(embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(embeds))
	}
	embed := embeds[0].(map[string]interface{})
	if embed["title"] != "🔴 Down: Nginx" {
		t.Errorf("embed title = %v", embed["title"])
	}
}

// --------------- SendTelegram tests ---------------

func TestSendTelegram(t *testing.T) {
	initTestDB(t)
	var receivedPayload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Replace the Telegram API URL by using a token that includes the server URL
	// Since SendTelegram constructs: https://api.telegram.org/bot{token}/sendMessage
	// We'll test via the mock server approach differently
	m := &Manager{
		config: &models.AlertConfig{
			TelegramEnabled:  true,
			TelegramBotToken: "testtoken",
			TelegramChatID:   "12345",
		},
	}

	// Since Telegram sends to api.telegram.org, we can't easily mock it with httptest
	// without changing the production code. Test that it doesn't panic.
	// (Integration test would use a real Telegram bot)
	m.SendTelegram("Test", "down", "Svc", "msg")
	// No assertion needed - just ensure no panic
}

// --------------- SendWebhook tests ---------------

func TestSendWebhook_BasicPayload(t *testing.T) {
	initTestDB(t)
	var receivedPayload map[string]interface{}
	var receivedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
		},
	}

	m.SendWebhook("Service Down", "down", "Nginx", "nginx-svc", "Service is down")

	if receivedPayload["event"] != "status_change" {
		t.Errorf("event = %v", receivedPayload["event"])
	}
	if receivedPayload["service_key"] != "nginx-svc" {
		t.Errorf("service_key = %v", receivedPayload["service_key"])
	}
	if receivedPayload["status"] != "down" {
		t.Errorf("status = %v", receivedPayload["status"])
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Error("missing Content-Type header")
	}
	if receivedHeaders.Get("User-Agent") != "Servicarr/1.0" {
		t.Error("missing User-Agent header")
	}
}

func TestSendWebhook_WithHMACSignature(t *testing.T) {
	initTestDB(t)
	var receivedBody []byte
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Servicarr-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	secret := "my-webhook-secret"
	m := &Manager{
		config: &models.AlertConfig{
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
			WebhookSecret:  secret,
		},
	}

	m.SendWebhook("Test", "up", "Svc", "svc-key", "recovered")

	// Verify HMAC
	if receivedSig == "" {
		t.Fatal("expected X-Servicarr-Signature header")
	}
	if !strings.HasPrefix(receivedSig, "sha256=") {
		t.Fatalf("sig should start with sha256=, got %q", receivedSig)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(receivedBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if receivedSig != expected {
		t.Errorf("HMAC mismatch: got %q, want %q", receivedSig, expected)
	}
}

func TestSendWebhook_NoHMACWithoutSecret(t *testing.T) {
	initTestDB(t)
	var receivedSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Servicarr-Signature")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	m := &Manager{
		config: &models.AlertConfig{
			WebhookEnabled: true,
			WebhookURL:     srv.URL,
			WebhookSecret:  "",
		},
	}

	m.SendWebhook("Test", "down", "Svc", "svc-key", "msg")

	if receivedSig != "" {
		t.Errorf("should not include signature when no secret, got %q", receivedSig)
	}
}

// --------------- SendEmail tests ---------------

func TestSendEmail_ConfigDisabled(t *testing.T) {
	m := &Manager{config: &models.AlertConfig{Enabled: false}}
	err := m.SendEmail("subject", "body")
	if err != nil {
		t.Errorf("should return nil when disabled, got %v", err)
	}
}

func TestSendEmail_NilConfig(t *testing.T) {
	m := &Manager{config: nil}
	err := m.SendEmail("subject", "body")
	if err != nil {
		t.Errorf("should return nil for nil config, got %v", err)
	}
}

func TestSendEmail_MissingHost(t *testing.T) {
	m := &Manager{config: &models.AlertConfig{Enabled: true, SMTPHost: "", AlertEmail: "test@test.com"}}
	err := m.SendEmail("subject", "body")
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("should return incomplete error, got %v", err)
	}
}

func TestSendEmail_MissingEmail(t *testing.T) {
	m := &Manager{config: &models.AlertConfig{Enabled: true, SMTPHost: "smtp.test.com", AlertEmail: ""}}
	err := m.SendEmail("subject", "body")
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("should return incomplete error, got %v", err)
	}
}

// --------------- updateStatusHistory tests ---------------

func TestUpdateStatusHistory(t *testing.T) {
	initTestDB(t)
	m := &Manager{config: &models.AlertConfig{Enabled: true}}

	// First insert
	m.updateStatusHistory("svc1", true, false)

	var ok, degraded int
	err := database.DB.QueryRow(`SELECT ok, degraded FROM service_status_history WHERE service_key = 'svc1'`).Scan(&ok, &degraded)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if ok != 1 || degraded != 0 {
		t.Errorf("ok=%d, degraded=%d, want 1, 0", ok, degraded)
	}

	// Update (conflict resolution)
	m.updateStatusHistory("svc1", false, true)
	err = database.DB.QueryRow(`SELECT ok, degraded FROM service_status_history WHERE service_key = 'svc1'`).Scan(&ok, &degraded)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if ok != 0 || degraded != 1 {
		t.Errorf("after update: ok=%d, degraded=%d, want 0, 1", ok, degraded)
	}
}

// --------------- helpers ---------------

func waitForCondition(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if cond() {
			return
		}
		// brief sleep
		sleepBriefly()
	}
	t.Errorf("timed out: %s", msg)
}

func waitBriefly() {
	sleepBriefly()
	sleepBriefly()
	sleepBriefly()
}

func sleepBriefly() {
	// Use a channel-based sleep to avoid importing time in a way that makes
	// the test non-deterministic. 50ms is enough for goroutine scheduling.
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		// Small spin to yield
		for i := 0; i < 5000000; i++ {
			_ = i
		}
	}()
	<-ch
}
