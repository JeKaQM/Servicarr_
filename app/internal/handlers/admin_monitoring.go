package handlers

import (
	"encoding/json"
	"net/http"
	"status/app/internal/checker"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"status/app/internal/monitor"
	"status/app/internal/stats"
	"time"
)

// HandleIngestNow forces an immediate check of all services
func HandleIngestNow(tracker *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		now := time.Now().UTC()
		pause, _, _ := maintenance.PausedAt(now)
		if pause.All {
			tracker.ResetAll()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"saved": false, "maintenance": true, "t": now})
			return
		}
		dbServices, err := database.GetAllServices()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		for _, sc := range dbServices {
			// Skip disabled services and those in a maintenance window
			disabled, _ := database.GetServiceDisabledState(sc.Key)
			if disabled {
				continue
			}
			if pause.Covers(sc.Key) {
				tracker.Reset(sc.Key)
				continue
			}

			timeout := time.Duration(sc.Timeout) * time.Second
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			checkOK, code, ms, errMsg := checker.Check(checker.CheckOptions{
				URL:         sc.URL,
				Timeout:     timeout,
				ExpectedMin: sc.ExpectedMin,
				ExpectedMax: sc.ExpectedMax,
				CheckType:   sc.CheckType,
				ServiceType: sc.ServiceType,
				APIToken:    sc.APIToken,
			})
			after, _, _ := maintenance.PausedAt(time.Now())
			if after.All {
				tracker.ResetAll()
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"saved": false, "maintenance": true, "t": time.Now().UTC()})
				return
			}
			if after.Covers(sc.Key) {
				tracker.Reset(sc.Key)
				continue
			}

			stats.RecordHeartbeat(sc.Key, checkOK, ms, code, errMsg)
			database.InsertSample(now, sc.Key, checkOK, code, ms)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"saved": true, "t": now})
	}
}

// HandleResetRecent clears recent failure incidents
func HandleResetRecent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		if _, err := database.DB.Exec(`DELETE FROM heartbeats WHERE status = 0 AND time >= ?`, cutoff); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if _, err := database.DB.Exec(`DELETE FROM samples WHERE ok = 0 AND taken_at >= ?`, cutoff); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if _, err := database.DB.Exec(`DELETE FROM service_outage_state WHERE is_down = 0 AND restored_at >= ?`, cutoff); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted_recent_incidents": true})
	}
}

// HandleAdminCheck performs a forced check on a specific service
func HandleAdminCheck(tracker *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Service string `json:"service"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		sc, err := database.GetServiceByKey(req.Service)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if sc == nil {
			http.Error(w, "unknown service", http.StatusNotFound)
			return
		}

		disabled, _ := database.GetServiceDisabledState(req.Service)
		if disabled {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(models.LiveResult{Label: sc.Name, OK: false, Status: 0, Degraded: false, Disabled: true, CheckType: sc.CheckType})
			return
		}

		if pause, _, _ := maintenance.PausedAt(time.Now()); pause.Covers(sc.Key) {
			resetPausedTracker(tracker, pause, sc.Key)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(models.LiveResult{
				Label: sc.Name, OK: true, Status: 0, Maintenance: true, CheckType: sc.CheckType,
			})
			return
		}

		now := time.Now().UTC()
		timeout := time.Duration(sc.Timeout) * time.Second
		if timeout == 0 {
			timeout = 5 * time.Second
		}

		checkOK, code, ms, errMsg := checker.Check(checker.CheckOptions{
			URL:         sc.URL,
			Timeout:     timeout,
			ExpectedMin: sc.ExpectedMin,
			ExpectedMax: sc.ExpectedMax,
			CheckType:   sc.CheckType,
			ServiceType: sc.ServiceType,
			APIToken:    sc.APIToken,
		})
		if after, _, _ := maintenance.PausedAt(time.Now()); after.Covers(sc.Key) {
			resetPausedTracker(tracker, after, sc.Key)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(models.LiveResult{
				Label: sc.Name, OK: true, Status: 0, Maintenance: true, CheckType: sc.CheckType,
			})
			return
		}

		stats.RecordHeartbeat(sc.Key, checkOK, ms, code, errMsg)
		database.InsertSample(now, sc.Key, checkOK, code, ms)

		degraded := models.IsDegraded(checkOK, ms)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.LiveResult{Label: sc.Name, OK: checkOK, Status: code, MS: ms, Degraded: degraded, CheckType: sc.CheckType})
	}
}

// resetPausedTracker clears failure counts for a paused service, or for every
// service when the window covers them all.
func resetPausedTracker(tracker *monitor.FailureTracker, pause maintenance.Pause, key string) {
	if pause.All {
		tracker.ResetAll()
		return
	}
	tracker.Reset(key)
}

// HandleToggleMonitoring enables or disables monitoring for a service
func HandleToggleMonitoring(tracker *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Service string `json:"service"`
			Enable  bool   `json:"enable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Service == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		sc, err := database.GetServiceByKey(req.Service)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if sc == nil {
			http.Error(w, "unknown service", http.StatusNotFound)
			return
		}

		disabled := !req.Enable
		if err := database.SetServiceDisabledState(req.Service, disabled); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		if disabled {
			tracker.Reset(req.Service)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service": sc.Key,
			"enabled": !disabled,
		})
	}
}
