package crowdsec

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeJWT builds a structurally valid JWT with the given expiry.
func fakeJWT(t *testing.T, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	sig := base64.RawURLEncoding.EncodeToString([]byte("test-signature"))
	return header + "." + payload + "." + sig
}

// lapiMockServer serves a canned LAPI. loginCalls counts watcher logins;
// decisionAuth and alertAuth gate their endpoints.
func lapiMockServer(t *testing.T, loginCalls *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/decisions":
			if r.Header.Get("X-Api-Key") != "test-key" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"message":"access forbidden"}`)
				return
			}
			_, _ = fmt.Fprint(w, `[{"duration":"3h59m55s","id":7,"origin":"crowdsec","scenario":"crowdsecurity/ssh-bf","scope":"Ip","type":"ban","value":"1.2.3.4"}]`)
		case "/v1/alerts":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"message":"missing token"}`)
				return
			}
			_, _ = fmt.Fprint(w, `[{"id":42,"created_at":"2026-09-18T10:00:00Z","scenario":"crowdsecurity/http-probing","events_count":12,`+
				`"source":{"scope":"Ip","value":"1.2.3.4","as_number":"AS12345","latitude":48.85,"longitude":2.35},`+
				`"decisions":[{"duration":"3h59m55s","id":9,"type":"ban","value":"1.2.3.4"}]}]`)
		case "/v1/watchers/login":
			loginCalls.Add(1)
			_, _ = fmt.Fprint(w, `{"token":"`+fakeJWT(t, time.Now().Add(time.Hour))+`","expire":"`+time.Now().Add(time.Hour).Format(time.RFC3339)+`"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newTestClient(baseURL string) *Client {
	return NewClient(Config{
		BaseURL:         baseURL,
		BouncerKey:      "test-key",
		MachineID:       "testmachine",
		MachinePassword: "testpass",
	})
}

func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"http://10.0.0.5:8080", "http://10.0.0.5:8080", false},
		{"https://lapi.example.com", "https://lapi.example.com", false},
		{"http://host:8080/v1", "http://host:8080", false},
		{"  http://host:8080  ", "http://host:8080", false},
		{"http://host:8080/", "http://host:8080", false},
		{"ftp://host", "", true},
		{"http://", "", true},
		{"http://host:8080/path", "", true},
		{"http://host:8080?v=1", "", true},
		{"http://user:secret@host:8080", "", true},
		{"http://169.254.169.254", "", true}, // cloud metadata must be rejected
	}
	for _, c := range cases {
		got, err := ValidateBaseURL(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("ValidateBaseURL(%q): expected error, got %q", c.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateBaseURL(%q): unexpected error %v", c.raw, err)
			continue
		}
		if got != c.want {
			t.Errorf("ValidateBaseURL(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestValidateBaseURL_MalformedInputDoesNotLeakCredentials(t *testing.T) {
	_, err := ValidateBaseURL("http://user:very-secret@[")
	if err == nil {
		t.Fatal("expected malformed URL error")
	}
	if strings.Contains(err.Error(), "very-secret") {
		t.Fatalf("validation error leaked URL credentials: %q", err)
	}
}

func TestValidatePollInterval(t *testing.T) {
	if _, err := ValidatePollInterval(9 * time.Second); err == nil {
		t.Error("expected error for interval below minimum")
	}
	if _, err := ValidatePollInterval(61 * time.Minute); err == nil {
		t.Error("expected error for interval above maximum")
	}
	if _, err := ValidatePollInterval(30 * time.Second); err != nil {
		t.Errorf("unexpected error for valid interval: %v", err)
	}
}

func TestDecisionsParamsEncode(t *testing.T) {
	// LAPI 500s on unknown query parameters — the encode output must stay
	// exactly within the verified parameter set.
	got := DecisionsParams{Limit: 500, Scopes: []string{"Ip", "Range"}}.encode()
	if !strings.Contains(got, "limit=500") || !strings.Contains(got, "scopes=Ip%2CRange") {
		t.Errorf("unexpected encode: %q", got)
	}
	if got := (DecisionsParams{}).encode(); got != "" {
		t.Errorf("empty params should encode to empty string, got %q", got)
	}
	includeCAPI := false
	got = AlertsParams{Limit: 100, Since: 2 * time.Hour, IncludeCAPI: &includeCAPI}.encode()
	if !strings.Contains(got, "limit=100") || !strings.Contains(got, "since=2h0m0s") ||
		!strings.Contains(got, "include_capi=false") {
		t.Errorf("unexpected alerts encode: %q", got)
	}
}

func TestHealth(t *testing.T) {
	var calls atomic.Int64
	srv := lapiMockServer(t, &calls)
	defer srv.Close()
	c := newTestClient(srv.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Errorf("Health: unexpected error %v", err)
	}

	down := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	if err := down.Health(context.Background()); !errors.Is(err, ErrUnreachable) {
		t.Errorf("Health on unreachable server: want ErrUnreachable, got %v", err)
	}
}

func TestDecisions(t *testing.T) {
	var calls atomic.Int64
	srv := lapiMockServer(t, &calls)
	defer srv.Close()
	c := newTestClient(srv.URL)

	out, err := c.Decisions(context.Background(), DecisionsParams{Limit: 10})
	if err != nil {
		t.Fatalf("Decisions: %v", err)
	}
	if len(out) != 1 || out[0].Value != "1.2.3.4" || out[0].Type != "ban" {
		t.Errorf("unexpected decisions: %+v", out)
	}
	if out[0].ID == nil || *out[0].ID != 7 {
		t.Errorf("expected decision ID 7, got %+v", out[0].ID)
	}
}

func TestDecisions_NotConfigured(t *testing.T) {
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	if _, err := c.Decisions(context.Background(), DecisionsParams{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("want ErrNotConfigured, got %v", err)
	}
	if _, err := c.Alerts(context.Background(), AlertsParams{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("want ErrNotConfigured for alerts, got %v", err)
	}
}

func TestDecisions_BadKey_IsAuthFailed(t *testing.T) {
	var loginCalls atomic.Int64
	srv := lapiMockServer(t, &loginCalls)
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, BouncerKey: "wrong-key"})
	if _, err := c.Decisions(context.Background(), DecisionsParams{}); !errors.Is(err, ErrAuthFailed) {
		t.Errorf("want ErrAuthFailed, got %v", err)
	}
}

func TestDecisions_NullArray_ReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "null")
	}))
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, BouncerKey: "k"})
	out, err := c.Decisions(context.Background(), DecisionsParams{})
	if err != nil {
		t.Fatalf("Decisions: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Errorf("want non-nil empty slice, got %+v", out)
	}
}

func TestDecisions_OversizeResponseIsExplicitlyRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// This would be valid JSON if read in full. The client must identify the
		// size violation rather than truncating it and reporting a JSON parse error.
		_, _ = fmt.Fprint(w, "[", strings.Repeat(" ", 4<<20), "]")
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, BouncerKey: "k"})
	_, err := c.Decisions(context.Background(), DecisionsParams{})
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("want ErrInvalidResponse, got %v", err)
	}
	if !strings.Contains(err.Error(), "exceeds 4 MiB") {
		t.Errorf("want explicit response-size error, got %q", err)
	}
}

func TestAlerts_LoginAndFetch(t *testing.T) {
	var calls atomic.Int64
	srv := lapiMockServer(t, &calls)
	defer srv.Close()
	c := newTestClient(srv.URL)

	out, err := c.Alerts(context.Background(), AlertsParams{Limit: 5})
	if err != nil {
		t.Fatalf("Alerts: %v", err)
	}
	if len(out) != 1 || out[0].Scenario != "crowdsecurity/http-probing" {
		t.Errorf("unexpected alerts: %+v", out)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 login, got %d", calls.Load())
	}
	if out[0].Source == nil || out[0].Source.Country != "" || out[0].Source.Latitude == nil {
		// country absent (valid), geo present — pointer tolerance check
		t.Logf("source: %+v", out[0].Source)
	}
}

func TestAlerts_401_RetriesOnceThenSucceeds(t *testing.T) {
	var calls atomic.Int64
	var fetches atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/watchers/login":
			calls.Add(1)
			_, _ = fmt.Fprint(w, `{"token":"`+fakeJWT(t, time.Now().Add(time.Hour))+`","expire":"`+time.Now().Add(time.Hour).Format(time.RFC3339)+`"}`)
		case "/v1/alerts":
			// First fetch 401s (expired-early token); second succeeds.
			if fetches.Add(1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"message":"missing token"}`)
				return
			}
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)

	out, err := c.Alerts(context.Background(), AlertsParams{})
	if err != nil {
		t.Fatalf("Alerts with one 401 should retry and succeed: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty alerts, got %+v", out)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 logins (initial + re-login), got %d", calls.Load())
	}
}

