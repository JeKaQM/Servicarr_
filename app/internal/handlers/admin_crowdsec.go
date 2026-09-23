package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"status/app/internal/crowdsec"
	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/monitor"
)

// crowdsecConfigResponse deliberately omits secrets. The *_configured flags
// let the UI distinguish an empty setting from a saved one without placing
// credentials back into form fields or browser caches.
type crowdsecConfigResponse struct {
	Enabled            bool    `json:"enabled"`
	LAPIURL            string  `json:"lapi_url"`
	MachineID          string  `json:"machine_id"`
	PollIntervalS      int     `json:"poll_interval_seconds"`
	TLSSkipVerify      bool    `json:"tls_skip_verify"`
	MapHomeLat         float64 `json:"map_home_latitude"`
	MapHomeLng         float64 `json:"map_home_longitude"`
	MachinePasswordSet bool    `json:"machine_password_configured"`
	BouncerKeySet      bool    `json:"bouncer_api_key_configured"`
}

type crowdsecConfigUpdate struct {
	models.CrowdSecConfig
	ClearMachinePassword bool `json:"clear_machine_password"`
	ClearBouncerAPIKey   bool `json:"clear_bouncer_api_key"`
}

// HandleGetCrowdSecConfig retrieves CrowdSec configuration (masked).
func HandleGetCrowdSecConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg, err := database.LoadCrowdSecConfig()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		response := crowdsecConfigResponse{
			PollIntervalS: 30,
		}
		if cfg != nil {
			response = crowdsecConfigResponse{
				Enabled:            cfg.Enabled,
				LAPIURL:            cfg.LAPIURL,
				MachineID:          cfg.MachineID,
				PollIntervalS:      cfg.PollIntervalS,
				TLSSkipVerify:      cfg.TLSSkipVerify,
				MapHomeLat:         cfg.MapHomeLat,
				MapHomeLng:         cfg.MapHomeLng,
				MachinePasswordSet: cfg.MachinePassword != "",
				BouncerKeySet:      cfg.BouncerAPIKey != "",
			}
			if response.PollIntervalS <= 0 {
				response.PollIntervalS = 30
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(response)
	}
}

// HandleSaveCrowdSecConfig saves CrowdSec configuration. Empty secrets are
// preserved unless an explicit clear_* flag is set (UIs send blank fields
// for unchanged secrets); the stored ciphertext survives unrelated edits.
func HandleSaveCrowdSecConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req crowdsecConfigUpdate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Preserve-on-empty merge for secrets.
		existing, err := database.LoadCrowdSecConfig()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if existing != nil {
			if req.MachinePassword == "" && !req.ClearMachinePassword {
				req.MachinePassword = existing.MachinePassword
			}
			if req.BouncerAPIKey == "" && !req.ClearBouncerAPIKey {
				req.BouncerAPIKey = existing.BouncerAPIKey
			}
		}
		if req.ClearMachinePassword {
			req.MachinePassword = ""
		}
		if req.ClearBouncerAPIKey {
			req.BouncerAPIKey = ""
		}
		req.LAPIURL = strings.TrimSpace(req.LAPIURL)
		req.MachineID = strings.TrimSpace(req.MachineID)
		req.BouncerAPIKey = strings.TrimSpace(req.BouncerAPIKey)

		// Validate the LAPI URL (scheme, host, no path/query; cloud metadata
		// targets rejected by the same SSRF guard the checker uses). A disabled
		// integration may be saved blank so it can be cleanly reset.
		if req.LAPIURL == "" {
			if req.Enabled {
				http.Error(w, "enabling requires a CrowdSec LAPI URL", http.StatusBadRequest)
				return
			}
		} else {
			normalizedURL, err := crowdsec.ValidateBaseURL(req.LAPIURL)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			req.LAPIURL = normalizedURL
		}

		// Enforce enable semantics: enabling requires at least one credential.
		if req.Enabled && req.BouncerAPIKey == "" && (req.MachineID == "" || req.MachinePassword == "") {
			http.Error(w, "enabling requires a bouncer API key or machine credentials", http.StatusBadRequest)
			return
		}

		// Clamp poll interval to [10s, 1h]; default 30s.
		if req.PollIntervalS <= 0 {
			req.PollIntervalS = 30
		}
		if req.PollIntervalS < 10 {
			req.PollIntervalS = 10
		}
		if req.PollIntervalS > 3600 {
			req.PollIntervalS = 3600
		}

		// Map destination position: clamp to valid lat/lng ranges; 0,0 means
		// unset and the UI hides destination arcs rather than guessing a place.
		if req.MapHomeLat < -90 || req.MapHomeLat > 90 {
			http.Error(w, "map home latitude must be between -90 and 90", http.StatusBadRequest)
			return
		}
		if req.MapHomeLng < -180 || req.MapHomeLng > 180 {
			http.Error(w, "map home longitude must be between -180 and 180", http.StatusBadRequest)
			return
		}

		if err := database.SaveCrowdSecConfig(&req.CrowdSecConfig); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		monitor.NotifyCrowdSecConfigChanged()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "CrowdSec configuration saved successfully",
		})
	}
}

