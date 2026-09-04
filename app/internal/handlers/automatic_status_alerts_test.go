package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"status/app/internal/database"
	"status/app/internal/models"
)

func TestAutomaticCriticalOutageBanner(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	createAutomaticBannerService(t, "critical", "Core Server")
	if err := database.RecordServiceOutageState("critical", true, true, now.Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}

	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	automatic := onlyAutomaticAlert(t, alerts)
	if automatic.Level != "error" || automatic.Kind != "critical_outage" {
		t.Fatalf("unexpected critical banner: %+v", automatic)
	}
	if !strings.Contains(automatic.Message, "Critical outage: Core Server is currently unavailable") ||
		!strings.Contains(automatic.Message, "An alert has been sent") {
		t.Fatalf("unexpected critical message: %q", automatic.Message)
	}
}

func TestAutomaticCriticalBannerDoesNotClaimUnsentAlert(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	createAutomaticBannerService(t, "no-alert", "Unnotified Service")
	if err := database.RecordServiceOutageState("no-alert", true, false, now); err != nil {
		t.Fatal(err)
	}

	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	message := onlyAutomaticAlert(t, alerts).Message
	if strings.Contains(message, "alert has been sent") || !strings.Contains(message, "has been detected") {
		t.Fatalf("unexpected no-notification message: %q", message)
	}
}

