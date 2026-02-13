package monitor

import "sync"

// FailureTracker keeps track of consecutive failures per service.
// It is safe for concurrent use.
type FailureTracker struct {
	mu     sync.Mutex
	counts map[string]int
}

// NewFailureTracker creates a new tracker.
func NewFailureTracker() *FailureTracker {
	return &FailureTracker{
		counts: make(map[string]int),
	}
}

// Update increments or resets the failure count for a service.
// It returns the updated consecutive failure count.
func (t *FailureTracker) Update(key string, ok bool) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if ok {
		t.counts[key] = 0
		return 0
	}

	t.counts[key]++
	return t.counts[key]
}

// Reset clears the failure count for a service.
func (t *FailureTracker) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counts, key)
}

// Prune removes entries for services that no longer exist.
func (t *FailureTracker) Prune(validKeys map[string]struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for key := range t.counts {
		if _, ok := validKeys[key]; !ok {
			delete(t.counts, key)
		}
	}
}
