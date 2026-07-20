package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ServiceOutageState is the latest confirmed availability transition for a service.
type ServiceOutageState struct {
	ServiceKey  string
	ServiceName string
	IsDown      bool
	DownSince   *time.Time
	RestoredAt  *time.Time
	AlertSent   bool
	UpdatedAt   time.Time
}

// RecordServiceOutageState records confirmed transitions without extending their
// original down or recovery timestamps on repeated checks.
func RecordServiceOutageState(serviceKey string, isDown, alertSent bool, observedAt time.Time) error {
	serviceKey = strings.TrimSpace(serviceKey)
	if serviceKey == "" {
		return errors.New("service key is required")
	}

	timestamp := observedAt.UTC().Format(time.RFC3339Nano)
	var downSince any
	alertSentInt := 0
	if isDown {
		downSince = timestamp
		if alertSent {
			alertSentInt = 1
		}
	}

	_, err := DB.Exec(`INSERT INTO service_outage_state
		(service_key, is_down, down_since, restored_at, alert_sent, updated_at)
		VALUES (?, ?, ?, NULL, ?, ?)
		ON CONFLICT(service_key) DO UPDATE SET
			is_down = excluded.is_down,
			down_since = CASE
				WHEN excluded.is_down = 1 AND service_outage_state.is_down = 0 THEN excluded.down_since
				WHEN excluded.is_down = 1 THEN service_outage_state.down_since
				ELSE NULL
			END,
			restored_at = CASE
				WHEN excluded.is_down = 0 AND service_outage_state.is_down = 1 THEN excluded.updated_at
				WHEN excluded.is_down = 0 THEN service_outage_state.restored_at
				ELSE NULL
			END,
			alert_sent = CASE
				WHEN excluded.is_down = 1 AND service_outage_state.is_down = 0 THEN excluded.alert_sent
				WHEN excluded.is_down = 1 THEN MAX(service_outage_state.alert_sent, excluded.alert_sent)
				ELSE 0
			END,
			updated_at = excluded.updated_at`,
		serviceKey, boolToDatabaseInt(isDown), downSince, alertSentInt, timestamp)
	return err
}

// GetVisibleServiceOutageStates returns state only for visible, enabled services.
func GetVisibleServiceOutageStates() ([]ServiceOutageState, error) {
	rows, err := DB.Query(`SELECT o.service_key,
		COALESCE(NULLIF(s.name, ''), o.service_key),
		o.is_down, o.down_since, o.restored_at, o.alert_sent, o.updated_at
		FROM service_outage_state o
		JOIN services s ON s.key = o.service_key
		LEFT JOIN service_state ss ON ss.service_key = o.service_key
		WHERE s.visible = 1 AND COALESCE(ss.disabled, 0) = 0
		ORDER BY o.is_down DESC, s.display_order ASC, s.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make([]ServiceOutageState, 0)
	for rows.Next() {
		var state ServiceOutageState
		var isDown, alertSent int
		var downSince, restoredAt sql.NullString
		var updatedAt string
		if err := rows.Scan(&state.ServiceKey, &state.ServiceName, &isDown, &downSince, &restoredAt, &alertSent, &updatedAt); err != nil {
			return nil, err
		}
		state.IsDown = isDown != 0
		state.AlertSent = alertSent != 0
		if state.DownSince, err = parseOptionalOutageTime(downSince); err != nil {
			return nil, err
		}
		if state.RestoredAt, err = parseOptionalOutageTime(restoredAt); err != nil {
			return nil, err
		}
		state.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse outage update time: %w", err)
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

// DeleteServiceOutageState removes transient banner state for a deleted service.
func DeleteServiceOutageState(serviceKey string) error {
	_, err := DB.Exec(`DELETE FROM service_outage_state WHERE service_key = ?`, serviceKey)
	return err
}

func parseOptionalOutageTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, fmt.Errorf("parse outage time: %w", err)
	}
	return &parsed, nil
}

func boolToDatabaseInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