func TestAutomaticRestorationBannerLasts24Hours(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	createAutomaticBannerService(t, "restored", "Restored Service")
	if err := database.RecordServiceOutageState("restored", false, false, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordServiceOutageState("restored", true, true, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	restoredAt := now.Add(-30 * time.Minute)
	if err := database.RecordServiceOutageState("restored", false, false, restoredAt); err != nil {
		t.Fatal(err)
	}

	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	automatic := onlyAutomaticAlert(t, alerts)
	if automatic.Level != "info" || automatic.Kind != "services_restored" {
		t.Fatalf("unexpected restoration banner: %+v", automatic)
	}
	if !strings.Contains(automatic.Message, "Services have been restored") || !strings.Contains(automatic.Message, "closely monitored") {
		t.Fatalf("unexpected restoration message: %q", automatic.Message)
	}
	wantEnd := restoredAt.Add(serviceRecoveryBannerDuration).Format(time.RFC3339)
	if automatic.EndsAt != wantEnd {
		t.Fatalf("restoration end=%q, want %q", automatic.EndsAt, wantEnd)
	}

	alerts, err = getPublicStatusAlerts(restoredAt.Add(serviceRecoveryBannerDuration))
	if err != nil {
		t.Fatal(err)
	}
	if got := automaticAlerts(alerts); len(got) != 0 {
		t.Fatalf("restoration banner remained after 24 hours: %+v", got)
	}
}

func TestCriticalOutageTakesPriorityOverRecentRecovery(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	createAutomaticBannerService(t, "down", "Down Service")
	createAutomaticBannerService(t, "recovered", "Recovered Service")
	if err := database.RecordServiceOutageState("down", true, true, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordServiceOutageState("recovered", true, true, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordServiceOutageState("recovered", false, false, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	automatic := automaticAlerts(alerts)
	if len(automatic) != 1 || automatic[0].Kind != "critical_outage" {
		t.Fatalf("unexpected automatic alerts: %+v", automatic)
	}
}

func TestMaintenanceSuppressesAutomaticServiceBanners(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Date(2026, time.July, 21, 10, 0, 30, 0, time.UTC)
	createAutomaticBannerService(t, "maintenance-down", "Maintenance Service")
	if err := database.RecordServiceOutageState("maintenance-down", true, true, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveMaintenanceSchedule(activeScheduleAt(now)); err != nil {
		t.Fatal(err)
	}

	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	if got := automaticAlerts(alerts); len(got) != 0 {
		t.Fatalf("automatic outage shown during maintenance: %+v", got)
	}
	if len(alerts) != 1 || !alerts[0].Scheduled {
		t.Fatalf("scheduled maintenance banner missing: %+v", alerts)
	}
}

func TestBannerOnlyScheduleDoesNotSuppressCriticalOutage(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Date(2026, time.July, 21, 10, 0, 30, 0, time.UTC)
	createAutomaticBannerService(t, "banner-only-down", "Banner-only Service")
	if err := database.RecordServiceOutageState("banner-only-down", true, true, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	schedule := activeScheduleAt(now)
	schedule.SuppressMonitoring = false
	if err := database.SaveMaintenanceSchedule(schedule); err != nil {
		t.Fatal(err)
	}

	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	automatic := automaticAlerts(alerts)
	if len(automatic) != 1 || automatic[0].Kind != "critical_outage" {
		t.Fatalf("banner-only schedule suppressed outage: %+v", alerts)
	}
}

func TestAutomaticUPSPowerLossIsManagedServerSide(t *testing.T) {
	initAutomaticBannerTest(t)
	if err := database.SaveResourcesUIConfig(&models.ResourcesUIConfig{
		Enabled: true,
		UPS:     true,
		NUTHost: "nut-host",
		UPSName: "apc",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveUPSPowerState("nut-host:3493/apc", false, true); err != nil {
		t.Fatal(err)
	}

	alerts, err := getPublicStatusAlerts(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var ups *models.StatusAlert
	for index := range alerts {
		if alerts[index].Kind == "ups_line_loss" {
			ups = &alerts[index]
			break
		}
	}
	if ups == nil || ups.Level != "warning" || !ups.Automatic || ups.Source != "automatic" {
		t.Fatalf("managed UPS alert missing: %+v", alerts)
	}
	if strings.Contains(ups.Message, "nut-host") || !strings.Contains(ups.Message, "notification has been sent") {
		t.Fatalf("unexpected UPS message: %q", ups.Message)
	}
}

func TestAutomaticUPSPowerLossIgnoresStateFromPreviousConfiguration(t *testing.T) {
	initAutomaticBannerTest(t)
	if err := database.SaveResourcesUIConfig(&models.ResourcesUIConfig{
		Enabled: true,
		UPS:     true,
		NUTHost: "new-host",
		UPSName: "apc",
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveUPSPowerState("old-host:3493/apc", false, true); err != nil {
		t.Fatal(err)
	}

	if alerts := automaticAlerts(mustPublicStatusAlerts(t, time.Now())); len(alerts) != 0 {
		t.Fatalf("stale UPS state produced an alert: %+v", alerts)
	}
}

func TestGeneratedBannerOverrideCanEditHideAndResetNextOccurrence(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	createAutomaticBannerService(t, "override", "Override Service")
	if err := database.RecordServiceOutageState("override", true, true, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	alert := onlyAutomaticAlert(t, mustPublicStatusAlerts(t, now))
	message, level := "Custom outage wording", "warning"
	if err := database.SaveStatusAlertOverride(database.StatusAlertOverride{
		AlertID: alert.ID, OccurrenceAt: alert.CreatedAt, Message: &message, Level: &level,
	}); err != nil {
		t.Fatal(err)
	}
	edited := onlyAutomaticAlert(t, mustPublicStatusAlerts(t, now))
	if edited.Message != message || edited.Level != level {
		t.Fatalf("override was not applied: %+v", edited)
	}
	if err := database.SaveStatusAlertOverride(database.StatusAlertOverride{
		AlertID: alert.ID, OccurrenceAt: alert.CreatedAt, Hidden: true,
	}); err != nil {
		t.Fatal(err)
	}
	if got := automaticAlerts(mustPublicStatusAlerts(t, now)); len(got) != 0 {
		t.Fatalf("hidden generated alert remained public: %+v", got)
	}
	adminAlerts, err := getAdminStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	if got := onlyAutomaticAlert(t, adminAlerts); !got.Hidden {
		t.Fatalf("hidden alert was not retained for admin restore: %+v", got)
	}

	if err := database.RecordServiceOutageState("override", false, false, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := database.RecordServiceOutageState("override", true, true, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	next := onlyAutomaticAlert(t, mustPublicStatusAlerts(t, now.Add(3*time.Minute)))
	if next.Hidden || next.Message == message || next.CreatedAt == alert.CreatedAt {
		t.Fatalf("old occurrence override leaked into a new outage: %+v", next)
	}
}

func TestStatusAlertHandlersEditManualAndHideGeneratedBanners(t *testing.T) {
	initAutomaticBannerTest(t)
	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/admin/status-alerts",
		strings.NewReader(`{"message":"Initial","level":"info"}`))
	HandleCreateStatusAlert().ServeHTTP(createRecorder, createRequest)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createRecorder.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/admin/status-alerts",
		strings.NewReader(`{"id":"`+created.ID+`","message":"Updated","level":"error","service_key":""}`))
	HandleUpdateStatusAlert().ServeHTTP(updateRecorder, updateRequest)
	manual, err := getStatusAlerts()
	if err != nil {
		t.Fatal(err)
	}
	if updateRecorder.Code != http.StatusOK || len(manual) != 1 || manual[0].Message != "Updated" || manual[0].Level != "error" {
		t.Fatalf("manual banner was not updated: status=%d alerts=%+v", updateRecorder.Code, manual)
	}

	now := time.Now().UTC()
	createAutomaticBannerService(t, "handler-generated", "Generated Service")
	if err := database.RecordServiceOutageState("handler-generated", true, true, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	generated := onlyAutomaticAlert(t, mustPublicStatusAlerts(t, now))
	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete,
		"/api/admin/status-alerts?id="+generated.ID+"&occurrence_at="+generated.CreatedAt, nil)
	HandleDeleteStatusAlert().ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusOK {
		t.Fatalf("generated hide status = %d: %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	if got := automaticAlerts(mustPublicStatusAlerts(t, now)); len(got) != 0 {
		t.Fatalf("generated banner remained public after hide: %+v", got)
	}
}

func mustPublicStatusAlerts(t *testing.T, now time.Time) []models.StatusAlert {
	t.Helper()
	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	return alerts
}

func initAutomaticBannerTest(t *testing.T) {
	t.Helper()
	initMaintenanceHandlerDB(t)
	if _, err := database.DB.Exec(`DELETE FROM maintenance_schedules`); err != nil {
		t.Fatal(err)
	}
}

func createAutomaticBannerService(t *testing.T, key, name string) {
	t.Helper()
	if _, err := database.CreateService(&models.ServiceConfig{
		Key: key, Name: name, URL: "http://example.test", ServiceType: "custom", CheckType: "http",
		CheckInterval: 60, Timeout: 2, ExpectedMin: 200, ExpectedMax: 299, Visible: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func automaticAlerts(alerts []models.StatusAlert) []models.StatusAlert {
	result := make([]models.StatusAlert, 0)
	for _, alert := range alerts {
		if alert.Automatic {
			result = append(result, alert)
		}
	}
	return result
}

func onlyAutomaticAlert(t *testing.T, alerts []models.StatusAlert) models.StatusAlert {
	t.Helper()
	automatic := automaticAlerts(alerts)
	if len(automatic) != 1 {
		t.Fatalf("got %d automatic alerts, want 1: %+v", len(automatic), alerts)
	}
	return automatic[0]
}
