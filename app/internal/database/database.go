package database

import (
	"database/sql"
	"status/app/internal/models"
	"time"

	_ "modernc.org/sqlite"
)

// DB is the global database instance
var DB *sql.DB

// Init initializes the database connection and creates schema
func Init(dbPath string) error {
	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	return EnsureSchema()
}

// EnsureSchema creates all necessary database tables
func EnsureSchema() error {
	_, err := DB.Exec(`
CREATE TABLE IF NOT EXISTS samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  taken_at TEXT NOT NULL,
  service_key TEXT NOT NULL,
  ok INTEGER NOT NULL,
  http_status INTEGER,
  latency_ms INTEGER
);
CREATE INDEX IF NOT EXISTS idx_samples_taken ON samples(taken_at);
CREATE INDEX IF NOT EXISTS idx_samples_service ON samples(service_key);

CREATE TABLE IF NOT EXISTS ip_blocks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip_address TEXT NOT NULL,
  blocked_at TEXT,
  attempts INTEGER NOT NULL DEFAULT 1,
  expires_at TEXT NOT NULL,
  reason TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ip_blocks_ip ON ip_blocks(ip_address);
CREATE INDEX IF NOT EXISTS idx_ip_blocks_expires ON ip_blocks(expires_at);

CREATE TABLE IF NOT EXISTS service_state (
  service_key TEXT PRIMARY KEY,
  disabled INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT
);

CREATE TABLE IF NOT EXISTS services (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  url TEXT NOT NULL,
  service_type TEXT NOT NULL DEFAULT 'custom',
  icon TEXT,
  icon_url TEXT,
  api_token TEXT,
  display_order INTEGER NOT NULL DEFAULT 0,
  visible INTEGER NOT NULL DEFAULT 1,
  check_type TEXT NOT NULL DEFAULT 'http',
  check_interval INTEGER NOT NULL DEFAULT 60,
  timeout INTEGER NOT NULL DEFAULT 5,
  expected_min INTEGER NOT NULL DEFAULT 200,
  expected_max INTEGER NOT NULL DEFAULT 399,
  created_at TEXT NOT NULL,
  updated_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_services_key ON services(key);
CREATE INDEX IF NOT EXISTS idx_services_order ON services(display_order);

CREATE TABLE IF NOT EXISTS alert_config (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  enabled INTEGER NOT NULL DEFAULT 0,
  smtp_host TEXT,
  smtp_port INTEGER DEFAULT 587,
  smtp_user TEXT,
  smtp_password TEXT,
  alert_email TEXT,
  from_email TEXT,
  alert_on_down INTEGER NOT NULL DEFAULT 1,
  alert_on_degraded INTEGER NOT NULL DEFAULT 1,
  alert_on_up INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT
);

CREATE TABLE IF NOT EXISTS resources_ui_config (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	enabled INTEGER NOT NULL DEFAULT 0,
	glances_url TEXT,
	cpu INTEGER NOT NULL DEFAULT 1,
	memory INTEGER NOT NULL DEFAULT 1,
	network INTEGER NOT NULL DEFAULT 1,
	temp INTEGER NOT NULL DEFAULT 1,
	storage INTEGER NOT NULL DEFAULT 1,
	updated_at TEXT
);

CREATE TABLE IF NOT EXISTS service_status_history (
  service_key TEXT PRIMARY KEY,
  ok INTEGER NOT NULL,
  degraded INTEGER NOT NULL,
  updated_at TEXT
);

CREATE TABLE IF NOT EXISTS status_alerts (
  id TEXT PRIMARY KEY,
  service_key TEXT,
  message TEXT NOT NULL,
  level TEXT NOT NULL DEFAULT 'info',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS app_settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  setup_complete INTEGER NOT NULL DEFAULT 0,
  username TEXT,
  password_hash TEXT,
  auth_secret TEXT,
  created_at TEXT,
  updated_at TEXT
);
`)
	if err != nil {
		return err
	}

	// For existing installs: add any newly introduced columns.
	// SQLite doesn't support IF NOT EXISTS on ADD COLUMN, so we ignore the error
	// if the column already exists.
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN storage INTEGER NOT NULL DEFAULT 1;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN glances_url TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE services ADD COLUMN icon_url TEXT;`)

	return nil
}

// InsertSample records a service check sample
func InsertSample(ts time.Time, key string, ok bool, status int, ms *int) {
	okInt := 0
	if ok {
		okInt = 1
	}
	var msVal any
	if ms != nil {
		msVal = *ms
	}

	_, _ = DB.Exec(`INSERT INTO samples (taken_at,service_key,ok,http_status,latency_ms)
		VALUES (?,?,?,?,?)`,
		ts.UTC().Format(time.RFC3339), key, okInt, status, msVal)
}

// LoadAlertConfig loads email alert configuration from database
func LoadAlertConfig() (*models.AlertConfig, error) {
	var config models.AlertConfig
	err := DB.QueryRow(`SELECT enabled, smtp_host, smtp_port, smtp_user, smtp_password, alert_email, from_email, alert_on_down, alert_on_degraded, alert_on_up 
		FROM alert_config WHERE id = 1`).Scan(
		&config.Enabled, &config.SMTPHost, &config.SMTPPort, &config.SMTPUser,
		&config.SMTPPassword, &config.AlertEmail, &config.FromEmail,
		&config.AlertOnDown, &config.AlertOnDegraded, &config.AlertOnUp)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveAlertConfig saves email alert configuration to database
func SaveAlertConfig(config *models.AlertConfig) error {
	_, err := DB.Exec(`INSERT INTO alert_config (id, enabled, smtp_host, smtp_port, smtp_user, smtp_password, alert_email, from_email, alert_on_down, alert_on_degraded, alert_on_up, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET 
			enabled=?, smtp_host=?, smtp_port=?, smtp_user=?, smtp_password=?, alert_email=?, from_email=?,
			alert_on_down=?, alert_on_degraded=?, alert_on_up=?, updated_at=datetime('now')`,
		config.Enabled, config.SMTPHost, config.SMTPPort, config.SMTPUser, config.SMTPPassword,
		config.AlertEmail, config.FromEmail, config.AlertOnDown, config.AlertOnDegraded, config.AlertOnUp,
		config.Enabled, config.SMTPHost, config.SMTPPort, config.SMTPUser, config.SMTPPassword,
		config.AlertEmail, config.FromEmail, config.AlertOnDown, config.AlertOnDegraded, config.AlertOnUp)
	return err
}

// LoadResourcesUIConfig loads resources UI configuration from database
func LoadResourcesUIConfig() (*models.ResourcesUIConfig, error) {
	var config models.ResourcesUIConfig
	var glancesURL sql.NullString
	err := DB.QueryRow(`SELECT enabled, COALESCE(glances_url, ''), cpu, memory, network, temp, storage
		FROM resources_ui_config WHERE id = 1`).Scan(
		&config.Enabled, &glancesURL, &config.CPU, &config.Memory, &config.Network, &config.Temp, &config.Storage,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	config.GlancesURL = glancesURL.String
	return &config, nil
}

// SaveResourcesUIConfig saves resources UI configuration to database
func SaveResourcesUIConfig(config *models.ResourcesUIConfig) error {
	_, err := DB.Exec(`INSERT INTO resources_ui_config (id, enabled, glances_url, cpu, memory, network, temp, storage, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			enabled=?, glances_url=?, cpu=?, memory=?, network=?, temp=?, storage=?, updated_at=datetime('now')`,
		config.Enabled, config.GlancesURL, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
		config.Enabled, config.GlancesURL, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
	)
	return err
}

// GetServiceDisabledState loads service disabled state from database
func GetServiceDisabledState(key string) (bool, error) {
	var disabled int
	err := DB.QueryRow(`SELECT disabled FROM service_state WHERE service_key = ?`, key).Scan(&disabled)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return disabled != 0, nil
}

// SetServiceDisabledState updates service disabled state in database
func SetServiceDisabledState(key string, disabled bool) error {
	disabledInt := 0
	if disabled {
		disabledInt = 1
	}
	_, err := DB.Exec(`
		INSERT INTO service_state (service_key, disabled, updated_at) 
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(service_key) DO UPDATE SET disabled=?, updated_at=datetime('now')`,
		key, disabledInt, disabledInt)
	return err
}

// GetAllServices returns all services from the database ordered by display_order
func GetAllServices() ([]models.ServiceConfig, error) {
	rows, err := DB.Query(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       created_at, COALESCE(updated_at, '')
		FROM services ORDER BY display_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []models.ServiceConfig
	for rows.Next() {
		var s models.ServiceConfig
		var visible int
		err := rows.Scan(&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
			&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
			&s.ExpectedMin, &s.ExpectedMax, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		s.Visible = visible != 0
		services = append(services, s)
	}
	return services, nil
}

// GetVisibleServices returns only visible services from the database
func GetVisibleServices() ([]models.ServiceConfig, error) {
	rows, err := DB.Query(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       created_at, COALESCE(updated_at, '')
		FROM services WHERE visible = 1 ORDER BY display_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []models.ServiceConfig
	for rows.Next() {
		var s models.ServiceConfig
		var visible int
		err := rows.Scan(&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
			&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
			&s.ExpectedMin, &s.ExpectedMax, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		s.Visible = visible != 0
		services = append(services, s)
	}
	return services, nil
}

// GetServiceByID returns a service by its ID
func GetServiceByID(id int) (*models.ServiceConfig, error) {
	var s models.ServiceConfig
	var visible int
	err := DB.QueryRow(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       created_at, COALESCE(updated_at, '')
		FROM services WHERE id = ?`, id).Scan(
		&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
		&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
		&s.ExpectedMin, &s.ExpectedMax, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Visible = visible != 0
	return &s, nil
}

// GetServiceByKey returns a service by its key
func GetServiceByKey(key string) (*models.ServiceConfig, error) {
	var s models.ServiceConfig
	var visible int
	err := DB.QueryRow(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       created_at, COALESCE(updated_at, '')
		FROM services WHERE key = ?`, key).Scan(
		&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
		&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
		&s.ExpectedMin, &s.ExpectedMax, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Visible = visible != 0
	return &s, nil
}

// CreateService inserts a new service into the database
func CreateService(s *models.ServiceConfig) (int64, error) {
	visible := 0
	if s.Visible {
		visible = 1
	}

	// Get the next display order
	var maxOrder int
	_ = DB.QueryRow(`SELECT COALESCE(MAX(display_order), -1) FROM services`).Scan(&maxOrder)
	s.DisplayOrder = maxOrder + 1

	result, err := DB.Exec(`
		INSERT INTO services (key, name, url, service_type, icon, icon_url, api_token, display_order, visible,
		                      check_type, check_interval, timeout, expected_min, expected_max, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		s.Key, s.Name, s.URL, s.ServiceType, s.Icon, s.IconURL, s.APIToken, s.DisplayOrder, visible,
		s.CheckType, s.CheckInterval, s.Timeout, s.ExpectedMin, s.ExpectedMax)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateService updates an existing service
func UpdateService(s *models.ServiceConfig) error {
	visible := 0
	if s.Visible {
		visible = 1
	}
	_, err := DB.Exec(`
		UPDATE services SET name=?, url=?, service_type=?, icon=?, icon_url=?, api_token=?, display_order=?,
		                    visible=?, check_type=?, check_interval=?, timeout=?, expected_min=?,
		                    expected_max=?, updated_at=datetime('now')
		WHERE id = ?`,
		s.Name, s.URL, s.ServiceType, s.Icon, s.IconURL, s.APIToken, s.DisplayOrder, visible,
		s.CheckType, s.CheckInterval, s.Timeout, s.ExpectedMin, s.ExpectedMax, s.ID)
	return err
}

// DeleteService removes a service from the database
func DeleteService(id int) error {
	_, err := DB.Exec(`DELETE FROM services WHERE id = ?`, id)
	return err
}

// UpdateServiceVisibility toggles service visibility
func UpdateServiceVisibility(id int, visible bool) error {
	v := 0
	if visible {
		v = 1
	}
	_, err := DB.Exec(`UPDATE services SET visible = ?, updated_at = datetime('now') WHERE id = ?`, v, id)
	return err
}

// UpdateServiceOrder updates the display order of services
func UpdateServiceOrder(orders map[int]int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE services SET display_order = ?, updated_at = datetime('now') WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for id, order := range orders {
		_, err := stmt.Exec(order, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetServiceCount returns the number of services
func GetServiceCount() (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM services`).Scan(&count)
	return count, err
}

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
		COALESCE(created_at, ''), COALESCE(updated_at, '')
		FROM app_settings WHERE id = 1`)
	
	var settings models.AppSettings
	var setupComplete int
	err := row.Scan(&setupComplete, &settings.Username, &settings.PasswordHash, 
		&settings.AuthSecret, &settings.CreatedAt, &settings.UpdatedAt)
	if err != nil {
		return nil, err
	}
	settings.SetupComplete = setupComplete == 1
	return &settings, nil
}

// SaveAppSettings saves application settings to database
func SaveAppSettings(settings *models.AppSettings) error {
	setupComplete := 0
	if settings.SetupComplete {
		setupComplete = 1
	}
	
	_, err := DB.Exec(`INSERT INTO app_settings (id, setup_complete, username, password_hash, auth_secret, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
		setup_complete = excluded.setup_complete,
		username = excluded.username,
		password_hash = excluded.password_hash,
		auth_secret = excluded.auth_secret,
		updated_at = datetime('now')`,
		setupComplete, settings.Username, settings.PasswordHash, settings.AuthSecret)
	return err
}
