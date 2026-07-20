package database

import (
	"database/sql"
	"time"
)

// LoadUPSPowerState returns the last confirmed power state for the configured UPS source.
func LoadUPSPowerState() (source string, powerPresent, lossNotified, found bool, err error) {
	var present, notified int
	err = DB.QueryRow(`SELECT source, power_present, loss_notified FROM ups_monitor_state WHERE id = 1`).Scan(&source, &present, &notified)
	if err == sql.ErrNoRows {
		return "", false, false, false, nil
	}
	if err != nil {
		return "", false, false, false, err
	}
	return source, present == 1, notified == 1, true, nil
}

// SaveUPSPowerState persists a confirmed UPS power state for transition detection.
func SaveUPSPowerState(source string, powerPresent, lossNotified bool) error {
	_, err := DB.Exec(`INSERT INTO ups_monitor_state (id, source, power_present, loss_notified, updated_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET source=excluded.source,
			power_present=excluded.power_present, loss_notified=excluded.loss_notified,
			updated_at=excluded.updated_at`,
		source, boolInt(powerPresent), boolInt(lossNotified), time.Now().UTC().Format(time.RFC3339))
	return err
}
