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
	"time"
)

// SecureHeaders adds security headers to responses
func SecureHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'none'; script-src 'self' https://cdn.jsdelivr.net https://static.cloudflareinsights.com; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' https://cdn.jsdelivr.net https://cloudflareinsights.com; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		next.ServeHTTP(w, r)
	})
}

// Rate limiter state
type rlEntry struct {
	tokens int
	last   time.Time
}

var rl = map[string]*rlEntry{}

// RateLimit implements token bucket rate limiting
func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)

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
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		e.tokens--
		next.ServeHTTP(w, r)
	})
}

// CheckIPBlock checks if an IP is blocked without rate limiting
func CheckIPBlock(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)

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
				blocked_at = CASE WHEN ? > 3 THEN datetime('now') ELSE NULL END,
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
