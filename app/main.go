package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"status/app/internal/alerts"
	"status/app/internal/auth"
	"status/app/internal/checker"
	"status/app/internal/config"
	"status/app/internal/database"
	"status/app/internal/handlers"
	"status/app/internal/models"
	"status/app/internal/monitor"
	"status/app/internal/resources"
	"status/app/internal/security"
	"status/app/internal/stats"
)

func main() {
	// Load configuration from environment (for basic settings)
	cfg, err := config.LoadBasic()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := database.Init(cfg.DBPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize statistics schema
	if err := stats.EnsureStatsSchema(); err != nil {
		log.Printf("Warning: Failed to initialize stats schema: %v", err)
	}

	// Start stats aggregator for efficient historical data
	stats.StartStatsAggregator()

	// Check if setup is complete and load auth accordingly
	authMgr := createAuthManager(cfg)

	// Create alert manager (loads config from database)
	alertMgr := alerts.NewManager(cfg.StatusPageURL)

	// Migrate services from environment config if needed
	migrateServicesFromEnv(cfg)

	// Ensure the demo service stays up even without outbound internet
	ensureDemoService()

	// Track consecutive failures across checks
	failureTracker := monitor.NewFailureTracker()

	// Start health check scheduler
	if cfg.EnableScheduler {
		go runScheduler(alertMgr, cfg.PollInterval, failureTracker)
		log.Printf("Scheduler started with %v interval", cfg.PollInterval)
	}

	// Setup HTTP routes
	gl := resources.NewClient(cfg.GlancesBaseURL)
	mux := handlers.SetupRoutes(authMgr, alertMgr, failureTracker, gl)

	// Wrap with security middleware
	handler := security.SecureHeaders(mux)

	// Create HTTP server
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server forced shutdown: %v", err)
		}
	}()

	log.Printf("Server starting on port %s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	log.Println("Server stopped gracefully")
}

// createAuthManager creates the auth manager from database settings or falls back to env config
func createAuthManager(cfg *config.Config) *auth.Auth {
	// Check if setup is complete
	complete, err := database.IsSetupComplete()
	if err != nil {
		log.Printf("Warning: Failed to check setup status: %v", err)
	}

	if complete {
		// Load auth from database
		settings, err := database.LoadAppSettings()
		if err != nil {
			log.Printf("Warning: Failed to load app settings: %v", err)
		} else {
			log.Println("Loading auth credentials from database")
			return auth.NewAuth(
				settings.Username,
				[]byte(settings.PasswordHash),
				[]byte(settings.AuthSecret),
				cfg.InsecureDev,
				cfg.SessionMaxAgeS,
			)
		}

		// Setup is complete but couldn't load from DB - fall back to env
		if cfg.AuthUser != "" && len(cfg.AuthHash) > 0 && len(cfg.HmacSecret) > 0 {
			log.Println("Falling back to auth credentials from environment")
			return auth.NewAuth(
				cfg.AuthUser,
				cfg.AuthHash,
				cfg.HmacSecret,
				cfg.InsecureDev,
				cfg.SessionMaxAgeS,
			)
		}
	}

	// Setup not complete - create a placeholder auth manager that won't validate
	// The setup middleware will redirect users to /setup anyway
	log.Println("Setup not complete - auth disabled until setup is finished")
	tempSecret := make([]byte, 32)
	if _, err := rand.Read(tempSecret); err != nil {
		log.Printf("Warning: Failed to generate temporary secret: %v", err)
		tempSecret = []byte("fallback-" + time.Now().String())
	}
	return auth.NewAuth(
		"",
		[]byte{},
		tempSecret,
		cfg.InsecureDev,
		cfg.SessionMaxAgeS,
	)
}

// migrateServicesFromEnv migrates services from env config if needed
func migrateServicesFromEnv(cfg *config.Config) {
	// Check if setup is complete
	setupComplete, _ := database.IsSetupComplete()

	// Check if we have services in the database
	dbServices, err := database.GetAllServices()
	if err != nil {
		log.Printf("Warning: Failed to load services from database: %v", err)
		return
	}

	// Only migrate from env config if setup IS complete (backward compatibility for existing installs)
	// New installs should go through the setup wizard instead
	if len(dbServices) == 0 && len(cfg.ServiceConfigs) > 0 && setupComplete {
		log.Println("No services in database, migrating from environment config...")
		for _, sc := range cfg.ServiceConfigs {
			// Skip services with empty URLs
			if sc.URL == "" {
				continue
			}

			svcConfig := &models.ServiceConfig{
				Key:           sc.Key,
				Name:          sc.Label,
				URL:           sc.URL,
				ServiceType:   sc.Key, // Use key as type for known services
				Icon:          sc.Key,
				CheckType:     "http",
				CheckInterval: 60,
				Timeout:       int(sc.Timeout.Seconds()),
				ExpectedMin:   sc.MinOK,
				ExpectedMax:   sc.MaxOK,
				Visible:       true,
				DisplayOrder:  -1, // auto-append
			}
			if _, err := database.CreateService(svcConfig); err != nil {
				log.Printf("Warning: Failed to migrate service %s: %v", sc.Key, err)
			}
		}
	}
}

