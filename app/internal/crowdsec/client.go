package crowdsec

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"status/app/internal/buildinfo"
	"status/app/internal/checker"
)

// Client is a pure CrowdSec LAPI fetcher. It performs no caching and no
// snapshotting — the syncer owns cadence and persistence. Safe for
// concurrent use.
type Client struct {
	cfg     Config
	baseURL string
	http    *http.Client
	tok     *tokenCache
}

// NewClient builds a Client. cfg.BaseURL must already be normalized by
// ValidateBaseURL. The HTTP client uses a proxyless transport that blocks
// cloud metadata targets at dial time (same guard as service checks).
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:     cfg,
		baseURL: cfg.BaseURL,
		http: &http.Client{
			Timeout:   requestTimeout,
			Transport: newTransport(cfg.InsecureSkipVerify),
			// A credentialed client never follows redirects: LAPI does not
			// legitimately redirect, and a cross-host hop would forward
			// X-Api-Key / Bearer credentials to an attacker-controlled host.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		tok: newTokenCache(),
	}
}

// newTransport builds on the checker's resolve-validate-dial transport so
// cloud metadata targets stay blocked at connection time (no DNS-rebinding
// gap between URL validation and the dial), disables environment proxies,
// and optionally allows self-signed certificates (explicit admin opt-in).
func newTransport(insecure bool) http.RoundTripper {
	t := checker.NewSafeTransport()
	if insecure {
		//nolint:gosec // explicit admin opt-in for self-signed LAPI certificates
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return t
}

func (c *Client) userAgent() string {
	// LAPI parses "Name/Version" from the User-Agent and records it as
	// the bouncer name, so `cscli bouncers list` shows Servicarr cleanly.
	return "Servicarr/" + buildinfo.Current().Version
}

// HasMachineCredentials reports whether alert fetching (machine JWT realm)
// is possible with this configuration. Lets the syncer degrade gracefully
// to a decisions-only dashboard when only a bouncer key is configured.
func (c *Client) HasMachineCredentials() bool {
	return c.cfg.MachineID != "" && c.cfg.MachinePassword != ""
}

// Health checks LAPI liveness via GET /health (no /v1 prefix, no auth).
// Useful for connection tests before credentials are configured. Any
// transport failure wraps ErrUnreachable; any HTTP answer (even 500)
// proves reachability, so non-200 maps to ErrInvalidResponse.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return classify(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return classify(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{StatusCode: resp.StatusCode, Message: "unexpected /health status"}
	}
	return nil
}

// Decisions fetches active decisions via the bouncer API key.
func (c *Client) Decisions(ctx context.Context, p DecisionsParams) ([]Decision, error) {
	if c.cfg.BouncerKey == "" {
		return nil, fmt.Errorf("%w: decisions require a bouncer API key", ErrNotConfigured)
	}
	out, err := c.getJSON(ctx, "/v1/decisions?"+p.encode(), func(req *http.Request) {
		req.Header.Set("X-Api-Key", c.cfg.BouncerKey)
	})
	if err != nil {
		return nil, err
	}
	var decisions []Decision
	if err := json.Unmarshal(out, &decisions); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponse, checker.SanitizeError(err.Error()))
	}
	if decisions == nil {
		decisions = []Decision{} // LAPI returns null when empty
	}
	return decisions, nil
}

// Alerts fetches alerts via the machine JWT. A 401 invalidates the cached
// token and retries exactly once (expired-early or revoked token); a second
// 401 surfaces ErrAuthFailed — the credentials themselves are wrong.
func (c *Client) Alerts(ctx context.Context, p AlertsParams) ([]Alert, error) {
	if c.cfg.MachineID == "" || c.cfg.MachinePassword == "" {
		return nil, fmt.Errorf("%w: alerts require machine credentials", ErrNotConfigured)
	}

	tok, err := c.tok.token(ctx, c.login)
	if err != nil {
		return nil, err
	}
	alerts, err := c.fetchAlerts(ctx, tok, p)
	if err == nil || !errors.Is(err, ErrAuthFailed) {
		return alerts, err
	}

	c.tok.invalidate()
	tok, err = c.tok.token(ctx, c.login)
	if err != nil {
		return nil, err
	}
	return c.fetchAlerts(ctx, tok, p)
}

func (c *Client) fetchAlerts(ctx context.Context, tok string, p AlertsParams) ([]Alert, error) {
	out, err := c.getJSON(ctx, "/v1/alerts?"+p.encode(), func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+tok)
	})
	if err != nil {
		return nil, err
	}
	var alerts []Alert
	if err := json.Unmarshal(out, &alerts); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidResponse, checker.SanitizeError(err.Error()))
	}
	if alerts == nil {
		alerts = []Alert{} // LAPI returns null when empty
	}
	return alerts, nil
}

// login performs POST /v1/watchers/login and returns (token, expiry).
// Prefers the JWT's own exp claim over the server-echoed expires field
// (immune to server clock skew in the echo).
func (c *Client) login(ctx context.Context) (string, time.Time, error) {
	if c.cfg.MachineID == "" || c.cfg.MachinePassword == "" {
		return "", time.Time{}, fmt.Errorf("%w: alerts require machine credentials", ErrNotConfigured)
	}
	body := []byte(fmt.Sprintf(`{"machine_id":%q,"password":%q}`, c.cfg.MachineID, c.cfg.MachinePassword))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/watchers/login", bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, classify(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", time.Time{}, classify(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, classifyStatus(resp.StatusCode, readLAPIError(resp))
	}

	var out struct {
		Token   string `json:"token"`
		Expires string `json:"expire"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.Token == "" {
		return "", time.Time{}, fmt.Errorf("%w: login response missing token", ErrInvalidResponse)
	}

	exp := parseJWTExpiry(out.Token)
	if exp.IsZero() {
		if t, err := time.Parse(time.RFC3339, out.Expires); err == nil {
			exp = t
		}
	}
	if exp.IsZero() {
		// Unparseable lease: assume short so we re-login soon.
		exp = time.Now().Add(5 * time.Minute)
	}
	return out.Token, exp, nil
}

// getJSON performs a GET with auth headers and returns the raw JSON body.
// Non-2xx statuses are classified; bodies are capped at 4 MB.
func (c *Client) getJSON(ctx context.Context, path string, setAuth func(*http.Request)) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, classify(err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent())
	setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, classify(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyStatus(resp.StatusCode, readLAPIError(resp))
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, classify(err)
	}
	return out, nil
}
