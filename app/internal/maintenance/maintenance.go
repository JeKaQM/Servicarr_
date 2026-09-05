package maintenance

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
	"time"
)

// MaxDurationMinutes prevents time.Duration overflow; it is approximately 292 years.
const MaxDurationMinutes = math.MaxInt64 / int64(time.Minute)

// ActiveWindow is one occurrence of a maintenance schedule. A zero EndsAt has no end.
type ActiveWindow struct {
	Schedule models.MaintenanceSchedule
	StartsAt time.Time
	EndsAt   time.Time
}

// ValidateSchedule normalizes a schedule, including legacy single-weekday rules.
func ValidateSchedule(schedule *models.MaintenanceSchedule) error {
	if schedule == nil {
		return errors.New("schedule is required")
	}
	schedule.Name = strings.TrimSpace(schedule.Name)
	schedule.Message = strings.TrimSpace(schedule.Message)
	schedule.Level = strings.ToLower(strings.TrimSpace(schedule.Level))
	schedule.StartTime = strings.TrimSpace(schedule.StartTime)
	schedule.Timezone = strings.TrimSpace(schedule.Timezone)
	schedule.ScheduleType = strings.ToLower(strings.TrimSpace(schedule.ScheduleType))
	schedule.StartsAt = strings.TrimSpace(schedule.StartsAt)
	schedule.EndsAt = strings.TrimSpace(schedule.EndsAt)
	if schedule.ScheduleType == "" {
		schedule.ScheduleType = "weekly"
	}

	if schedule.Name == "" {
		return errors.New("name is required")
	}
	if schedule.Message == "" {
		return errors.New("message is required")
	}
	if len(schedule.Name) > 100 || len(schedule.Message) > 500 {
		return errors.New("schedule text is too long")
	}
	if schedule.Level != "info" && schedule.Level != "warning" && schedule.Level != "error" {
		return errors.New("level must be info, warning, or error")
	}
	if schedule.Timezone == "" {
		return errors.New("timezone is required")
	}
	// "Local" is machine-dependent and would change a schedule after a restore.
	if schedule.Timezone == "Local" {
		return errors.New("use an IANA timezone such as Europe/London or UTC")
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	if schedule.ScheduleType == "once" {
		start, err := parseScheduledDate(schedule.StartsAt, location)
		if err != nil {
			return fmt.Errorf("starts_at: %w", err)
		}
		if schedule.EndsAt != "" {
			end, err := parseScheduledDate(schedule.EndsAt, location)
			if err != nil {
				return fmt.Errorf("ends_at: %w", err)
			}
			if !end.After(start) {
				return errors.New("end must be after start")
			}
			schedule.EndsAt = end.UTC().Format(time.RFC3339Nano)
		}
		schedule.StartsAt = start.UTC().Format(time.RFC3339Nano)
		schedule.StartTime = ""
		schedule.DurationMinutes = 0
		schedule.Weekdays = nil
		schedule.Weekday = 0
		return nil
	}
	if schedule.ScheduleType != "weekly" && schedule.ScheduleType != "daily" {
		return errors.New("schedule_type must be once, daily, or weekly")
	}
	parsed, err := time.Parse("15:04", schedule.StartTime)
	if err != nil || parsed.Format("15:04") != schedule.StartTime {
		return errors.New("start_time must use HH:MM format")
	}
	if schedule.DurationMinutes < 1 || int64(schedule.DurationMinutes) > MaxDurationMinutes {
		return fmt.Errorf("duration_minutes must be between 1 and %d", MaxDurationMinutes)
	}
	if schedule.ScheduleType == "weekly" {
		if schedule.Weekdays == nil {
			schedule.Weekdays = []int{schedule.Weekday}
		}
		if len(schedule.Weekdays) == 0 || len(schedule.Weekdays) > 7 {
			return errors.New("choose between 1 and 7 weekdays")
		}
		for _, day := range schedule.Weekdays {
			if day < 0 || day > 6 {
				return errors.New("weekdays must be between 0 and 6")
			}
		}
		schedule.Weekdays = slices.Clone(schedule.Weekdays)
		slices.Sort(schedule.Weekdays)
		schedule.Weekdays = slices.Compact(schedule.Weekdays)
		schedule.Weekday = schedule.Weekdays[0] // Keep old readers compatible.
	} else {
		schedule.Weekdays = nil
		schedule.Weekday = 0
	}
	schedule.StartsAt, schedule.EndsAt = "", ""
	return nil
}

// wallClock returns the first occurrence of a local time when clocks go back,
// and reports false for a missing local time when clocks go forward.
func wallClock(year int, month time.Month, day, hour, minute int, location *time.Location) (time.Time, bool) {
	wall := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	guess := time.Date(year, month, day, hour, minute, 0, 0, location)
	var first time.Time
	found := false
	// Include the offsets on both sides of a transition, including half-hour DST
	// and date-line changes. Avoid relying on time.Date's unspecified fold choice.
	for _, offsetHours := range []int{-48, 0, 48} {
		_, offset := guess.Add(time.Duration(offsetHours) * time.Hour).Zone()
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		local := candidate.In(location)
		if local.Year() == year && local.Month() == month && local.Day() == day &&
			local.Hour() == hour && local.Minute() == minute && (!found || candidate.Before(first)) {
			first = candidate
			found = true
		}
	}
	return first, found
}

func parseScheduledDate(value string, location *time.Location) (time.Time, error) {
	// Explicit offsets are accepted for API clients, including the second occurrence
	// of a repeated clock time. The calendar UI sends local wall time instead.
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		if parsed.Year() < 1 || parsed.UTC().Year() < 1 || parsed.UTC().Year() > 9999 {
			return time.Time{}, errors.New("date must be between years 0001 and 9999")
		}
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02T15:04", value)
	if err != nil || parsed.Year() < 1 || parsed.Format("2006-01-02T15:04") != value {
		return time.Time{}, errors.New("use a valid date and time (YYYY-MM-DDTHH:MM or RFC3339)")
	}
	resolved, exists := wallClock(parsed.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), location)
	if !exists {
		return time.Time{}, errors.New("this local time does not exist because the clocks change; choose another time")
	}
	if resolved.Year() < 1 || resolved.Year() > 9999 {
		return time.Time{}, errors.New("date must be between years 0001 and 9999")
	}
	return resolved, nil
}

