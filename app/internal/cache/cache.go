package cache

import (
	"sync"
	"time"
)

// Entry represents a cached value with expiration
type Entry struct {
	Value     interface{}
	ExpiresAt time.Time
}

// Cache provides a simple in-memory cache with TTL
type Cache struct {
	mu            sync.RWMutex
	items         map[string]Entry
	defaultTTL    time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// New creates a new cache with the given default TTL
func New(defaultTTL time.Duration) *Cache {
	c := &Cache{
		items:       make(map[string]Entry),
		defaultTTL:  defaultTTL,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	c.cleanupTicker = time.NewTicker(defaultTTL)
	go c.cleanup()

	return c
}

// cleanup removes expired entries periodically
func (c *Cache) cleanup() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.mu.Lock()
			now := time.Now()
			for key, entry := range c.items {
				if now.After(entry.ExpiresAt) {
					delete(c.items, key)
				}
			}
			c.mu.Unlock()
		case <-c.stopCleanup:
			c.cleanupTicker.Stop()
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (c *Cache) Stop() {
	close(c.stopCleanup)
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.items[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Value, true
}

// Set stores a value in the cache with the default TTL
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value in the cache with a custom TTL
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = Entry{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Delete removes a value from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// DeletePrefix removes all values with keys starting with the given prefix
func (c *Cache) DeletePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.items, key)
		}
	}
}

// Clear removes all values from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]Entry)
}

// SettingsCache is a global cache for settings with 60-second TTL (like Uptime Kuma)
var SettingsCache = New(60 * time.Second)

// StatsCache is a global cache for computed statistics with 30-second TTL
var StatsCache = New(30 * time.Second)

// ServiceCache is a global cache for service configurations with 30-second TTL
var ServiceCache = New(30 * time.Second)
