package stats

import (
	"database/sql"
	"log"
	"status/app/internal/cache"
	"status/app/internal/database"
	"sync"
	"time"
)

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

// UptimeCalculator manages uptime statistics for a service
type UptimeCalculator struct {
	ServiceKey string

	// Recent heartbeats (last 100)
	recentHeartbeats []Heartbeat
	mu               sync.RWMutex
}

var (
	calculators = make(map[string]*UptimeCalculator)
	calcMu      sync.RWMutex
)

// GetCalculator returns or creates an uptime calculator for a service
func GetCalculator(serviceKey string) *UptimeCalculator {
	calcMu.Lock()
	defer calcMu.Unlock()

	if calc, exists := calculators[serviceKey]; exists {
		return calc
	}

	calc := &UptimeCalculator{
		ServiceKey:       serviceKey,
		recentHeartbeats: make([]Heartbeat, 0, 100),
	}
	calculators[serviceKey] = calc
	return calc
}

// RemoveCalculator removes a calculator when a service is deleted
func RemoveCalculator(serviceKey string) {
	calcMu.Lock()
	defer calcMu.Unlock()
	delete(calculators, serviceKey)
}

// AddHeartbeat records a new heartbeat and returns whether it represents a status change.
func (c *UptimeCalculator) AddHeartbeat(status int, ping *int, httpStatus int, msg string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	hb := Heartbeat{
		Status:     status,
		Time:       time.Now().UTC(),
		Ping:       ping,
		HTTPStatus: httpStatus,
		Msg:        msg,
	}

	// Check if this is a status change (important event)
	if len(c.recentHeartbeats) > 0 {
		lastStatus := c.recentHeartbeats[len(c.recentHeartbeats)-1].Status
		hb.Important = lastStatus != status
	} else {
		hb.Important = true // First heartbeat is always important
	}

	// Keep only last 100 heartbeats in memory
	c.recentHeartbeats = append(c.recentHeartbeats, hb)
	if len(c.recentHeartbeats) > 100 {
		c.recentHeartbeats = c.recentHeartbeats[1:]
	}

	// Invalidate cache
	cache.StatsCache.Delete("uptime:" + c.ServiceKey)

	return hb.Important
}

// GetUptime calculates uptime percentage for a given duration
func (c *UptimeCalculator) GetUptime(duration time.Duration) float64 {
	cacheKey := "uptime:" + c.ServiceKey

	// Check cache first
	if cached, ok := cache.StatsCache.Get(cacheKey); ok {
		if uptimes, ok := cached.(map[string]float64); ok {
			key := duration.String()
			if uptime, exists := uptimes[key]; exists {
				return uptime
			}
		}
	}

	// Calculate from database
	since := time.Now().UTC().Add(-duration).Format(time.RFC3339)
	var upCount, totalCount int

	err := database.DB.QueryRow(`
		SELECT COALESCE(SUM(ok), 0), COUNT(*) 
		FROM samples 
		WHERE service_key = ? AND taken_at >= ?`,
		c.ServiceKey, since).Scan(&upCount, &totalCount)

	if err != nil || totalCount == 0 {
		return 100.0 // No data = assume up
	}

	uptime := float64(upCount) / float64(totalCount) * 100

	// Cache the result
	uptimes := make(map[string]float64)
	if cached, ok := cache.StatsCache.Get(cacheKey); ok {
		if u, ok := cached.(map[string]float64); ok {
			uptimes = u
		}
	}
	uptimes[duration.String()] = uptime
	cache.StatsCache.Set(cacheKey, uptimes)

	return uptime
}

// GetRecentHeartbeats returns the most recent heartbeats
func (c *UptimeCalculator) GetRecentHeartbeats(count int) []Heartbeat {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if count > len(c.recentHeartbeats) {
		count = len(c.recentHeartbeats)
	}

	// Return newest first
	result := make([]Heartbeat, count)
	for i := 0; i < count; i++ {
		result[i] = c.recentHeartbeats[len(c.recentHeartbeats)-1-i]
	}
	return result
}

