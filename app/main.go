package main

import (
	"fmt"
	"log"
	"net/http"
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
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Server starting on port %s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
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
	return auth.NewAuth(
		"",
		[]byte{},
		[]byte("temporary-secret-for-setup-only!"),
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

// runScheduler runs health checks on a regular interval
func runScheduler(alertMgr *alerts.Manager, interval time.Duration, tracker *monitor.FailureTracker) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Batch check results for efficient DB operations
	type checkResult struct {
		Key      string
		Name     string
		OK       bool
		Code     int
		MS       *int
		ErrMsg   string
		Degraded bool
	}

	for range ticker.C {
		// Reload services from DB on each tick to pick up changes
		dbServices, err := database.GetAllServices()
		if err != nil {
			log.Printf("Warning: Failed to reload services: %v", err)
			continue
		}

		// Prune failure counts for deleted services
		validKeys := make(map[string]struct{}, len(dbServices))
		for _, sc := range dbServices {
			validKeys[sc.Key] = struct{}{}
		}
		tracker.Prune(validKeys)

		// Collect results for batch processing
		results := make([]checkResult, 0, len(dbServices))

		for _, sc := range dbServices {
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

			// Service is considered OK if check passed OR if we haven't hit 2 consecutive failures yet
			ok := checkOK || consecutiveFailures < 2

			// Check if service is degraded (slow response)
			degraded := ok && msPtr != nil && *msPtr > 200

			results = append(results, checkResult{
				Key:      sc.Key,
				Name:     sc.Name,
				OK:       ok,
				Code:     code,
				MS:       msPtr,
				ErrMsg:   errMsg,
				Degraded: degraded,
			})

			// Log if there was an error
			if errMsg != "" {
				log.Printf("Check %s: %s (failures: %d/2)", sc.Key, errMsg, consecutiveFailures)
			}
		}

		// Batch process results
		for _, r := range results {
			// Record in new efficient stats system
			stats.RecordHeartbeat(r.Key, r.OK, r.MS, r.Code, r.ErrMsg)

			// Also record in legacy samples table for backward compatibility
			database.InsertSample(time.Now(), r.Key, r.OK, r.Code, r.MS)

			// Log the check result
			logLevel := database.LogLevelInfo
			logMsg := "Service check passed"
			logDetails := ""

			if r.MS != nil {
				logDetails = fmt.Sprintf("status=%d, latency=%dms", r.Code, *r.MS)
			} else {
				logDetails = fmt.Sprintf("status=%d", r.Code)
			}

			if !r.OK {
				logLevel = database.LogLevelError
				logMsg = "Service check failed"
				if r.ErrMsg != "" {
					logDetails += ", error=" + r.ErrMsg
				}
			} else if r.Degraded {
				logLevel = database.LogLevelWarn
				logMsg = "Service degraded (slow response)"
			}

			_ = database.InsertLog(logLevel, database.LogCategoryCheck, r.Key, logMsg, logDetails)

			// Send alerts if status changed
			name := r.Name
			if name == "" {
				name = r.Key
			}
			alertMgr.CheckAndSendAlerts(r.Key, name, r.OK, r.Degraded)
		}

		// Prune old logs to prevent database bloat (keep last 10000 entries)
		_ = database.PruneLogs(10000)
	}
}
