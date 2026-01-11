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
	enabled INTEGER NOT NULL DEFAULT 1,
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
`)
	if err != nil {
		return err
	}

	// For existing installs: add any newly introduced columns.
	// SQLite doesn't support IF NOT EXISTS on ADD COLUMN, so we ignore the error
	// if the column already exists.
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN storage INTEGER NOT NULL DEFAULT 1;`)

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
	err := DB.QueryRow(`SELECT enabled, cpu, memory, network, temp, storage
		FROM resources_ui_config WHERE id = 1`).Scan(
		&config.Enabled, &config.CPU, &config.Memory, &config.Network, &config.Temp, &config.Storage,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveResourcesUIConfig saves resources UI configuration to database
func SaveResourcesUIConfig(config *models.ResourcesUIConfig) error {
	_, err := DB.Exec(`INSERT INTO resources_ui_config (id, enabled, cpu, memory, network, temp, storage, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			enabled=?, cpu=?, memory=?, network=?, temp=?, storage=?, updated_at=datetime('now')`,
		config.Enabled, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
		config.Enabled, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
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
