package checker

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"status/app/internal/models"
	"strings"
	"time"
)

// urlPattern matches http(s) URLs that may contain tokens/credentials.
var urlPattern = regexp.MustCompile(`https?://[^\s"'\)\]>]+`)

// requestURLPrefixPattern strips Go HTTP client prefixes such as:
// Get "http://host/path?apikey=secret": i/o timeout
var requestURLPrefixPattern = regexp.MustCompile(`(?i)^(?:Get|Head|Post|Put|Patch|Delete|Options)\s+"(?:https?://[^"]+|\[redacted-url\])":\s*`)

// sensitiveParams matches token-like key/value pairs outside URLs.
var sensitiveParams = regexp.MustCompile(`(?i)\b(?:x-plex-token|token|apikey|api_key|key|secret|password|auth|authorization)(?:=|:)\s*(?:Bearer\s+)?[^&\s"',]+`)

var bearerTokenPattern = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)

// dialTargetPattern removes network targets while preserving the actual error.
var dialTargetPattern = regexp.MustCompile(`(?i)\b(?:dial|connect)\s+(?:tcp|udp)\s+(?:\[[^\]]+\]:\d+|[^\s:]+:\d+|\[redacted-host\]):\s*`)

var lookupTargetPattern = regexp.MustCompile(`(?i)\blookup\s+[^\s:]+(?:\s+on\s+(?:\[[^\]]+\]:\d+|[^\s:]+:\d+))?:\s*`)
var protocolOnlyPattern = regexp.MustCompile(`(?i)^\s*(?:dial|connect)\s+(?:tcp|udp):\s*`)
var legacyRedactionPattern = regexp.MustCompile(`(?i)\[redacted(?:-[a-z]+)?\]`)
var whitespacePattern = regexp.MustCompile(`\s+`)

// SanitizeError strips URLs, tokens, and other sensitive data from error strings
// while keeping the useful failure text suitable for display.
func SanitizeError(errStr string) string {
	if errStr == "" {
		return ""
	}
	sanitized := strings.TrimSpace(errStr)

	// Common net/http errors include the full request URL before the useful
	// network error. Drop that prefix entirely so credentials cannot leak and
	// users do not see placeholder text.
	sanitized = requestURLPrefixPattern.ReplaceAllString(sanitized, "")

	// Keep the actual failure while removing private targets.
	sanitized = dialTargetPattern.ReplaceAllString(sanitized, "")
	sanitized = lookupTargetPattern.ReplaceAllString(sanitized, "")
	sanitized = protocolOnlyPattern.ReplaceAllString(sanitized, "")

	// Defense in depth for arbitrary strings and older stored messages.
	sanitized = sensitiveParams.ReplaceAllString(sanitized, "credential omitted")
	sanitized = bearerTokenPattern.ReplaceAllString(sanitized, "Bearer credential omitted")
	sanitized = urlPattern.ReplaceAllString(sanitized, "request target")
	sanitized = legacyRedactionPattern.ReplaceAllString(sanitized, "credential omitted")

	sanitized = whitespacePattern.ReplaceAllString(sanitized, " ")
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.Trim(sanitized, ":- ")
	if sanitized == "" {
		return "connection failed"
	}
	return sanitized
}

