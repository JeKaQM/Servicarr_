package database

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"status/app/internal/models"
	"testing"
	"time"
)

func TestManualStatusAlertsRoundTrip(t *testing.T) {
	if err := Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	banner := ManualStatusAlert{
		ID: "alert_1", ServiceKeys: []string{"plex", "sonarr"}, Message: "Slow", Level: "warning",
		CreatedAt: "2026-10-03T10:00:00Z", StartsAt: "2026-10-03T12:00:00Z", EndsAt: "2026-10-03T14:00:00Z",
	}
	if err := CreateManualStatusAlert(banner); err != nil {
		t.Fatal(err)
	}
	if err := CreateManualStatusAlert(banner); err == nil {
		t.Fatal("creating a banner with an existing ID must fail, not overwrite")
	}
	got, err := GetManualStatusAlert("alert_1")
	if err != nil || got == nil || !reflect.DeepEqual(*got, banner) {
		t.Fatalf("got %+v, %v", got, err)
	}

	// An update replaces the content but keeps the creation time.
	banner.ServiceKeys, banner.Message, banner.CreatedAt = nil, "Fixed", "2030-01-01T00:00:00Z"
	if err := UpdateManualStatusAlert(banner); err != nil {
		t.Fatal(err)
	}
	got, _ = GetManualStatusAlert("alert_1")
	if got.Message != "Fixed" || got.ServiceKeys != nil || got.CreatedAt != "2026-10-03T10:00:00Z" {
		t.Fatalf("updated = %+v", got)
	}

	if err := CreateManualStatusAlert(ManualStatusAlert{ID: "alert_2", Message: "Open", Level: "info", CreatedAt: "2026-10-03T10:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	deleted, err := DeleteEndedStatusAlerts(time.Date(2026, 10, 3, 14, 0, 0, 0, time.UTC))
	if err != nil || deleted != 1 {
		t.Fatalf("deleted %d ended banners, %v; want 1", deleted, err)
	}
	if existed, err := DeleteManualStatusAlert("alert_2"); err != nil || !existed {
		t.Fatalf("delete = %v, %v", existed, err)
	}
	if existed, _ := DeleteManualStatusAlert("alert_2"); existed {
		t.Fatal("a second delete found the banner again")
	}
}

func TestMaintenanceScheduleKeepsServicesAndNotice(t *testing.T) {
	if err := Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	schedule := models.MaintenanceSchedule{
		ID: "disk", Name: "Disk", Message: "New disk", Level: "warning", ScheduleType: "once",
		StartsAt: "2026-10-04T21:00:00Z", Timezone: "UTC", Enabled: true,
		ServiceKeys: []string{"nas", "plex"}, NoticeMinutes: 60,
	}
	if err := SaveMaintenanceSchedule(&schedule); err != nil {
		t.Fatal(err)
	}
	schedules, err := GetMaintenanceSchedules()
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range schedules {
		if got.ID == "disk" {
			if !reflect.DeepEqual(got.ServiceKeys, []string{"nas", "plex"}) || got.NoticeMinutes != 60 {
				t.Fatalf("got %+v", got)
			}
			return
		}
	}
	t.Fatal("schedule not found")
}

func TestBannerColumnsUpgradeOlderDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v8.db")
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = old.Exec(`CREATE TABLE status_alerts (id TEXT PRIMARY KEY, service_key TEXT,
		message TEXT NOT NULL, level TEXT NOT NULL DEFAULT 'info', created_at TEXT NOT NULL);
		INSERT INTO status_alerts VALUES ('alert_old', 'plex', 'Keep me', 'warning', '2026-01-01T00:00:00Z');
		CREATE TABLE maintenance_schedules (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, message TEXT NOT NULL, level TEXT NOT NULL,
		weekday INTEGER NOT NULL, start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL,
		timezone TEXT NOT NULL, suppress_monitoring INTEGER NOT NULL, enabled INTEGER NOT NULL,
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO maintenance_schedules VALUES ('legacy', 'Legacy', 'Keep me', 'warning', 1, '02:55', 30,
		'Europe/London', 1, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DB.Close() })

	banner, err := GetManualStatusAlert("alert_old")
	if err != nil || banner == nil {
		t.Fatalf("banner lost: %v", err)
	}
	if !reflect.DeepEqual(banner.ServiceKeys, []string{"plex"}) || banner.StartsAt != "" || banner.EndsAt != "" {
		t.Fatalf("upgraded banner = %+v", banner)
	}
	schedules, err := GetMaintenanceSchedules()
	if err != nil {
		t.Fatal(err)
	}
	for _, schedule := range schedules {
		if schedule.ID == "legacy" {
			if schedule.ServiceKeys != nil || schedule.NoticeMinutes != 0 {
				t.Fatalf("upgraded schedule = %+v", schedule)
			}
			return
		}
	}
	t.Fatal("legacy schedule lost")
}
