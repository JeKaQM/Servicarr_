package database

import (
	"database/sql"
	"status/app/internal/models"
	"time"

	// Import SQLite driver for database/sql usage
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

	// SQLite tuning for production
	DB.SetMaxOpenConns(1)                  // SQLite only supports one writer at a time
	DB.SetMaxIdleConns(1)
	DB.SetConnMaxLifetime(0)               // Connections don't expire
	DB.Exec("PRAGMA journal_mode=WAL")      // Write-Ahead Logging for better concurrency
	DB.Exec("PRAGMA busy_timeout=5000")     // Wait up to 5s when database is locked
	DB.Exec("PRAGMA synchronous=NORMAL")    // Safe with WAL mode, better performance
	DB.Exec("PRAGMA foreign_keys=ON")       // Enforce foreign key constraints

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

CREATE TABLE IF NOT EXISTS ip_whitelist (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip_address TEXT NOT NULL UNIQUE,
  note TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ip_whitelist_ip ON ip_whitelist(ip_address);

CREATE TABLE IF NOT EXISTS ip_blacklist (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ip_address TEXT NOT NULL UNIQUE,
  permanent INTEGER NOT NULL DEFAULT 0,
  note TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ip_blacklist_ip ON ip_blacklist(ip_address);

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
  status_page_url TEXT,
  smtp_skip_verify INTEGER NOT NULL DEFAULT 0,
  alert_on_down INTEGER NOT NULL DEFAULT 1,
  alert_on_degraded INTEGER NOT NULL DEFAULT 1,
  alert_on_up INTEGER NOT NULL DEFAULT 0,
  discord_webhook_url TEXT,
  discord_enabled INTEGER NOT NULL DEFAULT 0,
  telegram_bot_token TEXT,
  telegram_chat_id TEXT,
  telegram_enabled INTEGER NOT NULL DEFAULT 0,
  webhook_url TEXT,
  webhook_secret TEXT,
  webhook_enabled INTEGER NOT NULL DEFAULT 0,
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
	swap INTEGER NOT NULL DEFAULT 0,
	load INTEGER NOT NULL DEFAULT 0,
	gpu INTEGER NOT NULL DEFAULT 0,
	containers INTEGER NOT NULL DEFAULT 0,
	processes INTEGER NOT NULL DEFAULT 0,
	uptime INTEGER NOT NULL DEFAULT 0,
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
  app_name TEXT DEFAULT 'Service Status',
  created_at TEXT,
  updated_at TEXT
);

CREATE TABLE IF NOT EXISTS system_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp TEXT NOT NULL,
  level TEXT NOT NULL,
  category TEXT NOT NULL,
  service TEXT,
  message TEXT NOT NULL,
  details TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON system_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_logs_level ON system_logs(level);
CREATE INDEX IF NOT EXISTS idx_logs_category ON system_logs(category);
CREATE INDEX IF NOT EXISTS idx_logs_service ON system_logs(service);
`)
	if err != nil {
		return err
	}

	// For existing installs: add any newly introduced columns.
	// SQLite doesn't support IF NOT EXISTS on ADD COLUMN, so we ignore the error
	// if the column already exists.
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN storage INTEGER NOT NULL DEFAULT 1;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN glances_url TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN swap INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN load INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN gpu INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN containers INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN processes INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE resources_ui_config ADD COLUMN uptime INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN status_page_url TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN smtp_skip_verify INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE services ADD COLUMN icon_url TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE app_settings ADD COLUMN app_name TEXT DEFAULT 'Service Status';`)

	// Multi-channel notification columns
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN discord_webhook_url TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN discord_enabled INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN telegram_bot_token TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN telegram_chat_id TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN telegram_enabled INTEGER NOT NULL DEFAULT 0;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN webhook_url TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN webhook_secret TEXT;`)
	_, _ = DB.Exec(`ALTER TABLE alert_config ADD COLUMN webhook_enabled INTEGER NOT NULL DEFAULT 0;`)

	// Service dependencies
	_, _ = DB.Exec(`ALTER TABLE services ADD COLUMN depends_on TEXT DEFAULT '';`)

	// Connected/integrated services
	_, _ = DB.Exec(`ALTER TABLE services ADD COLUMN connected_to TEXT DEFAULT '';`)

	// Maintenance windows
	_, _ = DB.Exec(`CREATE TABLE IF NOT EXISTS maintenance_windows (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_key TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL,
		start_time TEXT NOT NULL,
		end_time TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);`)

	// Incident events (auto-generated timeline)
	_, _ = DB.Exec(`CREATE TABLE IF NOT EXISTS incident_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_key TEXT NOT NULL,
		service_name TEXT NOT NULL DEFAULT '',
		event_type TEXT NOT NULL DEFAULT 'down',
		started_at TEXT NOT NULL,
		resolved_at TEXT,
		duration_s INTEGER DEFAULT 0,
		details TEXT DEFAULT '',
		postmortem TEXT DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);`)
	_, _ = DB.Exec(`CREATE INDEX IF NOT EXISTS idx_incidents_service ON incident_events(service_key);`)
	_, _ = DB.Exec(`CREATE INDEX IF NOT EXISTS idx_incidents_started ON incident_events(started_at);`)

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
	err := DB.QueryRow(`SELECT enabled, smtp_host, smtp_port, smtp_user, smtp_password, alert_email, from_email,
		COALESCE(status_page_url, ''), COALESCE(smtp_skip_verify, 0), alert_on_down, alert_on_degraded, alert_on_up,
		COALESCE(discord_webhook_url, ''), COALESCE(discord_enabled, 0),
		COALESCE(telegram_bot_token, ''), COALESCE(telegram_chat_id, ''), COALESCE(telegram_enabled, 0),
		COALESCE(webhook_url, ''), COALESCE(webhook_secret, ''), COALESCE(webhook_enabled, 0)
		FROM alert_config WHERE id = 1`).Scan(
		&config.Enabled, &config.SMTPHost, &config.SMTPPort, &config.SMTPUser,
		&config.SMTPPassword, &config.AlertEmail, &config.FromEmail, &config.StatusPageURL, &config.SMTPSkipVerify,
		&config.AlertOnDown, &config.AlertOnDegraded, &config.AlertOnUp,
		&config.DiscordWebhookURL, &config.DiscordEnabled,
		&config.TelegramBotToken, &config.TelegramChatID, &config.TelegramEnabled,
		&config.WebhookURL, &config.WebhookSecret, &config.WebhookEnabled)

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
	_, err := DB.Exec(`INSERT INTO alert_config (id, enabled, smtp_host, smtp_port, smtp_user, smtp_password, alert_email, from_email, status_page_url, smtp_skip_verify,
		alert_on_down, alert_on_degraded, alert_on_up,
		discord_webhook_url, discord_enabled,
		telegram_bot_token, telegram_chat_id, telegram_enabled,
		webhook_url, webhook_secret, webhook_enabled, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET 
			enabled=?, smtp_host=?, smtp_port=?, smtp_user=?, smtp_password=?, alert_email=?, from_email=?, status_page_url=?, smtp_skip_verify=?,
			alert_on_down=?, alert_on_degraded=?, alert_on_up=?,
			discord_webhook_url=?, discord_enabled=?,
			telegram_bot_token=?, telegram_chat_id=?, telegram_enabled=?,
			webhook_url=?, webhook_secret=?, webhook_enabled=?, updated_at=datetime('now')`,
		config.Enabled, config.SMTPHost, config.SMTPPort, config.SMTPUser, config.SMTPPassword,
		config.AlertEmail, config.FromEmail, config.StatusPageURL, config.SMTPSkipVerify,
		config.AlertOnDown, config.AlertOnDegraded, config.AlertOnUp,
		config.DiscordWebhookURL, config.DiscordEnabled,
		config.TelegramBotToken, config.TelegramChatID, config.TelegramEnabled,
		config.WebhookURL, config.WebhookSecret, config.WebhookEnabled,
		config.Enabled, config.SMTPHost, config.SMTPPort, config.SMTPUser, config.SMTPPassword,
		config.AlertEmail, config.FromEmail, config.StatusPageURL, config.SMTPSkipVerify,
		config.AlertOnDown, config.AlertOnDegraded, config.AlertOnUp,
		config.DiscordWebhookURL, config.DiscordEnabled,
		config.TelegramBotToken, config.TelegramChatID, config.TelegramEnabled,
		config.WebhookURL, config.WebhookSecret, config.WebhookEnabled)
	return err
}

// LoadResourcesUIConfig loads resources UI configuration from database
func LoadResourcesUIConfig() (*models.ResourcesUIConfig, error) {
	var config models.ResourcesUIConfig
	var glancesURL sql.NullString
	err := DB.QueryRow(`SELECT enabled, COALESCE(glances_url, ''), cpu, memory, network, temp, storage,
		COALESCE(swap, 0), COALESCE(load, 0), COALESCE(gpu, 0), COALESCE(containers, 0), COALESCE(processes, 0), COALESCE(uptime, 0)
		FROM resources_ui_config WHERE id = 1`).Scan(
		&config.Enabled, &glancesURL, &config.CPU, &config.Memory, &config.Network, &config.Temp, &config.Storage,
		&config.Swap, &config.Load, &config.GPU, &config.Containers, &config.Processes, &config.Uptime,
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
	_, err := DB.Exec(`INSERT INTO resources_ui_config (id, enabled, glances_url, cpu, memory, network, temp, storage, swap, load, gpu, containers, processes, uptime, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			enabled=?, glances_url=?, cpu=?, memory=?, network=?, temp=?, storage=?, swap=?, load=?, gpu=?, containers=?, processes=?, uptime=?, updated_at=datetime('now')`,
		config.Enabled, config.GlancesURL, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
		config.Swap, config.Load, config.GPU, config.Containers, config.Processes, config.Uptime,
		config.Enabled, config.GlancesURL, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
		config.Swap, config.Load, config.GPU, config.Containers, config.Processes, config.Uptime,
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
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
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
			&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
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
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
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
			&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
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
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
		FROM services WHERE id = ?`, id).Scan(
		&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
		&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
		&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
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
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
		FROM services WHERE key = ?`, key).Scan(
		&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
		&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
		&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
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

	// Auto-assign display order only when not explicitly provided
	if s.DisplayOrder < 0 {
		var maxOrder int
		_ = DB.QueryRow(`SELECT COALESCE(MAX(display_order), -1) FROM services`).Scan(&maxOrder)
		s.DisplayOrder = maxOrder + 1
	}

	result, err := DB.Exec(`
		INSERT INTO services (key, name, url, service_type, icon, icon_url, api_token, display_order, visible,
		                      check_type, check_interval, timeout, expected_min, expected_max, depends_on, connected_to, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		s.Key, s.Name, s.URL, s.ServiceType, s.Icon, s.IconURL, s.APIToken, s.DisplayOrder, visible,
		s.CheckType, s.CheckInterval, s.Timeout, s.ExpectedMin, s.ExpectedMax, s.DependsOn, s.ConnectedTo)
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
		                    expected_max=?, depends_on=?, connected_to=?, updated_at=datetime('now')
		WHERE id = ?`,
		s.Name, s.URL, s.ServiceType, s.Icon, s.IconURL, s.APIToken, s.DisplayOrder, visible,
		s.CheckType, s.CheckInterval, s.Timeout, s.ExpectedMin, s.ExpectedMax, s.DependsOn, s.ConnectedTo, s.ID)
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

// ============================================
// Logging Functions
// ============================================

// LogLevel constants
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// LogCategory constants
const (
	LogCategoryCheck    = "check"
	LogCategoryEmail    = "email"
	LogCategorySecurity = "security"
	LogCategorySystem   = "system"
	LogCategorySchedule = "schedule"
)

// InsertLog adds a new log entry
func InsertLog(level, category, service, message, details string) error {
	_, err := DB.Exec(`INSERT INTO system_logs (timestamp, level, category, service, message, details)
		VALUES (datetime('now'), ?, ?, ?, ?, ?)`,
		level, category, service, message, details)
	return err
}

// GetLogs retrieves logs with optional filtering
func GetLogs(limit int, level, category, service string, offset int) ([]models.LogEntry, error) {
	query := `SELECT id, timestamp, level, category, COALESCE(service, ''), message, COALESCE(details, '')
		FROM system_logs WHERE 1=1`
	args := []interface{}{}

	if level != "" {
		query += " AND level = ?"
		args = append(args, level)
	}
	if category != "" {
		query += " AND category = ?"
		args = append(args, category)
	}
	if service != "" {
		query += " AND service = ?"
		args = append(args, service)
	}

	query += " ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.LogEntry
	for rows.Next() {
		var log models.LogEntry
		if err := rows.Scan(&log.ID, &log.Timestamp, &log.Level, &log.Category, &log.Service, &log.Message, &log.Details); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

// GetLogStats returns statistics about logs
func GetLogStats() (*models.LogStats, error) {
	var stats models.LogStats

	err := DB.QueryRow(`SELECT COUNT(*) FROM system_logs`).Scan(&stats.TotalLogs)
	if err != nil {
		return nil, err
	}

	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'error'`).Scan(&stats.ErrorCount)
	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'warn'`).Scan(&stats.WarnCount)
	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'info'`).Scan(&stats.InfoCount)
	_ = DB.QueryRow(`SELECT COUNT(*) FROM system_logs WHERE level = 'debug'`).Scan(&stats.DebugCount)

	return &stats, nil
}

// ClearLogs clears logs older than specified days, or all logs if days is 0
func ClearLogs(days int) error {
	if days == 0 {
		_, err := DB.Exec(`DELETE FROM system_logs`)
		return err
	}
	_, err := DB.Exec(`DELETE FROM system_logs WHERE timestamp < datetime('now', '-' || ? || ' days')`, days)
	return err
}

// PruneLogs removes old logs to keep the database size manageable (keeps last N logs)
func PruneLogs(keepCount int) error {
	_, err := DB.Exec(`DELETE FROM system_logs WHERE id NOT IN (
		SELECT id FROM system_logs ORDER BY timestamp DESC, id DESC LIMIT ?
	)`, keepCount)
	return err
}
