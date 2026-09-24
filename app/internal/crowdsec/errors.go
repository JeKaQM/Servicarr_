package crowdsec

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"status/app/internal/checker"
)

// Sentinel errors classify LAPI failures for the UI. Every error returned by
// this package wraps exactly one of these via errors.Is-compatible chains.
var (
	// ErrUnreachable: DNS failure, refused connection, timeout, TLS error.
	ErrUnreachable = errors.New("crowdsec LAPI unreachable")

	// ErrAuthFailed: LAPI answered but rejected the credentials (401/403).
	ErrAuthFailed = errors.New("crowdsec authentication failed")

	// ErrInvalidResponse: HTTP success but body was not the expected shape,
	// or an unexpected non-2xx status.
	ErrInvalidResponse = errors.New("crowdsec returned an invalid response")

	// ErrNotConfigured: method called without its required credential.
	ErrNotConfigured = errors.New("crowdsec credential not configured")
)

// StatusError carries an unexpected HTTP status with LAPI's message.
type StatusError struct {
	StatusCode int
	Message    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("crowdsec http %d: %s", e.StatusCode, e.Message)
}

func (e *StatusError) Unwrap() error { return ErrInvalidResponse }

// classify converts a raw transport error into ErrUnreachable while
// sanitizing the detail text (URLs and credentials never leak).
func classify(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnreachable, checker.SanitizeError(err.Error()))
}

// classifyStatus converts a non-2xx response into a typed error.
func classifyStatus(status int, lapiMessage string) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		if lapiMessage == "" {
			lapiMessage = "LAPI rejected the credentials"
		}
		return fmt.Errorf("%w: http %d: %s", ErrAuthFailed, status, lapiMessage)
	}
	if lapiMessage == "" {
		lapiMessage = "unexpected response"
	}
	return &StatusError{StatusCode: status, Message: lapiMessage}
}

// readLAPIError extracts {"message": "..."} from an error response body,
// sanitized and length-capped for safe display.
func readLAPIError(resp *http.Response) string {
	defer resp.Body.Close()
	var out struct {
		Message string `json:"message"`
	}
	// Limit error body reads; LAPI messages are small.
	dec := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 8<<10))
	if err := dec.Decode(&out); err != nil {
		return ""
	}
	msg := checker.SanitizeError(out.Message)
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}
