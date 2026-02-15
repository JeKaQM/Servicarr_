package security

import (
	"database/sql"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"status/app/internal/database"
	"status/app/internal/models"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SecureHeaders adds security headers to responses
func SecureHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'none'; script-src 'self' https://cdn.jsdelivr.net https://static.cloudflareinsights.com; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data: https://raw.githubusercontent.com https://*.githubusercontent.com https://cdn.simpleicons.org https://cdn.jsdelivr.net; connect-src 'self' https://cdn.jsdelivr.net https://cloudflareinsights.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Limit request body to 35MB (covers multipart uploads + overhead)
		r.Body = http.MaxBytesReader(w, r.Body, 35<<20)
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// Rate limiter state
type rlEntry struct {
	tokens int
	last   time.Time
}

var (
	rl   = map[string]*rlEntry{}
	rlMu sync.Mutex
)

// RateLimit implements token bucket rate limiting
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)

		// Skip rate limiting for whitelisted IPs
		if IsWhitelisted(ip) {
			next.ServeHTTP(w, r)
			return
		}

		// Check if IP is blocked
		if block, err := GetIPBlock(ip); block != nil {
			// For API requests, return JSON
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":      "access_blocked",
					"message":    "Your access has been temporarily blocked due to excessive failed login attempts",
					"expires_at": block.ExpiresAt,
				})
				return
			}

			// For web requests, show blocked page
			serveBlockedPage(w, r, block)
			return
		} else if err != nil {
			log.Printf("error checking IP block: %v", err)
		}

		rlMu.Lock()
		e := rl[ip]
		now := time.Now()
		if e == nil {
			e = &rlEntry{tokens: 10, last: now}
			rl[ip] = e
		}
		refill := int(now.Sub(e.last).Seconds())
		if refill > 0 {
			e.tokens += refill
			if e.tokens > 10 {
				e.tokens = 10
			}
			e.last = now
		}
		if e.tokens <= 0 {
			rlMu.Unlock()
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		e.tokens--
		rlMu.Unlock()
		next.ServeHTTP(w, r)
	})
}

// CheckIPBlock checks if an IP is blocked without rate limiting
func CheckIPBlock(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)

		// Check if IP is blacklisted (permanent ban or login block)
		if blacklisted, permanent := IsBlacklisted(ip); blacklisted {
			block := &models.BlockInfo{
				IP:        ip,
				Attempts:  0,
				ExpiresAt: "Never",
			}
			if permanent {
				// Permanent ban - always show blocked page
				if strings.HasPrefix(r.URL.Path, "/api/") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"error":   "access_blocked",
						"message": "Your IP has been permanently blocked",
					})
					return
				}
				serveBlockedPage(w, r, block)
				return
			}
			// Non-permanent blacklist - block login attempts but allow viewing
			if r.URL.Path == "/api/login" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "access_blocked",
					"message": "Your IP has been blocked from logging in",
				})
				return
			}
		}

		// Check if IP is blocked (temporary from failed logins)
		if block, err := GetIPBlock(ip); block != nil {
			// For API requests, return JSON
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":      "access_blocked",
					"message":    "Your access has been temporarily blocked due to excessive failed login attempts",
					"expires_at": block.ExpiresAt,
				})
				return
			}

			// For web requests, show blocked page
			serveBlockedPage(w, r, block)
			return
		} else if err != nil {
			log.Printf("error checking IP block: %v", err)
		}

		next.ServeHTTP(w, r)
	})
}

