package maintenance

import (
	"reflect"
	"status/app/internal/models"
	"testing"
)

func TestPauseCoversOnlyChosenServices(t *testing.T) {
	scoped := weeklySchedule()
	scoped.ServiceKeys = []string{"nas", "plex"}
	bannerOnly := weeklySchedule()
	bannerOnly.SuppressMonitoring = false

	pause := PauseOf([]ActiveWindow{{Schedule: scoped}, {Schedule: bannerOnly}})
	if pause.All || !pause.Covers("nas") || !pause.Covers("plex") || pause.Covers("router") {
		t.Fatalf("scoped pause = %+v", pause)
	}
	if everything := PauseOf([]ActiveWindow{{Schedule: weeklySchedule()}}); !everything.All || !everything.Covers("router") {
		t.Fatalf("a window without services must pause every service: %+v", everything)
	}
	if none := PauseOf(nil); none.All || none.Covers("nas") {
		t.Fatalf("no windows paused something: %+v", none)
	}
}

func TestValidateScheduleTidiesServicesAndNotice(t *testing.T) {
	s := weeklySchedule()
	s.ServiceKeys = []string{" nas ", "", "plex", "nas"}
	s.NoticeMinutes = 60
	if err := ValidateSchedule(&s); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.ServiceKeys, []string{"nas", "plex"}) {
		t.Fatalf("services = %q", s.ServiceKeys)
	}
	s.ServiceKeys = []string{" "}
	if err := ValidateSchedule(&s); err != nil || s.ServiceKeys != nil {
		t.Fatalf("blank services should mean every service: %q, %v", s.ServiceKeys, err)
	}
	for _, notice := range []int{-1, MaxNoticeMinutes + 1} {
		s.NoticeMinutes = notice
		if err := ValidateSchedule(&s); err == nil {
			t.Fatalf("notice %d accepted", notice)
		}
	}
}

func TestNextWindow(t *testing.T) {
	// Thursday 1 October 2026, 12:00 in London (BST, UTC+1).
	now := mustTime(t, "2026-10-01T11:00:00Z")

	weekly := weeklySchedule() // Mondays 02:55 for 30 minutes
	next, ok := NextWindow(weekly, now)
	if !ok || !next.StartsAt.Equal(mustTime(t, "2026-10-05T01:55:00Z")) || !next.EndsAt.Equal(mustTime(t, "2026-10-05T02:25:00Z")) {
		t.Fatalf("weekly next = %+v, %v", next, ok)
	}

	daily := weeklySchedule()
	daily.ScheduleType, daily.StartTime = "daily", "13:00"
	if next, ok := NextWindow(daily, now); !ok || !next.StartsAt.Equal(mustTime(t, "2026-10-01T12:00:00Z")) {
		t.Fatalf("daily next should be later today: %+v, %v", next, ok)
	}
	daily.StartTime = "11:00"
	if next, ok := NextWindow(daily, now); !ok || !next.StartsAt.Equal(mustTime(t, "2026-10-02T10:00:00Z")) {
		t.Fatalf("daily next should be tomorrow: %+v, %v", next, ok)
	}

	once := weeklySchedule()
	once.ScheduleType, once.StartsAt, once.EndsAt = "once", "2026-10-04T22:00", "2026-10-04T23:30"
	if next, ok := NextWindow(once, now); !ok || !next.StartsAt.Equal(mustTime(t, "2026-10-04T21:00:00Z")) {
		t.Fatalf("one-time next = %+v, %v", next, ok)
	}
	if _, ok := NextWindow(once, mustTime(t, "2026-10-04T21:30:00Z")); ok {
		t.Fatal("a one-time window that has started has no next occurrence")
	}

	disabled := weeklySchedule()
	disabled.Enabled = false
	if _, ok := NextWindow(disabled, now); ok {
		t.Fatal("a disabled schedule has no next occurrence")
	}
}

func TestNextWindowSkipsATimeTheClocksJumpOver(t *testing.T) {
	// 01:30 doesn't exist in London on 29 March 2026; the next is on the 30th.
	daily := weeklySchedule()
	daily.ScheduleType, daily.StartTime = "daily", "01:30"
	next, ok := NextWindow(daily, mustTime(t, "2026-03-28T12:00:00Z"))
	if !ok || !next.StartsAt.Equal(mustTime(t, "2026-03-30T00:30:00Z")) {
		t.Fatalf("next = %+v, %v", next, ok)
	}
}

func TestUpcomingAtAnnouncesWithinTheNotice(t *testing.T) {
	once := weeklySchedule()
	once.ID = "disk"
	once.ScheduleType, once.StartsAt, once.EndsAt = "once", "2026-10-04T22:00", "2026-10-04T23:30"
	once.NoticeMinutes = 60
	silent := once
	silent.ID, silent.NoticeMinutes = "silent", 0
	schedules := []models.MaintenanceSchedule{once, silent}

	if got := UpcomingAt(schedules, nil, mustTime(t, "2026-10-04T19:59:00Z")); len(got) != 0 {
		t.Fatalf("announced too early: %+v", got)
	}
	got := UpcomingAt(schedules, nil, mustTime(t, "2026-10-04T20:00:00Z"))
	if len(got) != 1 || got[0].Schedule.ID != "disk" || !got[0].StartsAt.Equal(mustTime(t, "2026-10-04T21:00:00Z")) {
		t.Fatalf("upcoming = %+v", got)
	}

	// While a daily window runs, tomorrow's isn't announced.
	daily := weeklySchedule()
	daily.ID, daily.ScheduleType, daily.StartTime, daily.DurationMinutes = "daily", "daily", "02:00", 23*60
	daily.NoticeMinutes = 120
	now := mustTime(t, "2026-10-02T00:30:00Z") // 01:30 London; next start 02:00
	if got := UpcomingAt([]models.MaintenanceSchedule{daily}, nil, now); len(got) != 1 {
		t.Fatalf("daily window not announced: %+v", got)
	}
	running := []ActiveWindow{{Schedule: daily}}
	if got := UpcomingAt([]models.MaintenanceSchedule{daily}, running, now); len(got) != 0 {
		t.Fatalf("running window announced again: %+v", got)
	}
}
