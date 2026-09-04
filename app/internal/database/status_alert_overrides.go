package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// StatusAlertOverride stores an administrator adjustment for one generated
// banner occurrence. The timestamp prevents a hidden outage from suppressing
// a later outage that reuses the same stable alert ID.
type StatusAlertOverride struct {
	AlertID      string
	OccurrenceAt string
	Message      *string
	Level        *string
	Hidden       bool
}

func StatusAlertOverrideKey(alertID, occurrenceAt string) string {
	return alertID + "\x00" + occurrenceAt
}

func GetStatusAlertOverrides() (map[string]StatusAlertOverride, error) {
	rows, err := DB.Query(`SELECT alert_id, occurrence_at, message, level, hidden FROM status_alert_overrides`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	overrides := make(map[string]StatusAlertOverride)
	for rows.Next() {
		var override StatusAlertOverride
		var message, level sql.NullString
		var hidden int
		if err := rows.Scan(&override.AlertID, &override.OccurrenceAt, &message, &level, &hidden); err != nil {
			return nil, err
		}
		if message.Valid {
			value := message.String
			override.Message = &value
		}
		if level.Valid {
			value := level.String
			override.Level = &value
		}
		override.Hidden = hidden != 0
		overrides[StatusAlertOverrideKey(override.AlertID, override.OccurrenceAt)] = override
	}
	return overrides, rows.Err()
}

func SaveStatusAlertOverride(override StatusAlertOverride) error {
	override.AlertID = strings.TrimSpace(override.AlertID)
	override.OccurrenceAt = strings.TrimSpace(override.OccurrenceAt)
	if override.AlertID == "" || override.OccurrenceAt == "" {
		return errors.New("alert id and occurrence time are required")
	}
	_, err := DB.Exec(`INSERT INTO status_alert_overrides
		(alert_id, occurrence_at, message, level, hidden, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(alert_id, occurrence_at) DO UPDATE SET
			message = COALESCE(excluded.message, status_alert_overrides.message),
			level = COALESCE(excluded.level, status_alert_overrides.level),
			hidden = excluded.hidden,
			updated_at = excluded.updated_at`,
		override.AlertID, override.OccurrenceAt, override.Message, override.Level,
		boolToDatabaseInt(override.Hidden), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
