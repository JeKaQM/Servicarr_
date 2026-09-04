package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/cache"
	"status/app/internal/checker"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"status/app/internal/monitor"
	"status/app/internal/stats"
	"strconv"
	"time"
)

type incidentItem struct {
	TakenAt         string `json:"taken_at"`
	ServiceKey      string `json:"service_key"`
	ServiceName     string `json:"service_name"`
	HTTPStatus      int64  `json:"http_status"`
	Error           string `json:"error,omitempty"`
	LatencyMS       *int64 `json:"latency_ms,omitempty"`
	CheckType       string `json:"check_type,omitempty"`
	Ongoing         bool   `json:"ongoing,omitempty"`
	StartedAt       string `json:"started_at,omitempty"`
	DurationSeconds int64  `json:"duration_s,omitempty"`
}

type dayHourBucket struct {
	Hour       string   `json:"hour"`
	Uptime     float64  `json:"uptime"`
	AvgMS      *float64 `json:"avg_ms,omitempty"`
	Checks     int      `json:"checks"`
	DownChecks int      `json:"down_checks"`
}

type dayDownEvent struct {
	Time         string `json:"time"`
	HTTPStatus   *int64 `json:"http_status,omitempty"`
	Error        string `json:"error,omitempty"`
	LatencyMS    *int64 `json:"latency_ms,omitempty"`
	FailureCount int    `json:"failure_count"`
	Kind         string `json:"kind,omitempty"`
	AllDay       bool   `json:"all_day,omitempty"`
	Ongoing      bool   `json:"ongoing,omitempty"`
}

