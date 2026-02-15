package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a token bucket rate limiter
type Limiter struct {
	mu            sync.Mutex
	buckets       map[string]*bucket
	tokensPerMin  int
	maxTokens     int
	errorMessage  string
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// Config for creating a new rate limiter
type Config struct {
	TokensPerMinute int    // Number of tokens added per minute
	MaxTokens       int    // Maximum tokens that can be accumulated
	ErrorMessage    string // Message to return when rate limited
}

// New creates a new rate limiter
func New(cfg Config) *Limiter {
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = cfg.TokensPerMinute
	}

	l := &Limiter{
		buckets:      make(map[string]*bucket),
		tokensPerMin: cfg.TokensPerMinute,
		maxTokens:    cfg.MaxTokens,
		errorMessage: cfg.ErrorMessage,
		stopCleanup:  make(chan struct{}),
	}

	// Start cleanup goroutine to remove stale buckets
	l.cleanupTicker = time.NewTicker(5 * time.Minute)
	go l.cleanup()

	return l
}

// cleanup removes stale buckets periodically
func (l *Limiter) cleanup() {
	for {
		select {
		case <-l.cleanupTicker.C:
			l.mu.Lock()
			now := time.Now()
			for key, b := range l.buckets {
				// Remove buckets that haven't been used in 10 minutes
				if now.Sub(b.lastCheck) > 10*time.Minute {
					delete(l.buckets, key)
				}
			}
			l.mu.Unlock()
		case <-l.stopCleanup:
			l.cleanupTicker.Stop()
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (l *Limiter) Stop() {
	close(l.stopCleanup)
}

// Allow checks if a request is allowed for the given key (usually IP address)
// Returns true if allowed, false if rate limited
func (l *Limiter) Allow(key string) bool {
	return l.AllowN(key, 1)
}

// AllowN checks if n requests are allowed
func (l *Limiter) AllowN(key string, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, exists := l.buckets[key]

	if !exists {
		b = &bucket{
			tokens:    float64(l.maxTokens),
			lastCheck: now,
		}
		l.buckets[key] = b
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(b.lastCheck).Minutes()
	b.tokens += elapsed * float64(l.tokensPerMin)
	if b.tokens > float64(l.maxTokens) {
		b.tokens = float64(l.maxTokens)
	}
	b.lastCheck = now

	// Check if we have enough tokens
	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}

	return false
}

// Remaining returns the number of remaining tokens for a key
func (l *Limiter) Remaining(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, exists := l.buckets[key]
	if !exists {
		return l.maxTokens
	}

	// Update tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Minutes()
	tokens := b.tokens + elapsed*float64(l.tokensPerMin)
	if tokens > float64(l.maxTokens) {
		tokens = float64(l.maxTokens)
	}

	return int(tokens)
}

// ErrorMessage returns the error message for this limiter
func (l *Limiter) ErrorMessage() string {
	return l.errorMessage
}

// Reset resets the bucket for a key (useful after successful login)
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// Global rate limiters
var (
	// LoginLimiter limits login attempts to 10 per minute per IP
	LoginLimiter = New(Config{
		TokensPerMinute: 10,
		MaxTokens:       10,
		ErrorMessage:    "Too many login attempts. Please try again later.",
	})

	// APILimiter limits API requests to 120 per minute per IP
	APILimiter = New(Config{
		TokensPerMinute: 120,
		MaxTokens:       120,
		ErrorMessage:    "Too many requests. Please slow down.",
	})

	// CheckLimiter limits health check endpoint to 30 per minute per IP
	CheckLimiter = New(Config{
		TokensPerMinute: 30,
		MaxTokens:       30,
		ErrorMessage:    "Too many status check requests. Please slow down.",
	})
)
