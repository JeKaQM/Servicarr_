package monitor

import (
	"sync"
	"testing"
)

func TestNewFailureTracker(t *testing.T) {
	ft := NewFailureTracker()
	if ft == nil {
		t.Fatal("NewFailureTracker returned nil")
	}
}

func TestUpdate_IncrementOnFailure(t *testing.T) {
	ft := NewFailureTracker()

	count := ft.Update("svc1", false)
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	count = ft.Update("svc1", false)
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	count = ft.Update("svc1", false)
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestUpdate_ResetOnSuccess(t *testing.T) {
	ft := NewFailureTracker()

	ft.Update("svc1", false) // 1
	ft.Update("svc1", false) // 2
	ft.Update("svc1", false) // 3

	count := ft.Update("svc1", true)
	if count != 0 {
		t.Errorf("expected 0 after success, got %d", count)
	}

	// Next failure should start at 1 again
	count = ft.Update("svc1", false)
	if count != 1 {
		t.Errorf("expected 1 after reset, got %d", count)
	}
}

func TestUpdate_IndependentKeys(t *testing.T) {
	ft := NewFailureTracker()

	ft.Update("svc1", false) // svc1 -> 1
	ft.Update("svc2", false) // svc2 -> 1
	ft.Update("svc1", false) // svc1 -> 2

	count := ft.Update("svc2", false)
	if count != 2 {
		t.Errorf("expected svc2=2, got %d", count)
	}
}

func TestReset(t *testing.T) {
	ft := NewFailureTracker()

	ft.Update("svc1", false)
	ft.Update("svc1", false)
	ft.Reset("svc1")

	// After Reset, next failure starts at 1
	count := ft.Update("svc1", false)
	if count != 1 {
		t.Errorf("expected 1 after Reset, got %d", count)
	}
}

func TestReset_NonexistentKey(t *testing.T) {
	ft := NewFailureTracker()
	ft.Reset("nonexistent") // should not panic
}

func TestPrune(t *testing.T) {
	ft := NewFailureTracker()

	ft.Update("svc1", false)
	ft.Update("svc2", false)
	ft.Update("svc3", false)

	valid := map[string]struct{}{
		"svc1": {},
		"svc3": {},
	}
	ft.Prune(valid)

	// svc2 should be pruned, increment should start at 1
	count := ft.Update("svc2", false)
	if count != 1 {
		t.Errorf("expected svc2 pruned (count=1), got %d", count)
	}

	// svc1 should still have its count
	count = ft.Update("svc1", false)
	if count != 2 {
		t.Errorf("expected svc1=2 (kept), got %d", count)
	}
}

func TestPrune_EmptyValidKeys(t *testing.T) {
	ft := NewFailureTracker()
	ft.Update("svc1", false)
	ft.Update("svc2", false)

	// Prune with empty valid set removes everything
	ft.Prune(map[string]struct{}{})

	count := ft.Update("svc1", false)
	if count != 1 {
		t.Errorf("expected svc1 pruned (count=1), got %d", count)
	}
}

func TestConcurrentUpdates(t *testing.T) {
	ft := NewFailureTracker()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ft.Update("key", false)
		}()
	}

	wg.Wait()

	// After 100 concurrent failures, next should be 101
	count := ft.Update("key", false)
	if count != 101 {
		t.Errorf("expected 101, got %d", count)
	}
}
