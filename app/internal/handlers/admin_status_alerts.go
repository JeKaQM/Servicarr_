package handlers

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"status/app/internal/cache"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"status/app/internal/resources"
	"strings"
	"time"
)

const serviceRecoveryBannerDuration = 24 * time.Hour

// Admin changes clear the public cache, so they show at once; banners that
// start or end on a schedule appear within the TTL.
const (
	publicStatusAlertsCacheKey = "public:status-alerts"
	publicStatusAlertsTTL      = 10 * time.Second
)

// errBannerServices reports that the service list could not be loaded.
var errBannerServices = errors.New("failed to load services")

// HandleGetStatusAlerts returns manual banners in every state, and generated
// banners including hidden occurrences.
func HandleGetStatusAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alerts, err := getAdminStatusAlerts(time.Now())
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(alerts)
	}
}

// HandleGetPublicStatusAlerts returns the banners visitors see now.
func HandleGetPublicStatusAlerts() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alerts, err := cache.PublicCache.GetOrLoad(publicStatusAlertsCacheKey, publicStatusAlertsTTL, func() (interface{}, error) {
			return getPublicStatusAlerts(time.Now())
		})
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(alerts)
	}
}

func getPublicStatusAlerts(now time.Time) ([]models.StatusAlert, error) {
	return getEffectiveStatusAlerts(now, false)
}

func getAdminStatusAlerts(now time.Time) ([]models.StatusAlert, error) {
	return getEffectiveStatusAlerts(now, true)
}

// getEffectiveStatusAlerts lists maintenance, upcoming maintenance, automatic
// and manual banners. Visitors get only live banners about public services;
// admins also get scheduled, ended and hidden ones, each with its state.
func getEffectiveStatusAlerts(now time.Time, admin bool) ([]models.StatusAlert, error) {
	schedules, err := database.GetMaintenanceSchedules()
	if err != nil {
		return nil, err
	}
	// An invalid stored schedule is skipped here; the schedule list reports it.
	active, _ := maintenance.ActiveAt(schedules, now)
	alerts := make([]models.StatusAlert, 0, len(active)+4)
	for _, window := range active {
		alerts = append(alerts, maintenanceBanner(window))
	}
	for _, window := range maintenance.UpcomingAt(schedules, active, now) {
		alerts = append(alerts, upcomingMaintenanceBanner(window))
	}

	pause := maintenance.PauseOf(active)
	if !pause.All {
		automatic, err := getAutomaticServiceStatusAlert(now, pause)
		if err != nil {
			return nil, err
		}
		if automatic != nil {
			alerts = append(alerts, *automatic)
		}
	}
	upsAlert, err := getAutomaticUPSStatusAlert()
	if err != nil {
		return nil, err
	}
	if upsAlert != nil {
		alerts = append(alerts, *upsAlert)
	}
	if admin {
		for i := range alerts {
			alerts[i].State = "live"
		}
	}

	manual, err := getManualStatusAlerts(now, admin)
	if err != nil {
		return nil, err
	}
	alerts = append(alerts, manual...)
	return applyStatusAlertOverrides(alerts, admin)
}

func maintenanceBanner(window maintenance.ActiveWindow) models.StatusAlert {
	startsAt := window.StartsAt.UTC().Format(time.RFC3339)
	endsAt := ""
	if !window.EndsAt.IsZero() {
		endsAt = window.EndsAt.UTC().Format(time.RFC3339)
	}
	return models.StatusAlert{
		ID:         "scheduled:" + window.Schedule.ID,
		Message:    window.Schedule.Message,
		Level:      window.Schedule.Level,
		CreatedAt:  startsAt,
		StartsAt:   startsAt,
		EndsAt:     endsAt,
		Scheduled:  true,
		Kind:       "maintenance",
		Source:     "scheduled",
		Editable:   true,
		ScheduleID: window.Schedule.ID,
	}
}

// upcomingMaintenanceBanner announces a window before it starts. Visitors'
// browsers show its start and end in their own time.
func upcomingMaintenanceBanner(window maintenance.ActiveWindow) models.StatusAlert {
	banner := maintenanceBanner(window)
	banner.ID = "upcoming:" + window.Schedule.ID
	banner.Message = "Planned maintenance: " + window.Schedule.Name + "."
	banner.Level = "info"
	banner.Kind = "maintenance_upcoming"
	return banner
}

