package database

import (
	"testing"
	"time"
)

func TestPruneSamplesRemovesOnlyExpiredRowsAcrossBatches(t *testing.T) {
	initTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-500 * 24 * time.Hour)

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12000; i++ { // more than two delete batches
		if _, err := tx.Exec(`INSERT INTO samples (taken_at, service_key, ok) VALUES (?, 'svc', 1)`,
			old.Add(time.Duration(i)*time.Second).Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	InsertSample(now, "svc", true, 200, nil)

	removed, err := PruneSamples(now.Add(-400 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 12000 {
		t.Fatalf("removed = %d, want 12000", removed)
	}
	var remaining int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("remaining samples = %d, want only the recent one", remaining)
	}
}

func TestSchemaMigrationIsIdempotent(t *testing.T) {
	initTestDB(t)
	// A second run must find every column already present and change nothing.
	if err := EnsureSchema(); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}
	var idx int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_samples_service_taken'`).Scan(&idx); err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatal("composite samples index missing")
	}
}
