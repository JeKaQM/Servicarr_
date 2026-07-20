package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/buildinfo"
	"status/app/internal/database"
)

type databaseVersionInfo struct {
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	SchemaVersion int    `json:"schema_version"`
}

type systemInfoResponse struct {
	buildinfo.Info
	Summary     string                        `json:"summary"`
	Database    databaseVersionInfo           `json:"database"`
	Deployments []database.SoftwareDeployment `json:"deployments"`
}

// HandleGetSystemInfo returns authenticated build and database version details.
func HandleGetSystemInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		engineVersion, err := database.SQLiteVersion()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		deployments, err := database.GetSoftwareDeployments(10)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		build := buildinfo.Current()
		response := systemInfoResponse{
			Info:        build,
			Summary:     build.Summary(),
			Deployments: deployments,
			Database: databaseVersionInfo{
				Engine:        "SQLite",
				EngineVersion: engineVersion,
				SchemaVersion: database.SchemaVersion,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(response)
	}
}
