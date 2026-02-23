package cache

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := New(1 * time.Second)
	defer c.Stop()

	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.defaultTTL != 1*time.Second {
		t.Errorf("expected defaultTTL=1s, got %v", c.defaultTTL)
	}
}

func TestSet_Get(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	c.Set("key1", "value1")

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got %v", val)
	}
}

func TestGet_Missing(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestGet_Expired(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	// Manually set with past expiration
	c.mu.Lock()
	c.items["expired"] = Entry{
		Value:     "old",
		ExpiresAt: time.Now().Add(-1 * time.Second),
	}
	c.mu.Unlock()

	_, ok := c.Get("expired")
	if ok {
		t.Error("expected false for expired key")
	}
}

func TestSetWithTTL(t *testing.T) {
	c := New(1 * time.Hour)
	defer c.Stop()

	// Set with short TTL
	c.SetWithTTL("short", "data", 50*time.Millisecond)

	val, ok := c.Get("short")
	if !ok || val != "data" {
		t.Fatal("expected key to exist immediately after set")
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get("short")
	if ok {
		t.Error("expected key to be expired after TTL")
	}
}

func TestDelete(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	c.Set("del", "val")
	c.Delete("del")

	_, ok := c.Get("del")
	if ok {
		t.Error("expected key to be deleted")
	}
}

func TestDeletePrefix(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	c.Set("prefix:a", 1)
	c.Set("prefix:b", 2)
	c.Set("other:c", 3)

	c.DeletePrefix("prefix:")

	if _, ok := c.Get("prefix:a"); ok {
		t.Error("prefix:a should be deleted")
	}
	if _, ok := c.Get("prefix:b"); ok {
		t.Error("prefix:b should be deleted")
	}
	if _, ok := c.Get("other:c"); !ok {
		t.Error("other:c should still exist")
	}
}

func TestClear(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	c.Clear()

	if _, ok := c.Get("a"); ok {
		t.Error("expected all keys cleared")
	}
	if _, ok := c.Get("b"); ok {
		t.Error("expected all keys cleared")
	}
}

func TestOverwrite(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	c.Set("key", "first")
	c.Set("key", "second")

	val, ok := c.Get("key")
	if !ok || val != "second" {
		t.Errorf("expected 'second', got %v", val)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := New(1 * time.Minute)
	defer c.Stop()

	done := make(chan bool, 20)
	for i := 0; i < 10; i++ {
		go func(n int) {
			c.Set("key", n)
			done <- true
		}(i)
		go func() {
			c.Get("key")
			done <- true
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

// --- Global cache instances ---

func TestGlobalCachesExist(t *testing.T) {
	if SettingsCache == nil {
		t.Error("SettingsCache is nil")
	}
	if StatsCache == nil {
		t.Error("StatsCache is nil")
	}
	if ServiceCache == nil {
		t.Error("ServiceCache is nil")
	}
}
