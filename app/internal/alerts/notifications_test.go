package alerts

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"status/app/internal/database"
	"status/app/internal/models"
)

type notificationRoundTripper func(*http.Request) (*http.Response, error)

func (f notificationRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDiscordRichPayloadAndLimits(t *testing.T) {
	var payload map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wait") != "true" {
			t.Error("webhook must wait for delivery confirmation")
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	m := &Manager{config: &models.AlertConfig{DiscordWebhookURL: srv.URL, DiscordUsername: "Operations", DiscordSilent: true}}
	observedAt := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	details := notificationDetails{ObservedAt: observedAt, CheckType: "HTTP", HTTPStatus: sql.NullInt64{Int64: 200, Valid: true}, LatencyMS: sql.NullInt64{Int64: 84, Valid: true}, PreviousStatus: "down", PreviousSince: observedAt.Add(-5 * time.Minute)}
	if err := m.sendDiscord(strings.Repeat("😀", 400), "up", "@everyone *Service*", strings.Repeat("Long message &amp; ", 1000), "https://status.example.com", details); err != nil {
		t.Fatal(err)
	}
	if payload["username"] != "Operations" || payload["flags"] != float64(4096) {
		t.Fatalf("missing identity/silent setting: %v", payload)
	}
	mentions := payload["allowed_mentions"].(map[string]interface{})["parse"].([]interface{})
	if len(mentions) != 0 {
		t.Fatal("mentions must be disabled")
	}
	embed := payload["embeds"].([]interface{})[0].(map[string]interface{})
	if embed["url"] != "https://status.example.com" || embed["timestamp"] != observedAt.Format(time.RFC3339) {
		t.Fatalf("missing dashboard/time: %v", embed)
	}
	if len(utf16.Encode([]rune(embed["title"].(string)))) > 256 || len(utf16.Encode([]rune(embed["description"].(string)))) > 3500 {
		t.Fatal("embed exceeds Discord limits")
	}
	encoded, _ := json.Marshal(embed)
	for _, expected := range []string{"84 ms", "200", "5m0s", "HTTP", "Operational", "Unavailable"} {
		if !strings.Contains(string(encoded), expected) {
			t.Errorf("payload missing %q", expected)
		}
	}
}

func TestDiscordRetriesRateLimitAndReportsErrors(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if code == http.StatusTooManyRequests && calls == 2 {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(code)
				_, _ = io.WriteString(w, `{"retry_after":0.001,"message":"secret-token"}`)
			}))
			defer srv.Close()
			m := &Manager{config: &models.AlertConfig{DiscordWebhookURL: srv.URL + "/secret-token"}}
			err := m.SendDiscord("Test", "test", "Service", "Message", "")
			if code == http.StatusTooManyRequests {
				if err != nil || calls != 2 {
					t.Fatalf("expected rate-limit retry: calls=%d err=%v", calls, err)
				}
			} else if err == nil || strings.Contains(err.Error(), "secret-token") || calls != 1 {
				t.Fatalf("expected safe failure without retry: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestNotificationDeliveryRejectsRedirectsAndRedactsNetworkErrors(t *testing.T) {
	destinationCalls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls++ }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	m := &Manager{}
	if err := m.postNotification(redirect.URL, []byte(`{}`), nil, false); err == nil || destinationCalls != 0 {
		t.Fatalf("redirect must not forward payload: %v", err)
	}
	m.httpClient = &http.Client{Transport: notificationRoundTripper(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("request contains secret-token") })}
	err := m.postNotification("https://example.com/secret-token", []byte(`{}`), nil, false)
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("network error must not leak secret: %v", err)
	}
}

func TestGenericNotificationMovesURLCredentialsToBasicAuthHeader(t *testing.T) {
	called := false
	m := &Manager{httpClient: &http.Client{Transport: notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.URL.User != nil || strings.Contains(r.URL.String(), "api-user") || strings.Contains(r.URL.String(), "p%40ssword") {
			t.Fatalf("credentials remained in outbound request URL: %s", r.URL.Redacted())
		}
		user, password, ok := r.BasicAuth()
		if !ok || user != "api-user" || password != "p@ssword" {
			t.Fatalf("Basic Auth = (%q, %q, %v)", user, password, ok)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}}
	if err := m.postNotification("https://api-user:p%40ssword@hooks.example.com/events?sig=secret-token", []byte(`{}`), nil, false); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("notification request was not sent")
	}
}