// GetAverageLatency returns the average latency over a duration
func (c *UptimeCalculator) GetAverageLatency(duration time.Duration) float64 {
	since := time.Now().UTC().Add(-duration).Format(time.RFC3339)
	var avgMs sql.NullFloat64

	err := database.DB.QueryRow(`
		SELECT AVG(latency_ms) 
		FROM samples 
		WHERE service_key = ? AND taken_at >= ? AND latency_ms IS NOT NULL`,
		c.ServiceKey, since).Scan(&avgMs)

	if err != nil || !avgMs.Valid {
		return 0
	}

	return avgMs.Float64
}

// EnsureStatsSchema creates the statistics tables
func EnsureStatsSchema() error {
	_, err := database.DB.Exec(`
-- Minutely stats (kept for 24 hours)
CREATE TABLE IF NOT EXISTS stat_minutely (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	service_key TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	up INTEGER NOT NULL DEFAULT 0,
	down INTEGER NOT NULL DEFAULT 0,
	ping REAL,
	ping_min INTEGER,
	ping_max INTEGER,
	UNIQUE(service_key, timestamp)
);
CREATE INDEX IF NOT EXISTS idx_stat_minutely_key ON stat_minutely(service_key);
CREATE INDEX IF NOT EXISTS idx_stat_minutely_ts ON stat_minutely(timestamp);

-- Hourly stats (kept for 30 days)
CREATE TABLE IF NOT EXISTS stat_hourly (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	service_key TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	up INTEGER NOT NULL DEFAULT 0,
	down INTEGER NOT NULL DEFAULT 0,
	ping REAL,
	ping_min INTEGER,
	ping_max INTEGER,
	UNIQUE(service_key, timestamp)
);
CREATE INDEX IF NOT EXISTS idx_stat_hourly_key ON stat_hourly(service_key);
CREATE INDEX IF NOT EXISTS idx_stat_hourly_ts ON stat_hourly(timestamp);

-- Daily stats (kept for 365 days)
CREATE TABLE IF NOT EXISTS stat_daily (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	service_key TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	up INTEGER NOT NULL DEFAULT 0,
	down INTEGER NOT NULL DEFAULT 0,
	ping REAL,
	ping_min INTEGER,
	ping_max INTEGER,
	UNIQUE(service_key, timestamp)
);
CREATE INDEX IF NOT EXISTS idx_stat_daily_key ON stat_daily(service_key);
CREATE INDEX IF NOT EXISTS idx_stat_daily_ts ON stat_daily(timestamp);

-- Heartbeats table for recent raw data
CREATE TABLE IF NOT EXISTS heartbeats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	service_key TEXT NOT NULL,
	status INTEGER NOT NULL,
	time TEXT NOT NULL,
	msg TEXT,
	ping INTEGER,
	http_status INTEGER,
	important INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_heartbeats_key ON heartbeats(service_key);
CREATE INDEX IF NOT EXISTS idx_heartbeats_time ON heartbeats(time);
CREATE INDEX IF NOT EXISTS idx_heartbeats_important ON heartbeats(important);
`)
	return err
}