// HandleGetCrowdSecStatus returns the sync state (last sync, last error,
// decision counts) for the admin dashboard badge.
func HandleGetCrowdSecStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg, err := database.LoadCrowdSecConfig()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		status := models.CrowdSecSyncStatus{Enabled: cfg != nil && cfg.Enabled}
		state, err := database.GetCrowdSecState()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if state != nil {
			status.LastSync = ""
			if !state.LastSync.IsZero() {
				status.LastSync = state.LastSync.UTC().Format(time.RFC3339)
			}
			// A cached poll failure is not an active retry while disabled.
			if status.Enabled {
				status.LastError = state.LastError
				status.AuthFailed = state.AuthFailed
			}
			status.DecisionCount = state.DecisionCount
		}
		count, err := database.GetCrowdSecSnapshotCount()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		status.SnapshotCount = count

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(status)
	}
}

// HandleGetCrowdSecDecisions returns snapshot decisions for the admin
// dashboard. Reads only the local snapshot — the dashboard keeps working
// when LAPI is briefly down (stale by design, surfaced via status endpoint).
func HandleGetCrowdSecDecisions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		activeOnly := r.URL.Query().Get("active") == "true"
		decisions, err := database.GetCrowdSecDecisions(activeOnly, 0)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"decisions": decisions,
			"count":     len(decisions),
		})
	}
}

// HandleCrowdSecSyncNow triggers one immediate sync cycle (admin action).
// Runs synchronously so the UI gets fresh data on response.
func HandleCrowdSecSyncNow() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg, err := database.LoadCrowdSecConfig()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.LAPIURL) == "" {
			http.Error(w, "CrowdSec integration is disabled", http.StatusConflict)
			return
		}
		if err := monitor.PollCrowdSec(r.Context()); err != nil {
			http.Error(w, "crowdsec sync failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}
}

// crowdsecCompactAlert carries the fields used by the live dashboard. The
// default alerts response remains the full model for API compatibility.
type crowdsecCompactAlert struct {
	AlertID     string   `json:"alert_id"`
	Scenario    string   `json:"scenario"`
	SourceValue string   `json:"source_value"`
	Country     string   `json:"country"`
	ASNumber    string   `json:"as_number"`
	ASName      string   `json:"as_name"`
	Latitude    *float64 `json:"latitude,omitempty"`
	Longitude   *float64 `json:"longitude,omitempty"`
	EventsCount int64    `json:"events_count"`
	CreatedAt   string   `json:"created_at"`
	HasDecision bool     `json:"has_decision"`
	Simulated   bool     `json:"simulated"`
}

func compactCrowdSecAlerts(alerts []models.CrowdSecAlert) []crowdsecCompactAlert {
	out := make([]crowdsecCompactAlert, 0, len(alerts))
	for _, alert := range alerts {
		out = append(out, crowdsecCompactAlert{
			AlertID: alert.AlertID, Scenario: alert.Scenario,
			SourceValue: alert.SourceValue, Country: alert.Country,
			ASNumber: alert.ASNumber, ASName: alert.ASName,
			Latitude: alert.Latitude, Longitude: alert.Longitude,
			EventsCount: alert.EventsCount, CreatedAt: alert.CreatedAt,
			HasDecision: alert.HasDecision, Simulated: alert.Simulated,
		})
	}
	return out
}

// HandleGetCrowdSecAlerts returns the newest scenario-detection alerts for
// the live activity feed. Reads only the local snapshot — includes scans
// that produced no decision (that's the point).
func HandleGetCrowdSecAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= database.CrowdSecMaxAlertRows {
				limit = n
			}
		}
		compact := r.URL.Query().Get("compact") == "true"
		var alerts []models.CrowdSecAlert
		var err error
		if compact {
			alerts, err = database.GetCrowdSecCompactAlerts(limit)
		} else {
			alerts, err = database.GetCrowdSecAlerts(limit)
		}
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if compact {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"alerts": compactCrowdSecAlerts(alerts),
				"count":  len(alerts),
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"alerts": alerts, "count": len(alerts)})
	}
}

