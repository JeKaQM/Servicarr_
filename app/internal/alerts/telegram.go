package alerts

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"
)

// SendTelegram sends a message via Telegram Bot API.
func (m *Manager) SendTelegram(subject, statusType, serviceName, message string) error {
	config := m.GetConfig()
	if config == nil || config.TelegramBotToken == "" || config.TelegramChatID == "" {
		return errors.New("Telegram bot token or chat ID is not configured")
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
	err := m.postNotification(url, body, nil, false)
	logDelivery("Telegram", serviceName, err)
	return err
}
