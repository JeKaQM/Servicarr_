package stats

import "time"

// Heartbeat represents a single health check result
type Heartbeat struct {
	MonitorID  int       `json:"monitor_id"`
	Status     int       `json:"status"` // 0=DOWN, 1=UP, 2=PENDING, 3=MAINTENANCE
	Time       time.Time `json:"time"`
	Msg        string    `json:"msg"`
	Ping       *int      `json:"ping"`
	Important  bool      `json:"important"` // Status change events
	Duration   int       `json:"duration"`  // Duration of this status in seconds
	HTTPStatus int       `json:"http_status"`
}

// StatEntry represents aggregated statistics for a time period
type StatEntry struct {
	Up      int     `json:"up"`
	Down    int     `json:"down"`
	AvgPing float64 `json:"avg_ping"`
	MinPing int     `json:"min_ping"`
	MaxPing int     `json:"max_ping"`
}

// UptimeStats contains computed uptime and latency statistics for a service.
type UptimeStats struct {
	Uptime24h   float64 `json:"uptime_24h"`
	Uptime7d    float64 `json:"uptime_7d"`
	Uptime30d   float64 `json:"uptime_30d"`
	AvgLatency  float64 `json:"avg_latency"`
	LastChecked string  `json:"last_checked"`
}
