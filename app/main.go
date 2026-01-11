package main

import (
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
	"status/app/internal/resources"
	"status/app/internal/security"
)

func main() {
	// Load configuration from environment
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := database.Init(cfg.DBPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create auth manager
	authMgr := auth.NewAuth(
		cfg.AuthUser,
		cfg.AuthHash,
		cfg.HmacSecret,
		cfg.InsecureDev,
		cfg.SessionMaxAgeS,
	)

	// Create alert manager (loads config from database)
	alertMgr := alerts.NewManager(cfg.StatusPageURL)

	// Convert service configs to service models
	services := make([]*models.Service, 0, len(cfg.ServiceConfigs))
	for _, sc := range cfg.ServiceConfigs {
		svc := &models.Service{
			Key:     sc.Key,
			Label:   sc.Label,
			URL:     sc.URL,
			Timeout: sc.Timeout,
			MinOK:   sc.MinOK,
			MaxOK:   sc.MaxOK,
		}

		// Load disabled state from database
		if disabled, err := database.GetServiceDisabledState(sc.Key); err == nil {
			svc.Disabled = disabled
		}

		services = append(services, svc)
	}

	// Start health check scheduler
	if cfg.EnableScheduler {
		go runScheduler(services, alertMgr, cfg.PollInterval)
		log.Printf("Scheduler started with %v interval", cfg.PollInterval)
	}

	// Setup HTTP routes
	gl := resources.NewClient(cfg.GlancesBaseURL)
	mux := handlers.SetupRoutes(authMgr, alertMgr, services, gl)

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

// runScheduler runs health checks on a regular interval
func runScheduler(services []*models.Service, alertMgr *alerts.Manager, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		for _, svc := range services {
			if svc.Disabled {
				continue
			}

			// Perform health check
			checkOK, code, msPtr, errMsg := checker.HTTPCheck(svc.URL, svc.Timeout, svc.MinOK, svc.MaxOK)

			// Track consecutive failures - service is only DOWN after 2 consecutive failures
			if checkOK {
				svc.ConsecutiveFailures = 0
			} else {
				svc.ConsecutiveFailures++
			}

			// Service is considered OK if check passed OR if we haven't hit 2 consecutive failures yet
			ok := checkOK || svc.ConsecutiveFailures < 2

			// Record sample in database (use the adjusted ok status)
			database.InsertSample(time.Now(), svc.Key, ok, code, msPtr)

			// Check if service is degraded (slow response)
			degraded := ok && msPtr != nil && *msPtr > 200

			// Send alerts if status changed (based on adjusted ok status)
			alertMgr.CheckAndSendAlerts(svc.Key, svc.Label, ok, degraded)

			// Log if there was an error
			if errMsg != "" {
				log.Printf("Check %s: %s (failures: %d/2)", svc.Key, errMsg, svc.ConsecutiveFailures)
			}
		}
	}
}
