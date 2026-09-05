package security

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// SecureHeaders adds security headers to responses
func SecureHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'none'; script-src 'self' https://cdn.jsdelivr.net https://static.cloudflareinsights.com 'sha256-vlVlwb1evOfBxgr/T5qmkb6aludOm0Z8t44+tMBCnS0='; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data: https://raw.githubusercontent.com https://*.githubusercontent.com https://cdn.simpleicons.org https://cdn.jsdelivr.net; connect-src 'self' https://cdn.jsdelivr.net https://cloudflareinsights.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Limit request body to 70MB (covers multipart uploads + overhead)
		r.Body = http.MaxBytesReader(w, r.Body, 70<<20)
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		// Setup and login cannot require an authenticated CSRF token yet. Reject
		// browser cross-origin mutations there as well as on authenticated routes.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origin := r.Header.Get("Origin")
			if origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !strings.EqualFold(parsed.Host, r.Host) {
					http.Error(w, "cross-origin request forbidden", http.StatusForbidden)
					return
				}
			} else if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "cross-origin request forbidden", http.StatusForbidden)
				return
			}
		}
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

var trustedProxies = parseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))

// ConfigureTrustedProxies runs at startup after .env has been loaded, before
// serving requests. Configuration changes require restarting the application.
func ConfigureTrustedProxies(value string) {
	trustedProxies = parseTrustedProxies(value)
}

func parseTrustedProxies(value string) []netip.Prefix {
	var prefixes []netip.Prefix
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			if ip, ipErr := netip.ParseAddr(entry); ipErr == nil {
				ip = ip.Unmap()
				prefix = netip.PrefixFrom(ip, ip.BitLen())
			} else {
				log.Printf("Ignoring invalid TRUSTED_PROXIES entry %q", entry)
				continue
			}
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}

// ClientIP accepts forwarded addresses only from explicitly trusted proxies.
// Walk from the nearest hop to avoid trusting attacker-supplied XFF prefixes.
func ClientIP(r *http.Request) string {
	return clientIP(r, trustedProxies)
}

func clientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	isTrusted := func(ip netip.Addr) bool {
		for _, prefix := range trusted {
			if prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !isTrusted(peer) {
		return peer.String()
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	current := peer
	for i := len(parts) - 1; i >= 0; i-- {
		forwarded, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			// A malformed chain cannot establish a trustworthy client address.
			return peer.String()
		}
		current = forwarded.Unmap()
		if !isTrusted(current) {
			return current.String()
		}
	}
	return current.String()
}
