package stats

import (
	"database/sql"
	"log"
	"status/app/internal/database"
	"time"
)

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

// aggregatedRow holds one aggregation result for batch processing.
// Collected first so the rows iterator is closed before any writes,
// avoiding a deadlock with MaxOpenConns(1).
type aggregatedRow struct {
	ServiceKey string
	Ts         int64
	Up, Down   int
	AvgPing    sql.NullFloat64
	MinPing    sql.NullInt64
	MaxPing    sql.NullInt64
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

	// Collect all rows first, then close — prevents deadlock with MaxOpenConns(1)
	var batch []aggregatedRow
	for rows.Next() {
		var r aggregatedRow
		if err := rows.Scan(&r.ServiceKey, &r.Ts, &r.Up, &r.Down, &r.AvgPing, &r.MinPing, &r.MaxPing); err != nil {
			continue
		}
		batch = append(batch, r)
	}
	rows.Close()

	for _, r := range batch {
		_, err := database.DB.Exec(`
			INSERT INTO stat_hourly (service_key, timestamp, up, down, ping, ping_min, ping_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(service_key, timestamp) DO UPDATE SET
				up = up + excluded.up,
				down = down + excluded.down`,
			r.ServiceKey, r.Ts, r.Up, r.Down, r.AvgPing, r.MinPing, r.MaxPing)

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

	// Collect all rows first, then close — prevents deadlock with MaxOpenConns(1)
	var batch []aggregatedRow
	for rows.Next() {
		var r aggregatedRow
		if err := rows.Scan(&r.ServiceKey, &r.Ts, &r.Up, &r.Down, &r.AvgPing, &r.MinPing, &r.MaxPing); err != nil {
			continue
		}
		batch = append(batch, r)
	}
	rows.Close()

	for _, r := range batch {
		_, err := database.DB.Exec(`
			INSERT INTO stat_daily (service_key, timestamp, up, down, ping, ping_min, ping_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(service_key, timestamp) DO UPDATE SET
				up = up + excluded.up,
				down = down + excluded.down`,
			r.ServiceKey, r.Ts, r.Up, r.Down, r.AvgPing, r.MinPing, r.MaxPing)

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