// ActiveAt returns all enabled maintenance windows active at the given instant.
func ActiveAt(schedules []models.MaintenanceSchedule, now time.Time) ([]ActiveWindow, error) {
	active := make([]ActiveWindow, 0)
	var validationErrors []error

	for _, stored := range schedules {
		if !stored.Enabled {
			continue
		}
		schedule := stored
		if err := ValidateSchedule(&schedule); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("schedule %q: %w", stored.ID, err))
			continue
		}
		if schedule.ScheduleType == "once" {
			start, _ := time.Parse(time.RFC3339, schedule.StartsAt)
			var end time.Time
			if schedule.EndsAt != "" {
				end, _ = time.Parse(time.RFC3339, schedule.EndsAt)
			}
			if !now.Before(start) && (end.IsZero() || now.Before(end)) {
				active = append(active, ActiveWindow{Schedule: schedule, StartsAt: start, EndsAt: end})
			}
			continue
		}

		location, _ := time.LoadLocation(schedule.Timezone)
		localNow := now.In(location)
		clock, _ := time.Parse("15:04", schedule.StartTime)

		// The most recent start has the latest end, even for overlapping windows
		// lasting many weeks. Two weeks cover a weekly occurrence skipped by DST.
		// Iterate civil dates in UTC so AddDate cannot normalize across a DST gap.
		date := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
		for dayOffset := 0; dayOffset >= -14; dayOffset-- {
			day := date.AddDate(0, 0, dayOffset)
			if schedule.ScheduleType == "weekly" && !slices.Contains(schedule.Weekdays, int(day.Weekday())) {
				continue
			}
			start, exists := wallClock(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), location)
			if !exists || now.Before(start) {
				continue
			}
			end := start.Add(time.Duration(schedule.DurationMinutes) * time.Minute)
			if now.Before(end) {
				active = append(active, ActiveWindow{Schedule: schedule, StartsAt: start, EndsAt: end})
			}
			break
		}
	}

	return active, errors.Join(validationErrors...)
}

// Current loads schedules and returns active windows.
func Current(now time.Time) ([]ActiveWindow, error) {
	schedules, err := database.GetMaintenanceSchedules()
	if err != nil {
		return nil, err
	}
	return ActiveAt(schedules, now)
}

// MonitoringSuppressed reports whether any active window pauses monitoring.
func MonitoringSuppressed(now time.Time) (bool, []ActiveWindow, error) {
	active, err := Current(now)
	for _, window := range active {
		if window.Schedule.SuppressMonitoring {
			return true, active, err
		}
	}
	return false, active, err
}
