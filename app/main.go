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
	"status/app/internal/buildinfo"
	"status/app/internal/checker"
	"status/app/internal/config"
	"status/app/internal/crypto"
	"status/app/internal/database"
	"status/app/internal/handlers"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"status/app/internal/monitor"
	"status/app/internal/resources"
	"status/app/internal/security"
	"status/app/internal/stats"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("Servicarr %s\n", buildinfo.Current().Summary())
		return
	}

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	// Load configuration from environment (for basic settings)
	cfg, err := config.LoadBasic()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	security.ConfigureTrustedProxies(os.Getenv("TRUSTED_PROXIES"))

	// Initialize database
	if err := database.Init(cfg.DBPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	build := buildinfo.Current()
	if err := recordSoftwareStartup(build); err != nil {
		log.Printf("Warning: Failed to record software startup metadata: %v", err)
	}
	log.Printf("Servicarr %s starting (database schema %d, Go %s)", build.Summary(), database.SchemaVersion, build.GoVersion)

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
	go runUPSMonitor(appCtx, alertMgr, 10*time.Second)
	go runCrowdSecMonitor(appCtx, 30*time.Second)

	// Migrate services from environment config if needed
	migrateServicesFromEnv(cfg)

	// Ensure the demo service stays up even without outbound internet
	ensureDemoService()

	// Track consecutive failures across checks
	failureTracker := monitor.NewFailureTracker()

	// Start health check scheduler
	if cfg.EnableScheduler {
		go runScheduler(appCtx, alertMgr, cfg.PollInterval, failureTracker)
		log.Printf("Scheduler started with %v interval", cfg.PollInterval)
	}

	// Setup HTTP routes
	gl := resources.NewClient(cfg.GlancesBaseURL)
	handlers.InitBundles() // Build CSS/JS bundles from disk at startup
	mux := handlers.SetupRoutes(authMgr, alertMgr, failureTracker, gl)

	// Wrap with security middleware
	handler := security.SecureHeaders(mux)

	// Create HTTP server
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-quit
		log.Println("Shutting down server...")
		cancelApp()
		// Finish inside Docker's default 10s stop grace period.
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server forced shutdown: %v", err)
		}
		if !alerts.WaitForDeliveries(time.Until(deadlineOf(ctx))) {
			log.Println("Warning: shutting down with notifications still in flight")
		}
	}()

	log.Printf("Server starting on port %s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
	// ListenAndServe returns as soon as Shutdown begins; wait for in-flight
	// requests to drain before closing the database underneath them.
	<-shutdownDone
	if err := database.DB.Close(); err != nil {
		log.Printf("Warning: failed to close database: %v", err)
	}
	log.Println("Server stopped gracefully")
}

// deadlineOf returns ctx's deadline, or now when it has none.
func deadlineOf(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Now()
}

func recordSoftwareStartup(build buildinfo.Info) error {
	if err := database.RecordSoftwareDeployment(build); err != nil {
		return err
	}
	buildTime := build.BuildTime
	if buildTime == "" {
		buildTime = "unknown"
	}
	details := fmt.Sprintf("version=%s, commit=%s, build_time=%s, database_schema=%d, go=%s",
		build.Version, build.Commit, buildTime, database.SchemaVersion, build.GoVersion)
	return database.InsertLog(database.LogLevelInfo, database.LogCategorySystem, "", "Application started", details)
}

