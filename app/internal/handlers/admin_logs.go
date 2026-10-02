package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/models"
	"strconv"
)

// HandleGetLogs returns system logs with optional filtering
func HandleGetLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// SQLite treats a negative LIMIT as "no limit", so bounds are enforced here.
		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 1 {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			}
			limit = min(n, 500)
		}

		offset := 0
		if o := r.URL.Query().Get("offset"); o != "" {
			n, err := strconv.Atoi(o)
			if err != nil || n < 0 {
				http.Error(w, "invalid offset", http.StatusBadRequest)
				return
			}
			offset = n
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
			Days *int `json:"days"` // 0 means clear all
		}

		// Clearing everything must be explicit: a missing or malformed body is
		// rejected rather than treated as days=0.
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Days == nil || *req.Days < 0 {
			http.Error(w, "request body must be {\"days\": n} with n >= 0", http.StatusBadRequest)
			return
		}

		if err := database.ClearLogs(*req.Days); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}
