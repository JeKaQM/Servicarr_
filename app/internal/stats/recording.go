package stats

import (
	"log"
	"status/app/internal/checker"
	"status/app/internal/database"
	"time"
)

// RecordHeartbeat stores a heartbeat and updates statistics
func RecordHeartbeat(serviceKey string, ok bool, ping *int, httpStatus int, errMsg string) {
	calc := GetCalculator(serviceKey)

	// Sanitize error message before storing — prevents leaking URLs/tokens
	safeMsg := checker.SanitizeError(errMsg)

	status := 0
	if ok {
		status = 1
	}

	important := calc.AddHeartbeat(status, ping, httpStatus, safeMsg)

	// Store in heartbeats table
	importantInt := 0
	if important {
		importantInt = 1
	}

	_, err := database.DB.Exec(`
		INSERT INTO heartbeats (service_key, status, time, msg, ping, http_status, important)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		serviceKey, status, time.Now().UTC().Format(time.RFC3339), safeMsg, ping, httpStatus, importantInt)

	if err != nil {
		log.Printf("Error recording heartbeat: %v", err)
	}

	// Update minutely stats
	updateMinutelyStat(serviceKey, ok, ping)
}

// updateMinutelyStat updates the minutely statistics
func updateMinutelyStat(serviceKey string, ok bool, ping *int) {
	now := time.Now().UTC()
	// Round down to minute
	timestamp := now.Unix() / 60 * 60

	upDelta, downDelta := 0, 0
	if ok {
		upDelta = 1
	} else {
		downDelta = 1
	}

	_, err := database.DB.Exec(`
		INSERT INTO stat_minutely (service_key, timestamp, up, down, ping, ping_min, ping_max)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(service_key, timestamp) DO UPDATE SET
			up = up + excluded.up,
			down = down + excluded.down,
			ping = CASE 
				WHEN excluded.ping IS NOT NULL THEN 
					COALESCE((stat_minutely.ping * (stat_minutely.up + stat_minutely.down) + excluded.ping) / 
						(stat_minutely.up + stat_minutely.down + 1), excluded.ping)
				ELSE stat_minutely.ping 
			END,
			ping_min = CASE 
				WHEN excluded.ping IS NOT NULL THEN 
					COALESCE(MIN(stat_minutely.ping_min, excluded.ping_min), excluded.ping_min)
				ELSE stat_minutely.ping_min 
			END,
			ping_max = CASE 
				WHEN excluded.ping IS NOT NULL THEN 
					COALESCE(MAX(stat_minutely.ping_max, excluded.ping_max), excluded.ping_max)
				ELSE stat_minutely.ping_max 
			END`,
		serviceKey, timestamp, upDelta, downDelta, ping, ping, ping)

	if err != nil {
		log.Printf("Error updating minutely stat: %v", err)
	}
}