// HandleHealth reports whether the HTTP process is ready to serve requests.
// It intentionally avoids service checks so container probes do not affect
// monitoring state or depend on external services.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// HandleCheck returns current status of all services
func HandleCheck(_ *monitor.FailureTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		out := models.LivePayload{T: now, Status: map[string]models.LiveResult{}}

		// Load services dynamically from database to pick up new services
		dbServices, err := database.GetVisibleServices()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
			return
		}

		maintenanceActive, _, _ := maintenance.MonitoringSuppressed(now)

		for _, sc := range dbServices {
			// Check if monitoring is disabled
			disabled, _ := database.GetServiceDisabledState(sc.Key)
			if disabled {
				out.Status[sc.Key] = models.LiveResult{
					Label:       sc.Name,
					OK:          false,
					Status:      0,
					MS:          nil,
					Disabled:    true,
					Degraded:    false,
					CheckType:   sc.CheckType,
					DependsOn:   sc.DependsOn,
					ConnectedTo: sc.ConnectedTo,
				}
				continue
			}

			if maintenanceActive {
				out.Status[sc.Key] = models.LiveResult{
					Label:       sc.Name,
					OK:          true,
					Status:      0,
					Maintenance: true,
					CheckType:   sc.CheckType,
					DependsOn:   sc.DependsOn,
					ConnectedTo: sc.ConnectedTo,
				}
				continue
			}

			timeout := time.Duration(sc.Timeout) * time.Second
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			checkOK, code, ms, _ := checker.Check(checker.CheckOptions{
				URL:         sc.URL,
				Timeout:     timeout,
				ExpectedMin: sc.ExpectedMin,
				ExpectedMax: sc.ExpectedMax,
				CheckType:   sc.CheckType,
				ServiceType: sc.ServiceType,
				APIToken:    sc.APIToken,
			})
			maintenanceStarted, _, _ := maintenance.MonitoringSuppressed(time.Now())
			if maintenanceStarted {
				maintenanceActive = true
				out.Status[sc.Key] = models.LiveResult{
					Label:       sc.Name,
					OK:          true,
					Maintenance: true,
					CheckType:   sc.CheckType,
					DependsOn:   sc.DependsOn,
					ConnectedTo: sc.ConnectedTo,
				}
				continue
			}

			// Public refreshes must not mutate the scheduler's notification debounce.
			// The live status always represents this observed check.
			ok := checkOK
			degraded := ok && ms != nil && *ms > 200
			out.Status[sc.Key] = models.LiveResult{
				Label:       sc.Name,
				OK:          ok,
				Status:      code,
				MS:          ms,
				Disabled:    false,
				Degraded:    degraded,
				CheckType:   sc.CheckType,
				DependsOn:   sc.DependsOn,
				ConnectedTo: sc.ConnectedTo,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// HandleMetrics returns historical uptime metrics
func HandleMetrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days := 7
		hours := 0

		// Support both days and hours parameters
		if q := r.URL.Query().Get("days"); q != "" {
			if n, err := strconv.Atoi(q); err == nil {
				if n < 1 {
					n = 1
				}
				if n > 365 {
					n = 365
				}
				days = n
				hours = days * 24
			}
		} else if q := r.URL.Query().Get("hours"); q != "" {
			if n, err := strconv.Atoi(q); err == nil {
				if n < 1 {
					n = 1
				}
				if n > 24*365 {
					n = 24 * 365
				}
				hours = n
				days = 0
			}
		} else {
			hours = 24
		}

		var since string
		var groupBy string
		var timeField string

		if days > 0 {
			// Use daily aggregation
			since = time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
			groupBy = "substr(taken_at,1,10)"
			timeField = "day"
		} else {
			// Use hourly aggregation
			since = time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)
			groupBy = "substr(taken_at,1,13) || ':00:00Z'"
			timeField = "hour"
		}

		// #nosec G201 -- groupBy is derived from fixed string constants, not user input
		query := fmt.Sprintf(`
WITH aggregated AS (
  SELECT service_key,
         %s AS time_bin,
         SUM(ok) AS up_count,
         COUNT(*) AS total_count,
         AVG(latency_ms) AS avg_ms
  FROM samples
  WHERE taken_at >= ?
  GROUP BY service_key, time_bin
)
SELECT service_key, time_bin, up_count, total_count, avg_ms
FROM aggregated ORDER BY time_bin ASC`, groupBy)

		rows, err := database.DB.Query(query, since)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		series := map[string][]map[string]any{}
		for rows.Next() {
			var key, tb string
			var up, total int
			var avgMs sql.NullFloat64
			_ = rows.Scan(&key, &tb, &up, &total, &avgMs)
			var u float64
			if total > 0 {
				// Use float with 1 decimal place precision to show accurate uptime
				u = float64(up) / float64(total) * 100.0
				u = float64(int(u*10+0.5)) / 10.0 // Round to 1 decimal place
			}
			point := map[string]any{timeField: tb, "uptime": u}
			if avgMs.Valid {
				point["avg_ms"] = avgMs.Float64
			}
			series[key] = append(series[key], point)
		}

		overall := map[string]float64{}
		rows2, err := database.DB.Query(`SELECT service_key, SUM(ok), COUNT(*) FROM samples WHERE taken_at >= ? GROUP BY service_key`, since)
		if err == nil {
			defer rows2.Close()
			for rows2.Next() {
				var key string
				var up, total sql.NullInt64
				_ = rows2.Scan(&key, &up, &total)
				if total.Valid && total.Int64 > 0 {
					pct := float64(up.Int64) * 100.0 / float64(total.Int64)
					overall[key] = float64(int(pct*10+0.5)) / 10.0 // Round to 1 decimal place
				}
			}
		}

		downs, err := loadRecentIncidents(time.Now())
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}

		response := map[string]any{
			"series":  series,
			"overall": overall,
			"downs":   downs,
		}

		if days > 0 {
			response["window_days"] = days
		} else {
			response["window_hours"] = hours
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func loadRecentIncidents(now time.Time) ([]incidentItem, error) {
	now = now.UTC()
	downSince := now.Add(-24 * time.Hour).Format(time.RFC3339)
	rows, err := database.DB.Query(`
		SELECT hb.time,
		       hb.service_key,
		       COALESCE(s.name, hb.service_key) AS service_name,
		       hb.http_status,
		       hb.msg,
		       hb.ping,
		       COALESCE(s.check_type, '') AS check_type
		FROM heartbeats hb
		LEFT JOIN services s ON s.key = hb.service_key
		WHERE hb.status = 0 AND hb.important = 1 AND hb.time >= ?
		ORDER BY hb.time DESC
		LIMIT 50`, downSince)
	if err != nil {
		return nil, err
	}

	recent := make([]incidentItem, 0)
	for rows.Next() {
		var item incidentItem
		var status, ping sql.NullInt64
		var message, checkType sql.NullString
		if err := rows.Scan(&item.TakenAt, &item.ServiceKey, &item.ServiceName, &status, &message, &ping, &checkType); err != nil {
			rows.Close()
			return nil, err
		}
		if status.Valid {
			item.HTTPStatus = status.Int64
		}
		if message.Valid && message.String != "" {
			item.Error = checker.SanitizeError(message.String)
		}
		if ping.Valid {
			value := ping.Int64
			item.LatencyMS = &value
		}
		if checkType.Valid {
			item.CheckType = checkType.String
		}
		recent = append(recent, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	states, err := database.GetVisibleServiceOutageStates()
	if err != nil {
		return nil, err
	}

	ongoing := make([]incidentItem, 0)
	consumed := make(map[int]bool)
	for _, state := range states {
		if !state.IsDown {
			continue
		}
		started := state.UpdatedAt.UTC()
		if state.DownSince != nil {
			started = state.DownSince.UTC()
		}

		item := incidentItem{
			TakenAt:         started.Format(time.RFC3339),
			ServiceKey:      state.ServiceKey,
			ServiceName:     state.ServiceName,
			Ongoing:         true,
			StartedAt:       started.Format(time.RFC3339),
			DurationSeconds: max(0, int64(now.Sub(started).Seconds())),
			Error:           "Service remains unavailable",
		}
		for index, candidate := range recent {
			if consumed[index] || candidate.ServiceKey != state.ServiceKey {
				continue
			}
			item.HTTPStatus = candidate.HTTPStatus
			item.LatencyMS = candidate.LatencyMS
			item.CheckType = candidate.CheckType
			if candidate.Error != "" {
				item.Error = candidate.Error
			}
			consumed[index] = true
			break
		}
		ongoing = append(ongoing, item)
	}

	result := make([]incidentItem, 0, len(ongoing)+len(recent))
	result = append(result, ongoing...)
	for index, item := range recent {
		if !consumed[index] {
			result = append(result, item)
		}
	}
	if len(result) > 50 {
		result = result[:50]
	}
	return result, nil
}

// HandleUptimeStats returns pre-computed uptime statistics for services
func HandleUptimeStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serviceKey := r.URL.Query().Get("service")

		if serviceKey != "" {
			// Get stats for a specific service
			uptimeStats := stats.GetUptimeStats(serviceKey)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(uptimeStats)
			return
		}

		// Get stats for all services
		cacheKey := "all_uptime_stats"
		if cached, ok := cache.StatsCache.Get(cacheKey); ok {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cached)
			return
		}

		// Load all services and compute stats
		services, err := database.GetAllServices()
		if err != nil {
			http.Error(w, "failed to load services", http.StatusInternalServerError)
			return
		}

		result := make(map[string]*stats.UptimeStats)
		for _, svc := range services {
			result[svc.Key] = stats.GetUptimeStats(svc.Key)
		}

		// Cache for 30 seconds
		cache.StatsCache.Set(cacheKey, result)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

// HandleRecentHeartbeats returns recent heartbeats for a service
func HandleRecentHeartbeats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serviceKey := r.URL.Query().Get("service")
		if serviceKey == "" {
			http.Error(w, "service parameter required", http.StatusBadRequest)
			return
		}

		count := 20
		if q := r.URL.Query().Get("count"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 100 {
				count = n
			}
		}

		calc := stats.GetCalculator(serviceKey)
		heartbeats := calc.GetRecentHeartbeats(count)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(heartbeats)
	}
}

