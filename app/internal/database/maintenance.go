package database

import (
	"database/sql"
	"fmt"
	"status/app/internal/models"
	"time"
)

const defaultMaintenanceSeedKey = "default_maintenance_schedule_v1"

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

// GetMaintenanceSchedules returns all recurring maintenance rules.
func GetMaintenanceSchedules() ([]models.MaintenanceSchedule, error) {
	rows, err := DB.Query(`SELECT id, name, message, level, weekday, start_time, duration_minutes,
		timezone, suppress_monitoring, enabled, created_at, updated_at
		FROM maintenance_schedules ORDER BY weekday, start_time, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schedules := make([]models.MaintenanceSchedule, 0)
	for rows.Next() {
		var schedule models.MaintenanceSchedule
		var suppress, enabled int
		if err := rows.Scan(
			&schedule.ID, &schedule.Name, &schedule.Message, &schedule.Level,
			&schedule.Weekday, &schedule.StartTime, &schedule.DurationMinutes,
			&schedule.Timezone, &suppress, &enabled, &schedule.CreatedAt, &schedule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		schedule.SuppressMonitoring = suppress == 1
		schedule.Enabled = enabled == 1
		schedules = append(schedules, schedule)
	}
	return schedules, rows.Err()
}

// SaveMaintenanceSchedule creates or updates a recurring maintenance rule.
func SaveMaintenanceSchedule(schedule *models.MaintenanceSchedule) error {
	if schedule == nil || schedule.ID == "" {
		return fmt.Errorf("maintenance schedule ID is required")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := schedule.CreatedAt
	if createdAt == "" {
		createdAt = now
	}
	_, err := DB.Exec(`INSERT INTO maintenance_schedules
		(id, name, message, level, weekday, start_time, duration_minutes, timezone, suppress_monitoring, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			message=excluded.message,
			level=excluded.level,
			weekday=excluded.weekday,
			start_time=excluded.start_time,
			duration_minutes=excluded.duration_minutes,
			timezone=excluded.timezone,
			suppress_monitoring=excluded.suppress_monitoring,
			enabled=excluded.enabled,
			updated_at=excluded.updated_at`,
		schedule.ID, schedule.Name, schedule.Message, schedule.Level, schedule.Weekday,
		schedule.StartTime, schedule.DurationMinutes, schedule.Timezone,
		boolInt(schedule.SuppressMonitoring), boolInt(schedule.Enabled), createdAt, now)
	if err == nil {
		schedule.CreatedAt = createdAt
		schedule.UpdatedAt = now
	}
	return err
}

// DeleteMaintenanceSchedule removes a recurring maintenance rule.
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
