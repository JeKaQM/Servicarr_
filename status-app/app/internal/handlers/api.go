package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/checker"
	"status/app/internal/database"
	"status/app/internal/models"
	"strconv"
	"time"
)

// HandleCheck returns current status of all services
func HandleCheck(services []*models.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		out := models.LivePayload{T: now, Status: map[string]models.LiveResult{}}

		for _, s := range services {
			if s.Disabled {
				// Include disabled services in response
				out.Status[s.Key] = models.LiveResult{
					Label:    s.Label,
					OK:       false,
					Status:   0,
					MS:       nil,
					Disabled: true,
					Degraded: false,
				}
				continue
			}
			checkOK, code, ms, _ := checker.HTTPCheck(s.URL, s.Timeout, s.MinOK, s.MaxOK)

			// Update consecutive failure count
			if checkOK {
				s.ConsecutiveFailures = 0
			} else {
				s.ConsecutiveFailures++
			}

			// Service is only DOWN after 2 consecutive failures
			ok := checkOK || s.ConsecutiveFailures < 2
			degraded := ok && ms != nil && *ms > 200
			out.Status[s.Key] = models.LiveResult{
				Label:    s.Label,
				OK:       ok,
				Status:   code,
				MS:       ms,
				Disabled: false,
				Degraded: degraded,
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

		downs := []map[string]any{}
		downsSince := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		rows3, err := database.DB.Query(`SELECT taken_at, service_key, http_status
                             FROM samples
                             WHERE ok=0 AND taken_at >= ?
                             ORDER BY taken_at DESC LIMIT 50`, downsSince)
		if err == nil {
			defer rows3.Close()
			for rows3.Next() {
				var ts, key string
				var st sql.NullInt64
				_ = rows3.Scan(&ts, &key, &st)
				downs = append(downs, map[string]any{"taken_at": ts, "service_key": key, "http_status": st.Int64})
			}
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
