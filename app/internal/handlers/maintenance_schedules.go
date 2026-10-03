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

// maintenanceScheduleView is a schedule as the admin list shows it: whether it
// is running, when it runs next, and why it can't run if it is invalid.
type maintenanceScheduleView struct {
	models.MaintenanceSchedule
	Active         bool   `json:"active"`
	ActiveStartsAt string `json:"active_starts_at,omitempty"`
	ActiveEndsAt   string `json:"active_ends_at,omitempty"`
	NextStartsAt   string `json:"next_starts_at,omitempty"`
	NextEndsAt     string `json:"next_ends_at,omitempty"`
	Problem        string `json:"problem,omitempty"`
}

// HandleMaintenanceSchedules manages scheduled maintenance windows.
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
			_ = json.NewEncoder(w).Encode(maintenanceScheduleViews(schedules, time.Now()))

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
			keys, err := checkBannerServices(schedule.ServiceKeys)
			if err != nil {
				writeBannerError(w, err)
				return
			}
			schedule.ServiceKeys = keys
			if err := database.SaveMaintenanceSchedule(&schedule); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			// Wording changes or hides made to the old window's banners would
			// otherwise outlive the edit.
			if err := clearMaintenanceBannerOverrides(schedule.ID); err != nil {
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
			if err := clearMaintenanceBannerOverrides(id); err != nil {
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

func clearMaintenanceBannerOverrides(scheduleID string) error {
	return database.DeleteStatusAlertOverrides("scheduled:"+scheduleID, "upcoming:"+scheduleID)
}

func maintenanceScheduleViews(schedules []models.MaintenanceSchedule, now time.Time) []maintenanceScheduleView {
	active, _ := maintenance.ActiveAt(schedules, now)
	running := make(map[string]maintenance.ActiveWindow, len(active))
	for _, window := range active {
		if current, seen := running[window.Schedule.ID]; !seen || window.EndsAt.After(current.EndsAt) {
			running[window.Schedule.ID] = window
		}
	}
	views := make([]maintenanceScheduleView, 0, len(schedules))
	for _, schedule := range schedules {
		view := maintenanceScheduleView{MaintenanceSchedule: schedule}
		check := schedule
		if err := maintenance.ValidateSchedule(&check); err != nil {
			view.Problem = err.Error()
		}
		if window, ok := running[schedule.ID]; ok {
			view.Active = true
			view.ActiveStartsAt = formatWindowTime(window.StartsAt)
			view.ActiveEndsAt = formatWindowTime(window.EndsAt)
		}
		if next, ok := maintenance.NextWindow(schedule, now); ok {
			view.NextStartsAt = formatWindowTime(next.StartsAt)
			view.NextEndsAt = formatWindowTime(next.EndsAt)
		}
		views = append(views, view)
	}
	return views
}

// formatWindowTime leaves an open end empty.
func formatWindowTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}
