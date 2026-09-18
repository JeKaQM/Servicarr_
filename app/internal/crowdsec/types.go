package crowdsec

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Time tolerates LAPI timestamp variants: RFC3339 with or without
// sub-second precision, empty strings, and JSON null.
type Time struct{ time.Time }

func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		t.Time = time.Time{}
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if v, err := time.Parse(layout, s); err == nil {
			t.Time = v.UTC()
			return nil
		}
	}
	return fmt.Errorf("crowdsec: cannot parse timestamp %q", s)
}

// Source mirrors a LAPI alert source object. All fields optional.
type Source struct {
	Scope     string   `json:"scope,omitempty"` // "Ip", "Range"
	Value     string   `json:"value,omitempty"`
	ASNumber  string   `json:"as_number,omitempty"`
	ASName    string   `json:"as_name,omitempty"`
	Country   string   `json:"cn,omitempty"`
	Latitude  *float32 `json:"latitude,omitempty"`
	Longitude *float32 `json:"longitude,omitempty"`
}

// Decision mirrors a CrowdSec decision (ban, captcha, ...).
// LAPI returns `null` for empty decision lists; decode into nil slices.
type Decision struct {
	ID       *int64 `json:"id,omitempty"`
	Duration string `json:"duration,omitempty"` // remaining, e.g. "3h59m55s"
	Origin   string `json:"origin,omitempty"`   // "crowdsec", "cscli", "CAPI", "lists"
	Scenario string `json:"scenario,omitempty"`
	Scope    string `json:"scope,omitempty"` // "Ip", "Range" (capitalized)
	Type     string `json:"type,omitempty"`  // "ban", "captcha", ...
	Value    string `json:"value,omitempty"`
	// `until` and `simulated` are documented in the swagger but are NOT
	// populated by GET /v1/decisions — deliberately omitted. Expiry is
	// created_at + duration, computed by the syncer.
}

// Alert mirrors a CrowdSec alert. Fields are optional and pointer-tolerant
// because LAPI omits unset fields entirely rather than nulling them.
type Alert struct {
	ID          *int64     `json:"id,omitempty"`
	CreatedAt   Time       `json:"created_at,omitempty"`
	Scenario    string     `json:"scenario,omitempty"`
	Message     string     `json:"message,omitempty"`
	EventsCount *int64     `json:"events_count,omitempty"`
	StartAt     Time       `json:"start_at,omitempty"`
	StopAt      Time       `json:"stop_at,omitempty"`
	Simulated   *bool      `json:"simulated,omitempty"`
	Capacity    *int64     `json:"capacity,omitempty"`
	Source      *Source    `json:"source,omitempty"`
	Decisions   []Decision `json:"decisions,omitempty"`
}

// HasActiveDecision reports whether the alert carries any non-simulated
// decision that is still active (used for dashboard filtering).
func (a Alert) HasActiveDecision() bool {
	for _, d := range a.Decisions {
		if d.Type == "" || d.Type == "ban" || d.Type == "captcha" {
			return true
		}
	}
	return false
}

// DecisionsParams are the GET /v1/decisions query parameters.
// LAPI returns HTTP 500 for unknown parameters — only add fields verified
// against the official swagger (localapi_swagger.yaml).
type DecisionsParams struct {
	Limit   int // 0 = server default (100)
	Offset  int
	Scopes  []string // joined into "scopes"
	Type    string
	Origins []string // joined into "origins"
	Value   string
}

func (p DecisionsParams) encode() string {
	q := url.Values{}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	if len(p.Scopes) > 0 {
		q.Set("scopes", strings.Join(p.Scopes, ","))
	}
	if p.Type != "" {
		q.Set("type", p.Type)
	}
	if len(p.Origins) > 0 {
		q.Set("origins", strings.Join(p.Origins, ","))
	}
	if p.Value != "" {
		q.Set("value", p.Value)
	}
	return q.Encode()
}

// AlertsParams are the GET /v1/alerts query parameters.
type AlertsParams struct {
	Limit    int
	Since    time.Duration // encoded as a Go duration string, e.g. "2h"
	Scenario string
	IP       string
}

func (p AlertsParams) encode() string {
	q := url.Values{}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Since > 0 {
		q.Set("since", p.Since.String())
	}
	if p.Scenario != "" {
		q.Set("scenario", p.Scenario)
	}
	if p.IP != "" {
		q.Set("ip", p.IP)
	}
	return q.Encode()
}

// parseDuration parses a LAPI duration string ("3h59m55s") into a duration.
// LAPI durations are literal Go durations.
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	return time.ParseDuration(s)
}

// ExpiresAt computes a decision's expiry from an anchor time plus its
// remaining-duration string. Returns zero time when duration is unparsable.
func (d Decision) ExpiresAt(anchor time.Time) time.Time {
	dur, err := parseDuration(d.Duration)
	if err != nil {
		return time.Time{}
	}
	return anchor.Add(dur)
}
