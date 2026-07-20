package maintenance

import (
	"status/app/internal/models"
	"testing"
	"time"
)

func weeklySchedule() models.MaintenanceSchedule {
	return models.MaintenanceSchedule{
		ID:                 "weekly",
		Name:               "Monday maintenance",
		Message:            "Maintenance in progress",
		Level:              "warning",
		Weekday:            int(time.Monday),
		StartTime:          "02:55",
		DurationMinutes:    30,
		Timezone:           "Europe/London",
		SuppressMonitoring: true,
		Enabled:            true,
	}
}

func TestActiveAtMondayUKWindow(t *testing.T) {
	schedule := weeklySchedule()
	for _, test := range []struct {
		name   string
		at     string
		active bool
	}{
		{name: "before", at: "2026-07-20T01:54:59Z", active: false},
		{name: "start inclusive", at: "2026-07-20T01:55:00Z", active: true},
		{name: "during", at: "2026-07-20T02:10:00Z", active: true},
		{name: "end exclusive", at: "2026-07-20T02:25:00Z", active: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, test.at)
			if err != nil {
				t.Fatal(err)
			}
			active, err := ActiveAt([]models.MaintenanceSchedule{schedule}, at)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(active) == 1; got != test.active {
				t.Fatalf("active = %v, want %v", got, test.active)
			}
		})
	}
}

func TestActiveAtUsesGMTInWinter(t *testing.T) {
	schedule := weeklySchedule()
	at := time.Date(2026, time.January, 5, 2, 55, 0, 0, time.UTC)
	active, err := ActiveAt([]models.MaintenanceSchedule{schedule}, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected winter window to be active, got %d", len(active))
	}
}

func TestActiveAtSupportsWindowAcrossMidnight(t *testing.T) {
	schedule := weeklySchedule()
	schedule.StartTime = "23:55"
	schedule.DurationMinutes = 30
	at := time.Date(2026, time.July, 20, 23, 10, 0, 0, time.UTC) // Tuesday 00:10 BST
	active, err := ActiveAt([]models.MaintenanceSchedule{schedule}, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected cross-midnight window to be active, got %d", len(active))
	}
}

func TestActiveAtSupportsMultiDayWindow(t *testing.T) {
	schedule := weeklySchedule()
	schedule.StartTime = "03:00"
	schedule.DurationMinutes = 3 * 24 * 60
	at := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	active, err := ActiveAt([]models.MaintenanceSchedule{schedule}, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected multi-day window to be active, got %d", len(active))
	}
}

func TestActiveAtIgnoresDisabledSchedule(t *testing.T) {
	schedule := weeklySchedule()
	schedule.Enabled = false
	at := time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC)
	active, err := ActiveAt([]models.MaintenanceSchedule{schedule}, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active windows, got %d", len(active))
	}
}

func TestValidateScheduleRejectsInvalidValues(t *testing.T) {
	schedule := weeklySchedule()
	schedule.Timezone = "Not/AZone"
	if err := ValidateSchedule(&schedule); err == nil {
		t.Fatal("expected invalid timezone error")
	}
}
