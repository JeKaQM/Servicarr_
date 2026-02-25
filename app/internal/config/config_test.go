package config

import (
	"os"
	"testing"
	"time"
)

// --- helpers ---

func setEnvs(t *testing.T, m map[string]string) {
	t.Helper()
	for k, v := range m {
		t.Setenv(k, v)
	}
}

// --- getenv ---

func TestGetenv_Set(t *testing.T) {
	t.Setenv("TEST_KEY_GETENV", "hello")
	if got := getenv("TEST_KEY_GETENV", "fallback"); got != "hello" {
		t.Errorf("getenv returned %q, want %q", got, "hello")
	}
}

func TestGetenv_Unset(t *testing.T) {
	os.Unsetenv("TEST_KEY_GETENV_MISSING")
	if got := getenv("TEST_KEY_GETENV_MISSING", "fallback"); got != "fallback" {
		t.Errorf("getenv returned %q, want %q", got, "fallback")
	}
}

func TestGetenv_EmptyStringUsesDefault(t *testing.T) {
	t.Setenv("TEST_KEY_EMPTY", "")
	if got := getenv("TEST_KEY_EMPTY", "default"); got != "default" {
		t.Errorf("getenv returned %q, want %q for empty env var", got, "default")
	}
}

// --- envInt ---

func TestEnvInt_ValidNumber(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := envInt("TEST_INT", 0); got != 42 {
		t.Errorf("envInt returned %d, want 42", got)
	}
}

func TestEnvInt_InvalidNumber(t *testing.T) {
	t.Setenv("TEST_INT_BAD", "not_a_number")
	if got := envInt("TEST_INT_BAD", 99); got != 99 {
		t.Errorf("envInt returned %d, want default 99 for invalid input", got)
	}
}

func TestEnvInt_Unset(t *testing.T) {
	os.Unsetenv("TEST_INT_MISSING")
	if got := envInt("TEST_INT_MISSING", 7); got != 7 {
		t.Errorf("envInt returned %d, want default 7", got)
	}
}

func TestEnvInt_NegativeNumber(t *testing.T) {
	t.Setenv("TEST_INT_NEG", "-5")
	if got := envInt("TEST_INT_NEG", 0); got != -5 {
		t.Errorf("envInt returned %d, want -5", got)
	}
}

func TestEnvInt_Zero(t *testing.T) {
	t.Setenv("TEST_INT_ZERO", "0")
	if got := envInt("TEST_INT_ZERO", 99); got != 0 {
		t.Errorf("envInt returned %d, want 0", got)
	}
}

func TestEnvInt_FloatString(t *testing.T) {
	t.Setenv("TEST_INT_FLOAT", "3.14")
	if got := envInt("TEST_INT_FLOAT", 10); got != 10 {
		t.Errorf("envInt returned %d, want default 10 for float string", got)
	}
}

// --- envBool ---

func TestEnvBool_True(t *testing.T) {
	for _, val := range []string{"1", "true", "yes", "TRUE", "True", "YES", "Yes"} {
		t.Setenv("TEST_BOOL", val)
		if got := envBool("TEST_BOOL", false); !got {
			t.Errorf("envBool(%q) = false, want true", val)
		}
	}
}

func TestEnvBool_False(t *testing.T) {
	for _, val := range []string{"0", "false", "no", "FALSE", "random"} {
		t.Setenv("TEST_BOOL", val)
		if got := envBool("TEST_BOOL", true); got {
			t.Errorf("envBool(%q) = true, want false", val)
		}
	}
}

func TestEnvBool_Unset(t *testing.T) {
	os.Unsetenv("TEST_BOOL_MISSING")
	if got := envBool("TEST_BOOL_MISSING", true); !got {
		t.Error("envBool should return default true when unset")
	}
	if got := envBool("TEST_BOOL_MISSING", false); got {
		t.Error("envBool should return default false when unset")
	}
}

func TestEnvBool_EmptyString(t *testing.T) {
	t.Setenv("TEST_BOOL_EMPTY", "")
	if got := envBool("TEST_BOOL_EMPTY", true); !got {
		t.Error("envBool should return default true for empty string")
	}
}

// --- envDurSecs ---

func TestEnvDurSecs_Set(t *testing.T) {
	t.Setenv("TEST_DUR", "30")
	got := envDurSecs("TEST_DUR", 60)
	want := 30 * time.Second
	if got != want {
		t.Errorf("envDurSecs = %v, want %v", got, want)
	}
}

