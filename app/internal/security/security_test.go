package security

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"status/app/internal/database"
	"testing"
	"time"
)

// initTestDB creates an in-memory SQLite database with full schema for security tests
func initTestDB(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
}

// resetRateLimiter clears the in-memory rate limiter map between tests
func resetRateLimiter() {
	rlMu.Lock()
	rl = map[string]*rlEntry{}
	rlMu.Unlock()
}

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

// ======================== IP BLOCKS (DB-dependent) ========================

func TestGetIPBlock_NoBlock(t *testing.T) {
	initTestDB(t)
	block, err := GetIPBlock("1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block != nil {
		t.Error("expected nil block for unknown IP")
	}
}

func TestGetIPBlock_ActiveBlock(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('1.2.3.4', datetime('now'), 3, datetime('now', '+1 hour'), 'test')`)

	block, err := GetIPBlock("1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block == nil {
		t.Fatal("expected non-nil block")
	}
	if block.IP != "1.2.3.4" {
		t.Errorf("expected IP 1.2.3.4, got %s", block.IP)
	}
	if block.Attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", block.Attempts)
	}
}

func TestGetIPBlock_ExpiredBlock(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('1.2.3.4', datetime('now', '-2 hours'), 3, datetime('now', '-1 hour'), 'expired')`)

	block, err := GetIPBlock("1.2.3.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block != nil {
		t.Error("expected nil for expired block")
	}
}

func TestIsIPBlocked_NotBlocked(t *testing.T) {
	initTestDB(t)
	if IsIPBlocked("5.6.7.8") {
		t.Error("expected not blocked for unknown IP")
	}
}

func TestIsIPBlocked_Blocked(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('5.6.7.8', datetime('now'), 5, datetime('now', '+1 hour'), 'test')`)

	if !IsIPBlocked("5.6.7.8") {
		t.Error("expected IP to be blocked")
	}
}

func TestIsIPBlocked_Expired(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('5.6.7.8', datetime('now', '-2 hours'), 5, datetime('now', '-1 hour'), 'expired')`)

	if IsIPBlocked("5.6.7.8") {
		t.Error("expected expired block to return false")
	}
}

func TestLogFailedLoginAttempt_FirstAttempt(t *testing.T) {
	initTestDB(t)
	LogFailedLoginAttempt("10.0.0.1")

	var attempts int
	database.DB.QueryRow(`SELECT attempts FROM ip_blocks WHERE ip_address = '10.0.0.1'`).Scan(&attempts)
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
	// Should NOT be blocked after 1 attempt
	if IsIPBlocked("10.0.0.1") {
		t.Error("should not be blocked after 1 attempt")
	}
}

func TestLogFailedLoginAttempt_SecondAttempt(t *testing.T) {
	initTestDB(t)
	LogFailedLoginAttempt("10.0.0.2")
	LogFailedLoginAttempt("10.0.0.2")

	var attempts int
	database.DB.QueryRow(`SELECT attempts FROM ip_blocks WHERE ip_address = '10.0.0.2'`).Scan(&attempts)
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
	// Should NOT be blocked after 2 attempts
	if IsIPBlocked("10.0.0.2") {
		t.Error("should not be blocked after 2 attempts")
	}
}

func TestLogFailedLoginAttempt_ThirdAttemptBlocks(t *testing.T) {
	initTestDB(t)
	LogFailedLoginAttempt("10.0.0.3")
	LogFailedLoginAttempt("10.0.0.3")
	LogFailedLoginAttempt("10.0.0.3")

	var attempts int
	database.DB.QueryRow(`SELECT attempts FROM ip_blocks WHERE ip_address = '10.0.0.3'`).Scan(&attempts)
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	// Should be blocked after 3 attempts
	if !IsIPBlocked("10.0.0.3") {
		t.Error("should be blocked after 3 failed attempts")
	}
}

