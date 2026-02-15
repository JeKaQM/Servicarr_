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
	AlertOnUp       bool   `json:"alert_on_up"`

	// Multi-channel notification fields
	DiscordWebhookURL string `json:"discord_webhook_url"`
	DiscordEnabled    bool   `json:"discord_enabled"`
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
	Category  string `json:"category"` // check, email, security, system, schedule
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