func TestEnvDurSecs_Default(t *testing.T) {
	os.Unsetenv("TEST_DUR_MISSING")
	got := envDurSecs("TEST_DUR_MISSING", 120)
	want := 120 * time.Second
	if got != want {
		t.Errorf("envDurSecs = %v, want %v", got, want)
	}
}

func TestEnvDurSecs_Zero(t *testing.T) {
	t.Setenv("TEST_DUR_ZERO", "0")
	got := envDurSecs("TEST_DUR_ZERO", 60)
	if got != 0 {
		t.Errorf("envDurSecs = %v, want 0", got)
	}
}

// --- LoadBasic ---

func TestLoadBasic_Defaults(t *testing.T) {
	// Clear env to test defaults
	for _, k := range []string{
		"AUTH_USER", "AUTH_PASSWORD", "AUTH_PASSWORD_BCRYPT", "AUTH_SECRET",
		"PORT", "DB_PATH", "ENABLE_SCHEDULER", "POLL_SECONDS",
		"STATUS_PAGE_URL", "GLANCES_BASE_URL", "SESSION_MAX_AGE_SECONDS",
		"INSECURE_DEV", "UNBLOCK_TOKEN",
	} {
		os.Unsetenv(k)
	}

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}

	if cfg.AuthUser != "" {
		t.Errorf("AuthUser = %q, want empty", cfg.AuthUser)
	}
	if cfg.Port != "4555" {
		t.Errorf("Port = %q, want 4555", cfg.Port)
	}
	if cfg.DBPath != "./uptime.db" {
		t.Errorf("DBPath = %q, want ./uptime.db", cfg.DBPath)
	}
	if !cfg.EnableScheduler {
		t.Error("EnableScheduler should default to true")
	}
	if cfg.PollInterval != 60*time.Second {
		t.Errorf("PollInterval = %v, want 60s", cfg.PollInterval)
	}
	if cfg.SessionMaxAgeS != 86400 {
		t.Errorf("SessionMaxAgeS = %d, want 86400", cfg.SessionMaxAgeS)
	}
	if cfg.InsecureDev {
		t.Error("InsecureDev should default to false")
	}
}

func TestLoadBasic_WithPassword(t *testing.T) {
	setEnvs(t, map[string]string{
		"AUTH_USER":     "testuser",
		"AUTH_PASSWORD": "testpass123",
	})
	// Clear conflicting vars
	os.Unsetenv("AUTH_PASSWORD_BCRYPT")
	os.Unsetenv("AUTH_SECRET")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if cfg.AuthUser != "testuser" {
		t.Errorf("AuthUser = %q, want testuser", cfg.AuthUser)
	}
	if len(cfg.AuthHash) == 0 {
		t.Error("AuthHash should be set when AUTH_PASSWORD is provided")
	}
}

func TestLoadBasic_WithBcryptHash(t *testing.T) {
	hash := "$2a$10$xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	setEnvs(t, map[string]string{
		"AUTH_PASSWORD_BCRYPT": hash,
	})
	os.Unsetenv("AUTH_PASSWORD")
	os.Unsetenv("AUTH_SECRET")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if string(cfg.AuthHash) != hash {
		t.Errorf("AuthHash = %q, want %q", string(cfg.AuthHash), hash)
	}
}

func TestLoadBasic_HMACSecretTooShort(t *testing.T) {
	setEnvs(t, map[string]string{
		"AUTH_SECRET": "tooshort",
	})
	os.Unsetenv("AUTH_PASSWORD")
	os.Unsetenv("AUTH_PASSWORD_BCRYPT")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if len(cfg.HmacSecret) != 0 {
		t.Error("HmacSecret should be nil when AUTH_SECRET < 32 bytes")
	}
}

func TestLoadBasic_HMACSecretValid(t *testing.T) {
	secret := "this-is-a-very-long-secret-that-exceeds-thirty-two-bytes"
	setEnvs(t, map[string]string{
		"AUTH_SECRET": secret,
	})
	os.Unsetenv("AUTH_PASSWORD")
	os.Unsetenv("AUTH_PASSWORD_BCRYPT")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if string(cfg.HmacSecret) != secret {
		t.Errorf("HmacSecret = %q, want %q", string(cfg.HmacSecret), secret)
	}
}

func TestLoadBasic_CustomPort(t *testing.T) {
	t.Setenv("PORT", "8080")
	os.Unsetenv("AUTH_PASSWORD")
	os.Unsetenv("AUTH_PASSWORD_BCRYPT")
	os.Unsetenv("AUTH_SECRET")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
}

