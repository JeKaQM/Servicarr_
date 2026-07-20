package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"status/app/internal/database"
	"strings"
	"time"
)

// SendTelegram sends a message via Telegram Bot API.
func (m *Manager) SendTelegram(subject, statusType, serviceName, message string) {
	config := m.GetConfig()
	if config == nil || config.TelegramBotToken == "" || config.TelegramChatID == "" {
		return
	}
	plainMsg := strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", "<b>"), "</strong>", "</b>")
	text := fmt.Sprintf("<b>%s</b>\n\n%s\n\nTime: %s", html.EscapeString(subject), plainMsg, time.Now().Format(time.RFC1123))

	payload := map[string]interface{}{
		"chat_id":    config.TelegramChatID,
		"text":       text,
		"parse_mode": "HTML",
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.TelegramBotToken)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Telegram notification failed", err.Error())
		return
	}
	defer resp.Body.Close()
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, "Telegram notification sent", fmt.Sprintf("status=%d", resp.StatusCode))
}