// HandleGetCrowdSecStats returns rolling 24-hour observed-alert aggregates,
// an hourly series, ranked dimensions, and the active-decision composition.
func HandleGetCrowdSecStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stats, err := database.GetCrowdSecStats()
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(stats)
	}
}

// HandleTestCrowdSecConnection tests form values without saving them.
// Accepts raw URL + credentials from the request; a health probe runs
// without credentials, then decisions/alerts are attempted with whichever
// credentials were supplied.
//
// Trust boundary note: like the SMTP password and other admin integration
// settings, this endpoint may send stored credentials to an admin-supplied
// URL. That is the same trust boundary as saving the configuration itself
// (the poller would fetch that URL anyway); accepted risk, same as the
// existing resources test handler.
func HandleTestCrowdSecConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req crowdsecTestRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			writeCrowdSecTestError(w, http.StatusBadRequest, "invalid_request", "Invalid connection test request")
			return
		}

		normalizedURL, err := crowdsec.ValidateBaseURL(req.LAPIURL)
		if err != nil {
			writeCrowdSecTestError(w, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}

		// Preserve-on-empty: fall back to stored secrets for blank fields so
		// the test button works with saved credentials. Explicit clear flags
		// suppress that fallback, otherwise Test Connection could report success
		// for credentials the same form is about to remove.
		if (!req.ClearMachinePassword && req.MachinePassword == "") ||
			(!req.ClearBouncerAPIKey && req.BouncerKey == "") {
			if stored, err := database.LoadCrowdSecConfig(); err == nil && stored != nil {
				if !req.ClearMachinePassword && req.MachinePassword == "" {
					req.MachinePassword = stored.MachinePassword
				}
				if !req.ClearBouncerAPIKey && req.BouncerKey == "" {
					req.BouncerKey = stored.BouncerAPIKey
				}
			}
		}

		client := crowdsec.NewClient(crowdsec.Config{
			BaseURL:            normalizedURL,
			BouncerKey:         req.BouncerKey,
			MachineID:          req.MachineID,
			MachinePassword:    req.MachinePassword,
			InsecureSkipVerify: req.TLSSkipVerify,
		})

		result := map[string]any{"reachable": false}
		if err := client.Health(r.Context()); err != nil {
			writeCrowdSecTestError(w, http.StatusBadGateway, "lapi_unreachable", err.Error())
			return
		}
		result["reachable"] = true

		if req.BouncerKey != "" {
			if _, err := client.Decisions(r.Context(), crowdsec.DecisionsParams{Limit: 1}); err != nil {
				writeCrowdSecTestError(w, http.StatusBadGateway, "decisions_failed", err.Error())
				return
			}
			result["decisions_ok"] = true
		}
		if req.MachineID != "" && req.MachinePassword != "" {
			if _, err := client.Alerts(r.Context(), crowdsec.AlertsParams{Limit: 1}); err != nil {
				writeCrowdSecTestError(w, http.StatusBadGateway, "alerts_failed", err.Error())
				return
			}
			result["alerts_ok"] = true
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

type crowdsecTestRequest struct {
	LAPIURL              string `json:"lapi_url"`
	MachineID            string `json:"machine_id"`
	MachinePassword      string `json:"machine_password"`
	BouncerKey           string `json:"bouncer_key"`
	ClearMachinePassword bool   `json:"clear_machine_password"`
	ClearBouncerAPIKey   bool   `json:"clear_bouncer_api_key"`
	TLSSkipVerify        bool   `json:"tls_skip_verify"`
}

func writeCrowdSecTestError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}
