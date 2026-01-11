package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

// Config holds all application configuration
type Config struct {
	// Auth
	AuthUser       string
	AuthHash       []byte
	HmacSecret     []byte
	InsecureDev    bool
	SessionMaxAgeS int

	// Server
	Port            string
	DBPath          string
	EnableScheduler bool
	PollInterval    time.Duration
	StatusPageURL   string

	// Services (loaded from env)
	ServiceConfigs []ServiceConfig

	// Resources (Glances)
	GlancesBaseURL string
}

// ServiceConfig holds configuration for a single service
type ServiceConfig struct {
	Key     string
	Label   string
	URL     string
	Timeout time.Duration
	MinOK   int
	MaxOK   int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AuthUser:        getenv("AUTH_USER", "admin"),
		InsecureDev:     envBool("INSECURE_DEV", true),
		SessionMaxAgeS:  envInt("SESSION_MAX_AGE_SECONDS", 86400),
		Port:            getenv("PORT", "4555"),
		DBPath:          getenv("DB_PATH", "./uptime.db"),
		EnableScheduler: strings.ToLower(getenv("ENABLE_SCHEDULER", "true")) == "true",
		PollInterval:    envDurSecs("POLL_SECONDS", 60),
		StatusPageURL:   getenv("STATUS_PAGE_URL", ""),
		GlancesBaseURL:  strings.TrimSuffix(getenv("GLANCES_BASE_URL", "http://10.0.0.2:61208/api/4"), "/"),
	}

	// Load auth password/hash
	if hp := getenv("AUTH_PASSWORD_BCRYPT", ""); hp != "" {
		cfg.AuthHash = []byte(hp)
	} else {
		pw := getenv("AUTH_PASSWORD", "")
		if pw == "" {
			log.Fatal("missing AUTH_PASSWORD or AUTH_PASSWORD_BCRYPT")
		}
		h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		cfg.AuthHash = h
	}

	// Load HMAC secret
	secret := getenv("AUTH_SECRET", "")
	if len(secret) < 32 {
		log.Fatal("AUTH_SECRET must be at least 32 bytes (use a long random string)")
	}
	cfg.HmacSecret = []byte(secret)

	// Load service configurations
	cfg.ServiceConfigs = loadServiceConfigs()

	return cfg, nil
}

func loadServiceConfigs() []ServiceConfig {
	plexURL := getenv("PLEX_BASE_URL", "")
	plexToken := getenv("PLEX_TOKEN", "")
	plexIdentity := ""
	if plexURL != "" && plexToken != "" {
		plexURL = strings.TrimSuffix(plexURL, "/")
		plexIdentity = plexURL + "/identity?X-Plex-Token=" + plexToken
	}

	return []ServiceConfig{
		{
			Key:     "server",
			Label:   "Server",
			URL:     getenv("SERVER_HEALTH_URL", "tcp://10.0.0.2:22"),
			Timeout: envDurSecs("SERVER_TIMEOUT_SECS", 4),
			MinOK:   envInt("SERVER_OK_MIN", 200),
			MaxOK:   envInt("SERVER_OK_MAX", 399),
		},
		{
			Key:     "plex",
			Label:   "Plex",
			URL:     getenv("PLEX_IDENTITY_URL", plexIdentity),
			Timeout: envDurSecs("PLEX_TIMEOUT_SECS", 5),
			MinOK:   envInt("PLEX_OK_MIN", 200),
			MaxOK:   envInt("PLEX_OK_MAX", 399),
		},
		{
			Key:     "overseerr",
			Label:   "Overseerr",
			URL:     getenv("OVERSEERR_STATUS_URL", "http://10.0.0.2:5055/api/v1/status"),
			Timeout: envDurSecs("OVERSEERR_TIMEOUT_SECS", 4),
			MinOK:   envInt("OVERSEERR_OK_MIN", 200),
			MaxOK:   envInt("OVERSEERR_OK_MAX", 399),
		},
	}
}

// Helper functions
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	v := strings.ToLower(getenv(k, ""))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}

func envDurSecs(k string, def int) time.Duration {
	return time.Duration(envInt(k, def)) * time.Second
}
