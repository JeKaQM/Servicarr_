package ratelimit

import (
	"testing"
	"time"
)

func TestNew_DefaultMaxTokens(t *testing.T) {
	l := New(Config{TokensPerMinute: 10, ErrorMessage: "rate limited"})
	defer l.Stop()

	if l.maxTokens != 10 {
		t.Errorf("expected maxTokens=10, got %d", l.maxTokens)
	}
}

func TestNew_CustomMaxTokens(t *testing.T) {
	l := New(Config{TokensPerMinute: 10, MaxTokens: 20, ErrorMessage: "rate limited"})
	defer l.Stop()

	if l.maxTokens != 20 {
		t.Errorf("expected maxTokens=20, got %d", l.maxTokens)
	}
}

func TestAllow_WithinLimit(t *testing.T) {
	l := New(Config{TokensPerMinute: 10, MaxTokens: 10})
	defer l.Stop()

	for i := 0; i < 10; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	l := New(Config{TokensPerMinute: 5, MaxTokens: 5})
	defer l.Stop()

	// Drain all tokens
	for i := 0; i < 5; i++ {
		l.Allow("1.2.3.4")
	}

	// Next request should be denied
	if l.Allow("1.2.3.4") {
		t.Error("request should be denied after exceeding limit")
	}
}

func TestAllow_DifferentKeys(t *testing.T) {
	l := New(Config{TokensPerMinute: 2, MaxTokens: 2})
	defer l.Stop()

	// Drain IP1
	l.Allow("ip1")
	l.Allow("ip1")

	// IP2 should still be allowed
	if !l.Allow("ip2") {
		t.Error("different key should have its own bucket")
	}

	// IP1 should now be blocked
	if l.Allow("ip1") {
		t.Error("ip1 should be rate limited")
	}
}

func TestAllowN(t *testing.T) {
	l := New(Config{TokensPerMinute: 10, MaxTokens: 10})
	defer l.Stop()

	if !l.AllowN("key", 5) {
		t.Error("AllowN(5) should pass when 10 tokens available")
	}

	if !l.AllowN("key", 5) {
		t.Error("AllowN(5) should pass when 5 tokens remaining")
	}

	if l.AllowN("key", 1) {
		t.Error("AllowN(1) should fail when 0 tokens remaining")
	}
}

func TestRemaining(t *testing.T) {
	l := New(Config{TokensPerMinute: 10, MaxTokens: 10})
	defer l.Stop()

	rem := l.Remaining("new-key")
	if rem != 10 {
		t.Errorf("expected 10 remaining for new key, got %d", rem)
	}

	l.Allow("new-key")
	rem = l.Remaining("new-key")
	if rem != 9 {
		t.Errorf("expected 9 remaining after 1 request, got %d", rem)
	}
}

func TestReset(t *testing.T) {
	l := New(Config{TokensPerMinute: 5, MaxTokens: 5})
	defer l.Stop()

	// Drain all tokens
	for i := 0; i < 5; i++ {
		l.Allow("victim")
	}

	if l.Allow("victim") {
		t.Error("should be rate limited")
	}

	// Reset
	l.Reset("victim")

	// Should be allowed again
	if !l.Allow("victim") {
		t.Error("should be allowed after reset")
	}
}

func TestErrorMessage(t *testing.T) {
	msg := "custom rate limit message"
	l := New(Config{TokensPerMinute: 1, ErrorMessage: msg})
	defer l.Stop()

	if l.ErrorMessage() != msg {
		t.Errorf("expected %q, got %q", msg, l.ErrorMessage())
	}
}

func TestTokenRefill(t *testing.T) {
	l := New(Config{TokensPerMinute: 60, MaxTokens: 60}) // 1 per second
	defer l.Stop()

	// Drain all tokens
	for i := 0; i < 60; i++ {
		l.Allow("refill-test")
	}

	if l.Allow("refill-test") {
		t.Error("should be rate limited after draining")
	}

	// Simulate time passing by manipulating the bucket directly
	l.mu.Lock()
	b := l.buckets["refill-test"]
	b.lastCheck = time.Now().Add(-1 * time.Minute) // 1 minute ago
	l.mu.Unlock()

	// Should be allowed now (1 minute = 60 new tokens)
	if !l.Allow("refill-test") {
		t.Error("should be allowed after token refill")
	}
}

func TestStop(t *testing.T) {
	l := New(Config{TokensPerMinute: 10})
	l.Stop()
	// Just verify it doesn't panic or deadlock
}

// --- Global Limiters ---

func TestGlobalLimitersExist(t *testing.T) {
	if LoginLimiter == nil {
		t.Error("LoginLimiter is nil")
	}
	if APILimiter == nil {
		t.Error("APILimiter is nil")
	}
	if CheckLimiter == nil {
		t.Error("CheckLimiter is nil")
	}
	if SetupLimiter == nil {
		t.Error("SetupLimiter is nil")
	}
}

func TestGlobalLimiters_ErrorMessages(t *testing.T) {
	if LoginLimiter.ErrorMessage() == "" {
		t.Error("LoginLimiter should have an error message")
	}
	if APILimiter.ErrorMessage() == "" {
		t.Error("APILimiter should have an error message")
	}
	if CheckLimiter.ErrorMessage() == "" {
		t.Error("CheckLimiter should have an error message")
	}
	if SetupLimiter.ErrorMessage() == "" {
		t.Error("SetupLimiter should have an error message")
	}
}