func TestClearIPBlock(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('9.9.9.9', datetime('now'), 3, datetime('now', '+1 hour'), 'test')`)

	err := ClearIPBlock("9.9.9.9")
	if err != nil {
		t.Fatalf("ClearIPBlock error: %v", err)
	}

	if IsIPBlocked("9.9.9.9") {
		t.Error("IP should no longer be blocked after clearing")
	}
}

func TestClearIPBlock_NonExistent(t *testing.T) {
	initTestDB(t)
	err := ClearIPBlock("nonexistent")
	if err != nil {
		t.Errorf("ClearIPBlock on non-existent should not error, got: %v", err)
	}
}

func TestClearAllIPBlocks(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('1.1.1.1', datetime('now'), 3, datetime('now', '+1 hour'), 'a')`)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('2.2.2.2', datetime('now'), 3, datetime('now', '+1 hour'), 'b')`)

	affected, err := ClearAllIPBlocks()
	if err != nil {
		t.Fatalf("ClearAllIPBlocks error: %v", err)
	}
	if affected != 2 {
		t.Errorf("expected 2 affected rows, got %d", affected)
	}

	var count int
	database.DB.QueryRow(`SELECT COUNT(*) FROM ip_blocks`).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 blocks remaining, got %d", count)
	}
}

func TestClearAllIPBlocks_Empty(t *testing.T) {
	initTestDB(t)
	affected, err := ClearAllIPBlocks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if affected != 0 {
		t.Errorf("expected 0 affected rows, got %d", affected)
	}
}

func TestListBlockedIPs_Empty(t *testing.T) {
	initTestDB(t)
	list, err := ListBlockedIPs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestListBlockedIPs_ReturnsActive(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('3.3.3.3', datetime('now'), 3, datetime('now', '+1 hour'), 'active block')`)
	// Also add an expired block
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('4.4.4.4', datetime('now', '-2 hours'), 5, datetime('now', '-1 hour'), 'expired')`)

	list, err := ListBlockedIPs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 active block, got %d", len(list))
	}
	if list[0]["ip"] != "3.3.3.3" {
		t.Errorf("expected IP 3.3.3.3, got %v", list[0]["ip"])
	}
	if list[0]["reason"] != "active block" {
		t.Errorf("expected reason 'active block', got %v", list[0]["reason"])
	}
}

func TestListBlockedIPs_NullBlockedAtFallback(t *testing.T) {
	initTestDB(t)
	// Insert with NULL blocked_at (pre-threshold) — reason must be non-null per schema
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('7.7.7.7', NULL, 1, datetime('now', '+1 hour'), 'Failed login attempts')`)

	list, err := ListBlockedIPs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	// blocked_at should fallback to expires_at when blocked_at is NULL
	if list[0]["blocked_at"] == "" {
		t.Error("expected blocked_at to have fallback value from expires_at")
	}
}

// ======================== IP LISTS (WHITELIST / BLACKLIST) ========================

func TestIsWhitelisted_NotInList(t *testing.T) {
	initTestDB(t)
	if IsWhitelisted("192.168.1.1") {
		t.Error("expected false for unlisted IP")
	}
}

func TestIsWhitelisted_InList(t *testing.T) {
	initTestDB(t)
	AddToWhitelist("192.168.1.1", "trusted")
	if !IsWhitelisted("192.168.1.1") {
		t.Error("expected true for whitelisted IP")
	}
}

func TestIsBlacklisted_NotInList(t *testing.T) {
	initTestDB(t)
	blacklisted, permanent := IsBlacklisted("192.168.1.1")
	if blacklisted {
		t.Error("expected false for unlisted IP")
	}
	if permanent {
		t.Error("expected permanent false for unlisted IP")
	}
}

func TestIsBlacklisted_NonPermanent(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("192.168.1.2", "temp ban", false)
	blacklisted, permanent := IsBlacklisted("192.168.1.2")
	if !blacklisted {
		t.Error("expected blacklisted")
	}
	if permanent {
		t.Error("expected non-permanent")
	}
}

func TestIsBlacklisted_Permanent(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("192.168.1.3", "perm ban", true)
	blacklisted, permanent := IsBlacklisted("192.168.1.3")
	if !blacklisted {
		t.Error("expected blacklisted")
	}
	if !permanent {
		t.Error("expected permanent")
	}
}

func TestAddToWhitelist(t *testing.T) {
	initTestDB(t)
	err := AddToWhitelist("10.10.10.10", "test note")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !IsWhitelisted("10.10.10.10") {
		t.Error("IP should be whitelisted after adding")
	}
}

