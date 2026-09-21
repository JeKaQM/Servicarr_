// Package crowdsec implements a client for the CrowdSec Local API (LAPI).
//
// The client supports both LAPI authentication realms:
//   - Bouncer API keys ("X-Api-Key" header) for reading decisions
//   - Machine credentials (JWT via POST /v1/watchers/login) for reading alerts
//
// All network errors are sanitized through checker.SanitizeError so LAPI
// credentials can never appear in returned error strings.
package crowdsec

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"status/app/internal/checker"
)

// Poll interval bounds enforced by the admin handlers.
const (
	MinPollInterval     = 10 * time.Second
	MaxPollInterval     = time.Hour
	DefaultPollInterval = 30 * time.Second
)

// requestTimeout bounds every single LAPI HTTP round trip.
const requestTimeout = 5 * time.Second

// Config holds connection settings for a CrowdSec Local API.
// Populated from admin settings; never from untrusted request input.
type Config struct {
	// BaseURL is the normalized LAPI root, e.g. "http://10.0.0.5:8080".
	BaseURL string

	// BouncerKey authenticates GET /v1/decisions via the X-Api-Key header.
	BouncerKey string

	// MachineID and MachinePassword authenticate POST /v1/watchers/login
	// for a JWT used by GET /v1/alerts.
	MachineID       string
	MachinePassword string

	// InsecureSkipVerify allows self-signed certificates on HTTPS LAPI.
	// Explicit admin opt-in; disabled by default.
	InsecureSkipVerify bool
}

// ValidateBaseURL parses, validates, and normalizes a LAPI base URL.
// A trailing "/v1" is tolerated and stripped. Cloud metadata targets are
// rejected (same SSRF guard the checker applies to monitored services).
func ValidateBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil {
		// net/url errors can echo the original input. Keep malformed URLs out
		// of responses because operators occasionally paste credentials here.
		return "", errors.New("crowdsec URL is invalid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("crowdsec URL must use http or https")
	}
	if u.Host == "" {
		return "", errors.New("crowdsec URL must include a host")
	}
	if u.User != nil {
		return "", errors.New("crowdsec URL must not contain credentials")
	}
	switch u.Path {
	case "", "/", "/v1":
	default:
		return "", errors.New("crowdsec URL must be the LAPI root (no path)")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("crowdsec URL must not contain query or fragment")
	}
	if err := checker.ValidateURLTarget(u.String()); err != nil {
		return "", fmt.Errorf("crowdsec URL rejected: %w", err)
	}
	base := u.Scheme + "://" + u.Host
	return strings.TrimSuffix(base, "/"), nil
}

// ValidatePollInterval clamps/validates a configured poll interval.
func ValidatePollInterval(d time.Duration) (time.Duration, error) {
	if d < MinPollInterval {
		return 0, fmt.Errorf("crowdsec poll interval must be at least %s", MinPollInterval)
	}
	if d > MaxPollInterval {
		return 0, fmt.Errorf("crowdsec poll interval must be at most %s", MaxPollInterval)
	}
	return d, nil
}
