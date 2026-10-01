package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetOrLoadSharesOneComputation(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	var calls atomic.Int32
	release := make(chan struct{})
	load := func() (interface{}, error) {
		calls.Add(1)
		<-release
		return "value", nil
	}

	const callers = 20
	var wg sync.WaitGroup
	results := make([]interface{}, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := c.GetOrLoad("k", time.Minute, load)
			if err != nil {
				t.Errorf("GetOrLoad error: %v", err)
			}
			results[i] = v
		}(i)
	}
	time.Sleep(50 * time.Millisecond) // let every caller join the in-flight load
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("load ran %d times, want 1", got)
	}
	for i, v := range results {
		if v != "value" {
			t.Fatalf("caller %d got %v", i, v)
		}
	}
	if v, err := c.GetOrLoad("k", time.Minute, load); err != nil || v != "value" || calls.Load() != 1 {
		t.Fatalf("cached call = %v, %v (loads=%d), want cached value without reloading", v, err, calls.Load())
	}
}

func TestGetOrLoadDoesNotCacheErrors(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	calls := 0
	failing := func() (interface{}, error) { calls++; return nil, errors.New("boom") }
	if _, err := c.GetOrLoad("k", time.Minute, failing); err == nil {
		t.Fatal("expected error")
	}
	if _, err := c.GetOrLoad("k", time.Minute, failing); err == nil || calls != 2 {
		t.Fatalf("second call err=%v calls=%d, want a fresh attempt", err, calls)
	}
}

func TestGetOrLoadRecoversPanics(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	_, err := c.GetOrLoad("k", time.Minute, func() (interface{}, error) { panic("bad") })
	if err == nil {
		t.Fatal("expected panic to surface as an error")
	}
	// The key must not stay stuck in flight after a panic.
	v, err := c.GetOrLoad("k", time.Minute, func() (interface{}, error) { return 1, nil })
	if err != nil || v != 1 {
		t.Fatalf("after panic got %v, %v", v, err)
	}
}

func TestGetOrLoadDoesNotStoreResultsInvalidatedMidLoad(t *testing.T) {
	c := New(time.Minute)
	defer c.Stop()

	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = c.GetOrLoad("k", time.Minute, func() (interface{}, error) {
			close(started)
			<-release
			return "stale", nil
		})
	}()
	<-started
	c.Clear() // an admin edit lands while the old data is being computed

	// New callers must not join the stale load...
	v, err := c.GetOrLoad("k", time.Minute, func() (interface{}, error) { return "fresh", nil })
	if err != nil || v != "fresh" {
		t.Fatalf("after Clear got %v, %v; want a fresh load", v, err)
	}
	close(release)
	<-finished

	// ...and the stale load must not overwrite the fresh value when it finishes.
	if v, ok := c.Get("k"); !ok || v != "fresh" {
		t.Fatalf("cached value = %v (ok=%v), want fresh", v, ok)
	}
}