// ensureDemoService updates the default demo service to an always-up check.
func ensureDemoService() {
	sc, err := database.GetServiceByKey("demo-service")
	if err != nil || sc == nil {
		return
	}

	// Only auto-update the built-in demo service.
	if sc.Name != "Demo Service" {
		return
	}

	if sc.CheckType == "always_up" {
		return
	}

	if sc.URL == "" || sc.URL == "https://httpstat.us/200" {
		sc.URL = "http://localhost"
		sc.CheckType = "always_up"
		if sc.ExpectedMin == 0 {
			sc.ExpectedMin = 200
		}
		if sc.ExpectedMax == 0 {
			sc.ExpectedMax = 299
		}
		_ = database.UpdateService(sc)
	}
}

// runScheduler runs health checks using per-service intervals.
// Each service runs on its own timer based on its configured check_interval.
// A global coordination ticker reloads services and prunes stale data.
func runScheduler(alertMgr *alerts.Manager, defaultInterval time.Duration, tracker *monitor.FailureTracker) {
	type serviceTimer struct {
		key      string
		interval time.Duration
		lastRun  time.Time
	}

	// Coordination ticker runs every 5 seconds to check if any service is due
	coordTicker := time.NewTicker(5 * time.Second)
	defer coordTicker.Stop()

	timers := make(map[string]*serviceTimer)
	var lastPrune time.Time

	for range coordTicker.C {
		// Reload services from DB to pick up changes (new services, interval changes)
		dbServices, err := database.GetAllServices()
		if err != nil {
			log.Printf("Warning: Failed to reload services: %v", err)
			continue
		}

		// Build set of valid keys and update timers
		validKeys := make(map[string]struct{}, len(dbServices))
		for _, sc := range dbServices {
			validKeys[sc.Key] = struct{}{}

			interval := time.Duration(sc.CheckInterval) * time.Second
			if interval < 10*time.Second {
				interval = defaultInterval
			}

			if t, ok := timers[sc.Key]; ok {
				// Update interval if it changed
				t.interval = interval
			} else {
				// New service — run immediately on first tick
				timers[sc.Key] = &serviceTimer{
					key:      sc.Key,
					interval: interval,
					lastRun:  time.Time{}, // zero = run immediately
				}
			}
		}

		// Remove timers for deleted services
		for k := range timers {
			if _, exists := validKeys[k]; !exists {
				delete(timers, k)
			}
		}
		tracker.Prune(validKeys)

		now := time.Now()

		for _, sc := range dbServices {
			t := timers[sc.Key]
			if t == nil {
				continue
			}

			// Check if this service is due for a check
			if !t.lastRun.IsZero() && now.Sub(t.lastRun) < t.interval {
				continue
			}
			t.lastRun = now

			// Check disabled state
			disabled, _ := database.GetServiceDisabledState(sc.Key)
			if disabled {
				continue
			}

			timeout := time.Duration(sc.Timeout) * time.Second
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			// Perform health check
			checkOK, code, msPtr, errMsg := checker.Check(checker.CheckOptions{
				URL:         sc.URL,
				Timeout:     timeout,
				ExpectedMin: sc.ExpectedMin,
				ExpectedMax: sc.ExpectedMax,
				CheckType:   sc.CheckType,
				ServiceType: sc.ServiceType,
				APIToken:    sc.APIToken,
			})

			// Track consecutive failures
			consecutiveFailures := tracker.Update(sc.Key, checkOK)

			// OK if check passed OR haven't hit 2 consecutive failures yet
			ok := checkOK || consecutiveFailures < 2

			// Degraded = responding but slow
			degraded := ok && msPtr != nil && *msPtr > 200

			// Record stats
			stats.RecordHeartbeat(sc.Key, ok, msPtr, code, errMsg)
			database.InsertSample(now, sc.Key, ok, code, msPtr)

			// Log the check result
			logLevel := database.LogLevelInfo
			logMsg := "Service check passed"
			logDetails := ""

			if msPtr != nil {
				logDetails = fmt.Sprintf("status=%d, latency=%dms, interval=%ds", code, *msPtr, sc.CheckInterval)
			} else {
				logDetails = fmt.Sprintf("status=%d, interval=%ds", code, sc.CheckInterval)
			}

			if !ok {
				logLevel = database.LogLevelError
				logMsg = "Service check failed"
				if errMsg != "" {
					logDetails += ", error=" + errMsg
				}
			} else if degraded {
				logLevel = database.LogLevelWarn
				logMsg = "Service degraded (slow response)"
			}

			_ = database.InsertLog(logLevel, database.LogCategoryCheck, sc.Key, logMsg, logDetails)

			if errMsg != "" {
				log.Printf("Check %s: %s (failures: %d/2)", sc.Key, errMsg, consecutiveFailures)
			}

			// Send alerts (dependency-aware, multi-channel)
			name := sc.Name
			if name == "" {
				name = sc.Key
			}
			alertMgr.CheckAndSendAlerts(sc.Key, name, ok, degraded)
		}

		// Prune old logs every 5 minutes
		if now.Sub(lastPrune) > 5*time.Minute {
			_ = database.PruneLogs(10000)
			lastPrune = now
		}
	}
}
