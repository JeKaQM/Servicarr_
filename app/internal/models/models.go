package models

import "time"

// Service represents a monitored service
type Service struct {
	Key                 string
	Label               string
	URL                 string
	Timeout             time.Duration
	MinOK               int
	MaxOK               int
	Disabled            bool   `json:"disabled"`
	ConsecutiveFailures int    // Track consecutive check failures
	CheckType           string // http, tcp, dns, etc.
}

// ServiceConfig represents a service stored in the database
type ServiceConfig struct {
	ID            int    `json:"id"`
	Key           string `json:"key"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	ServiceType   string `json:"service_type"`   // plex, overseerr, jellyfin, sonarr, radarr, custom, etc.
	Icon          string `json:"icon"`           // Icon name or custom icon path
	IconURL       string `json:"icon_url"`       // Custom icon URL (overrides built-in icons)
	APIToken      string `json:"api_token"`      // Optional API token for services that need it
	DisplayOrder  int    `json:"display_order"`  // Order in the UI
	Visible       bool   `json:"visible"`        // Whether to show in the UI
	CheckType     string `json:"check_type"`     // http, tcp, ping
	CheckInterval int    `json:"check_interval"` // Seconds between checks
	Timeout       int    `json:"timeout"`        // Timeout in seconds
	ExpectedMin   int    `json:"expected_min"`   // Min HTTP status code for OK
	ExpectedMax   int    `json:"expected_max"`   // Max HTTP status code for OK
	DependsOn     string `json:"depends_on"`     // Comma-separated keys of upstream dependencies
	ConnectedTo   string `json:"connected_to"`   // Comma-separated keys of connected/integrated services
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// ServiceTemplate defines a preset for common services
type ServiceTemplate struct {
	Type          string `json:"type"`
	Name          string `json:"name"`
	Icon          string `json:"icon"`
	IconURL       string `json:"icon_url"` // Default icon URL for this service type
	DefaultURL    string `json:"default_url"`
	CheckType     string `json:"check_type"`
	URLSuffix     string `json:"url_suffix"`     // e.g., /api/v1/status for Overseerr
	RequiresToken bool   `json:"requires_token"` // Whether API token is needed
	TokenHeader   string `json:"token_header"`   // e.g., X-Plex-Token, X-Api-Key
	HelpText      string `json:"help_text"`
}

// LiveResult represents the current status of a service
type LiveResult struct {
	Label       string `json:"label"`
	OK          bool   `json:"ok"`
	Status      int    `json:"status"`
	MS          *int   `json:"ms,omitempty"`
	Disabled    bool   `json:"disabled"`
	Degraded    bool   `json:"degraded"`
	Maintenance bool   `json:"maintenance,omitempty"`
	CheckType   string `json:"check_type,omitempty"`
	DependsOn   string `json:"depends_on,omitempty"`   // Comma-separated upstream dependency keys
	ConnectedTo string `json:"connected_to,omitempty"` // Comma-separated connected/integrated service keys
}

// LivePayload represents a collection of service statuses
type LivePayload struct {
	T      time.Time             `json:"t"`
	Status map[string]LiveResult `json:"status"`
}

// AlertConfig stores alert configuration (multi-channel)
type AlertConfig struct {
	Enabled         bool   `json:"enabled"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPUser        string `json:"smtp_user"`
	SMTPPassword    string `json:"smtp_password"`
	AlertEmail      string `json:"alert_email"`
	FromEmail       string `json:"from_email"`
	StatusPageURL   string `json:"status_page_url"`
	SMTPSkipVerify  bool   `json:"smtp_skip_verify"`
	AlertOnDown     bool   `json:"alert_on_down"`
	AlertOnDegraded bool   `json:"alert_on_degraded"`
	// AlertOnUp is the backward-compatible outage (down -> healthy) recovery setting.
	AlertOnUp bool `json:"alert_on_up"`
	// AlertOnDegradedRecovery independently controls degraded -> healthy notifications.
	AlertOnDegradedRecovery bool `json:"alert_on_degraded_recovery"`

	// Multi-channel notification fields
	DiscordWebhookURL string `json:"discord_webhook_url"`
	DiscordEnabled    bool   `json:"discord_enabled"`
	DiscordUsername   string `json:"discord_username"`
	DiscordSilent     bool   `json:"discord_silent"`
	TelegramBotToken  string `json:"telegram_bot_token"`
	TelegramChatID    string `json:"telegram_chat_id"`
	TelegramEnabled   bool   `json:"telegram_enabled"`
	WebhookURL        string `json:"webhook_url"`
	WebhookSecret     string `json:"webhook_secret"`
	WebhookEnabled    bool   `json:"webhook_enabled"`
}

