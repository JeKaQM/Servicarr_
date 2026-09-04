package database

import (
	"database/sql"
	"time"
)

type UPSPowerState struct {
	Source       string
	PowerPresent bool
	LossNotified bool
	UpdatedAt    time.Time
}

func GetUPSPowerState() (*UPSPowerState, error) {
	var state UPSPowerState
	var present, notified int
	var updatedAt string
	err := DB.QueryRow(`SELECT source, power_present, loss_notified, updated_at FROM ups_monitor_state WHERE id = 1`).Scan(
		&state.Source, &present, &notified, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, err
	}
	state.PowerPresent = present != 0
	state.LossNotified = notified != 0
	state.UpdatedAt = parsed
	return &state, nil
}

// LoadUPSPowerState returns the last confirmed power state for the configured UPS source.
func LoadUPSPowerState() (source string, powerPresent, lossNotified, found bool, err error) {
	state, err := GetUPSPowerState()
	if err != nil {
		return "", false, false, false, err
	}
	if state == nil {
		return "", false, false, false, nil
	}
	return state.Source, state.PowerPresent, state.LossNotified, true, nil
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
