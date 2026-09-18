package handlers

import (
	"encoding/json"
	"net/http"
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
	Enabled            bool   `json:"enabled"`
	LAPIURL            string `json:"lapi_url"`
	MachineID          string `json:"machine_id"`
	PollIntervalS      int    `json:"poll_interval_seconds"`
	TLSSkipVerify      bool   `json:"tls_skip_verify"`
	MachinePasswordSet bool   `json:"machine_password_configured"`
	BouncerKeySet      bool   `json:"bouncer_api_key_configured"`
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

		// Validate the LAPI URL (scheme, host, no path/query; cloud metadata
		// targets rejected by the same SSRF guard the checker uses).
		normalizedURL, err := crowdsec.ValidateBaseURL(req.LAPIURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.LAPIURL = normalizedURL

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

		if err := database.SaveCrowdSecConfig(&req.CrowdSecConfig); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

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
		status := models.CrowdSecSyncStatus{SnapshotCount: 0}
		if state, err := database.GetCrowdSecState(); err == nil && state != nil {
			status.LastSync = ""
			if !state.LastSync.IsZero() {
				status.LastSync = state.LastSync.UTC().Format(time.RFC3339)
			}
			status.LastError = state.LastError
			status.AuthFailed = state.AuthFailed
			status.DecisionCount = state.DecisionCount
		}
		if count, err := database.GetCrowdSecSnapshotCount(); err == nil {
			status.SnapshotCount = count
		}

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
		if err := monitor.PollCrowdSec(r.Context()); err != nil {
			http.Error(w, "crowdsec sync failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
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
		// the test button works with saved credentials.
		if req.MachinePassword == "" || req.BouncerKey == "" {
			if stored, err := database.LoadCrowdSecConfig(); err == nil && stored != nil {
				if req.MachinePassword == "" {
					req.MachinePassword = stored.MachinePassword
				}
				if req.BouncerKey == "" {
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
	LAPIURL         string `json:"lapi_url"`
	MachineID       string `json:"machine_id"`
	MachinePassword string `json:"machine_password"`
	BouncerKey      string `json:"bouncer_key"`
	TLSSkipVerify   bool   `json:"tls_skip_verify"`
}

func writeCrowdSecTestError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   code,
		"message": message,
	})
}