func TestLoadBasic_DisableScheduler(t *testing.T) {
	t.Setenv("ENABLE_SCHEDULER", "false")
	os.Unsetenv("AUTH_PASSWORD")
	os.Unsetenv("AUTH_PASSWORD_BCRYPT")
	os.Unsetenv("AUTH_SECRET")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if cfg.EnableScheduler {
		t.Error("EnableScheduler should be false when set to 'false'")
	}
}

func TestLoadBasic_GlancesURLTrailingSlash(t *testing.T) {
	t.Setenv("GLANCES_BASE_URL", "http://10.0.0.2:61208/api/4/")
	os.Unsetenv("AUTH_PASSWORD")
	os.Unsetenv("AUTH_PASSWORD_BCRYPT")
	os.Unsetenv("AUTH_SECRET")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if cfg.GlancesBaseURL != "http://10.0.0.2:61208/api/4" {
		t.Errorf("GlancesBaseURL = %q, trailing slash should be stripped", cfg.GlancesBaseURL)
	}
}

func TestLoadBasic_ServiceConfigsLoaded(t *testing.T) {
	os.Unsetenv("AUTH_PASSWORD")
	os.Unsetenv("AUTH_PASSWORD_BCRYPT")
	os.Unsetenv("AUTH_SECRET")

	cfg, err := LoadBasic()
	if err != nil {
		t.Fatalf("LoadBasic failed: %v", err)
	}
	if len(cfg.ServiceConfigs) != 3 {
		t.Errorf("ServiceConfigs length = %d, want 3", len(cfg.ServiceConfigs))
	}

	keys := map[string]bool{}
	for _, sc := range cfg.ServiceConfigs {
		keys[sc.Key] = true
		if sc.Label == "" {
			t.Errorf("ServiceConfig %q has empty label", sc.Key)
		}
	}
	for _, k := range []string{"server", "plex", "overseerr"} {
		if !keys[k] {
			t.Errorf("expected service config key %q not found", k)
		}
	}
}

// --- loadServiceConfigs ---

func TestLoadServiceConfigs_PlexIdentityURL(t *testing.T) {
	setEnvs(t, map[string]string{
		"PLEX_BASE_URL": "http://10.0.0.2:32400",
		"PLEX_TOKEN":    "abc123",
	})
	os.Unsetenv("PLEX_IDENTITY_URL")

	cfgs := loadServiceConfigs()
	var plexCfg *ServiceConfig
	for i := range cfgs {
		if cfgs[i].Key == "plex" {
			plexCfg = &cfgs[i]
			break
		}
	}
	if plexCfg == nil {
		t.Fatal("plex config not found")
	}
	want := "http://10.0.0.2:32400/identity?X-Plex-Token=abc123"
	if plexCfg.URL != want {
		t.Errorf("Plex URL = %q, want %q", plexCfg.URL, want)
	}
}

func TestLoadServiceConfigs_PlexTrailingSlash(t *testing.T) {
	setEnvs(t, map[string]string{
		"PLEX_BASE_URL": "http://10.0.0.2:32400/",
		"PLEX_TOKEN":    "abc123",
	})
	os.Unsetenv("PLEX_IDENTITY_URL")

	cfgs := loadServiceConfigs()
	for _, c := range cfgs {
		if c.Key == "plex" {
			want := "http://10.0.0.2:32400/identity?X-Plex-Token=abc123"
			if c.URL != want {
				t.Errorf("Plex URL = %q, want %q (trailing slash should be trimmed)", c.URL, want)
			}
			return
		}
	}
	t.Fatal("plex config not found")
}

func TestLoadServiceConfigs_PlexNoToken(t *testing.T) {
	t.Setenv("PLEX_BASE_URL", "http://10.0.0.2:32400")
	os.Unsetenv("PLEX_TOKEN")
	os.Unsetenv("PLEX_IDENTITY_URL")

	cfgs := loadServiceConfigs()
	for _, c := range cfgs {
		if c.Key == "plex" {
			if c.URL != "" {
				t.Errorf("Plex URL should be empty without token, got %q", c.URL)
			}
			return
		}
	}
	t.Fatal("plex config not found")
}

func TestLoadServiceConfigs_CustomTimeouts(t *testing.T) {
	t.Setenv("SERVER_TIMEOUT_SECS", "10")
	cfgs := loadServiceConfigs()
	for _, c := range cfgs {
		if c.Key == "server" {
			want := 10 * time.Second
			if c.Timeout != want {
				t.Errorf("Server timeout = %v, want %v", c.Timeout, want)
			}
			return
		}
	}
	t.Fatal("server config not found")
}
