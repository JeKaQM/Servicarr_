package alerts

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"status/app/internal/database"
	"strings"
	"time"
)

// SendWebhook sends a JSON payload to a generic webhook URL with optional HMAC signing
func (m *Manager) SendWebhook(subject, statusType, serviceName, serviceKey, message string) {
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

	req, err := http.NewRequest("POST", m.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Webhook request failed", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Servicarr/1.0")

	// HMAC-SHA256 signature
	if m.config.WebhookSecret != "" {
		mac := hmac.New(sha256.New, []byte(m.config.WebhookSecret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Servicarr-Signature", "sha256="+sig)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, "Webhook notification failed", err.Error())
		return
	}
	defer resp.Body.Close()
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, "Webhook notification sent", fmt.Sprintf("url=%s, status=%d", m.config.WebhookURL, resp.StatusCode))
}
