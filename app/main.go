package main

import (
	"log"
	"net/http"
	"sync"
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

// ServiceManager manages the list of services and their state
type ServiceManager struct {
	mu       sync.RWMutex
	services []*models.Service
}

// Global service manager for dynamic service updates
var svcManager = &ServiceManager{}

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

	// Check if setup is complete and load auth accordingly
	authMgr := createAuthManager(cfg)

	// Create alert manager (loads config from database)
	alertMgr := alerts.NewManager(cfg.StatusPageURL)

	// Load services from database, fall back to env config if none exist
	services := loadServicesFromDB(cfg)
	svcManager.services = services

	// Start health check scheduler
	if cfg.EnableScheduler {
		go runScheduler(alertMgr, cfg.PollInterval)
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

// loadServicesFromDB loads services from database, migrating from env config if needed
func loadServicesFromDB(cfg *config.Config) []*models.Service {
	// Check if setup is complete
	setupComplete, _ := database.IsSetupComplete()
	
	// Check if we have services in the database
	dbServices, err := database.GetAllServices()
	if err != nil {
		log.Printf("Warning: Failed to load services from database: %v", err)
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
			}
			if _, err := database.CreateService(svcConfig); err != nil {
				log.Printf("Warning: Failed to migrate service %s: %v", sc.Key, err)
			}
		}
		// Reload from database
		dbServices, _ = database.GetAllServices()
	}

	// Convert to Service models
	services := make([]*models.Service, 0, len(dbServices))
	for _, sc := range dbServices {
		// Build the check URL (append token if needed)
		checkURL := sc.URL

		svc := &models.Service{
			Key:     sc.Key,
			Label:   sc.Name,
			URL:     checkURL,
			Timeout: time.Duration(sc.Timeout) * time.Second,
			MinOK:   sc.ExpectedMin,
			MaxOK:   sc.ExpectedMax,
		}

		// Load disabled state from service_state table
		if disabled, err := database.GetServiceDisabledState(sc.Key); err == nil {
			svc.Disabled = disabled
		}

		services = append(services, svc)
	}

	return services
}

// GetServices returns the current list of services (thread-safe)
func GetServices() []*models.Service {
	svcManager.mu.RLock()
	defer svcManager.mu.RUnlock()
	return svcManager.services
}

// ReloadServices reloads services from the database
func ReloadServices(cfg *config.Config) {
	svcManager.mu.Lock()
	defer svcManager.mu.Unlock()
	svcManager.services = loadServicesFromDB(cfg)
	log.Println("Services reloaded from database")
}

// runScheduler runs health checks on a regular interval
func runScheduler(alertMgr *alerts.Manager, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		// Reload services from DB on each tick to pick up changes
		dbServices, err := database.GetAllServices()
		if err != nil {
			log.Printf("Warning: Failed to reload services: %v", err)
			continue
		}

		// Sync svcManager.services with database
		svcManager.mu.Lock()
		// Create a map of existing services for quick lookup
		existingMap := make(map[string]*models.Service)
		for _, s := range svcManager.services {
			existingMap[s.Key] = s
		}
		
		// Build updated services list
		updatedServices := make([]*models.Service, 0, len(dbServices))
		for _, sc := range dbServices {
			if existing, ok := existingMap[sc.Key]; ok {
				// Keep existing service (preserves ConsecutiveFailures)
				updatedServices = append(updatedServices, existing)
			} else {
				// New service from database
				timeout := time.Duration(sc.Timeout) * time.Second
				if timeout == 0 {
					timeout = 5 * time.Second
				}
				newSvc := &models.Service{
					Key:     sc.Key,
					Label:   sc.Name,
					URL:     sc.URL,
					Timeout: timeout,
					MinOK:   sc.ExpectedMin,
					MaxOK:   sc.ExpectedMax,
				}
				updatedServices = append(updatedServices, newSvc)
				log.Printf("Added new service to scheduler: %s", sc.Key)
			}
		}
		svcManager.services = updatedServices
		svcManager.mu.Unlock()

		for _, sc := range dbServices {
			// Check disabled state
			disabled, _ := database.GetServiceDisabledState(sc.Key)
			if disabled {
				continue
			}

			// Build check URL
			checkURL := sc.URL

			timeout := time.Duration(sc.Timeout) * time.Second
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			// Perform health check
			checkOK, code, msPtr, errMsg := checker.HTTPCheck(checkURL, timeout, sc.ExpectedMin, sc.ExpectedMax)

			// Get the service from manager to track consecutive failures
			svcManager.mu.Lock()
			var svc *models.Service
			for _, s := range svcManager.services {
				if s.Key == sc.Key {
					svc = s
					break
				}
			}

			// Track consecutive failures
			if svc != nil {
				if checkOK {
					svc.ConsecutiveFailures = 0
				} else {
					svc.ConsecutiveFailures++
				}
			}
			svcManager.mu.Unlock()

			// Service is considered OK if check passed OR if we haven't hit 2 consecutive failures yet
			consecutiveFailures := 0
			if svc != nil {
				consecutiveFailures = svc.ConsecutiveFailures
			}
			ok := checkOK || consecutiveFailures < 2

			// Record sample in database
			database.InsertSample(time.Now(), sc.Key, ok, code, msPtr)

			// Check if service is degraded (slow response)
			degraded := ok && msPtr != nil && *msPtr > 200

			// Send alerts if status changed
			alertMgr.CheckAndSendAlerts(sc.Key, sc.Name, ok, degraded)

			// Log if there was an error
			if errMsg != "" {
				log.Printf("Check %s: %s (failures: %d/2)", sc.Key, errMsg, consecutiveFailures)
			}
		}
	}
}
