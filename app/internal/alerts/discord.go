package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"strings"
	"time"
)

// SendDiscord sends a rich embed message via Discord webhook
func (m *Manager) SendDiscord(subject, statusType, serviceName, message, statusPageURL string) {
	colorMap := map[string]int{"down": 0xef4444, "degraded": 0xeab308, "up": 0x22c55e}
	color := colorMap[statusType]

	payload := map[string]interface{}{
		"username":   "Servicarr",
		"avatar_url": "https://raw.githubusercontent.com/JeKaQM/Servicarr_/main/web/static/images/icon.png",
		"embeds": []map[string]interface{}{
			{
				"title":       subject,
				"description": strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", "**"), "</strong>", "**"),
				"color":       color,
				"fields": []map[string]interface{}{
					{"name": "Service", "value": serviceName, "inline": true},
					{"name": "Status", "value": strings.ToUpper(statusType), "inline": true},
					{"name": "Time", "value": time.Now().Format(time.RFC1123), "inline": false},
				},
				"footer": map[string]string{"text": "Servicarr Status Monitor"},
			},
		},
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(m.config.DiscordWebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Discord notification failed", err.Error())
		return
	}
	defer resp.Body.Close()
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, "Discord notification sent", fmt.Sprintf("status=%d", resp.StatusCode))
}
