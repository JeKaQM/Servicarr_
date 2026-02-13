package checker

import (
	"context"
	"log"
	"net"
	"net/http"
	"status/app/internal/models"
	"strings"
	"time"
)

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
		conn, err := net.DialTimeout("tcp", addr, opts.Timeout)
		d := int(time.Since(t0).Milliseconds())
		ms = &d
		if err != nil {
			log.Printf("tcp check error addr=%s err=%v", addr, err)
			return false, 0, nil, err.Error()
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
			log.Printf("dns check error hostname=%s err=%v", hostname, err)
			return false, 0, ms, err.Error()
		}
		if len(addrs) == 0 {
			log.Printf("dns check error hostname=%s no addresses returned", hostname)
			return false, 0, ms, "no addresses returned"
		}
		log.Printf("dns check success hostname=%s resolved to %v", hostname, addrs)
		return true, 0, ms, ""
	default:
		// HTTP/HTTPS
		client := &http.Client{Timeout: opts.Timeout}
		t0 := time.Now()

		testURL := url
		if opts.APIToken != "" && strings.ToLower(opts.ServiceType) == "plex" {
			if strings.Contains(testURL, "?") {
				testURL += "&X-Plex-Token=" + opts.APIToken
			} else {
				testURL += "?X-Plex-Token=" + opts.APIToken
			}
		}

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
				if strings.Contains(req.URL.String(), "?") {
					req.URL.RawQuery += "&apikey=" + token
				} else {
					req.URL.RawQuery = "apikey=" + token
				}
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
			log.Printf("http check error url=%s err=%v", url, err)
			return false, 0, nil, err.Error()
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
