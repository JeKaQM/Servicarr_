package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- SecureHeaders ---

func TestSecureHeaders_SetsAllHeaders(t *testing.T) {
	handler := SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	expected := map[string]string{
		"Content-Security-Policy":   "",
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Permissions-Policy":        "geolocation=(), microphone=(), camera=()",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	}

	for header, expectedVal := range expected {
		got := rr.Header().Get(header)
		if got == "" {
			t.Errorf("expected header %q to be set", header)
		}
		// For CSP we just check it's non-empty; for others, check exact value
		if expectedVal != "" && got != expectedVal {
			t.Errorf("header %q: expected %q, got %q", header, expectedVal, got)
		}
	}
}

func TestSecureHeaders_CSP_ContentsCheck(t *testing.T) {
	handler := SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	csp := rr.Header().Get("Content-Security-Policy")
	// Check key directives
	checks := []string{
		"default-src 'none'",
		"script-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}
	for _, c := range checks {
		if !containsStr(csp, c) {
			t.Errorf("CSP missing directive %q", c)
		}
	}
}

func TestSecureHeaders_CallsNext(t *testing.T) {
	called := false
	handler := SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler was not called")
	}
}

// --- ClientIP ---

func TestClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")

	ip := ClientIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("expected first IP from XFF, got %q", ip)
	}
}

func TestClientIP_XFF_SingleIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.1")

	ip := ClientIP(req)
	if ip != "198.51.100.1" {
		t.Errorf("expected %q, got %q", "198.51.100.1", ip)
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"

	ip := ClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("expected '192.168.1.100', got %q", ip)
	}
}

func TestClientIP_RemoteAddr_NoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100"

	ip := ClientIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("expected '192.168.1.100', got %q", ip)
	}
}

func TestClientIP_XFF_TakePrecedence(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.RemoteAddr = "192.168.1.100:54321"

	ip := ClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("XFF should take precedence, got %q", ip)
	}
}

func TestClientIP_XFF_WhitespaceHandling(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "  203.0.113.1  , 10.0.0.1")

	ip := ClientIP(req)
	if ip != "203.0.113.1" {
		t.Errorf("expected trimmed IP, got %q", ip)
	}
}

// helper
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
