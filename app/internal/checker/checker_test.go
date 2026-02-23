package checker

import (
	"net"
	"net/http"
	"net/http/httptest"
	"status/app/internal/models"
	"testing"
	"time"
)

// --- isCloudMetadataIP ---

func TestIsCloudMetadataIP_AWS(t *testing.T) {
	ip := net.ParseIP("169.254.169.254")
	if !isCloudMetadataIP(ip) {
		t.Error("expected 169.254.169.254 to be detected as cloud metadata IP")
	}
}

func TestIsCloudMetadataIP_AWSv6(t *testing.T) {
	ip := net.ParseIP("fd00:ec2::254")
	if !isCloudMetadataIP(ip) {
		t.Error("expected fd00:ec2::254 to be detected as cloud metadata IP")
	}
}

func TestIsCloudMetadataIP_Normal(t *testing.T) {
	ips := []string{"10.0.0.1", "192.168.1.1", "8.8.8.8", "172.16.0.1"}
	for _, s := range ips {
		ip := net.ParseIP(s)
		if isCloudMetadataIP(ip) {
			t.Errorf("%s should not be detected as cloud metadata IP", s)
		}
	}
}

// --- ValidateURLTarget ---

func TestValidateURLTarget_NormalURL(t *testing.T) {
	err := ValidateURLTarget("http://192.168.1.100:8080")
	if err != nil {
		t.Errorf("normal private IP should be allowed: %v", err)
	}
}

func TestValidateURLTarget_MetadataHostname(t *testing.T) {
	tests := []string{
		"http://metadata.google.internal/computeMetadata/v1",
		"http://METADATA.GOOGLE.INTERNAL/foo",
		"http://metadata/",
	}
	for _, u := range tests {
		if err := ValidateURLTarget(u); err == nil {
			t.Errorf("expected %q to be blocked", u)
		}
	}
}

func TestValidateURLTarget_EmptyHost(t *testing.T) {
	// Edge case: URL with no host
	err := ValidateURLTarget("/relative/path")
	if err != nil {
		t.Errorf("empty host should be allowed (non-routable): %v", err)
	}
}

func TestValidateURLTarget_InvalidURL(t *testing.T) {
	err := ValidateURLTarget("://bad")
	if err == nil {
		t.Error("invalid URL should return error")
	}
}

// --- Check with always_up / demo ---

func TestCheck_AlwaysUp(t *testing.T) {
	ok, code, ms, errStr := Check(CheckOptions{CheckType: "always_up"})
	if !ok {
		t.Error("always_up should always return ok")
	}
	if code != http.StatusOK {
		t.Errorf("expected 200, got %d", code)
	}
	if ms == nil || *ms != 0 {
		t.Error("always_up should have 0ms latency")
	}
	if errStr != "" {
		t.Errorf("expected no error string, got %q", errStr)
	}
}

func TestCheck_Demo(t *testing.T) {
	ok, code, _, _ := Check(CheckOptions{CheckType: "demo"})
	if !ok || code != http.StatusOK {
		t.Error("demo should behave like always_up")
	}
}

// --- Check HTTP against local test server ---

func TestCheck_HTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ok, code, ms, errStr := Check(CheckOptions{
		URL:         srv.URL,
		Timeout:     2 * time.Second,
		ExpectedMin: 200,
		ExpectedMax: 299,
	})
	if !ok {
		t.Errorf("expected ok, got code=%d err=%q", code, errStr)
	}
	if ms == nil {
		t.Error("expected non-nil latency")
	}
}

func TestCheck_HTTP_WrongStatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ok, code, _, _ := Check(CheckOptions{
		URL:         srv.URL,
		Timeout:     2 * time.Second,
		ExpectedMin: 200,
		ExpectedMax: 299,
	})
	if ok {
		t.Errorf("expected not ok for 500, got code=%d", code)
	}
	if code != 500 {
		t.Errorf("expected code=500, got %d", code)
	}
}

func TestCheck_HTTP_DefaultRange(t *testing.T) {
	// Test that defaults (200-399) work correctly
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently) // 301
	}))
	defer srv.Close()

	// Don't follow redirects
	ok, code, _, _ := Check(CheckOptions{
		URL:     srv.URL,
		Timeout: 2 * time.Second,
		// ExpectedMin/Max default to 200-399
	})
	if !ok {
		t.Errorf("301 should be ok with default range, got code=%d", code)
	}
}

func TestCheck_HTTP_WithAPIToken(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Api-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	Check(CheckOptions{
		URL:         srv.URL,
		Timeout:     2 * time.Second,
		APIToken:    "test-token-123",
		ServiceType: "sonarr",
	})

	if gotHeader != "test-token-123" {
		t.Errorf("expected X-Api-Key header, got %q", gotHeader)
	}
}

func TestCheck_HTTP_PlexToken(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Plex-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	Check(CheckOptions{
		URL:         srv.URL,
		Timeout:     2 * time.Second,
		APIToken:    "plex-token-abc",
		ServiceType: "plex",
	})

	if gotHeader != "plex-token-abc" {
		t.Errorf("expected X-Plex-Token header, got %q", gotHeader)
	}
}

func TestCheck_TCP_Success(t *testing.T) {
	// Start a TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start TCP listener: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	ok, _, ms, errStr := Check(CheckOptions{
		URL:       "tcp://" + ln.Addr().String(),
		Timeout:   2 * time.Second,
		CheckType: "tcp",
	})
	if !ok {
		t.Errorf("TCP check should succeed, err=%q", errStr)
	}
	if ms == nil {
		t.Error("expected non-nil latency")
	}
}

func TestCheck_TCP_Failure(t *testing.T) {
	ok, _, _, errStr := Check(CheckOptions{
		URL:       "tcp://127.0.0.1:1", // unlikely to be open
		Timeout:   1 * time.Second,
		CheckType: "tcp",
	})
	if ok {
		t.Error("TCP check to closed port should fail")
	}
	if errStr == "" {
		t.Error("expected error string for TCP failure")
	}
}

func TestCheck_InferTCPFromURL(t *testing.T) {
	// When checkType is empty but URL has tcp:// prefix, should infer TCP
	ok, _, _, errStr := Check(CheckOptions{
		URL:     "tcp://127.0.0.1:1",
		Timeout: 1 * time.Second,
	})
	// Should fail because port 1 is closed, but the point is it doesn't panic
	_ = ok
	_ = errStr
}

// --- FindServiceByKey ---

func TestFindServiceByKey_Found(t *testing.T) {
	services := []*models.Service{
		{Key: "svc1", Label: "Service 1"},
		{Key: "svc2", Label: "Service 2"},
		{Key: "svc3", Label: "Service 3"},
	}

	found := FindServiceByKey(services, "svc2")
	if found == nil {
		t.Fatal("expected to find svc2")
	}
	if found.Label != "Service 2" {
		t.Errorf("expected 'Service 2', got %q", found.Label)
	}
}

func TestFindServiceByKey_NotFound(t *testing.T) {
	services := []*models.Service{
		{Key: "svc1", Label: "Service 1"},
	}

	found := FindServiceByKey(services, "nonexistent")
	if found != nil {
		t.Error("expected nil for nonexistent key")
	}
}

func TestFindServiceByKey_EmptySlice(t *testing.T) {
	found := FindServiceByKey(nil, "any")
	if found != nil {
		t.Error("expected nil for empty slice")
	}
}
