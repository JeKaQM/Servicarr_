package alerts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// SendWebhook sends a JSON payload to a generic webhook URL with optional HMAC signing
func (m *Manager) SendWebhook(subject, statusType, serviceName, serviceKey, message string) error {
	config := m.GetConfig()
	if config == nil || config.WebhookURL == "" {
		return errors.New("Webhook URL is not configured")
	}
	payload := map[string]interface{}{
		"event":        "status_change",
		"service_key":  serviceKey,
		"service_name": serviceName,
		"status":       statusType,
		"subject":      subject,
		"message":      strings.ReplaceAll(strings.ReplaceAll(message, "<strong>", ""), "</strong>", ""),
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}

	body, _ := json.Marshal(payload)

	headers := make(http.Header)

	// HMAC-SHA256 signature
	if config.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(config.WebhookSecret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		headers.Set("X-Servicarr-Signature", "sha256="+sig)
	}

	err := m.postNotification(config.WebhookURL, body, headers, false)
	logDelivery("Webhook", serviceName, err)
	return err
}