// getAutomaticServiceStatusAlert reports outages and recent recoveries of
// public services, leaving out those a maintenance window pauses.
func getAutomaticServiceStatusAlert(now time.Time, pause maintenance.Pause) (*models.StatusAlert, error) {
	all, err := database.GetVisibleServiceOutageStates()
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	states := make([]database.ServiceOutageState, 0, len(all))
	for _, state := range all {
		if !pause.Covers(state.ServiceKey) {
			states = append(states, state)
		}
	}

	down := make([]database.ServiceOutageState, 0)
	alertSent := false
	var outageStarted time.Time
	for _, state := range states {
		if !state.IsDown {
			continue
		}
		down = append(down, state)
		alertSent = alertSent || state.AlertSent
		started := state.UpdatedAt
		if state.DownSince != nil {
			started = *state.DownSince
		}
		if outageStarted.IsZero() || started.Before(outageStarted) {
			outageStarted = started
		}
	}

	if len(down) > 0 {
		message := ""
		if len(down) == 1 {
			message = fmt.Sprintf("Critical outage: %s is currently unavailable.", down[0].ServiceName)
		} else {
			message = fmt.Sprintf("Critical outage: %d services are currently unavailable.", len(down))
		}
		if alertSent {
			message += " An alert has been sent and the outage is being investigated."
		} else {
			message += " The outage has been detected and is being investigated."
		}
		return &models.StatusAlert{
			ID:        "automatic:critical-outage",
			Message:   message,
			Level:     "error",
			CreatedAt: outageStarted.UTC().Format(time.RFC3339),
			Automatic: true,
			Kind:      "critical_outage",
			Source:    "automatic",
			Editable:  true,
		}, nil
	}

	var latestRestored time.Time
	for _, state := range states {
		if state.IsDown || state.RestoredAt == nil {
			continue
		}
		restored := state.RestoredAt.UTC()
		if !now.Before(restored.Add(serviceRecoveryBannerDuration)) {
			continue
		}
		if restored.After(latestRestored) {
			latestRestored = restored
		}
	}
	if latestRestored.IsZero() {
		return nil, nil
	}

	return &models.StatusAlert{
		ID:        "automatic:services-restored",
		Message:   "Services have been restored. Performance is being closely monitored for 24 hours.",
		Level:     "info",
		CreatedAt: latestRestored.Format(time.RFC3339),
		Automatic: true,
		Kind:      "services_restored",
		EndsAt:    latestRestored.Add(serviceRecoveryBannerDuration).Format(time.RFC3339),
		Source:    "automatic",
		Editable:  true,
	}, nil
}

func getAutomaticUPSStatusAlert() (*models.StatusAlert, error) {
	config, err := database.LoadResourcesUIConfig()
	if err != nil {
		return nil, err
	}
	if config == nil || !config.Enabled || !config.UPS {
		return nil, nil
	}
	nutAddress := resources.NormalizeNUTAddress(config.NUTHost)
	upsName := strings.TrimSpace(config.UPSName)
	if nutAddress == "" || upsName == "" {
		return nil, nil
	}
	expectedSource := nutAddress + "/" + upsName
	state, err := database.GetUPSPowerState()
	if err != nil || state == nil || state.PowerPresent || state.Source != expectedSource {
		return nil, err
	}
	message := "Mains power lost. The monitored system is running on UPS battery."
	if state.LossNotified {
		message += " A power-loss notification has been sent."
	}
	return &models.StatusAlert{
		ID:        "automatic:ups-line-loss",
		Message:   message,
		Level:     "warning",
		CreatedAt: state.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Automatic: true,
		Kind:      "ups_line_loss",
		Source:    "automatic",
		Editable:  true,
	}, nil
}

func applyStatusAlertOverrides(alerts []models.StatusAlert, includeHidden bool) ([]models.StatusAlert, error) {
	overrides, err := database.GetStatusAlertOverrides()
	if err != nil {
		return nil, err
	}
	result := make([]models.StatusAlert, 0, len(alerts))
	for _, alert := range alerts {
		if alert.Source != "manual" {
			key := database.StatusAlertOverrideKey(alert.ID, alert.CreatedAt)
			if override, exists := overrides[key]; exists {
				if override.Message != nil {
					alert.Message = *override.Message
				}
				if override.Level != nil {
					alert.Level = *override.Level
				}
				alert.Hidden = override.Hidden
			}
		}
		if alert.Hidden && !includeHidden {
			continue
		}
		result = append(result, alert)
	}
	return result, nil
}