// ClientIP extracts the client IP from the request
func ClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// GetIPBlock retrieves an IP block record if it exists and is active
func GetIPBlock(ip string) (*models.BlockInfo, error) {
	var block models.BlockInfo
	err := database.DB.QueryRow(`SELECT ip_address, attempts, expires_at 
		FROM ip_blocks 
		WHERE ip_address = ? AND blocked_at IS NOT NULL AND expires_at > datetime('now')`, ip).
		Scan(&block.IP, &block.Attempts, &block.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &block, nil
}

// IsIPBlocked checks if an IP is currently blocked
func IsIPBlocked(ip string) bool {
	var blockedAt sql.NullString
	err := database.DB.QueryRow(`SELECT blocked_at FROM ip_blocks 
		WHERE ip_address = ? AND expires_at > datetime('now')`, ip).Scan(&blockedAt)

	return err == nil && blockedAt.Valid // Only blocked if blocked_at is set
}

// LogFailedLoginAttempt records a failed login attempt and blocks if threshold reached
func LogFailedLoginAttempt(ip string) {
	// First, delete any expired blocks for this IP
	_, _ = database.DB.Exec(`DELETE FROM ip_blocks WHERE ip_address = ? AND expires_at <= datetime('now')`, ip)

	// Check if there's an existing non-expired record
	var attempts int
	var blockedAt sql.NullString
	err := database.DB.QueryRow(`SELECT attempts, blocked_at FROM ip_blocks 
		WHERE ip_address = ?`, ip).Scan(&attempts, &blockedAt)

	if err == nil {
		newAttempts := attempts + 1
		// Update existing record, block after 3 attempts
		_, _ = database.DB.Exec(`UPDATE ip_blocks 
			SET attempts = ?,
				blocked_at = CASE WHEN ? >= 3 THEN datetime('now') ELSE NULL END,
				expires_at = datetime('now', '+24 hours'),
				reason = 'Failed login attempts'
			WHERE ip_address = ?`, newAttempts, newAttempts, ip)
	} else {
		// Create completely new record
		_, _ = database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
			VALUES (?, NULL, 1, datetime('now', '+24 hours'), 'Failed login attempts')`,
			ip)
	}
}

// ClearIPBlock removes an IP block
func ClearIPBlock(ip string) error {
	_, err := database.DB.Exec(`DELETE FROM ip_blocks WHERE ip_address = ?`, ip)
	return err
}

// ClearAllIPBlocks removes all IP blocks
func ClearAllIPBlocks() (int64, error) {
	result, err := database.DB.Exec("DELETE FROM ip_blocks")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListBlockedIPs returns all currently blocked IPs
func ListBlockedIPs() ([]map[string]interface{}, error) {
	rows, err := database.DB.Query(`
		SELECT ip_address, blocked_at, attempts, expires_at, reason 
		FROM ip_blocks 
		WHERE expires_at > datetime('now')
		ORDER BY blocked_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		var ip, expiresAt string
		var blockedAt, reason sql.NullString
		var attempts int
		if err := rows.Scan(&ip, &blockedAt, &attempts, &expiresAt, &reason); err != nil {
			continue
		}

		blockedAtStr := blockedAt.String
		if !blockedAt.Valid {
			blockedAtStr = expiresAt // fallback to expires_at if blocked_at is NULL
		}

		reasonStr := reason.String
		if !reason.Valid {
			reasonStr = "Too many failed login attempts"
		}

		results = append(results, map[string]interface{}{
			"ip":         ip,
			"blocked_at": blockedAtStr,
			"attempts":   attempts,
			"expires_at": expiresAt,
			"reason":     reasonStr,
		})
	}
	return results, nil
}

func serveBlockedPage(w http.ResponseWriter, r *http.Request, block *models.BlockInfo) {
	// Set block info in URL parameters
	q := url.Values{}
	q.Set("ip", block.IP)
	q.Set("attempts", strconv.Itoa(block.Attempts))
	q.Set("expires", block.ExpiresAt)

	// Redirect to blocked page with params
	http.Redirect(w, r, "/static/blocked.html?"+q.Encode(), http.StatusSeeOther)
}

// IsWhitelisted checks if an IP is in the whitelist
func IsWhitelisted(ip string) bool {
	var count int
	err := database.DB.QueryRow(`SELECT COUNT(*) FROM ip_whitelist WHERE ip_address = ?`, ip).Scan(&count)
	return err == nil && count > 0
}

// IsBlacklisted checks if an IP is in the blacklist
func IsBlacklisted(ip string) (bool, bool) {
	var permanent int
	err := database.DB.QueryRow(`SELECT permanent FROM ip_blacklist WHERE ip_address = ?`, ip).Scan(&permanent)
	if err != nil {
		return false, false
	}
	return true, permanent == 1
}

// AddToWhitelist adds an IP to the whitelist
func AddToWhitelist(ip, note string) error {
	_, err := database.DB.Exec(`
		INSERT OR REPLACE INTO ip_whitelist (ip_address, note, created_at) 
		VALUES (?, ?, datetime('now'))
	`, ip, note)
	return err
}

// RemoveFromWhitelist removes an IP from the whitelist
func RemoveFromWhitelist(ip string) error {
	_, err := database.DB.Exec(`DELETE FROM ip_whitelist WHERE ip_address = ?`, ip)
	return err
}

// AddToBlacklist adds an IP to the blacklist
func AddToBlacklist(ip, note string, permanent bool) error {
	perm := 0
	if permanent {
		perm = 1
	}
	_, err := database.DB.Exec(`
		INSERT OR REPLACE INTO ip_blacklist (ip_address, permanent, note, created_at) 
		VALUES (?, ?, ?, datetime('now'))
	`, ip, perm, note)
	return err
}

// RemoveFromBlacklist removes an IP from the blacklist
func RemoveFromBlacklist(ip string) error {
	_, err := database.DB.Exec(`DELETE FROM ip_blacklist WHERE ip_address = ?`, ip)
	return err
}

// ListWhitelist returns all whitelisted IPs
func ListWhitelist() ([]map[string]interface{}, error) {
	rows, err := database.DB.Query(`SELECT ip_address, note, created_at FROM ip_whitelist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ip string
		var note sql.NullString
		var createdAt string
		if err := rows.Scan(&ip, &note, &createdAt); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"ip":         ip,
			"note":       note.String,
			"created_at": createdAt,
		})
	}
	return results, nil
}

// ListBlacklist returns all blacklisted IPs
func ListBlacklist() ([]map[string]interface{}, error) {
	rows, err := database.DB.Query(`SELECT ip_address, permanent, note, created_at FROM ip_blacklist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ip string
		var permanent int
		var note sql.NullString
		var createdAt string
		if err := rows.Scan(&ip, &permanent, &note, &createdAt); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"ip":         ip,
			"permanent":  permanent == 1,
			"note":       note.String,
			"created_at": createdAt,
		})
	}
	return results, nil
}
