package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/models"
)

func defaultResourcesUIConfig() *models.ResourcesUIConfig {
	return &models.ResourcesUIConfig{
		Enabled: true,
		CPU:     true,
		Memory:  true,
		Network: true,
		Temp:    true,
		Storage: true,
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