func TestAddToWhitelist_Replace(t *testing.T) {
	initTestDB(t)
	AddToWhitelist("10.10.10.10", "first note")
	err := AddToWhitelist("10.10.10.10", "updated note")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, _ := ListWhitelist()
	if len(list) != 1 {
		t.Fatalf("expected 1 entry after replace, got %d", len(list))
	}
	if list[0]["note"] != "updated note" {
		t.Errorf("expected updated note, got %v", list[0]["note"])
	}
}

func TestRemoveFromWhitelist(t *testing.T) {
	initTestDB(t)
	AddToWhitelist("10.10.10.10", "test")
	err := RemoveFromWhitelist("10.10.10.10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if IsWhitelisted("10.10.10.10") {
		t.Error("IP should not be whitelisted after removal")
	}
}

func TestRemoveFromWhitelist_NonExistent(t *testing.T) {
	initTestDB(t)
	err := RemoveFromWhitelist("nonexistent")
	if err != nil {
		t.Errorf("removing non-existent should not error: %v", err)
	}
}

func TestAddToBlacklist(t *testing.T) {
	initTestDB(t)
	err := AddToBlacklist("11.11.11.11", "bad actor", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blacklisted, perm := IsBlacklisted("11.11.11.11")
	if !blacklisted {
		t.Error("IP should be blacklisted")
	}
	if !perm {
		t.Error("should be permanent")
	}
}

func TestAddToBlacklist_Replace(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("11.11.11.11", "temp", false)
	AddToBlacklist("11.11.11.11", "now permanent", true)
	_, perm := IsBlacklisted("11.11.11.11")
	if !perm {
		t.Error("should be permanent after replace")
	}
}

func TestRemoveFromBlacklist(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("11.11.11.11", "test", true)
	err := RemoveFromBlacklist("11.11.11.11")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	blacklisted, _ := IsBlacklisted("11.11.11.11")
	if blacklisted {
		t.Error("IP should not be blacklisted after removal")
	}
}

func TestRemoveFromBlacklist_NonExistent(t *testing.T) {
	initTestDB(t)
	err := RemoveFromBlacklist("nonexistent")
	if err != nil {
		t.Errorf("removing non-existent should not error: %v", err)
	}
}

func TestListWhitelist_Empty(t *testing.T) {
	initTestDB(t)
	list, err := ListWhitelist()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if list != nil && len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestListWhitelist_WithEntries(t *testing.T) {
	initTestDB(t)
	AddToWhitelist("10.0.0.1", "first")
	AddToWhitelist("10.0.0.2", "second")

	list, err := ListWhitelist()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
}

func TestListBlacklist_Empty(t *testing.T) {
	initTestDB(t)
	list, err := ListBlacklist()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if list != nil && len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestListBlacklist_WithEntries(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("20.0.0.1", "temp", false)
	AddToBlacklist("20.0.0.2", "perm", true)

	list, err := ListBlacklist()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
	// Check permanent field is a bool
	for _, entry := range list {
		if _, ok := entry["permanent"].(bool); !ok {
			t.Errorf("permanent should be bool, got %T", entry["permanent"])
		}
	}
}

func TestListBlacklist_PermanentField(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("30.0.0.1", "perm", true)
	AddToBlacklist("30.0.0.2", "temp", false)

	list, err := ListBlacklist()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, entry := range list {
		ip := entry["ip"].(string)
		perm := entry["permanent"].(bool)
		if ip == "30.0.0.1" && !perm {
			t.Error("30.0.0.1 should be permanent")
		}
		if ip == "30.0.0.2" && perm {
			t.Error("30.0.0.2 should not be permanent")
		}
	}
}

// ======================== MIDDLEWARE: RateLimit ========================

func TestRateLimit_AllowsRequests(t *testing.T) {
	initTestDB(t)
	resetRateLimiter()

	called := false
	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "100.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler should have been called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimit_ExhaustsTokens(t *testing.T) {
	initTestDB(t)
	resetRateLimiter()

	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Make 11 requests rapidly (bucket starts at 10, -1 per request)
	var lastCode int
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "101.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		lastCode = rr.Code
	}

	if lastCode != http.StatusTooManyRequests {
		t.Errorf("expected 429 after exhausting tokens, got %d", lastCode)
	}
}

func TestRateLimit_SkipsWhitelistedIPs(t *testing.T) {
	initTestDB(t)
	resetRateLimiter()

	AddToWhitelist("102.0.0.1", "trusted")

	callCount := 0
	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))

	// Make 15 requests — should all pass for whitelisted IP
	for i := 0; i < 15; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "102.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200 for whitelisted IP, got %d", i, rr.Code)
		}
	}
	if callCount != 15 {
		t.Errorf("expected 15 calls for whitelisted IP, got %d", callCount)
	}
}

