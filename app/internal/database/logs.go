package database

import "status/app/internal/models"

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
	return logs, nil
}

// GetLogStats returns statistics about logs
func GetLogStats() (*models.LogStats, error) {
	var stats models.LogStats

	err := DB.QueryRow(`SELECT COUNT(*) FROM system_logs`).Scan(&stats.TotalLogs)
	if err != nil {
		return nil, err
	}

	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'error'`).Scan(&stats.ErrorCount)
	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'warn'`).Scan(&stats.WarnCount)
	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'info'`).Scan(&stats.InfoCount)
	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'debug'`).Scan(&stats.DebugCount)

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

// PruneLogs removes old logs to keep the database size manageable (keeps last N logs)
func PruneLogs(keepCount int) error {
	_, err := DB.Exec(`DELETE FROM system_logs WHERE id NOT IN (
		SELECT id FROM system_logs ORDER BY timestamp DESC, id DESC LIMIT ?
	)`, keepCount)
	return err
}