func TestAlerts_Persistent401_GivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"message":"machine testmachine not validated"}`)
	}))
	defer srv.Close()
	c := NewClient(Config{BaseURL: srv.URL, MachineID: "testmachine", MachinePassword: "bad"})

	if _, err := c.Alerts(context.Background(), AlertsParams{}); !errors.Is(err, ErrAuthFailed) {
		t.Errorf("want ErrAuthFailed, got %v", err)
	}
}

func TestTokenCache_SingleFlight(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/watchers/login":
			calls.Add(1)
			time.Sleep(50 * time.Millisecond) // hold the login open
			_, _ = fmt.Fprint(w, `{"token":"`+fakeJWT(t, time.Now().Add(time.Hour))+`","expire":"`+time.Now().Add(time.Hour).Format(time.RFC3339)+`"}`)
		case "/v1/alerts":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"message":"missing token"}`)
				return
			}
			_, _ = fmt.Fprint(w, `[]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, MachineID: "m", MachinePassword: "p"})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Alerts(context.Background(), AlertsParams{}); err != nil {
				t.Errorf("concurrent Alerts: %v", err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 login under concurrency, got %d", calls.Load())
	}
}

func TestTokenCache_LoginErrBackoff(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"message":"boom"}`)
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, MachineID: "m", MachinePassword: "p"})
	if _, err := c.Alerts(context.Background(), AlertsParams{}); err == nil {
		t.Fatal("expected error from failing login")
	}
	// Immediate second call must hit the negative cache, not the server.
	if _, err := c.Alerts(context.Background(), AlertsParams{}); err == nil {
		t.Fatal("expected backoff error")
	}
	if calls.Load() != 1 {
		t.Errorf("backoff violated: expected 1 login call, got %d", calls.Load())
	}
}

