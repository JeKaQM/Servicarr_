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
	Disabled            bool `json:"disabled"`
	ConsecutiveFailures int  // Track consecutive check failures
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
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// ServiceTemplate defines a preset for common services
type ServiceTemplate struct {
	Type          string `json:"type"`
	Name          string `json:"name"`
	Icon          string `json:"icon"`
	IconURL       string `json:"icon_url"`       // Default icon URL for this service type
	DefaultURL    string `json:"default_url"`
	CheckType     string `json:"check_type"`
	URLSuffix     string `json:"url_suffix"`     // e.g., /api/v1/status for Overseerr
	RequiresToken bool   `json:"requires_token"` // Whether API token is needed
	TokenHeader   string `json:"token_header"`   // e.g., X-Plex-Token, X-Api-Key
	HelpText      string `json:"help_text"`
}

// LiveResult represents the current status of a service
type LiveResult struct {
	Label    string `json:"label"`
	OK       bool   `json:"ok"`
	Status   int    `json:"status"`
	MS       *int   `json:"ms,omitempty"`
	Disabled bool   `json:"disabled"`
	Degraded bool   `json:"degraded"`
}

// LivePayload represents a collection of service statuses
type LivePayload struct {
	T      time.Time             `json:"t"`
	Status map[string]LiveResult `json:"status"`
}

// AlertConfig stores email alert configuration
type AlertConfig struct {
	Enabled         bool   `json:"enabled"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPUser        string `json:"smtp_user"`
	SMTPPassword    string `json:"smtp_password"`
	AlertEmail      string `json:"alert_email"`
	FromEmail       string `json:"from_email"`
	AlertOnDown     bool   `json:"alert_on_down"`
	AlertOnDegraded bool   `json:"alert_on_degraded"`
	AlertOnUp       bool   `json:"alert_on_up"`
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
	PasswordHash  string `json:"-"`              // Never expose in JSON
	AuthSecret    string `json:"-"`              // Never expose in JSON
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}
