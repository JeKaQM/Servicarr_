package database

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"status/app/internal/models"
	"testing"
)

func TestMaintenanceSchedulesUpgradeLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE maintenance_schedules (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, message TEXT NOT NULL, level TEXT NOT NULL,
		weekday INTEGER NOT NULL, start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL,
		timezone TEXT NOT NULL, suppress_monitoring INTEGER NOT NULL, enabled INTEGER NOT NULL,
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO maintenance_schedules VALUES ('legacy', 'Legacy', 'Keep me', 'warning', 6, '23:00', 1440,
		'Asia/Kathmandu', 0, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = DB.Close() })
	if err := EnsureSchema(); err != nil {
		t.Fatal(err)
	}
	schedules, err := GetMaintenanceSchedules()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range schedules {
		if s.ID == "legacy" {
			if s.ScheduleType != "weekly" || s.Weekday != 6 || s.StartTime != "23:00" || s.DurationMinutes != 1440 || s.Timezone != "Asia/Kathmandu" || s.SuppressMonitoring || !s.Enabled || s.Weekdays != nil {
				t.Fatalf("legacy schedule changed during migration: %+v", s)
			}
			return
		}
	}
	t.Fatal("legacy schedule missing after migration")
}

func TestMaintenanceScheduleNewFieldsRoundTrip(t *testing.T) {
	initTestDB(t)
	for _, schedule := range []models.MaintenanceSchedule{
		{ID: "once", Name: "Upgrade", Message: "Upgrade", Level: "warning", ScheduleType: "once", StartsAt: "2026-09-11T01:00:00Z", EndsAt: "2027-09-20T01:00:00Z", Timezone: "Europe/London", Enabled: true},
		{ID: "open", Name: "Open", Message: "Upgrade", Level: "warning", ScheduleType: "once", StartsAt: "2026-09-11T01:00:00Z", Timezone: "UTC", Enabled: true},
		{ID: "weekdays", Name: "Weekdays", Message: "Upgrade", Level: "info", ScheduleType: "weekly", Weekdays: []int{1, 3, 5}, Weekday: 1, StartTime: "11:00", DurationMinutes: 40000, Timezone: "UTC", SuppressMonitoring: true, Enabled: true},
		{ID: "daily", Name: "Daily", Message: "Upgrade", Level: "info", ScheduleType: "daily", StartTime: "23:00", DurationMinutes: 120, Timezone: "Pacific/Auckland", Enabled: false},
	} {
		if err := SaveMaintenanceSchedule(&schedule); err != nil {
			t.Fatal(err)
		}
		schedules, err := GetMaintenanceSchedules()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, actual := range schedules {
			if actual.ID == schedule.ID {
				found = true
				if !reflect.DeepEqual(actual, schedule) {
					t.Fatalf("got %+v, want %+v", actual, schedule)
				}
			}
		}
		if !found {
			t.Fatalf("missing schedule %q", schedule.ID)
		}
		createdAt := schedule.CreatedAt
		schedule.CreatedAt = ""
		schedule.Name = "Edited"
		if err := SaveMaintenanceSchedule(&schedule); err != nil {
			t.Fatal(err)
		}
		if schedule.CreatedAt != createdAt {
			t.Fatal("update changed creation timestamp")
		}
	}
}