func runUPSMonitor(ctx context.Context, alertMgr *alerts.Manager, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastError string
	var lastErrorLog time.Time
	poll := func() {
		err := monitor.PollUPSPower(ctx, alertMgr)
		if err == nil {
			lastError = ""
			return
		}
		errText := err.Error()
		if errText != lastError || time.Since(lastErrorLog) >= 15*time.Minute {
			log.Printf("UPS monitor unavailable: %v", err)
			lastError = errText
			lastErrorLog = time.Now()
		}
	}

	poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

// runCrowdSecMonitor syncs CrowdSec decisions on the configured interval.
// The config is re-read every cycle so admin edits (URL, credentials,
// interval, enable) apply live without a restart. Errors are deduped in
// memory and persisted to crowdsec_state for the dashboard badge.
func runCrowdSecMonitor(ctx context.Context, defaultInterval time.Duration) {
	interval := defaultInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	refreshInterval := func() {
		newInterval := defaultInterval
		if cfg, err := database.LoadCrowdSecConfig(); err == nil && cfg != nil && cfg.Enabled {
			secs := cfg.PollIntervalS
			if secs < 10 {
				secs = 10
			}
			if secs > 3600 {
				secs = 3600
			}
			newInterval = time.Duration(secs) * time.Second
		}
		if newInterval != interval {
			interval = newInterval
			ticker.Reset(interval)
		}
	}

	var lastError string
	var lastErrorLog time.Time
	poll := func() {
		err := monitor.PollCrowdSec(ctx)
		if err == nil {
			lastError = ""
			return
		}
		errText := err.Error()
		if errText != lastError || time.Since(lastErrorLog) >= 15*time.Minute {
			log.Printf("CrowdSec monitor unavailable: %v", err)
			lastError = errText
			lastErrorLog = time.Now()
		}
	}

	poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-monitor.CrowdSecConfigChanges():
			// Apply new credentials/enable state immediately. In particular, a
			// change from a long interval must not wait for the old ticker to fire.
			refreshInterval()
			poll()
		case <-ticker.C:
			refreshInterval()
			poll()
		}
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
			// Initialize encryption key for API token encryption at rest
			crypto.SetKey([]byte(settings.AuthSecret))
			migrateTokenEncryption() // encrypt any legacy plaintext tokens
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
			crypto.SetKey(cfg.HmacSecret)
			migrateTokenEncryption()
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

// schedulerCheckConcurrency bounds simultaneous outbound checks per tick.
const schedulerCheckConcurrency = 8

// runScheduler runs health checks using per-service intervals.
// Each service runs on its own timer based on its configured check_interval.
// A global coordination ticker reloads services and prunes stale data.
func runScheduler(ctx context.Context, alertMgr *alerts.Manager, defaultInterval time.Duration, tracker *monitor.FailureTracker) {
	// Coordination ticker runs every 5 seconds to check if any service is due
	coordTicker := time.NewTicker(5 * time.Second)
	defer coordTicker.Stop()

	s := &scheduler{
		alertMgr:        alertMgr,
		defaultInterval: defaultInterval,
		tracker:         tracker,
		timers:          make(map[string]*serviceTimer),
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-coordTicker.C:
			s.safeTick()
		}
	}
}

type serviceTimer struct {
	interval time.Duration
	lastRun  time.Time
}

type scheduler struct {
	alertMgr        *alerts.Manager
	defaultInterval time.Duration
	tracker         *monitor.FailureTracker
	timers          map[string]*serviceTimer
	lastPrune       time.Time
}

// safeTick keeps the scheduler alive if a single tick panics; an unrecovered
// panic in this goroutine would otherwise stop all monitoring.
func (s *scheduler) safeTick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Scheduler tick panicked: %v", r)
		}
	}()
	s.tick()
}

func (s *scheduler) tick() {
	// Reload services from DB to pick up changes (new services, interval changes)
	dbServices, err := database.GetAllServices()
	if err != nil {
		log.Printf("Warning: Failed to reload services: %v", err)
		return
	}

	// Build set of valid keys and update timers
	validKeys := make(map[string]struct{}, len(dbServices))
	for _, sc := range dbServices {
		validKeys[sc.Key] = struct{}{}

		interval := time.Duration(sc.CheckInterval) * time.Second
		if interval < 10*time.Second {
			interval = s.defaultInterval
		}

		if t, ok := s.timers[sc.Key]; ok {
			// Update interval if it changed
			t.interval = interval
		} else {
			// New service: a zero lastRun runs it on this tick
			s.timers[sc.Key] = &serviceTimer{interval: interval}
		}
	}

	// Remove timers for deleted services
	for k := range s.timers {
		if _, exists := validKeys[k]; !exists {
			delete(s.timers, k)
		}
	}
	s.tracker.Prune(validKeys)

	now := time.Now()
	maintenanceActive, _, maintenanceErr := maintenance.MonitoringSuppressed(now)
	if maintenanceErr != nil {
		log.Printf("Warning: Failed to evaluate maintenance schedules: %v", maintenanceErr)
	}
	if maintenanceActive {
		s.tracker.ResetAll()
		return
	}

	var due []models.ServiceConfig
	for _, sc := range dbServices {
		t := s.timers[sc.Key]
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
		due = append(due, sc)
	}

	// Network checks run concurrently so one slow target cannot delay the
	// others by its whole timeout. Results are then processed in display order,
	// keeping failure tracking, dependency-aware alert suppression and
	// recording exactly as sequential as before.
	opts := make([]checker.CheckOptions, len(due))
	for i, sc := range due {
		opts[i] = checker.OptionsFor(sc)
	}
	results := checker.CheckAll(opts, schedulerCheckConcurrency)

	// A batch can straddle the start of a maintenance window. Discard it.
	if maintenanceStarted, _, _ := maintenance.MonitoringSuppressed(time.Now()); maintenanceStarted {
		s.tracker.ResetAll()
		results = nil
	}

	for i, res := range results {
		s.record(due[i], res, now)
	}

	// Prune old logs every 5 minutes
	if now.Sub(s.lastPrune) > 5*time.Minute {
		if err := database.PruneLogs(database.DefaultLogRetention); err != nil {
			log.Printf("Warning: Failed to prune logs: %v", err)
		}
		s.lastPrune = now
	}
}