func TestRateLimit_BlockedIP_API(t *testing.T) {
	initTestDB(t)
	resetRateLimiter()

	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('103.0.0.1', datetime('now'), 3, datetime('now', '+1 hour'), 'blocked')`)

	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("blocked IP handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "103.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for blocked IP API request, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "access_blocked" {
		t.Errorf("expected error=access_blocked, got %v", resp["error"])
	}
}

func TestRateLimit_BlockedIP_Web(t *testing.T) {
	initTestDB(t)
	resetRateLimiter()

	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('104.0.0.1', datetime('now'), 3, datetime('now', '+1 hour'), 'blocked')`)

	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("blocked IP handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.RemoteAddr = "104.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should redirect to blocked page
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for blocked IP web request, got %d", rr.Code)
	}
	location := rr.Header().Get("Location")
	if !containsStr(location, "/static/blocked.html") {
		t.Errorf("expected redirect to blocked.html, got %s", location)
	}
}

func TestRateLimit_TokenRefill(t *testing.T) {
	initTestDB(t)
	resetRateLimiter()

	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "105.0.0.1:1234"

	// Exhaust all tokens
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// Manually set the last time back 5 seconds to simulate refill
	rlMu.Lock()
	if e, ok := rl["105.0.0.1"]; ok {
		e.last = time.Now().Add(-5 * time.Second)
	}
	rlMu.Unlock()

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 after token refill, got %d", rr.Code)
	}
}

// ======================== MIDDLEWARE: CheckIPBlock ========================

func TestCheckIPBlock_NoBlock(t *testing.T) {
	initTestDB(t)

	called := false
	handler := CheckIPBlock(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "200.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler should have been called for non-blocked IP")
	}
}

func TestCheckIPBlock_PermanentBlacklist_API(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("201.0.0.1", "perm banned", true)

	handler := CheckIPBlock(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call next for permanently blacklisted IP")
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "201.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for perm blacklisted API, got %d", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] != "access_blocked" {
		t.Errorf("expected error=access_blocked, got %v", resp["error"])
	}
}

func TestCheckIPBlock_PermanentBlacklist_Web(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("202.0.0.1", "perm banned", true)

	handler := CheckIPBlock(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call next for permanently blacklisted IP")
	}))

	req := httptest.NewRequest("GET", "/dashboard", nil)
	req.RemoteAddr = "202.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for perm blacklisted web, got %d", rr.Code)
	}
}

func TestCheckIPBlock_NonPermanentBlacklist_BlocksLogin(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("203.0.0.1", "temp ban", false)

	handler := CheckIPBlock(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call next for login from non-perm blacklisted IP")
	}))

	req := httptest.NewRequest("POST", "/api/login", nil)
	req.RemoteAddr = "203.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-perm blacklisted login, got %d", rr.Code)
	}
}

func TestCheckIPBlock_NonPermanentBlacklist_AllowsViewing(t *testing.T) {
	initTestDB(t)
	AddToBlacklist("204.0.0.1", "temp ban", false)

	called := false
	handler := CheckIPBlock(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "204.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler should be called for non-perm blacklisted non-login request")
	}
}

func TestCheckIPBlock_TempBlock_API(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('205.0.0.1', datetime('now'), 3, datetime('now', '+1 hour'), 'too many attempts')`)

	handler := CheckIPBlock(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call next for temp-blocked IP")
	}))

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.RemoteAddr = "205.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for temp-blocked API, got %d", rr.Code)
	}
}

func TestCheckIPBlock_TempBlock_Web(t *testing.T) {
	initTestDB(t)
	database.DB.Exec(`INSERT INTO ip_blocks (ip_address, blocked_at, attempts, expires_at, reason)
		VALUES ('206.0.0.1', datetime('now'), 3, datetime('now', '+1 hour'), 'too many attempts')`)

	handler := CheckIPBlock(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call next for temp-blocked IP")
	}))

	req := httptest.NewRequest("GET", "/page", nil)
	req.RemoteAddr = "206.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect for temp-blocked web, got %d", rr.Code)
	}
}