// isCloudMetadataIP returns true if the IP matches a known cloud metadata endpoint.
// These are blocked to prevent SSRF attacks from leaking cloud credentials.
func isCloudMetadataIP(ip net.IP) bool {
	metadataIPs := []string{
		"169.254.169.254/32", // AWS, GCP, Azure metadata
		"169.254.170.2/32",   // AWS ECS task credentials
		"169.254.170.23/32",  // AWS EKS pod credentials
		"100.100.100.200/32", // Alibaba Cloud metadata
		"fd00:ec2::254/128",  // AWS IMDSv2 IPv6
		"fd00:ec2::23/128",   // AWS EKS pod credentials IPv6
	}
	for _, cidr := range metadataIPs {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateURLTarget rejects invalid URLs and known cloud metadata targets.
// Resolved addresses are additionally checked when establishing the connection.
// Private RFC1918 IPs are allowed since monitoring internal services is the core use case.
func ValidateURLTarget(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("URL must include a host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "tcp" && parsed.Scheme != "dns" {
		return fmt.Errorf("unsupported URL scheme")
	}
	return validateTargetHost(host)
}

// CheckOptions defines parameters for a service health check.
type CheckOptions struct {
	URL         string
	Timeout     time.Duration
	ExpectedMin int
	ExpectedMax int
	CheckType   string // http, tcp, dns
	ServiceType string // plex, sonarr, etc. (used for token/header rules)
	APIToken    string
}

// HTTPCheck performs a basic HTTP/TCP/DNS check (backward-compatible wrapper).
func HTTPCheck(url string, timeout time.Duration, minOK, maxOK int) (ok bool, code int, ms *int, errStr string) {
	return Check(CheckOptions{
		URL:         url,
		Timeout:     timeout,
		ExpectedMin: minOK,
		ExpectedMax: maxOK,
	})
}

// Check performs a health check on a service with support for http/tcp/dns and API tokens.
func Check(opts CheckOptions) (ok bool, code int, ms *int, errStr string) {
	checkType := strings.ToLower(strings.TrimSpace(opts.CheckType))
	url := strings.TrimSpace(opts.URL)

	if opts.ExpectedMin == 0 {
		opts.ExpectedMin = 200
	}
	if opts.ExpectedMax == 0 {
		opts.ExpectedMax = 399
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}

	// Infer check type from URL if not explicitly set.
	if checkType == "" || checkType == "http" {
		if strings.HasPrefix(url, "tcp://") {
			checkType = "tcp"
		} else if strings.HasPrefix(url, "dns://") {
			checkType = "dns"
		} else {
			checkType = "http"
		}
	}

	switch checkType {
	case "always_up", "demo":
		d := 0
		ms = &d
		return true, http.StatusOK, ms, ""
	case "tcp":
		addr := strings.TrimPrefix(url, "tcp://")
		t0 := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		defer cancel()
		conn, err := dialTargetContext(ctx, "tcp", addr)
		d := int(time.Since(t0).Milliseconds())
		ms = &d
		if err != nil {
			safeErr := SanitizeError(err.Error())
			log.Printf("tcp check error err=%s", safeErr)
			return false, 0, nil, safeErr
		}
		_ = conn.Close()
		return true, 0, ms, ""
	case "dns":
		hostname := strings.TrimPrefix(url, "dns://")
		t0 := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		defer cancel()
		addrs, err := net.DefaultResolver.LookupHost(ctx, hostname)
		d := int(time.Since(t0).Milliseconds())
		ms = &d
		if err != nil {
			safeErr := SanitizeError(err.Error())
			log.Printf("dns check error err=%s", safeErr)
			return false, 0, ms, safeErr
		}
		if len(addrs) == 0 {
			log.Printf("dns check error hostname=%s no addresses returned", hostname)
			return false, 0, ms, "no addresses returned"
		}
		log.Printf("dns check success hostname=%s resolved to %v", hostname, addrs)
		return true, 0, ms, ""
	default:
		// HTTP/HTTPS — SSRF: block cloud metadata endpoints
		if err := ValidateURLTarget(url); err != nil {
			log.Printf("SSRF blocked: %v", err)
			return false, 0, nil, err.Error()
		}
		client := newCheckHTTPClient(opts.Timeout, opts.APIToken != "" || strings.Contains(url, "?") || strings.Contains(url, "@"))
		t0 := time.Now()

		testURL := url

		req, err := http.NewRequest("GET", testURL, nil)
		if err != nil {
			return false, 0, nil, "invalid URL"
		}
		req.Header.Set("User-Agent", "Servicarr/1.0")
		req.Header.Set("Accept", "application/json")

		if token := strings.TrimSpace(opts.APIToken); token != "" {
			switch strings.ToLower(opts.ServiceType) {
			case "plex":
				req.Header.Set("X-Plex-Token", token)
			case "sonarr", "radarr", "lidarr", "readarr", "prowlarr", "bazarr":
				req.Header.Set("X-Api-Key", token)
			case "overseerr", "jellyseerr":
				req.Header.Set("X-Api-Key", token)
			case "tautulli":
				query := req.URL.Query()
				query.Set("apikey", token)
				req.URL.RawQuery = query.Encode()
			case "jellyfin", "emby":
				req.Header.Set("X-Emby-Token", token)
			case "homeassistant":
				if strings.HasPrefix(strings.ToLower(token), "bearer ") {
					req.Header.Set("Authorization", token)
				} else {
					req.Header.Set("Authorization", "Bearer "+token)
				}
			default:
				req.Header.Set("X-Api-Key", token)
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}

		resp, err := client.Do(req)
		d := int(time.Since(t0).Milliseconds())
		ms = &d
		if err != nil {
			safeErr := SanitizeError(err.Error())
			log.Printf("http check error err=%s", safeErr)
			return false, 0, nil, safeErr
		}
		defer resp.Body.Close()
		ok = resp.StatusCode >= opts.ExpectedMin && resp.StatusCode <= opts.ExpectedMax
		return ok, resp.StatusCode, ms, ""
	}
}

// FindServiceByKey finds a service in the slice by its key
func FindServiceByKey(services []*models.Service, key string) *models.Service {
	for _, s := range services {
		if s.Key == key {
			return s
		}
	}
	return nil
}
