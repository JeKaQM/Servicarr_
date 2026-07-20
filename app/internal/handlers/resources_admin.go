package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/resources"
	"strings"
	"time"
)

func defaultResourcesUIConfig() *models.ResourcesUIConfig {
	return &models.ResourcesUIConfig{
		Enabled:    false,
		GlancesURL: "",
		NUTHost:    "",
		UPSName:    "",
		CPU:        true,
		Memory:     true,
		Network:    true,
		Temp:       true,
		Storage:    true,
		UPS:        false,
	}
}

type publicResourcesUIConfig struct {
	Enabled           bool `json:"enabled"`
	GlancesConfigured bool `json:"glances_configured"`
	UPSConfigured     bool `json:"ups_configured"`
	CPU               bool `json:"cpu"`
	Memory            bool `json:"memory"`
	Network           bool `json:"network"`
	Temp              bool `json:"temp"`
	Storage           bool `json:"storage"`
	Swap              bool `json:"swap"`
	Load              bool `json:"load"`
	GPU               bool `json:"gpu"`
	Containers        bool `json:"containers"`
	Processes         bool `json:"processes"`
	Uptime            bool `json:"uptime"`
	UPS               bool `json:"ups"`
}

// HandleGetPublicResourcesUIConfig returns visibility data without exposing
// internal Glances or NUT addresses on the public status page.
func HandleGetPublicResourcesUIConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cfg, err := database.LoadResourcesUIConfig()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if cfg == nil {
			cfg = defaultResourcesUIConfig()
		}

		publicCfg := publicResourcesUIConfig{
			Enabled:           cfg.Enabled,
			GlancesConfigured: strings.TrimSpace(cfg.GlancesURL) != "",
			UPSConfigured:     strings.TrimSpace(cfg.NUTHost) != "" && strings.TrimSpace(cfg.UPSName) != "",
			CPU:               cfg.CPU,
			Memory:            cfg.Memory,
			Network:           cfg.Network,
			Temp:              cfg.Temp,
			Storage:           cfg.Storage,
			Swap:              cfg.Swap,
			Load:              cfg.Load,
			GPU:               cfg.GPU,
			Containers:        cfg.Containers,
			Processes:         cfg.Processes,
			Uptime:            cfg.Uptime,
			UPS:               cfg.UPS,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(publicCfg)
	}
}

// HandleGetResourcesUIConfig retrieves resources widget visibility configuration
func HandleGetResourcesUIConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := database.LoadResourcesUIConfig()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if cfg == nil {
			cfg = defaultResourcesUIConfig()
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	}
}

// HandleSaveResourcesUIConfig saves resources widget visibility configuration
func HandleSaveResourcesUIConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg models.ResourcesUIConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if err := database.SaveResourcesUIConfig(&cfg); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "Resources UI configuration saved successfully",
		})
	}
}

type resourcesConnectionTestRequest struct {
	Source     string `json:"source"`
	GlancesURL string `json:"glances_url"`
	NUTHost    string `json:"nut_host"`
	UPSName    string `json:"ups_name"`
}

// HandleTestResourcesConnection tests form values without saving them.
func HandleTestResourcesConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req resourcesConnectionTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeResourcesTestError(w, http.StatusBadRequest, "invalid_request", "Invalid connection test request")
			return
		}

		switch strings.ToLower(strings.TrimSpace(req.Source)) {
		case "glances":
			baseURL := normalizeGlancesURL(req.GlancesURL)
			if baseURL == "" {
				writeResourcesTestError(w, http.StatusBadRequest, "invalid_glances_url", "Glances host:port is required")
				return
			}

			client := resources.NewClient(baseURL)
			client.SetCacheTTL(0)
			snapshot, err := client.FetchSnapshot(r.Context())
			if err != nil {
				writeResourcesTestError(w, http.StatusBadGateway, "glances_unavailable", err.Error())
				return
			}
			if !hasGlancesData(snapshot) {
				writeResourcesTestError(w, http.StatusBadGateway, "glances_invalid_response", "Glances responded without resource data")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(snapshot)

		case "ups":
			client := resources.NewNUTClient(req.NUTHost)
			if client.Address == "" {
				writeResourcesTestError(w, http.StatusBadRequest, "invalid_nut_host", "NUT host:port is required")
				return
			}
			info, err := client.FetchUPS(r.Context(), req.UPSName)
			if err != nil {
				err = enrichNUTError(r.Context(), client, err)
				writeResourcesTestError(w, http.StatusBadGateway, "ups_unavailable", err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resources.Snapshot{
				TakenAt: time.Now().UTC(),
				UPS:     info,
			})

		default:
			writeResourcesTestError(w, http.StatusBadRequest, "invalid_source", "Source must be glances or ups")
		}
	}
}

func hasGlancesData(snapshot resources.Snapshot) bool {
	return snapshot.Host != "" || snapshot.CPUPercent != nil || snapshot.MemPercent != nil ||
		snapshot.TempC != nil || snapshot.NetRxBytesPerSec != nil || snapshot.FSTotalBytes != nil
}

func writeResourcesTestError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}
