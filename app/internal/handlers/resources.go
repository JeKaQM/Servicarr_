package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/resources"
	"strings"
	"sync"
	"time"
)

// Cached Glances client - recreated when URL changes
var (
	glClient    *resources.Client
	glClientURL string
	glClientMu  sync.RWMutex
)

// getGlancesClient returns a Glances client for the configured URL
func getGlancesClient(glancesURL string) *resources.Client {
	if glancesURL == "" {
		return nil
	}

	// Build full URL: http://{host:port}/api/4
	fullURL := glancesURL
	if !strings.HasPrefix(fullURL, "http://") && !strings.HasPrefix(fullURL, "https://") {
		fullURL = "http://" + fullURL
	}
	fullURL = strings.TrimSuffix(fullURL, "/") + "/api/4"

	glClientMu.RLock()
	if glClient != nil && glClientURL == fullURL {
		glClientMu.RUnlock()
		return glClient
	}
	glClientMu.RUnlock()

	// Need to create or update client
	glClientMu.Lock()
	defer glClientMu.Unlock()

	// Double-check after acquiring write lock
	if glClient != nil && glClientURL == fullURL {
		return glClient
	}

	glClient = resources.NewClient(fullURL)
	glClientURL = fullURL
	return glClient
}

// HandleResources returns a normalized snapshot of system resources from Glances.
// It dynamically reads the Glances URL from database config.
func HandleResources(gl *resources.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Load config to get Glances URL
		cfg, err := database.LoadResourcesUIConfig()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "config_error",
				"message": "Failed to load resources config",
			})
			return
		}

		// Check if resources are enabled and configured
		if cfg == nil || !cfg.Enabled || cfg.GlancesURL == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "not_configured",
				"message": "Resources monitoring is not configured. Set Glances host:port in admin settings.",
			})
			return
		}

		// Get or create Glances client for the configured URL
		client := getGlancesClient(cfg.GlancesURL)
		if client == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "not_configured",
				"message": "Invalid Glances URL configuration",
			})
			return
		}

		// Fetch snapshot
		ctx := r.Context()
		snap, err := client.FetchSnapshot(ctx)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    "glances_unavailable",
				"message":  err.Error(),
				"taken_at": time.Now().UTC(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	}
}
