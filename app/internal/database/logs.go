package database

import (
	"fmt"

	"status/app/internal/models"
)

// ============================================
// Logging Functions
// ============================================

// LogLevel constants
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// LogCategory constants
const (
	LogCategoryCheck    = "check"
	LogCategoryEmail    = "email"
	LogCategorySecurity = "security"
	LogCategorySystem   = "system"
	LogCategorySchedule = "schedule"
	LogCategoryAudit    = "audit"
)

// InsertLog adds a new log entry
func InsertLog(level, category, service, message, details string) error {
	_, err := DB.Exec(`INSERT INTO system_logs (timestamp, level, category, service, message, details)
		VALUES (datetime('now'), ?, ?, ?, ?, ?)`,
		level, category, service, message, details)
	return err
}

// GetLogs retrieves logs with optional filtering
func GetLogs(limit int, level, category, service string, offset int) ([]models.LogEntry, error) {
	query := `SELECT id, timestamp, level, category, COALESCE(service, ''), message, COALESCE(details, '')
		FROM system_logs WHERE 1=1`
	args := []interface{}{}

	if level != "" {
		query += " AND level = ?"
		args = append(args, level)
	}
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if service != "" {
		query += " AND service = ?"
		args = append(args, service)
	}

	query += " ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.LogEntry
	for rows.Next() {
		var log models.LogEntry
		if err := rows.Scan(&log.ID, &log.Timestamp, &log.Level, &log.Category, &log.Service, &log.Message, &log.Details); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// GetLogStats returns statistics about logs
func GetLogStats() (*models.LogStats, error) {
	var stats models.LogStats

	// One pass over the table instead of a scan per counter.
	err := DB.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(level = 'error'), 0), COALESCE(SUM(level = 'warn'), 0),
		COALESCE(SUM(level = 'info'), 0), COALESCE(SUM(level = 'debug'), 0),
		COALESCE(SUM(category = 'audit'), 0)
		FROM system_logs`).Scan(&stats.TotalLogs, &stats.ErrorCount, &stats.WarnCount,
		&stats.InfoCount, &stats.DebugCount, &stats.AuditCount)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

// ClearLogs clears logs older than specified days, or all logs if days is 0
func ClearLogs(days int) error {
	if days == 0 {
		_, err := DB.Exec(`DELETE FROM system_logs`)
		return err
	}
	_, err := DB.Exec(`DELETE FROM system_logs WHERE timestamp < datetime('now', '-' || ? || ' days')`, days)
	return err
}

// LogRetention bounds the system log. Routine service-check entries are capped
// separately so their volume cannot evict audit, security and system history.
type LogRetention struct {
	CheckEntries int // newest service-check entries kept
	OtherEntries int // newest entries kept across all other categories
	OtherDays    int // other entries older than this many days are removed
}

// DefaultLogRetention keeps the recent check log plus ninety days of audit and
// security history; the entry cap guards against floods of failed logins.
var DefaultLogRetention = LogRetention{CheckEntries: 10000, OtherEntries: 20000, OtherDays: 90}

// PruneLogs applies a retention policy to the system log.
func PruneLogs(policy LogRetention) error {
	if _, err := DB.Exec(`DELETE FROM system_logs WHERE category = ? AND id NOT IN (
		SELECT id FROM system_logs WHERE category = ? ORDER BY timestamp DESC, id DESC LIMIT ?
	)`, LogCategoryCheck, LogCategoryCheck, policy.CheckEntries); err != nil {
		return err
	}
	if _, err := DB.Exec(`DELETE FROM system_logs WHERE category != ? AND timestamp < datetime('now', ?)`,
		LogCategoryCheck, fmt.Sprintf("-%d days", policy.OtherDays)); err != nil {
		return err
	}
	_, err := DB.Exec(`DELETE FROM system_logs WHERE category != ? AND id NOT IN (
		SELECT id FROM system_logs WHERE category != ? ORDER BY timestamp DESC, id DESC LIMIT ?
	)`, LogCategoryCheck, LogCategoryCheck, policy.OtherEntries)
	return err
}