// getManualStatusAlerts lists administrator-written banners. Visitors get the
// live ones, showing only on services the public page lists.
func getManualStatusAlerts(now time.Time, admin bool) ([]models.StatusAlert, error) {
	stored, err := database.GetManualStatusAlerts()
	if err != nil {
		return nil, err
	}
	var public map[string]bool
	if !admin {
		services, err := database.GetVisibleServices()
		if err != nil {
			return nil, err
		}
		public = make(map[string]bool, len(services))
		for _, service := range services {
			public[service.Key] = true
		}
	}

	alerts := make([]models.StatusAlert, 0, len(stored))
	for _, banner := range stored {
		state := manualBannerState(banner, now)
		keys := banner.ServiceKeys
		if !admin {
			if state != "live" {
				continue
			}
			keys = make([]string, 0, len(banner.ServiceKeys))
			for _, key := range banner.ServiceKeys {
				if public[key] {
					keys = append(keys, key)
				}
			}
			// A banner for hidden services only would otherwise jump to the top.
			if len(banner.ServiceKeys) > 0 && len(keys) == 0 {
				continue
			}
		}
		alert := models.StatusAlert{
			ID:          banner.ID,
			ServiceKeys: keys,
			Message:     banner.Message,
			Level:       banner.Level,
			CreatedAt:   banner.CreatedAt,
			StartsAt:    banner.StartsAt,
			EndsAt:      banner.EndsAt,
			Source:      "manual",
			Editable:    true,
		}
		if len(keys) > 0 {
			alert.ServiceKey = keys[0]
		}
		if admin {
			alert.State = state
		}
		alerts = append(alerts, alert)
	}
	return alerts, nil
}

// manualBannerState is "scheduled" before the start, "ended" from the end on,
// and "live" in between.
func manualBannerState(banner database.ManualStatusAlert, now time.Time) string {
	if banner.EndsAt != "" {
		if end, err := time.Parse(time.RFC3339, banner.EndsAt); err == nil && !now.Before(end) {
			return "ended"
		}
	}
	if banner.StartsAt != "" {
		if start, err := time.Parse(time.RFC3339, banner.StartsAt); err == nil && now.Before(start) {
			return "scheduled"
		}
	}
	return "live"
}

// statusAlertRequest creates or changes a manual banner, or adjusts one
// occurrence of a generated banner.
type statusAlertRequest struct {
	ID           string `json:"id"`
	OccurrenceAt string `json:"occurrence_at"`
	ServiceKey   string `json:"service_key"`
	// ServiceKeys replaces ServiceKey when present; an empty list means the
	// top of the page.
	ServiceKeys *[]string `json:"service_keys"`
	Message     string    `json:"message"`
	Level       string    `json:"level"`
	// StartsAt and EndsAt are RFC3339; empty clears them, absent keeps them.
	StartsAt *string `json:"starts_at"`
	EndsAt   *string `json:"ends_at"`
	// EndNow ends a live banner at once.
	EndNow bool  `json:"end_now"`
	Hidden *bool `json:"hidden"`
}

func decodeStatusAlertRequest(w http.ResponseWriter, r *http.Request) (statusAlertRequest, bool) {
	var req statusAlertRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

// writeBannerError sends a validation problem as plain text for the editor.
func writeBannerError(w http.ResponseWriter, err error) {
	if errors.Is(err, errBannerServices) {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

// HandleCreateStatusAlert creates a manual banner.
func HandleCreateStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeStatusAlertRequest(w, r)
		if !ok {
			return
		}
		now := time.Now().UTC()
		banner := database.ManualStatusAlert{ID: newBannerID(now), CreatedAt: now.Format(time.RFC3339)}
		if err := applyManualBanner(&banner, req, now); err != nil {
			writeBannerError(w, err)
			return
		}
		if err := database.CreateManualStatusAlert(banner); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "id": banner.ID})
	}
}

// HandleUpdateStatusAlert changes a manual banner or overrides one generated
// banner occurrence.
func HandleUpdateStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeStatusAlertRequest(w, r)
		if !ok {
			return
		}
		req.ID = strings.TrimSpace(req.ID)
		if req.ID == "" {
			req.ID = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if req.ID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		banner, err := database.GetManualStatusAlert(req.ID)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if banner != nil {
			if err := applyManualBanner(banner, req, time.Now().UTC()); err != nil {
				writeBannerError(w, err)
				return
			}
			if err := database.UpdateManualStatusAlert(*banner); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeStatusAlertSuccess(w, "updated")
			return
		}

		alert, err := findGeneratedStatusAlert(time.Now(), req.ID, req.OccurrenceAt)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if alert == nil {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		hidden := alert.Hidden
		if req.Hidden != nil {
			hidden = *req.Hidden
		}
		override := database.StatusAlertOverride{
			AlertID: alert.ID, OccurrenceAt: alert.CreatedAt, Hidden: hidden,
		}
		if strings.TrimSpace(req.Message) != "" || strings.TrimSpace(req.Level) != "" {
			message, level, err := checkBannerText(req.Message, req.Level)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			override.Message = &message
			override.Level = &level
		}
		if err := database.SaveStatusAlertOverride(override); err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		writeStatusAlertSuccess(w, "updated")
	}
}

