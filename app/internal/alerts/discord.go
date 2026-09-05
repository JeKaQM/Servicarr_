package alerts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"status/app/internal/database"
)

type notificationDetails struct {
	ObservedAt     time.Time
	CheckType      string
	HTTPStatus     sql.NullInt64
	LatencyMS      sql.NullInt64
	PreviousStatus string
	PreviousSince  time.Time
}

// Capture only safe monitoring metadata before queueing; never send monitor URLs, headers or errors.
func loadNotificationDetails(serviceKey string, now time.Time) notificationDetails {
	details := notificationDetails{ObservedAt: now}
	var takenAt string
	if err := database.DB.QueryRow(`SELECT taken_at, http_status, latency_ms FROM samples WHERE service_key = ? ORDER BY id DESC LIMIT 1`, serviceKey).
		Scan(&takenAt, &details.HTTPStatus, &details.LatencyMS); err == nil {
		if parsed, err := time.Parse(time.RFC3339Nano, takenAt); err == nil {
			details.ObservedAt = parsed
		}
	}
	var checkType string
	_ = database.DB.QueryRow(`SELECT check_type FROM services WHERE key = ?`, serviceKey).Scan(&checkType)
	switch checkType {
	case "http", "tcp", "ping", "dns":
		details.CheckType = strings.ToUpper(checkType)
	}
	var previousOK, previousDegraded int
	var previousSince string
	if err := database.DB.QueryRow(`SELECT ok, degraded, updated_at FROM service_status_history WHERE service_key = ?`, serviceKey).
		Scan(&previousOK, &previousDegraded, &previousSince); err == nil {
		details.PreviousStatus = notificationStatus(previousOK == 1, previousDegraded == 1)
		details.PreviousSince, _ = time.Parse("2006-01-02 15:04:05", previousSince)
	}
	return details
}

// SendDiscord sends a rich embed and reports whether Discord accepted delivery.
func (m *Manager) SendDiscord(subject, statusType, serviceName, message, statusPageURL string) error {
	return m.sendDiscord(subject, statusType, serviceName, message, statusPageURL, notificationDetails{ObservedAt: time.Now()})
}

func (m *Manager) sendDiscord(subject, statusType, serviceName, message, statusPageURL string, details notificationDetails) error {
	config := m.GetConfig()
	if config == nil || config.DiscordWebhookURL == "" {
		return errors.New("Discord webhook URL is not configured")
	}
	color := map[string]int{"down": 0xef4444, "degraded": 0xeab308, "up": 0x22c55e, "test": 0x5865f2}[statusType]
	statusLabel := map[string]string{"down": "Unavailable", "degraded": "Degraded", "up": "Operational", "test": "Test notification"}[statusType]
	if statusLabel == "" {
		statusLabel = "Status update"
	}
	fields := []map[string]interface{}{
		{"name": "Service", "value": discordText(cleanNotificationName(serviceName, "Service"), 512), "inline": true},
		{"name": "Status", "value": statusLabel, "inline": true},
	}
	if details.PreviousStatus != "" {
		fields = append(fields, map[string]interface{}{"name": "Previous status", "value": map[string]string{"down": "Unavailable", "degraded": "Degraded", "up": "Operational"}[details.PreviousStatus], "inline": true})
	}
	if details.CheckType != "" {
		fields = append(fields, map[string]interface{}{"name": "Check", "value": details.CheckType, "inline": true})
	}
	if details.LatencyMS.Valid && details.LatencyMS.Int64 >= 0 {
		fields = append(fields, map[string]interface{}{"name": "Response time", "value": fmt.Sprintf("%d ms", details.LatencyMS.Int64), "inline": true})
	}
	if details.HTTPStatus.Valid && details.HTTPStatus.Int64 >= 100 && details.HTTPStatus.Int64 <= 599 {
		fields = append(fields, map[string]interface{}{"name": "HTTP status", "value": fmt.Sprint(details.HTTPStatus.Int64), "inline": true})
	}
	if details.PreviousStatus != "" && details.PreviousStatus != "up" && !details.PreviousSince.IsZero() {
		duration := details.ObservedAt.Sub(details.PreviousSince).Round(time.Second)
		if duration >= 0 {
			fields = append(fields, map[string]interface{}{"name": "Time in previous status", "value": duration.String(), "inline": true})
		}
	}
	if details.ObservedAt.IsZero() {
		details.ObservedAt = time.Now()
	}
	// Generated messages use only <strong>; remove it before decoding HTML entities and escaping Markdown.
	plainMessage := html.UnescapeString(strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", ""), "</strong>", ""))
	embed := map[string]interface{}{
		"title":       discordText(subject, 256),
		"description": discordText(plainMessage, 3500),
		"color":       color,
		"fields":      fields,
		"timestamp":   details.ObservedAt.UTC().Format(time.RFC3339),
		"footer":      map[string]string{"text": "Servicarr Status Monitor"},
	}
	if dashboardURL := safeStatusPageURL(statusPageURL); dashboardURL != "" {
		embed["url"] = dashboardURL
		// A link on the title is convenient without copying sensitive monitor endpoints.
	}
	username := cleanNotificationName(config.DiscordUsername, "Servicarr")
	payload := map[string]interface{}{
		"username":         truncateDiscord(username, 80),
		"embeds":           []map[string]interface{}{embed},
		"allowed_mentions": map[string]interface{}{"parse": []string{}},
	}
	if config.DiscordSilent {
		payload["flags"] = 4096
	}
	body, err := json.Marshal(payload)
	if err == nil {
		endpoint, parseErr := url.Parse(config.DiscordWebhookURL)
		if parseErr != nil {
			err = errors.New("invalid Discord webhook URL")
		} else {
			query := endpoint.Query()
			query.Set("wait", "true")
			endpoint.RawQuery = query.Encode()
			err = m.postNotification(endpoint.String(), body, nil, true)
		}
	}
	logDelivery("Discord", serviceName, err)
	return err
}

func safeStatusPageURL(raw string) string {
	u, err := url.Parse(normalizeStatusPageURL(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return u.String()
}

func discordText(value string, limit int) string {
	value = strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "`", "\\`", "~", "\\~", "|", "\\|", "[", "\\[", "]", "\\]", "<", "\\<", ">", "\\>").Replace(value)
	return truncateDiscord(value, limit)
}

// Discord limits UTF-16 code units. Keep astral characters intact while respecting those limits.
func truncateDiscord(value string, limit int) string {
	used := 0
	for index, r := range value {
		size := 1
		if r > 0xffff {
			size = 2
		}
		if used+size > limit {
			return value[:index]
		}
		used += size
	}
	return value
}