func TestGenericNotificationRejectsCredentialsOverHTTP(t *testing.T) {
	called := false
	m := &Manager{httpClient: &http.Client{Transport: notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}}
	if err := m.postNotification("http://api-user:password@hooks.example.com/events", []byte(`{}`), nil, false); err == nil {
		t.Fatal("expected an HTTP webhook with credentials to be rejected")
	}
	if called {
		t.Fatal("notification with plaintext credentials was sent")
	}
}

func TestDiscordRetryBudget(t *testing.T) {
	for _, value := range []string{"0", "-1", "6", "NaN", "+Inf"} {
		if _, valid := discordRetryDelay(value, nil); valid {
			t.Errorf("accepted unbounded/invalid retry %q", value)
		}
	}
	calls := 0
	m := &Manager{httpClient: &http.Client{Transport: notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"retry_after":0.001}`)), Header: make(http.Header)}, nil
	})}}
	if err := m.postNotification("https://example.com", []byte(`{}`), nil, true); err == nil || calls != 3 {
		t.Fatalf("retry budget not enforced: calls=%d err=%v", calls, err)
	}
}

func TestDiscordStatusLinksRejectCredentialsAndQueries(t *testing.T) {
	for _, raw := range []string{"https://user:password@example.com", "https://example.com?token=secret", "javascript:alert(1)", "https://example.com#secret"} {
		if got := safeStatusPageURL(raw); got != "" {
			t.Errorf("unsafe link %q accepted as %q", raw, got)
		}
	}
}

func TestStatusTransitionDegradedRecoveryAndPartialRecovery(t *testing.T) {
	initTestDB(t)
	statuses := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Status string `json:"status"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		statuses <- payload.Status
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	m := &Manager{config: &models.AlertConfig{Enabled: true, AlertOnDown: true, AlertOnDegraded: true, AlertOnUp: true, WebhookEnabled: true, WebhookURL: srv.URL}}
	for _, state := range []struct {
		ok, degraded bool
		expected     string
	}{
		{true, true, "degraded"}, {true, false, "up"}, {false, false, "down"}, {true, true, "degraded"}, {true, false, "up"},
	} {
		if !m.CheckAndSendAlerts("service", "Service", state.ok, state.degraded) {
			t.Fatal("transition did not queue")
		}
		select {
		case got := <-statuses:
			if got != state.expected {
				t.Errorf("got %q, want %q", got, state.expected)
			}
		case <-time.After(time.Second):
			t.Fatal("notification never arrived")
		}
		if m.CheckAndSendAlerts("service", "Service", state.ok, state.degraded) {
			t.Error("duplicate state queued a notification")
		}
	}
}

func TestStatusHistoryPreservesTransitionTime(t *testing.T) {
	initTestDB(t)
	m := &Manager{}
	m.updateStatusHistory("service", false, false)
	_, err := database.DB.Exec(`UPDATE service_status_history SET updated_at='2026-01-01 00:00:00' WHERE service_key='service'`)
	if err != nil {
		t.Fatal(err)
	}
	m.updateStatusHistory("service", false, false)
	var since string
	_ = database.DB.QueryRow(`SELECT updated_at FROM service_status_history WHERE service_key='service'`).Scan(&since)
	if since != "2026-01-01 00:00:00" {
		t.Fatalf("stable check reset status duration: %q", since)
	}
	m.updateStatusHistory("service", true, false)
	_ = database.DB.QueryRow(`SELECT updated_at FROM service_status_history WHERE service_key='service'`).Scan(&since)
	if since == "2026-01-01 00:00:00" {
		t.Fatal("transition did not reset status duration")
	}
}

func TestNotificationDetailsUseLatestSafeCheckMetadata(t *testing.T) {
	initTestDB(t)
	_, err := database.CreateService(&models.ServiceConfig{Key: "service", Name: "Service", URL: "http://user:secret@internal.example:8080/private?api_key=secret", CheckType: "http", APIToken: "secret", ServiceType: "custom"})
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Now().UTC().Truncate(time.Second)
	latency := 320
	database.InsertSample(observedAt.Add(-time.Minute), "service", false, 503, nil)
	database.InsertSample(observedAt, "service", true, 200, &latency)
	details := loadNotificationDetails("service", observedAt)
	if details.CheckType != "HTTP" || details.HTTPStatus.Int64 != 200 || details.LatencyMS.Int64 != 320 || !details.ObservedAt.Equal(observedAt) {
		t.Fatalf("incorrect latest check: %+v", details)
	}
	encoded, _ := json.Marshal(details)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "internal.example") {
		t.Fatal("notification metadata contains private monitor configuration")
	}
}
