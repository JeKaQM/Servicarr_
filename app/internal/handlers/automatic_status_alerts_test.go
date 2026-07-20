package handlers

import (
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
