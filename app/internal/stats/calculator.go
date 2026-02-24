package stats

import (
	"database/sql"
	"status/app/internal/cache"
	"status/app/internal/database"
	"sync"
	"time"
)

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