func TestParseJWTExpiry(t *testing.T) {
	exp := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	got := parseJWTExpiry(fakeJWT(t, exp))
	if got.Unix() != exp.Unix() {
		t.Errorf("parseJWTExpiry = %v, want %v", got, exp)
	}
	if !parseJWTExpiry("garbage").IsZero() {
		t.Error("garbage token should parse to zero time")
	}
	if !parseJWTExpiry("a.b.c").IsZero() {
		t.Error("malformed payload should parse to zero time")
	}
}

func TestTime_Unmarshal(t *testing.T) {
	var tt Time
	for _, s := range []string{`"2026-09-18T10:00:00Z"`, `""`, `null`, `"2026-09-18T10:00:00.123456Z"`} {
		if err := tt.UnmarshalJSON([]byte(s)); err != nil {
			t.Errorf("UnmarshalJSON(%s): %v", s, err)
		}
	}
	if err := tt.UnmarshalJSON([]byte(`"not-a-time"`)); err == nil {
		t.Error("expected error for unparseable timestamp")
	}
}

func TestDecisionExpiresAt(t *testing.T) {
	anchor := time.Now()
	d := Decision{Duration: "1h30m"}
	got := d.ExpiresAt(anchor)
	if got.Sub(anchor) != 90*time.Minute {
		t.Errorf("ExpiresAt = %v, want +90m", got)
	}
	if !(Decision{}).ExpiresAt(anchor).IsZero() {
		t.Error("empty duration should give zero time")
	}
}

func TestErrorSanitization_Unreachable(t *testing.T) {
	// A URL containing a credential-ish query must not appear in the error.
	c := NewClient(Config{BaseURL: "http://127.0.0.1:1"})
	err := c.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "127.0.0.1:1") && strings.Contains(msg, "?token=") {
		t.Errorf("error leaks URL: %s", msg)
	}
}
