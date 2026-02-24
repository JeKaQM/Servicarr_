package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/models"
)

// HandleGetLogs returns system logs with optional filtering
func HandleGetLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
			if limit > 500 {
				limit = 500
			}
		}

		offset := 0
		if o := r.URL.Query().Get("offset"); o != "" {
			fmt.Sscanf(o, "%d", &offset)
		}

		level := r.URL.Query().Get("level")
		category := r.URL.Query().Get("category")
		service := r.URL.Query().Get("service")

		logs, err := database.GetLogs(limit, level, category, service, offset)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if logs == nil {
			logs = []models.LogEntry{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"logs": logs,
		})
	}
}

// HandleGetLogStats returns log statistics
func HandleGetLogStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := database.GetLogStats()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	}
}

// HandleClearLogs clears logs older than specified days
func HandleClearLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Days int `json:"days"` // 0 means clear all
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Default to clearing all if no body
			req.Days = 0
		}

		if err := database.ClearLogs(req.Days); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}
