package maintenance

import (
	"errors"
	"fmt"
	"status/app/internal/database"
	"status/app/internal/models"
	"strings"
	"time"
)

// ActiveWindow is one occurrence of a recurring maintenance schedule.
type ActiveWindow struct {
	Schedule models.MaintenanceSchedule
	StartsAt time.Time
	EndsAt   time.Time
}

// ValidateSchedule validates values used to calculate a recurring weekly window.
func ValidateSchedule(schedule *models.MaintenanceSchedule) error {
	if schedule == nil {
		return errors.New("schedule is required")
	}
	schedule.Name = strings.TrimSpace(schedule.Name)
	schedule.Message = strings.TrimSpace(schedule.Message)
	schedule.Level = strings.ToLower(strings.TrimSpace(schedule.Level))
	schedule.StartTime = strings.TrimSpace(schedule.StartTime)
	schedule.Timezone = strings.TrimSpace(schedule.Timezone)

	if schedule.Name == "" {
		return errors.New("name is required")
	}
	if schedule.Message == "" {
		return errors.New("message is required")
	}
	if schedule.Level != "info" && schedule.Level != "warning" && schedule.Level != "error" {
		return errors.New("level must be info, warning, or error")
	}
	if schedule.Weekday < 0 || schedule.Weekday > 6 {
		return errors.New("weekday must be between 0 and 6")
	}
	parsed, err := time.Parse("15:04", schedule.StartTime)
	if err != nil || parsed.Format("15:04") != schedule.StartTime {
		return errors.New("start_time must use HH:MM format")
	}
	if schedule.DurationMinutes < 1 || schedule.DurationMinutes > 7*24*60 {
		return errors.New("duration_minutes must be between 1 and 10080")
	}
	if schedule.Timezone == "" {
		return errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(schedule.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	return nil
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

		location, _ := time.LoadLocation(schedule.Timezone)
		localNow := now.In(location)
		clock, _ := time.Parse("15:04", schedule.StartTime)

		// Inspect the prior week so long windows that cross days remain active.
		for dayOffset := 0; dayOffset >= -7; dayOffset-- {
			date := localNow.AddDate(0, 0, dayOffset)
			if int(date.Weekday()) != schedule.Weekday {
				continue
			}
			start := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, location)
			end := start.Add(time.Duration(schedule.DurationMinutes) * time.Minute)
			if !now.Before(start) && now.Before(end) {
				active = append(active, ActiveWindow{Schedule: schedule, StartsAt: start, EndsAt: end})
				break
			}
		}
	}

	return active, errors.Join(validationErrors...)
}

// Current loads the recurring rules and returns active windows.
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
