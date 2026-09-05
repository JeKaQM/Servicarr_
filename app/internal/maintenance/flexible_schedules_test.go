package maintenance

import (
	"reflect"
	"status/app/internal/models"
	"strings"
	"testing"
	"time"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestOneTimeWindows(t *testing.T) {
	schedule := weeklySchedule()
	schedule.ScheduleType = "once"
	schedule.StartsAt = "2026-09-10T22:30"
	schedule.EndsAt = "2027-02-20T09:00"
	for _, test := range []struct {
		at     string
		active bool
	}{
		{"2026-09-10T21:29:59Z", false},
		{"2026-09-10T21:30:00Z", true},
		{"2026-12-25T00:00:00Z", true},
		{"2027-02-20T08:59:59Z", true},
		{"2027-02-20T09:00:00Z", false},
	} {
		t.Run(test.at, func(t *testing.T) {
			active, err := ActiveAt([]models.MaintenanceSchedule{schedule}, mustTime(t, test.at))
			if err != nil || (len(active) == 1) != test.active {
				t.Fatalf("active = %+v, err = %v", active, err)
			}
			if len(active) == 1 && active[0].EndsAt != mustTime(t, "2027-02-20T09:00:00Z") {
				t.Fatalf("wrong end: %v", active[0].EndsAt)
			}
		})
	}
	schedule.EndsAt = ""
	active, err := ActiveAt([]models.MaintenanceSchedule{schedule}, mustTime(t, "2526-09-10T22:00:00Z"))
	if err != nil || len(active) != 1 || !active[0].EndsAt.IsZero() {
		t.Fatalf("open-ended window: %+v, %v", active, err)
	}
}

func TestRecurringSchedulesAndLongDurations(t *testing.T) {
	for _, test := range []struct {
		name, scheduleType, start, at, expectedStart string
		days                                         []int
		duration                                     int
		active                                       bool
	}{
		{"multiple weekdays", "weekly", "10:00", "2026-09-11T10:15:00Z", "2026-09-11T10:00:00Z", []int{1, 5, 6}, 60, true},
		{"unselected weekday", "weekly", "10:00", "2026-09-10T10:15:00Z", "", []int{1, 5, 6}, 60, false},
		{"daily yesterday", "daily", "23:00", "2026-09-12T00:30:00Z", "2026-09-11T23:00:00Z", nil, 120, true},
		{"three week overlap", "weekly", "10:00", "2026-09-13T12:00:00Z", "2026-09-07T10:00:00Z", []int{1}, 21 * 24 * 60, true},
		{"before today's overlapping start", "weekly", "23:00", "2026-09-14T12:00:00Z", "2026-09-07T23:00:00Z", []int{1}, 30 * 24 * 60, true},
		{"maximum safe duration", "daily", "10:00", "2026-09-12T11:00:00Z", "2026-09-12T10:00:00Z", nil, int(MaxDurationMinutes), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := weeklySchedule()
			s.ScheduleType, s.StartTime, s.Weekdays, s.DurationMinutes, s.Timezone = test.scheduleType, test.start, test.days, test.duration, "UTC"
			active, err := ActiveAt([]models.MaintenanceSchedule{s}, mustTime(t, test.at))
			if err != nil || (len(active) == 1) != test.active {
				t.Fatalf("active = %+v, err = %v", active, err)
			}
			if len(active) == 1 && !active[0].StartsAt.Equal(mustTime(t, test.expectedStart)) {
				t.Fatalf("start = %v", active[0].StartsAt)
			}
		})
	}
}

func TestClockChanges(t *testing.T) {
	for _, test := range []struct {
		name, zone, local, expected string
		valid                       bool
	}{
		{"London missing hour", "Europe/London", "2026-03-29T01:30", "", false},
		{"London first repeated hour", "Europe/London", "2026-10-25T01:30", "2026-10-25T00:30:00Z", true},
		{"New York missing hour", "America/New_York", "2026-03-08T02:30", "", false},
		{"New York first repeated hour", "America/New_York", "2026-11-01T01:30", "2026-11-01T05:30:00Z", true},
		{"half-hour DST fold", "Australia/Lord_Howe", "2026-04-05T01:45", "2026-04-04T14:45:00Z", true},
		{"half-hour DST gap", "Australia/Lord_Howe", "2026-10-04T02:15", "", false},
		{"date line skipped day", "Pacific/Apia", "2011-12-30T12:00", "", false},
		{"quarter-hour offset", "Asia/Kathmandu", "2026-09-12T12:00", "2026-09-12T06:15:00Z", true},
		{"explicit second occurrence", "Europe/London", "2026-10-25T01:30:00+00:00", "2026-10-25T01:30:00Z", true},
		{"API fractional precision preserved", "UTC", "2026-09-12T12:00:00.125Z", "2026-09-12T12:00:00.125Z", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := weeklySchedule()
			s.ScheduleType, s.Timezone, s.StartsAt = "once", test.zone, test.local
			err := ValidateSchedule(&s)
			if (err == nil) != test.valid {
				t.Fatalf("validation err = %v, want valid=%v", err, test.valid)
			}
			if test.valid && s.StartsAt != test.expected {
				t.Fatalf("start = %q, want %q", s.StartsAt, test.expected)
			}
		})
	}
}

