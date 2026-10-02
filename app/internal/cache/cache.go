package cache

import (
	"fmt"
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

	flightMu sync.Mutex
	inflight map[string]*loadCall
	// generation changes whenever entries are invalidated. A load that started
	// before an invalidation must not store its now-stale result. Guarded by
	// flightMu.
	generation uint64
}

// loadCall is one in-progress GetOrLoad computation shared by concurrent callers.
type loadCall struct {
	done  chan struct{}
	value interface{}
	err   error
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

// GetOrLoad returns the cached value for key, or runs load once and shares its
// result with every concurrent caller (single flight). Successful results are
// cached for ttl; errors are returned but not cached. This bounds the work a
// burst of identical requests can cause to one computation per ttl.
func (c *Cache) GetOrLoad(key string, ttl time.Duration, load func() (interface{}, error)) (interface{}, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}

	c.flightMu.Lock()
	if call, ok := c.inflight[key]; ok {
		c.flightMu.Unlock()
		<-call.done
		return call.value, call.err
	}
	// Another caller may have stored the value since the unlocked Get.
	if v, ok := c.Get(key); ok {
		c.flightMu.Unlock()
		return v, nil
	}
	call := &loadCall{done: make(chan struct{})}
	if c.inflight == nil {
		c.inflight = make(map[string]*loadCall)
	}
	c.inflight[key] = call
	generation := c.generation
	c.flightMu.Unlock()

	func() {
		defer func() {
			if r := recover(); r != nil {
				call.err = fmt.Errorf("cache load for %q panicked: %v", key, r)
			}
		}()
		call.value, call.err = load()
	}()
	c.flightMu.Lock()
	if call.err == nil && c.generation == generation {
		c.SetWithTTL(key, call.value, ttl)
	}
	if c.inflight[key] == call {
		delete(c.inflight, key)
	}
	c.flightMu.Unlock()
	close(call.done)
	return call.value, call.err
}

// invalidate stops loads already in flight from storing or sharing results
// computed from data that is about to change.
func (c *Cache) invalidate() {
	c.flightMu.Lock()
	c.generation++
	c.inflight = nil
	c.flightMu.Unlock()
}

// Delete removes a value from the cache
func (c *Cache) Delete(key string) {
	c.invalidate()
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// DeletePrefix removes all values with keys starting with the given prefix
func (c *Cache) DeletePrefix(prefix string) {
	c.invalidate()
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
	c.invalidate()
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

// PublicCache holds responses served to anonymous dashboard visitors. Entries
// are short-lived and the cache is cleared after admin changes, so every
// visitor shares one computation instead of each triggering its own.
var PublicCache = New(15 * time.Second)
