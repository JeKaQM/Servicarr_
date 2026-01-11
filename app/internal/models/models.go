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
	Enabled bool `json:"enabled"`
	CPU     bool `json:"cpu"`
	Memory  bool `json:"memory"`
	Network bool `json:"network"`
	Temp    bool `json:"temp"`
	Storage bool `json:"storage"`
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