// RecordHeartbeat stores a heartbeat and updates statistics
func RecordHeartbeat(serviceKey string, ok bool, ping *int, httpStatus int, errMsg string) {
	calc := GetCalculator(serviceKey)

	status := 0
	if ok {
		status = 1
	}

	important := calc.AddHeartbeat(status, ping, httpStatus, errMsg)

	// Store in heartbeats table
	importantInt := 0
	if important {
		importantInt = 1
	}

	_, err := database.DB.Exec(`
		INSERT INTO heartbeats (service_key, status, time, msg, ping, http_status, important)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		serviceKey, status, time.Now().UTC().Format(time.RFC3339), errMsg, ping, httpStatus, importantInt)

	if err != nil {
		log.Printf("Error recording heartbeat: %v", err)
	}

	// Update minutely stats
	updateMinutelyStat(serviceKey, ok, ping)
}

// updateMinutelyStat updates the minutely statistics
func updateMinutelyStat(serviceKey string, ok bool, ping *int) {
	now := time.Now().UTC()
	// Round down to minute
	timestamp := now.Unix() / 60 * 60

	upDelta, downDelta := 0, 0
	if ok {
		upDelta = 1
	} else {
		downDelta = 1
	}

	_, err := database.DB.Exec(`
		INSERT INTO stat_minutely (service_key, timestamp, up, down, ping, ping_min, ping_max)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(service_key, timestamp) DO UPDATE SET
			up = up + excluded.up,
			down = down + excluded.down,
			ping = CASE 
				WHEN excluded.ping IS NOT NULL THEN 
					COALESCE((stat_minutely.ping * (stat_minutely.up + stat_minutely.down) + excluded.ping) / 
						(stat_minutely.up + stat_minutely.down + 1), excluded.ping)
				ELSE stat_minutely.ping 
			END,
			ping_min = CASE 
				WHEN excluded.ping IS NOT NULL THEN 
					COALESCE(MIN(stat_minutely.ping_min, excluded.ping_min), excluded.ping_min)
				ELSE stat_minutely.ping_min 
			END,
			ping_max = CASE 
				WHEN excluded.ping IS NOT NULL THEN 
					COALESCE(MAX(stat_minutely.ping_max, excluded.ping_max), excluded.ping_max)
				ELSE stat_minutely.ping_max 
			END`,
		serviceKey, timestamp, upDelta, downDelta, ping, ping, ping)

	if err != nil {
		log.Printf("Error updating minutely stat: %v", err)
	}
}

// AggregateHourlyStats aggregates minutely stats into hourly stats
func AggregateHourlyStats() {
	now := time.Now().UTC()
	// Aggregate stats from more than 1 hour ago
	hourAgo := now.Add(-1*time.Hour).Unix() / 3600 * 3600

	rows, err := database.DB.Query(`
		SELECT service_key, 
			   (timestamp / 3600) * 3600 as hour_ts,
			   SUM(up), SUM(down), AVG(ping), MIN(ping_min), MAX(ping_max)
		FROM stat_minutely
		WHERE timestamp < ?
		GROUP BY service_key, hour_ts`,
		hourAgo)

	if err != nil {
		log.Printf("Error aggregating hourly stats: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var serviceKey string
		var hourTs int64
		var up, down int
		var avgPing sql.NullFloat64
		var minPing, maxPing sql.NullInt64

		if err := rows.Scan(&serviceKey, &hourTs, &up, &down, &avgPing, &minPing, &maxPing); err != nil {
			continue
		}

		_, err := database.DB.Exec(`
			INSERT INTO stat_hourly (service_key, timestamp, up, down, ping, ping_min, ping_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(service_key, timestamp) DO UPDATE SET
				up = up + excluded.up,
				down = down + excluded.down`,
			serviceKey, hourTs, up, down, avgPing, minPing, maxPing)

		if err != nil {
			log.Printf("Error inserting hourly stat: %v", err)
		}
	}

	// Delete old minutely stats (keep 24 hours)
	dayAgo := now.Add(-24 * time.Hour).Unix()
	_, _ = database.DB.Exec(`DELETE FROM stat_minutely WHERE timestamp < ?`, dayAgo)
}

// AggregateDailyStats aggregates hourly stats into daily stats
func AggregateDailyStats() {
	now := time.Now().UTC()
	// Aggregate stats from more than 1 day ago
	dayAgo := now.Add(-24*time.Hour).Unix() / 86400 * 86400

	rows, err := database.DB.Query(`
		SELECT service_key, 
			   (timestamp / 86400) * 86400 as day_ts,
			   SUM(up), SUM(down), AVG(ping), MIN(ping_min), MAX(ping_max)
		FROM stat_hourly
		WHERE timestamp < ?
		GROUP BY service_key, day_ts`,
		dayAgo)

	if err != nil {
		log.Printf("Error aggregating daily stats: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var serviceKey string
		var dayTs int64
		var up, down int
		var avgPing sql.NullFloat64
		var minPing, maxPing sql.NullInt64

		if err := rows.Scan(&serviceKey, &dayTs, &up, &down, &avgPing, &minPing, &maxPing); err != nil {
			continue
		}

		_, err := database.DB.Exec(`
			INSERT INTO stat_daily (service_key, timestamp, up, down, ping, ping_min, ping_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(service_key, timestamp) DO UPDATE SET
				up = up + excluded.up,
				down = down + excluded.down`,
			serviceKey, dayTs, up, down, avgPing, minPing, maxPing)

		if err != nil {
			log.Printf("Error inserting daily stat: %v", err)
		}
	}

	// Delete old hourly stats (keep 30 days)
	monthAgo := now.Add(-30 * 24 * time.Hour).Unix()
	_, _ = database.DB.Exec(`DELETE FROM stat_hourly WHERE timestamp < ?`, monthAgo)

	// Delete old daily stats (keep 365 days)
	yearAgo := now.Add(-365 * 24 * time.Hour).Unix()
	_, _ = database.DB.Exec(`DELETE FROM stat_daily WHERE timestamp < ?`, yearAgo)
}

// CleanupOldHeartbeats removes heartbeats older than 24 hours, keeping important ones for 7 days
func CleanupOldHeartbeats() {
	now := time.Now().UTC()

	// Delete non-important heartbeats older than 24 hours
	dayAgo := now.Add(-24 * time.Hour).Format(time.RFC3339)
	_, _ = database.DB.Exec(`DELETE FROM heartbeats WHERE time < ? AND important = 0`, dayAgo)

	// Delete important heartbeats older than 7 days
	weekAgo := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	_, _ = database.DB.Exec(`DELETE FROM heartbeats WHERE time < ?`, weekAgo)
}

// UptimeStats contains computed uptime and latency statistics for a service.
type UptimeStats struct {
	Uptime24h   float64 `json:"uptime_24h"`
	Uptime7d    float64 `json:"uptime_7d"`
	Uptime30d   float64 `json:"uptime_30d"`
	AvgLatency  float64 `json:"avg_latency"`
	LastChecked string  `json:"last_checked"`
}

// GetUptimeStats returns computed uptime statistics for a service
func GetUptimeStats(serviceKey string) *UptimeStats {
	cacheKey := "uptime_stats:" + serviceKey

	// Check cache first
	if cached, ok := cache.StatsCache.Get(cacheKey); ok {
		if stats, ok := cached.(*UptimeStats); ok {
			return stats
		}
	}

	calc := GetCalculator(serviceKey)
	stats := &UptimeStats{
		Uptime24h:  calc.GetUptime(24 * time.Hour),
		Uptime7d:   calc.GetUptime(7 * 24 * time.Hour),
		Uptime30d:  calc.GetUptime(30 * 24 * time.Hour),
		AvgLatency: calc.GetAverageLatency(24 * time.Hour),
	}

	// Get last checked time
	var lastChecked sql.NullString
	_ = database.DB.QueryRow(`
		SELECT time FROM heartbeats 
		WHERE service_key = ? 
		ORDER BY time DESC LIMIT 1`,
		serviceKey).Scan(&lastChecked)

	if lastChecked.Valid {
		stats.LastChecked = lastChecked.String
	}

	// Cache for 30 seconds
	cache.StatsCache.Set(cacheKey, stats)

	return stats
}

// StartStatsAggregator starts background aggregation tasks
func StartStatsAggregator() {
	// Run hourly aggregation every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			AggregateHourlyStats()
		}
	}()

	// Run daily aggregation every day
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		for range ticker.C {
			AggregateDailyStats()
		}
	}()

	// Run heartbeat cleanup every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			CleanupOldHeartbeats()
		}
	}()

	log.Println("Stats aggregator started")
}
