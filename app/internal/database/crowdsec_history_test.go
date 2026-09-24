package database

import (
	"testing"
	"time"

	"status/app/internal/models"
)

func TestCrowdSecArchive_V7MigrationPreservesSourceAndPayload(t *testing.T) {
	initCrowdSecTestDB(t)
	config := &models.CrowdSecConfig{LAPIURL: "http://lapi.example:8080", MachineID: "machine", PollIntervalS: 30}
	if err := SaveCrowdSecConfig(config); err != nil {
		t.Fatal(err)
	}
	// Recreate the deployed v7 table, including its single-column primary key.
	if _, err := DB.Exec(`DROP TABLE crowdsec_alerts;
		CREATE TABLE crowdsec_alerts (
		alert_id TEXT PRIMARY KEY, scenario TEXT NOT NULL DEFAULT '', message TEXT NOT NULL DEFAULT '',
		source_value TEXT NOT NULL DEFAULT '', country TEXT NOT NULL DEFAULT '',
		as_number TEXT NOT NULL DEFAULT '', as_name TEXT NOT NULL DEFAULT '', latitude REAL, longitude REAL,
		events_count INTEGER NOT NULL DEFAULT 0, start_at TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '',
		has_decision INTEGER NOT NULL DEFAULT 0, simulated INTEGER NOT NULL DEFAULT 0, synced_at TEXT NOT NULL);
		INSERT INTO crowdsec_alerts (alert_id, scenario, message, latitude, longitude, events_count, created_at, synced_at)
		VALUES ('42', 'ssh-bf', 'preserve me', 51.5, -0.1, 17, '2026-09-23T12:00:00Z', '2026-09-23T12:01:00Z');
		UPDATE app_metadata SET value = '7' WHERE key = 'database_schema_version'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := EnsureSchema(); err != nil {
			t.Fatalf("migration pass %d: %v", i, err)
		}
	}
	var source, message, version string
	var events int
	var lat, lng float64
	if err := DB.QueryRow(`SELECT source_id, message, events_count, latitude, longitude FROM crowdsec_alerts WHERE alert_id = '42'`).
		Scan(&source, &message, &events, &lat, &lng); err != nil {
		t.Fatal(err)
	}
	if source != CrowdSecSourceID(config.LAPIURL, config.MachineID) || message != "preserve me" || events != 17 || lat != 51.5 || lng != -0.1 {
		t.Fatalf("migration lost source or payload: %q %q %d %v %v", source, message, events, lat, lng)
	}
	if err := DB.QueryRow(`SELECT value FROM app_metadata WHERE key = 'database_schema_version'`).Scan(&version); err != nil || version != "8" {
		t.Fatalf("schema version = %q, %v", version, err)
	}
	if _, err := DB.Exec(`INSERT INTO crowdsec_alerts (source_id, alert_id, synced_at) VALUES ('other-source', '42', '')`); err != nil {
		t.Fatalf("migrated primary key still collides across sources: %v", err)
	}
}

func TestCrowdSecArchive_SourceIsolationAndAllRetainedOldest(t *testing.T) {
	initCrowdSecTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	config := &models.CrowdSecConfig{LAPIURL: "http://first.example:8080", MachineID: "machine", PollIntervalS: 30}
	for i, host := range []string{"http://first.example:8080", "http://second.example:8080"} {
		config.LAPIURL = host
		if err := SaveCrowdSecConfig(config); err != nil {
			t.Fatal(err)
		}
		alerts := []models.CrowdSecAlert{{AlertID: "same-id", Scenario: host, CreatedAt: now.Add(-time.Duration(i+1) * 48 * time.Hour).Format(time.RFC3339), EventsCount: int64(i + 1)}}
		if inserted, err := SyncCrowdSecAlerts(alerts, now); err != nil || inserted != 1 {
			t.Fatalf("source %d insert: %d, %v", i, inserted, err)
		}
		if err := SaveCrowdSecHistoryState(CrowdSecHistoryState{Complete: i == 0}); err != nil {
			t.Fatal(err)
		}
	}
	for _, host := range []string{"http://first.example:8080", "http://second.example:8080"} {
		config.LAPIURL = host
		if err := SaveCrowdSecConfig(config); err != nil {
			t.Fatal(err)
		}
		coverage, err := GetCrowdSecHistoryCoverage()
		if err != nil {
			t.Fatal(err)
		}
		window := NewCrowdSecWindow(now, 24, true, coverage.Oldest)
		alerts, err := GetCrowdSecAlertsWindow(10, false, window)
		if err != nil || len(alerts) != 1 || alerts[0].Scenario != host {
			t.Fatalf("source leaked or oldest all-retained row omitted: %+v, %v", alerts, err)
		}
		stats, err := GetCrowdSecStatsWindow(window)
		if err != nil || stats.Alerts24h != 1 || stats.WindowHours != 0 || stats.HistoryComplete != (host == "http://first.example:8080") {
			t.Fatalf("source statistics/coverage mismatch: %+v, %v", stats, err)
		}
	}
}

func TestCrowdSecArchive_OneYearBoundaryAndPruneCoverage(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(-CrowdSecArchiveMaxAge)
	alerts := []models.CrowdSecAlert{
		{AlertID: "expired", CreatedAt: cutoff.Add(-time.Second).Format(time.RFC3339)},
		{AlertID: "boundary", CreatedAt: cutoff.Format(time.RFC3339)},
		{AlertID: "retained", CreatedAt: cutoff.Add(time.Second).Format(time.RFC3339)},
		{AlertID: "month-old", CreatedAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)},
	}
	if inserted, err := SyncCrowdSecAlerts(alerts, now); err != nil || inserted != 2 {
		t.Fatalf("one-year boundary: inserted %d, %v", inserted, err)
	}
	if err := SaveCrowdSecHistoryState(CrowdSecHistoryState{Complete: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncCrowdSecAlerts(nil, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	coverage, err := GetCrowdSecHistoryCoverage()
	if err != nil || coverage.Complete || !coverage.Truncated || coverage.Oldest != alerts[3].CreatedAt {
		t.Fatalf("age prune must retain recent history and disclose truncation: %+v, %v", coverage, err)
	}
}

func TestCrowdSecStats_CustomWindowsUseAdaptiveBuckets(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		hours, buckets int
		seconds        int64
	}{{6, 6, 3600}, {48, 48, 3600}, {7 * 24, 7, 86400}, {90 * 24, 13, 7 * 86400}, {365 * 24, 13, 30 * 86400}} {
		window := NewCrowdSecWindow(now, tc.hours, false, "")
		stats, err := GetCrowdSecStatsWindow(window)
		if err != nil || stats.WindowHours != tc.hours || len(stats.Hourly) != tc.buckets || stats.BucketSeconds != tc.seconds {
			t.Fatalf("%dh range: %+v, %v", tc.hours, stats, err)
		}
	}
}
