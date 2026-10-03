package database

import (
	"database/sql"
	"strings"
	"time"
)

// EndedStatusAlertRetention is how long an ended banner stays listed, so it
// can be reused, before the scheduler deletes it.
const EndedStatusAlertRetention = 30 * 24 * time.Hour

// ManualStatusAlert is an administrator-written banner as stored. Start and
// end are UTC RFC3339 instants, so they compare correctly as text.
type ManualStatusAlert struct {
	ID          string
	ServiceKeys []string // Empty shows the banner at the top of the page
	Message     string
	Level       string
	CreatedAt   string
	StartsAt    string // Empty shows the banner from creation
	EndsAt      string // Empty shows it until it is removed
}

// GetManualStatusAlerts returns the stored banners, newest first.
func GetManualStatusAlerts() ([]ManualStatusAlert, error) {
	rows, err := DB.Query(`SELECT id, service_key, message, level, created_at, starts_at, ends_at
		FROM status_alerts ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := make([]ManualStatusAlert, 0)
	for rows.Next() {
		var alert ManualStatusAlert
		var serviceKeys sql.NullString
		if err := rows.Scan(&alert.ID, &serviceKeys, &alert.Message, &alert.Level, &alert.CreatedAt,
			&alert.StartsAt, &alert.EndsAt); err != nil {
			return nil, err
		}
		alert.ServiceKeys = splitStatusAlertKeys(serviceKeys.String)
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

// CreateManualStatusAlert stores a new banner; an existing ID is an error.
func CreateManualStatusAlert(alert ManualStatusAlert) error {
	_, err := DB.Exec(`INSERT INTO status_alerts (id, service_key, message, level, created_at, starts_at, ends_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		alert.ID, joinStatusAlertKeys(alert.ServiceKeys), alert.Message, alert.Level, alert.CreatedAt, alert.StartsAt, alert.EndsAt)
	return err
}

// UpdateManualStatusAlert replaces a banner's content and schedule, keeping
// its creation time.
func UpdateManualStatusAlert(alert ManualStatusAlert) error {
	_, err := DB.Exec(`UPDATE status_alerts SET service_key = ?, message = ?, level = ?, starts_at = ?, ends_at = ?
		WHERE id = ?`,
		joinStatusAlertKeys(alert.ServiceKeys), alert.Message, alert.Level, alert.StartsAt, alert.EndsAt, alert.ID)
	return err
}

// GetManualStatusAlert returns one banner, or nil when there is none.
func GetManualStatusAlert(id string) (*ManualStatusAlert, error) {
	alerts, err := GetManualStatusAlerts()
	if err != nil {
		return nil, err
	}
	for i := range alerts {
		if alerts[i].ID == id {
			return &alerts[i], nil
		}
	}
	return nil, nil
}

// DeleteManualStatusAlert removes a banner and reports whether it existed.
func DeleteManualStatusAlert(id string) (bool, error) {
	result, err := DB.Exec(`DELETE FROM status_alerts WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// DeleteEndedStatusAlerts removes banners that ended at or before the cutoff
// and reports how many there were.
func DeleteEndedStatusAlerts(cutoff time.Time) (int64, error) {
	result, err := DB.Exec(`DELETE FROM status_alerts WHERE ends_at != '' AND ends_at <= ?`,
		cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// joinStatusAlertKeys stores no services as NULL, as older versions did.
func joinStatusAlertKeys(keys []string) any {
	if len(keys) == 0 {
		return nil
	}
	return strings.Join(keys, ",")
}

func splitStatusAlertKeys(value string) []string {
	var keys []string
	for _, key := range strings.Split(value, ",") {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}