func TestRecurringClockChangeOccurrences(t *testing.T) {
	s := weeklySchedule()
	s.ScheduleType, s.Weekdays, s.StartTime = "weekly", []int{0}, "01:30"
	s.DurationMinutes = 30
	for _, at := range []string{"2026-03-29T00:45:00Z", "2026-03-29T01:45:00Z", "2026-10-25T01:45:00Z"} {
		active, err := ActiveAt([]models.MaintenanceSchedule{s}, mustTime(t, at))
		if err != nil || len(active) != 0 {
			t.Fatalf("nonexistent or second occurrence %s: %+v, %v", at, active, err)
		}
	}
	active, err := ActiveAt([]models.MaintenanceSchedule{s}, mustTime(t, "2026-10-25T00:45:00Z"))
	if err != nil || len(active) != 1 {
		t.Fatalf("first repeated occurrence: %+v, %v", active, err)
	}
	s.DurationMinutes = 14 * 24 * 60
	active, err = ActiveAt([]models.MaintenanceSchedule{s}, mustTime(t, "2026-03-30T12:00:00Z"))
	if err != nil || len(active) != 1 || !active[0].StartsAt.Equal(mustTime(t, "2026-03-22T01:30:00Z")) {
		t.Fatalf("previous long occurrence survives skipped week: %+v, %v", active, err)
	}
}

func TestScheduleValidationBoundaries(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*models.MaintenanceSchedule)
	}{
		{"zero duration", func(s *models.MaintenanceSchedule) { s.DurationMinutes = 0 }},
		{"negative duration", func(s *models.MaintenanceSchedule) { s.DurationMinutes = -1 }},
		{"duration overflow", func(s *models.MaintenanceSchedule) { s.DurationMinutes = int(MaxDurationMinutes) + 1 }},
		{"empty weekday selection", func(s *models.MaintenanceSchedule) { s.Weekdays = []int{} }},
		{"invalid weekday", func(s *models.MaintenanceSchedule) { s.Weekdays = []int{0, 7} }},
		{"invalid legacy weekday", func(s *models.MaintenanceSchedule) { s.Weekday = -1 }},
		{"invalid recurrence", func(s *models.MaintenanceSchedule) { s.ScheduleType = "monthly" }},
		{"machine timezone", func(s *models.MaintenanceSchedule) { s.Timezone = "Local" }},
		{"invalid clock", func(s *models.MaintenanceSchedule) { s.StartTime = "24:00" }},
		{"long name", func(s *models.MaintenanceSchedule) { s.Name = strings.Repeat("a", 101) }},
		{"invalid calendar date", func(s *models.MaintenanceSchedule) { s.ScheduleType, s.StartsAt = "once", "2026-02-30T10:00" }},
		{"missing start", func(s *models.MaintenanceSchedule) { s.ScheduleType = "once" }},
		{"equal end", func(s *models.MaintenanceSchedule) {
			s.ScheduleType, s.StartsAt, s.EndsAt = "once", "2026-09-12T12:00", "2026-09-12T12:00"
		}},
		{"end before start", func(s *models.MaintenanceSchedule) {
			s.ScheduleType, s.StartsAt, s.EndsAt = "once", "2026-09-12T12:00", "2026-09-11T12:00"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := weeklySchedule()
			test.change(&s)
			if err := ValidateSchedule(&s); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := ValidateSchedule(nil); err == nil {
		t.Fatal("expected nil error")
	}
	s := weeklySchedule()
	s.Weekdays = []int{5, 1, 5, 0}
	original := append([]int(nil), s.Weekdays...)
	if _, err := ActiveAt([]models.MaintenanceSchedule{s}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Weekdays, original) {
		t.Fatal("ActiveAt mutated the caller's weekdays")
	}
	if err := ValidateSchedule(&s); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Weekdays, []int{0, 1, 5}) {
		t.Fatalf("weekdays not normalized: %v", s.Weekdays)
	}
}
