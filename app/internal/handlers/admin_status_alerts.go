package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/models"
	"time"
)

// HandleGetStatusAlerts returns all status alerts
func HandleGetStatusAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.DB.Query(`SELECT id, service_key, message, level, created_at FROM status_alerts ORDER BY created_at DESC`)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		alerts := []models.StatusAlert{}
		for rows.Next() {
			var a models.StatusAlert
			var serviceKey sql.NullString
			if err := rows.Scan(&a.ID, &serviceKey, &a.Message, &a.Level, &a.CreatedAt); err != nil {
				continue
			}
			if serviceKey.Valid {
				a.ServiceKey = serviceKey.String
			}
			alerts = append(alerts, a)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(alerts)
	}
}

// HandleCreateStatusAlert creates a new alert
func HandleCreateStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ServiceKey string `json:"service_key"`
			Message    string `json:"message"`
			Level      string `json:"level"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, "message required", http.StatusBadRequest)
			return
		}
		if req.Level == "" {
			req.Level = "info"
		}

		id := fmt.Sprintf("alert_%d", time.Now().UnixNano())
		now := time.Now().UTC().Format(time.RFC3339)

		var serviceKey interface{}
		if req.ServiceKey != "" {
			serviceKey = req.ServiceKey
		}

		_, err := database.DB.Exec(`INSERT INTO status_alerts (id, service_key, message, level, created_at) VALUES (?, ?, ?, ?, ?)`,
			id, serviceKey, req.Message, req.Level, now)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "id": id})
	}
}

// HandleDeleteStatusAlert deletes an alert by ID
func HandleDeleteStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		_, err := database.DB.Exec(`DELETE FROM status_alerts WHERE id = ?`, id)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}
}
