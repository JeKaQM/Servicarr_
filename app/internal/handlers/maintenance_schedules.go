package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"strings"
	"time"
)

// HandleMaintenanceSchedules manages recurring scheduled maintenance banners.
func HandleMaintenanceSchedules() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			schedules, err := database.GetMaintenanceSchedules()
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(schedules)

		case http.MethodPost:
			var schedule models.MaintenanceSchedule
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&schedule); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			if schedule.ID == "" {
				schedule.ID = fmt.Sprintf("schedule_%d", time.Now().UnixNano())
			}
			if len(schedule.ID) > 120 || strings.ContainsAny(schedule.ID, "\r\n") {
				http.Error(w, "invalid schedule ID", http.StatusBadRequest)
				return
			}
			if len(schedule.Name) > 100 || len(schedule.Message) > 500 {
				http.Error(w, "schedule text is too long", http.StatusBadRequest)
				return
			}
			if err := maintenance.ValidateSchedule(&schedule); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := database.SaveMaintenanceSchedule(&schedule); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(schedule)

		case http.MethodDelete:
			id := strings.TrimSpace(r.URL.Query().Get("id"))
			if id == "" {
				http.Error(w, "id required", http.StatusBadRequest)
				return
			}
			if err := database.DeleteMaintenanceSchedule(id); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