// ResourcesUIConfig stores admin configuration for the Resources section/widgets
type ResourcesUIConfig struct {
	Enabled    bool   `json:"enabled"`
	GlancesURL string `json:"glances_url"` // Host:port for Glances (e.g., 10.0.0.2:61208)
	NUTHost    string `json:"nut_host"`    // Host:port for NUT upsd (e.g., 10.0.0.2:3493)
	UPSName    string `json:"ups_name"`    // UPS name in NUT (e.g., apc)
	CPU        bool   `json:"cpu"`
	Memory     bool   `json:"memory"`
	Network    bool   `json:"network"`
	Temp       bool   `json:"temp"`
	Storage    bool   `json:"storage"`
	Swap       bool   `json:"swap"`
	Load       bool   `json:"load"`
	GPU        bool   `json:"gpu"`
	Containers bool   `json:"containers"`
	Processes  bool   `json:"processes"`
	Uptime     bool   `json:"uptime"`
	UPS        bool   `json:"ups"`
}

// ServiceStatus tracks service state for change detection
type ServiceStatus struct {
	Key      string
	OK       bool
	Degraded bool
}

// BlockInfo represents an IP block record
type BlockInfo struct {
	IP        string
	Attempts  int
	ExpiresAt string
}

// StatusAlert represents a site-wide or service-specific alert banner
type StatusAlert struct {
	ID         string `json:"id"`
	ServiceKey string `json:"service_key"`
	Message    string `json:"message"`
	Level      string `json:"level"`
	CreatedAt  string `json:"created_at"`
	Scheduled  bool   `json:"scheduled,omitempty"`
	Automatic  bool   `json:"automatic,omitempty"`
	Kind       string `json:"kind,omitempty"`
	EndsAt     string `json:"ends_at,omitempty"`
	Source     string `json:"source,omitempty"`
	Editable   bool   `json:"editable,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
}

// MaintenanceSchedule describes a one-time, daily, or weekly maintenance banner.
type MaintenanceSchedule struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Message            string `json:"message"`
	Level              string `json:"level"`
	ScheduleType       string `json:"schedule_type"` // once, daily, weekly; empty means legacy weekly
	Weekdays           []int  `json:"weekdays,omitempty"`
	Weekday            int    `json:"weekday"`
	StartTime          string `json:"start_time"`
	StartsAt           string `json:"starts_at,omitempty"` // RFC3339 instant for one-time windows
	EndsAt             string `json:"ends_at,omitempty"`   // Empty means no scheduled end
	DurationMinutes    int    `json:"duration_minutes"`
	Timezone           string `json:"timezone"`
	SuppressMonitoring bool   `json:"suppress_monitoring"`
	Enabled            bool   `json:"enabled"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// AppSettings stores application configuration including auth credentials
type AppSettings struct {
	SetupComplete bool   `json:"setup_complete"`
	Username      string `json:"username"`
	PasswordHash  string `json:"-"`        // Never expose in JSON
	AuthSecret    string `json:"-"`        // Never expose in JSON
	AppName       string `json:"app_name"` // Customizable app name displayed in header
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// LogEntry represents a log entry in the system
type LogEntry struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`    // info, warn, error, debug
	Category  string `json:"category"` // check, email, security, system, schedule, audit
	Service   string `json:"service"`  // service key if applicable
	Message   string `json:"message"`
	Details   string `json:"details"` // Additional details (JSON or plain text)
}

// LogStats represents log statistics
type LogStats struct {
	TotalLogs  int `json:"total_logs"`
	ErrorCount int `json:"error_count"`
	WarnCount  int `json:"warn_count"`
	InfoCount  int `json:"info_count"`
	DebugCount int `json:"debug_count"`
	AuditCount int `json:"audit_count"`
}

// ScheduleInfo represents a scheduled task
type ScheduleInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Interval    string `json:"interval"`
	LastRun     string `json:"last_run"`
	NextRun     string `json:"next_run"`
	Status      string `json:"status"` // running, idle, error
}

// CrowdSecConfig stores CrowdSec Local API (LAPI) connection configuration.
// Secrets are encrypted at rest; handlers must never serialize this struct
// directly into admin responses (use a response struct with *_configured
// flags, mirroring the alert config pattern).
type CrowdSecConfig struct {
	Enabled         bool    `json:"enabled"`
	LAPIURL         string  `json:"lapi_url"`
	MachineID       string  `json:"machine_id"`
	MachinePassword string  `json:"machine_password"` // write-only from the UI
	BouncerAPIKey   string  `json:"bouncer_api_key"`  // write-only from the UI
	PollIntervalS   int     `json:"poll_interval_seconds"`
	TLSSkipVerify   bool    `json:"tls_skip_verify"`
	MapHomeLat      float64 `json:"map_home_latitude"`  // 0 = unset (defaults to London)
	MapHomeLng      float64 `json:"map_home_longitude"` // 0 = unset (defaults to London)
}

// CrowdSecDecision is a single active decision mirrored from LAPI into the
// snapshot table. Expired rows are filtered at read time via expires_at.
type CrowdSecDecision struct {
	DecisionID string `json:"decision_id"`
	Value      string `json:"value"` // IP or CIDR
	Type       string `json:"type"`  // ban, captcha, ...
	Scope      string `json:"scope"` // Ip, Range
	Origin     string `json:"origin"`
	Scenario   string `json:"scenario"`
	Duration   string `json:"duration"` // remaining, e.g. "3h59m55s"
	Simulated  bool   `json:"simulated"`
	CreatedAt  string `json:"created_at"` // RFC3339
	ExpiresAt  string `json:"expires_at"` // RFC3339
	SyncedAt   string `json:"synced_at"`  // RFC3339, our ingest time
}

// CrowdSecSyncStatus is the poller-owned runtime state surfaced to the UI.
type CrowdSecSyncStatus struct {
	LastSync      string `json:"last_sync"`            // RFC3339; empty = never
	LastError     string `json:"last_error,omitempty"` // sanitized error text
	AuthFailed    bool   `json:"auth_failed"`          // credentials rejected
	DecisionCount int    `json:"decision_count"`       // true LAPI total
	SnapshotCount int    `json:"snapshot_count"`       // rows actually stored (<= cap)
}

// CrowdSecAlert is a scenario detection event mirrored from LAPI into the
// alerts snapshot. Alerts fire on detection — a decision (ban) may or may
// not follow, which is exactly the "scan that didn't lead to a decision"
// activity the dashboard surfaces. Latitude/Longitude are LAPI's geo
// enrichment of the source; nil when unavailable (the map skips those).
type CrowdSecAlert struct {
	AlertID     string   `json:"alert_id"`
	Scenario    string   `json:"scenario"`
	Message     string   `json:"message"`
	SourceValue string   `json:"source_value"` // attacking IP/range
	Country     string   `json:"country"`      // ISO 3166-1 alpha-2, e.g. "US"
	ASNumber    string   `json:"as_number"`
	ASName      string   `json:"as_name"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	EventsCount int64    `json:"events_count"` // how many raw events tripped the scenario
	StartAt     string   `json:"start_at"`     // RFC3339
	CreatedAt   string   `json:"created_at"`   // RFC3339
	HasDecision bool     `json:"has_decision"`
	Simulated   bool     `json:"simulated"`
}

// CrowdSecStats aggregates the alert history for the dashboard overview.
type CrowdSecStats struct {
	ActiveDecisions    int                     `json:"active_decisions"`
	Alerts24h          int64                   `json:"alerts_24h"` // scenario detections, last 24h
	AlertsWithDecision int64                   `json:"alerts_with_decision_24h"`
	TopCountry         string                  `json:"top_country"`
	TopCountryCount    int64                   `json:"top_country_count"`
	TopScenario        string                  `json:"top_scenario"`
	Countries          []CrowdSecCountryCount  `json:"countries"`
	Scenarios          []CrowdSecScenarioCount `json:"scenarios"`
}

// CrowdSecCountryCount pairs an ISO country code with its alert volume.
type CrowdSecCountryCount struct {
	Country string `json:"country"`
	Count   int64  `json:"count"`
}

// CrowdSecScenarioCount pairs a scenario name with its alert volume.
type CrowdSecScenarioCount struct {
	Scenario string `json:"scenario"`
	Count    int64  `json:"count"`
}