// HandleDayDetail returns hour-by-hour uptime and downtime events for a service on a specific day
func HandleDayDetail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serviceKey := r.URL.Query().Get("key")
		dateStr := r.URL.Query().Get("date") // YYYY-MM-DD

		if serviceKey == "" || dateStr == "" {
			http.Error(w, "key and date parameters required", http.StatusBadRequest)
			return
		}

		// Parse date
		dayStart, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			http.Error(w, "invalid date format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		dayEnd := dayStart.Add(24 * time.Hour)

		startStr := dayStart.Format(time.RFC3339)
		endStr := dayEnd.Format(time.RFC3339)

		// Hour-by-hour uptime from observed samples.
		rows, err := database.DB.Query(`
			SELECT substr(taken_at,1,13) AS hour_bin,
			       SUM(ok) AS up_count,
			       COUNT(*) AS total_count,
			       AVG(latency_ms) AS avg_ms
			FROM samples
			WHERE service_key = ? AND taken_at >= ? AND taken_at < ?
			GROUP BY hour_bin ORDER BY hour_bin ASC`,
			serviceKey, startStr, endStr)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		hourMap := map[string]*dayHourBucket{}
		totalUp := 0
		totalChecks := 0
		for rows.Next() {
			var hourBin string
			var up, total int
			var avgMs sql.NullFloat64
			if err := rows.Scan(&hourBin, &up, &total, &avgMs); err != nil {
				rows.Close()
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			pct := 0.0
			if total > 0 {
				pct = float64(up) / float64(total) * 100.0
				pct = float64(int(pct*10+0.5)) / 10.0
			}
			bucket := &dayHourBucket{Hour: hourBin + ":00", Uptime: pct, Checks: total, DownChecks: total - up}
			if avgMs.Valid {
				v := avgMs.Float64
				bucket.AvgMS = &v
			}
			hourMap[hourBin] = bucket
			totalUp += up
			totalChecks += total
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		rows.Close()

		// Build 24 hour buckets
		hours := make([]dayHourBucket, 24)
		for h := 0; h < 24; h++ {
			hourKey := fmt.Sprintf("%s %02d", dateStr, h)
			// Also try ISO format
			hourKeyISO := fmt.Sprintf("%sT%02d", dateStr, h)
			if b, ok := hourMap[hourKey]; ok {
				hours[h] = *b
			} else if b, ok := hourMap[hourKeyISO]; ok {
				hours[h] = *b
			} else {
				hours[h] = dayHourBucket{
					Hour:   fmt.Sprintf("%sT%02d:00", dateStr, h),
					Uptime: -1, // -1 means no data
					Checks: 0,
				}
			}
		}

		// Build at most one representative downtime event per hour from raw
		// failed samples. This also covers outages that began on an earlier day.
		rows2, err := database.DB.Query(`
			SELECT taken_at, http_status, latency_ms
			FROM samples
			WHERE service_key = ? AND ok = 0 AND taken_at >= ? AND taken_at < ?
			ORDER BY taken_at ASC`,
			serviceKey, startStr, endStr)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		downEvents := make([]dayDownEvent, 0)
		eventByHour := make(map[string]int)
		for rows2.Next() {
			var ts string
			var status, latency sql.NullInt64
			if err := rows2.Scan(&ts, &status, &latency); err != nil {
				rows2.Close()
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			hourKey := ts
			if len(hourKey) > 13 {
				hourKey = hourKey[:13]
			}
			if index, exists := eventByHour[hourKey]; exists {
				downEvents[index].FailureCount++
				continue
			}
			ev := dayDownEvent{Time: ts, FailureCount: 1, Kind: "hourly_outage"}
			if status.Valid {
				value := status.Int64
				ev.HTTPStatus = &value
			}
			if latency.Valid {
				value := latency.Int64
				ev.LatencyMS = &value
			}
			eventByHour[hourKey] = len(downEvents)
			downEvents = append(downEvents, ev)
		}
		if err := rows2.Err(); err != nil {
			rows2.Close()
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		rows2.Close()

		// Add a sanitized diagnostic to the representative event when retained
		// heartbeat data exists for that hour.
		rows3, err := database.DB.Query(`SELECT time, msg FROM heartbeats
			WHERE service_key = ? AND status = 0 AND time >= ? AND time < ?
			ORDER BY time ASC`, serviceKey, startStr, endStr)
		if err == nil {
			for rows3.Next() {
				var ts string
				var message sql.NullString
				if rows3.Scan(&ts, &message) != nil || !message.Valid || message.String == "" {
					continue
				}
				hourKey := ts
				if len(hourKey) > 13 {
					hourKey = hourKey[:13]
				}
				if index, exists := eventByHour[hourKey]; exists && downEvents[index].Error == "" {
					downEvents[index].Error = checker.SanitizeError(message.String)
				}
			}
			rows3.Close()
		}

		ongoing := false
		var currentDown int
		if err := database.DB.QueryRow(`SELECT is_down FROM service_outage_state WHERE service_key = ?`, serviceKey).Scan(&currentDown); err == nil {
			ongoing = currentDown != 0
		}
		if totalChecks > 0 && totalUp == 0 && len(eventByHour) == 24 && len(downEvents) > 0 {
			allDay := downEvents[0]
			allDay.Kind = "all_day_outage"
			allDay.AllDay = true
			allDay.Ongoing = ongoing
			allDay.FailureCount = totalChecks
			if allDay.Error == "" {
				allDay.Error = "Every recorded check failed"
			}
			downEvents = []dayDownEvent{allDay}
		} else if ongoing && len(downEvents) > 0 && !dayEnd.Before(time.Now().UTC()) {
			downEvents[len(downEvents)-1].Ongoing = true
		}

		response := map[string]any{
			"service_key": serviceKey,
			"date":        dateStr,
			"hours":       hours,
			"down_events": downEvents,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}
