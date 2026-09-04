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
		now := time.Now().UTC()
		maintenanceActive, _, _ := maintenance.MonitoringSuppressed(now)
		if maintenanceActive {
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
			// Skip disabled services
			disabled, _ := database.GetServiceDisabledState(sc.Key)
			if disabled {
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
			maintenanceStarted, _, _ := maintenance.MonitoringSuppressed(time.Now())
			if maintenanceStarted {
				tracker.ResetAll()
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"saved": false, "maintenance": true, "t": time.Now().UTC()})
				return
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

		maintenanceActive, _, _ := maintenance.MonitoringSuppressed(time.Now())
		if maintenanceActive {
			tracker.ResetAll()
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
		maintenanceStarted, _, _ := maintenance.MonitoringSuppressed(time.Now())
		if maintenanceStarted {
			tracker.ResetAll()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(models.LiveResult{
				Label: sc.Name, OK: true, Status: 0, Maintenance: true, CheckType: sc.CheckType,
			})
			return
		}

		stats.RecordHeartbeat(sc.Key, checkOK, ms, code, errMsg)
		database.InsertSample(now, sc.Key, checkOK, code, ms)

		degraded := checkOK && ms != nil && *ms > 200
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(models.LiveResult{Label: sc.Name, OK: checkOK, Status: code, MS: ms, Degraded: degraded, CheckType: sc.CheckType})
	}
}

// HandleToggleMonitoring enables or disables monitoring for a service
func HandleToggleMonitoring(tracker *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
