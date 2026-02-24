package database

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
