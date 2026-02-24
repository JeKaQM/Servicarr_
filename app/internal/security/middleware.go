package security

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
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

func init() {
	// Cleanup stale rate limiter entries every 5 minutes
	go func() {
		for range time.Tick(5 * time.Minute) {
			rlMu.Lock()
			now := time.Now()
			for k, e := range rl {
				if now.Sub(e.last) > 10*time.Minute {
					delete(rl, k)
				}
			}
			rlMu.Unlock()
		}
	}()
}

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
			block := &blockInfoInternal{
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