// record stores one check result and advances notification state.
func (s *scheduler) record(sc models.ServiceConfig, res checker.Result, checkedAt time.Time) {
	checkOK, code, msPtr, errMsg := res.OK, res.Code, res.MS, res.ErrMsg

	// Track consecutive failures
	consecutiveFailures := s.tracker.Update(sc.Key, checkOK)

	// History and the live dashboard reflect the observed check immediately.
	// Consecutive failures are used only to debounce notifications below.
	observedOK := checkOK
	degraded := models.IsDegraded(observedOK, msPtr)

	// Record stats
	importantHeartbeat := stats.RecordHeartbeat(sc.Key, observedOK, msPtr, code, errMsg)
	database.InsertSample(checkedAt, sc.Key, observedOK, code, msPtr)

	// Log the check result
	logLevel := database.LogLevelInfo
	logMsg := "Service check passed"
	logDetails := ""

	if msPtr != nil {
		logDetails = fmt.Sprintf("status=%d, latency=%dms, interval=%ds", code, *msPtr, sc.CheckInterval)
	} else {
		logDetails = fmt.Sprintf("status=%d, interval=%ds", code, sc.CheckInterval)
	}

	if !observedOK {
		logLevel = database.LogLevelError
		logMsg = "Service check failed"
		if errMsg != "" {
			logDetails += ", error=" + errMsg
		}
	} else if degraded {
		logLevel = database.LogLevelWarn
		logMsg = "Service degraded (slow response)"
	}

	if observedOK || importantHeartbeat {
		_ = database.InsertLog(logLevel, database.LogCategoryCheck, sc.Key, logMsg, logDetails)
	}

	if errMsg != "" {
		log.Printf("Check %s: %s (failures: %d/2)", sc.Key, errMsg, consecutiveFailures)
	}

	// Notification and banner state only advance on a real success or confirmed failure.
	if err := recordConfirmedServiceState(s.alertMgr, sc.Key, sc.Name, checkOK, consecutiveFailures, degraded, time.Now()); err != nil {
		log.Printf("Warning: Failed to record outage state for %s: %v", sc.Key, err)
	}
}

func checkResultIsConfirmed(checkOK bool, consecutiveFailures int) bool {
	return checkOK || consecutiveFailures >= 2
}

type serviceAlertNotifier interface {
	CheckAndSendAlerts(serviceKey, serviceName string, ok, degraded bool) bool
}

func recordConfirmedServiceState(notifier serviceAlertNotifier, serviceKey, serviceName string, checkOK bool, consecutiveFailures int, degraded bool, observedAt time.Time) error {
	if !checkResultIsConfirmed(checkOK, consecutiveFailures) {
		return nil
	}
	if serviceName == "" {
		serviceName = serviceKey
	}
	alertSent := notifier != nil && notifier.CheckAndSendAlerts(serviceKey, serviceName, checkOK, degraded)
	return database.RecordServiceOutageState(serviceKey, !checkOK, alertSent, observedAt)
}

// migrateTokenEncryption encrypts any legacy plaintext API tokens at startup
func migrateTokenEncryption() {
	services, err := database.GetAllServices()
	if err != nil {
		log.Printf("Warning: Failed to load services for token migration: %v", err)
		return
	}
	migrated := 0
	for _, s := range services {
		if s.APIToken != "" {
			// GetAllServices already decrypted – re-encrypt via UpdateService
			// If it was already encrypted, it will re-encrypt (new nonce = good)
			// If it was plaintext, it will now be encrypted
			if err := database.UpdateService(&s); err != nil {
				log.Printf("Warning: Failed to migrate token for service %s: %v", s.Key, err)
			} else {
				migrated++
			}
		}
	}
	if migrated > 0 {
		log.Printf("Migrated %d service API token(s) to encrypted storage", migrated)
	}
}
