package database

import "status/app/internal/models"

// IsSetupComplete checks if initial setup has been completed
func IsSetupComplete() (bool, error) {
	var complete int
	err := DB.QueryRow(`SELECT COALESCE((SELECT setup_complete FROM app_settings WHERE id = 1), 0)`).Scan(&complete)
	if err != nil {
		return false, err
	}
	return complete == 1, nil
}

// LoadAppSettings loads application settings from database
func LoadAppSettings() (*models.AppSettings, error) {
	row := DB.QueryRow(`SELECT setup_complete, username, password_hash, auth_secret, 
		COALESCE(app_name, 'Service Status'), COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM app_settings WHERE id = 1`)

	var settings models.AppSettings
	var setupComplete int
	err := row.Scan(&setupComplete, &settings.Username, &settings.PasswordHash,
		&settings.AuthSecret, &settings.AppName, &settings.CreatedAt, &settings.UpdatedAt)
	if err != nil {
		return nil, err
	}
	settings.SetupComplete = setupComplete == 1
	if settings.AppName == "" {
		settings.AppName = "Service Status"
	}
	return &settings, nil
}

// SaveAppSettings saves application settings to database
func SaveAppSettings(settings *models.AppSettings) error {
	setupComplete := 0
	if settings.SetupComplete {
		setupComplete = 1
	}
	if settings.AppName == "" {
		settings.AppName = "Service Status"
	}

	_, err := DB.Exec(`INSERT INTO app_settings (id, setup_complete, username, password_hash, auth_secret, app_name, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
		setup_complete = excluded.setup_complete,
		username = excluded.username,
		password_hash = excluded.password_hash,
		auth_secret = excluded.auth_secret,
		app_name = excluded.app_name,
		updated_at = datetime('now')`,
		setupComplete, settings.Username, settings.PasswordHash, settings.AuthSecret, settings.AppName)
	return err
}