// HandleDeleteStatusAlert deletes a manual banner, hides one generated banner
// occurrence, or with ?ended=1 deletes every manual banner that has ended.
func HandleDeleteStatusAlert() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ended := r.URL.Query().Get("ended"); ended == "1" || ended == "true" {
			deleted, err := database.DeleteEndedStatusAlerts(time.Now())
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "action": "deleted", "deleted": deleted})
			return
		}

		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}

		deleted, err := database.DeleteManualStatusAlert(id)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if !deleted {
			alert, findErr := findGeneratedStatusAlert(time.Now(), id, r.URL.Query().Get("occurrence_at"))
			if findErr != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if alert == nil {
				http.Error(w, "alert not found", http.StatusNotFound)
				return
			}
			if err := database.SaveStatusAlertOverride(database.StatusAlertOverride{
				AlertID: alert.ID, OccurrenceAt: alert.CreatedAt, Hidden: true,
			}); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
		}
		writeStatusAlertSuccess(w, "deleted")
	}
}

// newBannerID is unique even when two banners share a clock reading, which
// happens on Windows, whose clock advances in steps of hundreds of nanoseconds.
func newBannerID(now time.Time) string {
	var suffix [4]byte
	_, _ = rand.Read(suffix[:])
	return fmt.Sprintf("alert_%d_%x", now.UnixNano(), suffix)
}

// applyManualBanner validates a request and copies it onto the banner. On an
// update, a schedule field the request leaves out keeps its value.
func applyManualBanner(banner *database.ManualStatusAlert, req statusAlertRequest, now time.Time) error {
	message, level, err := checkBannerText(req.Message, req.Level)
	if err != nil {
		return err
	}
	keys := []string(nil)
	if req.ServiceKeys != nil {
		keys = *req.ServiceKeys
	} else if key := strings.TrimSpace(req.ServiceKey); key != "" {
		keys = []string{key}
	}
	if keys, err = checkBannerServices(keys); err != nil {
		return err
	}

	startsAt, endsAt := banner.StartsAt, banner.EndsAt
	if req.StartsAt != nil {
		if startsAt, err = parseBannerTime(*req.StartsAt); err != nil {
			return fmt.Errorf("Start: %w", err)
		}
	}
	if req.EndsAt != nil {
		if endsAt, err = parseBannerTime(*req.EndsAt); err != nil {
			return fmt.Errorf("End: %w", err)
		}
	}
	start, _ := time.Parse(time.RFC3339, startsAt)
	if req.EndNow {
		if startsAt != "" && now.Before(start) {
			return errors.New("This banner hasn't started yet; delete it instead")
		}
		endsAt = now.Format(time.RFC3339)
	} else if endsAt != "" {
		end, _ := time.Parse(time.RFC3339, endsAt)
		if startsAt != "" && !end.After(start) {
			return errors.New("The end must be after the start")
		}
		if req.EndsAt != nil && !end.After(now) {
			return errors.New("That end time has already passed")
		}
	}

	banner.Message, banner.Level, banner.ServiceKeys = message, level, keys
	banner.StartsAt, banner.EndsAt = startsAt, endsAt
	return nil
}

func checkBannerText(message, level string) (string, string, error) {
	message = strings.TrimSpace(message)
	level = strings.ToLower(strings.TrimSpace(level))
	if message == "" {
		return "", "", errors.New("Write a message for the banner")
	}
	if len(message) > 500 {
		return "", "", errors.New("Keep the message to 500 characters")
	}
	if level == "" {
		level = "info"
	}
	if level != "info" && level != "warning" && level != "error" {
		return "", "", errors.New("Type must be info, warning or error")
	}
	return message, level, nil
}

// checkBannerServices tidies a list of service keys and rejects unknown ones.
func checkBannerServices(keys []string) ([]string, error) {
	keys = splitServiceKeys(strings.Join(keys, ","))
	if len(keys) == 0 {
		return nil, nil
	}
	services, err := database.GetAllServices()
	if err != nil {
		return nil, errBannerServices
	}
	known := make(map[string]bool, len(services))
	for _, service := range services {
		known[service.Key] = true
	}
	for _, key := range keys {
		if !known[key] {
			return nil, fmt.Errorf("Unknown service %q", key)
		}
	}
	return keys, nil
}

// parseBannerTime normalises an RFC3339 instant to UTC seconds, so stored
// times compare correctly as text. Empty stays empty.
func parseBannerTime(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || parsed.Year() < 2000 || parsed.Year() > 9999 {
		return "", errors.New("use a date and time such as 2026-10-04T22:00:00Z")
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

func findGeneratedStatusAlert(now time.Time, id, occurrenceAt string) (*models.StatusAlert, error) {
	alerts, err := getAdminStatusAlerts(now)
	if err != nil {
		return nil, err
	}
	for index := range alerts {
		alert := &alerts[index]
		if alert.Source == "manual" || alert.ID != id {
			continue
		}
		if occurrenceAt == "" || alert.CreatedAt == occurrenceAt {
			return alert, nil
		}
	}
	return nil, nil
}

func writeStatusAlertSuccess(w http.ResponseWriter, action string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "action": action})
}
