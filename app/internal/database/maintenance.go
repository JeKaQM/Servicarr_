package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"status/app/internal/models"
	"strings"
	"time"
)

const defaultMaintenanceSeedKey = "default_maintenance_schedule_v1"

// Check the schema before adding columns, so genuine migration failures are not ignored.
func migrateMaintenanceSchedules() error {
	rows, err := DB.Query(`PRAGMA table_info(maintenance_schedules)`)
	if err != nil {
		return err
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = true
	}
	err = rows.Err()
	_ = rows.Close() // Release the single SQLite connection before running ALTER TABLE.
	if err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"schedule_type", "TEXT NOT NULL DEFAULT 'weekly'"},
		{"weekdays", "TEXT NOT NULL DEFAULT '[]'"},
		{"starts_at", "TEXT NOT NULL DEFAULT ''"},
		{"ends_at", "TEXT NOT NULL DEFAULT ''"},
	} {
		if !columns[column.name] {
			if _, err := DB.Exec(`ALTER TABLE maintenance_schedules ADD COLUMN ` + column.name + ` ` + column.definition); err != nil {
				return fmt.Errorf("migrate maintenance schedules: %w", err)
			}
		}
	}
	return nil
}

// EnsureDefaultMaintenanceSchedule adds the initial Monday UK maintenance rule once.
func EnsureDefaultMaintenanceSchedule() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var marker string
	err = tx.QueryRow(`SELECT value FROM app_metadata WHERE key = ?`, defaultMaintenanceSeedKey).Scan(&marker)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = tx.Exec(`INSERT OR IGNORE INTO maintenance_schedules
			(id, name, message, level, weekday, start_time, duration_minutes, timezone, suppress_monitoring, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"weekly-monday-maintenance",
			"Monday maintenance",
			"Scheduled maintenance is in progress. Service monitoring is temporarily paused.",
			"warning", 1, "02:55", 30, "Europe/London", 1, 1, now, now)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO app_metadata (key, value) VALUES (?, ?)`, defaultMaintenanceSeedKey, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetMaintenanceSchedules returns all maintenance schedules.
func GetMaintenanceSchedules() ([]models.MaintenanceSchedule, error) {
	rows, err := DB.Query(`SELECT id, name, message, level, weekday, start_time, duration_minutes,
		timezone, suppress_monitoring, enabled, created_at, updated_at, schedule_type, weekdays, starts_at, ends_at
		FROM maintenance_schedules ORDER BY weekday, start_time, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schedules := make([]models.MaintenanceSchedule, 0)
	for rows.Next() {
		var schedule models.MaintenanceSchedule
		var suppress, enabled int
		var weekdays string
		if err := rows.Scan(
			&schedule.ID, &schedule.Name, &schedule.Message, &schedule.Level,
			&schedule.Weekday, &schedule.StartTime, &schedule.DurationMinutes,
			&schedule.Timezone, &suppress, &enabled, &schedule.CreatedAt, &schedule.UpdatedAt,
			&schedule.ScheduleType, &weekdays, &schedule.StartsAt, &schedule.EndsAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(weekdays), &schedule.Weekdays); err != nil {
			return nil, fmt.Errorf("decode maintenance weekdays: %w", err)
		}
		if len(schedule.Weekdays) == 0 {
			schedule.Weekdays = nil // Legacy rows use the single weekday column.
		}
		schedule.SuppressMonitoring = suppress == 1
		schedule.Enabled = enabled == 1
		schedules = append(schedules, schedule)
	}
	return schedules, rows.Err()
}

// SaveMaintenanceSchedule creates or updates a maintenance schedule.
func SaveMaintenanceSchedule(schedule *models.MaintenanceSchedule) error {
	if schedule == nil || schedule.ID == "" {
		return fmt.Errorf("maintenance schedule ID is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := schedule.CreatedAt
	if createdAt == "" {
		createdAt = now
	}
	weekdays := "[]"
	if len(schedule.Weekdays) > 0 {
		encoded, err := json.Marshal(schedule.Weekdays)
		if err != nil {
			return err
		}
		weekdays = string(encoded)
	}
	scheduleType := strings.TrimSpace(schedule.ScheduleType)
	if scheduleType == "" {
		scheduleType = "weekly"
	}
	_, err := DB.Exec(`INSERT INTO maintenance_schedules
		(id, name, message, level, weekday, start_time, duration_minutes, timezone, suppress_monitoring, enabled, created_at, updated_at,
		schedule_type, weekdays, starts_at, ends_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			message=excluded.message,
			level=excluded.level,
			schedule_type=excluded.schedule_type,
			weekdays=excluded.weekdays,
			starts_at=excluded.starts_at,
			ends_at=excluded.ends_at,
			weekday=excluded.weekday,
			start_time=excluded.start_time,
			duration_minutes=excluded.duration_minutes,
			timezone=excluded.timezone,
			suppress_monitoring=excluded.suppress_monitoring,
			enabled=excluded.enabled,
			updated_at=excluded.updated_at`,
		schedule.ID, schedule.Name, schedule.Message, schedule.Level, schedule.Weekday,
		schedule.StartTime, schedule.DurationMinutes, schedule.Timezone,
		boolInt(schedule.SuppressMonitoring), boolInt(schedule.Enabled), createdAt, now,
		scheduleType, weekdays, schedule.StartsAt, schedule.EndsAt)
	if err == nil {
		// Preserve the original creation time when updating a schedule by ID.
		err = DB.QueryRow(`SELECT created_at FROM maintenance_schedules WHERE id = ?`, schedule.ID).Scan(&schedule.CreatedAt)
		schedule.ScheduleType = scheduleType
		schedule.UpdatedAt = now
	}
	return err
}

// DeleteMaintenanceSchedule removes a maintenance schedule.
func DeleteMaintenanceSchedule(id string) error {
	_, err := DB.Exec(`DELETE FROM maintenance_schedules WHERE id = ?`, id)
	return err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
