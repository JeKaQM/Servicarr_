package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"status/app/internal/database"
)

// Delivery has a deadline even when a custom transport is supplied by a test.
// Redirects are rejected so webhook signatures and payloads stay at their configured destination.
func (m *Manager) postNotification(endpoint string, body []byte, headers http.Header, discord bool) error {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" || (u.User != nil && (discord || u.Scheme != "https")) {
		return errors.New("invalid notification URL; use a complete HTTP or HTTPS URL")
	}
	var basicAuthUser, basicAuthPassword string
	if u.User != nil {
		basicAuthUser = u.User.Username()
		basicAuthPassword, _ = u.User.Password()
		// Redirects are rejected below. Move user-info to the standard header so
		// credentials cannot leak through the request URI or transport errors.
		u.User = nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	if m.httpClient != nil {
		*client = *m.httpClient
		client.Timeout = 10 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
		if err != nil {
			return errors.New("could not create notification request")
		}
		req.Header = headers.Clone()
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Servicarr/1.0")
		if basicAuthUser != "" || basicAuthPassword != "" {
			req.SetBasicAuth(basicAuthUser, basicAuthPassword)
		}
		resp, err := client.Do(req)
		if err != nil {
			// net/url errors include webhook tokens. Never return or log them.
			return errors.New("notification connection failed or timed out; check the destination and network")
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if readErr != nil {
				return errors.New("could not read notification delivery response")
			}
			return nil
		}
		if discord && resp.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			delay, valid := discordRetryDelay(resp.Header.Get("Retry-After"), responseBody)
			if valid {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
					continue
				case <-ctx.Done():
					timer.Stop()
					return errors.New("notification delivery deadline exceeded")
				}
			}
		}
		return fmt.Errorf("notification destination returned HTTP %d; check its configuration and delivery logs", resp.StatusCode)
	}
	return errors.New("notification delivery failed")
}

func discordRetryDelay(header string, body []byte) (time.Duration, bool) {
	seconds, err := strconv.ParseFloat(header, 64)
	if err != nil {
		var response struct {
			RetryAfter float64 `json:"retry_after"`
		}
		if json.Unmarshal(body, &response) != nil {
			return 0, false
		}
		seconds = response.RetryAfter
	}
	// Leave long rate limits for the operator to retry; do not block the delivery queue indefinitely.
	if !(seconds > 0 && seconds <= 5) {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

func logDelivery(channel, serviceName string, err error) {
	if database.DB == nil {
		return
	}
	if err != nil {
		_ = database.InsertLog(database.LogLevelError, "notification", serviceName, channel+" notification failed", err.Error())
		return
	}
	_ = database.InsertLog(database.LogLevelInfo, "notification", serviceName, channel+" notification sent", "Delivery accepted by destination")
}
