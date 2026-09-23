package database

import (
	"fmt"
	"testing"
	"time"

	"status/app/internal/models"
)

func initCrowdSecAlertsTestDB(t *testing.T) {
	t.Helper()
	initTestDB(t)
}

func TestSyncCrowdSecAlerts_InsertsNewAndSkipsExisting(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC()

	alerts := []models.CrowdSecAlert{
		{AlertID: "1", Scenario: "crowdsecurity/ssh-bf", SourceValue: "1.2.3.4", Country: "CN", EventsCount: 12, CreatedAt: now.Format(time.RFC3339), HasDecision: true},
		{AlertID: "2", Scenario: "crowdsecurity/http-probing", SourceValue: "5.6.7.8", Country: "RU", EventsCount: 3, CreatedAt: now.Format(time.RFC3339), HasDecision: false},
	}
	inserted, err := SyncCrowdSecAlerts(alerts, now)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if inserted != 2 {
		t.Errorf("inserted = %d, want 2", inserted)
	}

	// Re-sync the same alerts (plus one new): existing IDs are skipped.
	alerts = append(alerts, models.CrowdSecAlert{AlertID: "3", Scenario: "crowdsecurity/ssh-bf", SourceValue: "9.9.9.9", Country: "US", CreatedAt: now.Format(time.RFC3339)})
	inserted, err = SyncCrowdSecAlerts(alerts, now)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if inserted != 1 {
		t.Errorf("inserted = %d, want 1 (dedup)", inserted)
	}

	got, err := GetCrowdSecAlerts(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("stored alerts = %d, want 3", len(got))
	}
	byID := map[string]models.CrowdSecAlert{}
	for _, a := range got {
		byID[a.AlertID] = a
	}
	if byID["1"].Country != "CN" || !byID["1"].HasDecision {
		t.Errorf("alert 1 roundtrip mismatch: %+v", byID["1"])
	}
	if byID["2"].HasDecision {
		t.Errorf("alert 2 should have no decision: %+v", byID["2"])
	}
}

func TestSyncCrowdSecAlerts_GeoRoundtrip(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC()

	lat, lng := 55.7558, 37.6173 // Moscow
	alerts := []models.CrowdSecAlert{
		{AlertID: "geo-1", Scenario: "crowdsecurity/ssh-bf", SourceValue: "5.6.7.8", Country: "RU", CreatedAt: now.Format(time.RFC3339), Latitude: &lat, Longitude: &lng},
		{AlertID: "geo-2", Scenario: "crowdsecurity/http-probing", SourceValue: "9.9.9.9", Country: "US", CreatedAt: now.Format(time.RFC3339)}, // no geo
	}
	if _, err := SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got, err := GetCrowdSecAlerts(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := map[string]models.CrowdSecAlert{}
	for _, a := range got {
		byID[a.AlertID] = a
	}
	geo := byID["geo-1"]
	if geo.Latitude == nil || geo.Longitude == nil {
		t.Fatalf("geo-1 lost coordinates: %+v", geo)
	}
	if *geo.Latitude != lat || *geo.Longitude != lng {
		t.Errorf("geo-1 roundtrip = (%v, %v), want (%v, %v)", *geo.Latitude, *geo.Longitude, lat, lng)
	}
	plain := byID["geo-2"]
	if plain.Latitude != nil || plain.Longitude != nil {
		t.Errorf("geo-2 should have nil coordinates: %+v", plain)
	}
}

func TestGetCrowdSecCompactAlerts_SkipsUnusedText(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC()
	alert := models.CrowdSecAlert{
		AlertID: "compact", Message: "large diagnostic payload",
		StartAt:   now.Add(-time.Minute).Format(time.RFC3339),
		CreatedAt: now.Format(time.RFC3339), SourceValue: "1.2.3.4",
	}
	if _, err := SyncCrowdSecAlerts([]models.CrowdSecAlert{alert}, now); err != nil {
		t.Fatal(err)
	}
	compact, err := GetCrowdSecCompactAlerts(1)
	if err != nil || len(compact) != 1 {
		t.Fatalf("compact read: %+v, %v", compact, err)
	}
	if compact[0].Message != "" || compact[0].StartAt != "" || compact[0].SourceValue != alert.SourceValue {
		t.Fatalf("compact read included unused text or lost dashboard field: %+v", compact[0])
	}
	full, err := GetCrowdSecAlerts(1)
	if err != nil || len(full) != 1 || full[0].Message != alert.Message || full[0].StartAt != alert.StartAt {
		t.Fatalf("full read changed: %+v, %v", full, err)
	}
}

func TestSyncCrowdSecAlerts_EmptyIsNoop(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	inserted, err := SyncCrowdSecAlerts(nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("empty sync: %v", err)
	}
	if inserted != 0 {
		t.Errorf("inserted = %d, want 0", inserted)
	}
}

func TestSyncCrowdSecAlerts_EmptyPrunesExpiredHistory(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC()
	_, err := DB.Exec(`INSERT INTO crowdsec_alerts
		(alert_id, scenario, message, source_value, country, as_number, as_name,
		 events_count, start_at, created_at, has_decision, simulated, synced_at)
		VALUES ('old', 'scenario', '', '1.2.3.4', '', '', '', 1, '', ?, 0, 0, ?)`,
		now.Add(-25*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed old alert: %v", err)
	}
	if _, err := SyncCrowdSecAlerts(nil, now); err != nil {
		t.Fatalf("empty sync: %v", err)
	}
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expired alerts remaining = %d, want 0", count)
	}
}

func TestSyncCrowdSecAlerts_CapsAtMaxRows(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC()
	alerts := make([]models.CrowdSecAlert, 0, CrowdSecMaxAlertRows+10)
	for i := 0; i < CrowdSecMaxAlertRows+10; i++ {
		alerts = append(alerts, models.CrowdSecAlert{
			AlertID:   fmt.Sprintf("a-%d", i),
			Scenario:  "s",
			CreatedAt: now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	if _, err := SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatalf("bulk sync: %v", err)
	}
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count > CrowdSecMaxAlertRows {
		t.Errorf("alerts count %d exceeds cap %d", count, CrowdSecMaxAlertRows)
	}
	// The newest alert must survive the prune.
	got, err := GetCrowdSecAlerts(1)
	if err != nil || len(got) != 1 || got[0].AlertID != "a-0" {
		t.Fatalf("newest alert missing after prune: %+v err=%v", got, err)
	}
}

func TestSyncCrowdSecAlerts_FutureRowsCannotPinRetentionOrAppearInFeed(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	alerts := make([]models.CrowdSecAlert, 0, CrowdSecMaxAlertRows+1)
	for i := 0; i < CrowdSecMaxAlertRows; i++ {
		alerts = append(alerts, models.CrowdSecAlert{
			AlertID:   fmt.Sprintf("current-%04d", i),
			CreatedAt: now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339),
		})
	}
	alerts = append(alerts, models.CrowdSecAlert{
		AlertID: "incoming-future", CreatedAt: now.Add(time.Hour).Format(time.RFC3339),
	})
	inserted, err := SyncCrowdSecAlerts(alerts, now)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != CrowdSecMaxAlertRows {
		t.Fatalf("inserted = %d, want only %d current alerts", inserted, CrowdSecMaxAlertRows)
	}

	// Old builds could already have future and malformed rows in the mirror.
	for _, row := range []struct{ id, createdAt string }{
		{"stored-future", now.Add(2 * time.Hour).Format(time.RFC3339)},
		{"stored-malformed", "not-a-time"},
	} {
		if _, err := DB.Exec(`INSERT INTO crowdsec_alerts (alert_id, created_at, synced_at)
			VALUES (?, ?, ?)`, row.id, row.createdAt, now.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	beforePrune, err := GetCrowdSecAlerts(CrowdSecMaxAlertRows)
	if err != nil {
		t.Fatal(err)
	}
	if len(beforePrune) != CrowdSecMaxAlertRows {
		t.Fatalf("feed included invalid future rows: got %d", len(beforePrune))
	}
	if _, err := SyncCrowdSecAlerts(nil, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != CrowdSecMaxAlertRows {
		t.Fatalf("old future rows consumed cap: retained %d of %d current alerts", count, CrowdSecMaxAlertRows)
	}
	for _, id := range []string{"incoming-future", "stored-future", "stored-malformed"} {
		var found int
		if err := DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts WHERE alert_id = ?`, id).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found != 0 {
			t.Errorf("invalid alert %q remained in mirror", id)
		}
	}
}

func TestGetCrowdSecStats_Aggregates24h(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC()

	// Seed decisions: 2 active, 1 expired.
	seed := []models.CrowdSecDecision{
		{DecisionID: "d1", Value: "1.1.1.1", Type: "ban", CreatedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), SyncedAt: now.Format(time.RFC3339)},
		{DecisionID: "d2", Value: "2.2.2.2", Type: "ban", CreatedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(2 * time.Hour).Format(time.RFC3339), SyncedAt: now.Format(time.RFC3339)},
		{DecisionID: "d3", Value: "3.3.3.3", Type: "ban", CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(-time.Hour).Format(time.RFC3339), SyncedAt: now.Format(time.RFC3339)},
	}
	if _, err := SyncCrowdSecDecisions(seed, len(seed), now); err != nil {
		t.Fatalf("seed decisions: %v", err)
	}

	// Seed alerts: 3 CN recent, 1 RU recent, 1 old CN outside 24h;
	// one alert with a decision, three without.
	alerts := []models.CrowdSecAlert{
		{AlertID: "a1", Scenario: "crowdsecurity/ssh-bf", Country: "CN", CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339), HasDecision: true},
		{AlertID: "a2", Scenario: "crowdsecurity/ssh-bf", Country: "CN", CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), HasDecision: false},
		{AlertID: "a3", Scenario: "crowdsecurity/http-probing", Country: "CN", CreatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339), HasDecision: false},
		{AlertID: "a4", Scenario: "crowdsecurity/http-probing", Country: "RU", CreatedAt: now.Add(-4 * time.Hour).Format(time.RFC3339), HasDecision: false},
		{AlertID: "a5", Scenario: "crowdsecurity/ssh-bf", Country: "CN", CreatedAt: now.Add(-30 * time.Hour).Format(time.RFC3339), HasDecision: false},
	}
	if _, err := SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatalf("seed alerts: %v", err)
	}

	stats, err := GetCrowdSecStats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.ActiveDecisions != 2 {
		t.Errorf("active decisions = %d, want 2", stats.ActiveDecisions)
	}
	if stats.Alerts24h != 4 {
		t.Errorf("alerts 24h = %d, want 4 (old excluded)", stats.Alerts24h)
	}
	if stats.AlertsWithDecision != 1 {
		t.Errorf("alerts with decision = %d, want 1", stats.AlertsWithDecision)
	}
	if stats.TopCountry != "CN" {
		t.Errorf("top country = %q, want CN", stats.TopCountry)
	}
	if stats.TopCountryCount != 3 {
		t.Errorf("top country count = %d, want 3", stats.TopCountryCount)
	}
	if stats.TopScenario != "crowdsecurity/ssh-bf" && stats.TopScenario != "crowdsecurity/http-probing" {
		t.Errorf("top scenario = %q", stats.TopScenario)
	}
	if len(stats.Countries) != 2 {
		t.Errorf("countries = %+v, want 2 entries", stats.Countries)
	}
	if len(stats.Scenarios) != 2 {
		t.Errorf("scenarios = %+v, want 2 entries", stats.Scenarios)
	}
}

func TestGetCrowdSecStatsAt_ExpandedRollingWindow(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Date(2026, time.September, 21, 12, 34, 56, 0, time.UTC)

	decisions := []models.CrowdSecDecision{
		{DecisionID: "d1", Value: "1.1.1.1", Type: "BAN", Origin: "CrowdSec", CreatedAt: now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), SyncedAt: now.Format(time.RFC3339)},
		{DecisionID: "d2", Value: "2.2.2.2", Type: "captcha", Origin: "CSCLI", CreatedAt: now.Add(-time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(2 * time.Hour).Format(time.RFC3339), SyncedAt: now.Format(time.RFC3339)},
		{DecisionID: "d3", Value: "3.3.3.3", Type: "ban", Origin: "crowdsec", CreatedAt: now.Add(-2 * time.Hour).Format(time.RFC3339), ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339), SyncedAt: now.Format(time.RFC3339)},
	}
	if _, err := SyncCrowdSecDecisions(decisions, len(decisions), now); err != nil {
		t.Fatalf("seed decisions: %v", err)
	}

	lat, lng := 51.5, -0.12
	invalidLat, invalidLng := 100.0, 0.0
	plusTwo := time.FixedZone("plus-two", 2*60*60)
	alerts := []models.CrowdSecAlert{
		{
			AlertID: "a1", Scenario: "crowdsecurity/ssh-bf", SourceValue: "1.1.1.1",
			Country: "us", ASNumber: "as64500", ASName: "Example Net", EventsCount: 5,
			CreatedAt: now.Add(-30 * time.Minute).Format(time.RFC3339), HasDecision: true,
			Latitude: &lat, Longitude: &lng,
		},
		{
			AlertID: "a2", Scenario: "crowdsecurity/ssh-bf", SourceValue: "1.1.1.1",
			Country: "US", ASNumber: "AS64500", ASName: "Example Net",
			CreatedAt: now.Add(-90 * time.Minute).In(plusTwo).Format(time.RFC3339), Simulated: true,
		},
		{
			AlertID: "a3", EventsCount: 3,
			CreatedAt: now.Add(-23*time.Hour - 59*time.Minute).Format(time.RFC3339),
			Latitude:  &invalidLat, Longitude: &invalidLng,
		},
		{
			AlertID: "a4", Scenario: "crowdsecurity/http-probing", SourceValue: "2.2.2.2",
			Country: "gb", ASNumber: "AS64501", ASName: "Other Net", EventsCount: 2,
			CreatedAt: now.Format(time.RFC3339),
		},
		{AlertID: "future", Scenario: "future", CreatedAt: now.Add(time.Minute).Format(time.RFC3339), EventsCount: 100},
		{AlertID: "invalid-time", Scenario: "invalid", CreatedAt: "not-a-time", EventsCount: 100},
	}
	if _, err := SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatalf("seed alerts: %v", err)
	}
	// Sync pruning removes an exact-cutoff row, so insert one directly to prove
	// the statistics helper applies the documented open lower boundary itself.
	if _, err := DB.Exec(`INSERT INTO crowdsec_alerts (alert_id, created_at, synced_at)
		VALUES ('at-cutoff', ?, ?)`, now.Add(-24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed cutoff alert: %v", err)
	}

	stats, err := getCrowdSecStatsAt(now)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.WindowStart != now.Add(-24*time.Hour).Format(time.RFC3339) || stats.WindowEnd != now.Format(time.RFC3339) || stats.WindowHours != 24 {
		t.Errorf("window mismatch: %+v", stats)
	}
	if stats.Alerts24h != 4 || stats.AlertsWithDecision != 1 {
		t.Errorf("alert totals = %d/%d, want 4/1", stats.Alerts24h, stats.AlertsWithDecision)
	}
	if stats.DecisionActionRatePercent != 25 {
		t.Errorf("decision action rate = %v, want 25", stats.DecisionActionRatePercent)
	}
	if stats.ReportedEvents24h != 10 || stats.UniqueSources24h != 2 {
		t.Errorf("reported events/unique sources = %d/%d, want 10/2", stats.ReportedEvents24h, stats.UniqueSources24h)
	}
	if stats.SimulatedAlerts24h != 1 || stats.GeolocatedAlerts24h != 1 {
		t.Errorf("simulated/geolocated = %d/%d, want 1/1", stats.SimulatedAlerts24h, stats.GeolocatedAlerts24h)
	}
	if len(stats.Hourly) != 24 {
		t.Fatalf("hourly buckets = %d, want 24", len(stats.Hourly))
	}
	if stats.Hourly[0].Detections != 1 || stats.Hourly[22].Detections != 1 || stats.Hourly[23].Detections != 2 {
		t.Errorf("unexpected hourly distribution: first=%+v hour22=%+v last=%+v", stats.Hourly[0], stats.Hourly[22], stats.Hourly[23])
	}
	var hourlyDetections, hourlyEvents int64
	for _, bucket := range stats.Hourly {
		hourlyDetections += bucket.Detections
		hourlyEvents += bucket.ReportedEvents
	}
	if hourlyDetections != stats.Alerts24h || hourlyEvents != stats.ReportedEvents24h {
		t.Errorf("hourly sums %d/%d do not match totals %d/%d", hourlyDetections, hourlyEvents, stats.Alerts24h, stats.ReportedEvents24h)
	}

	if stats.TopCountry != "US" || stats.TopCountryCount != 2 || stats.TopScenario != "crowdsecurity/ssh-bf" {
		t.Errorf("top dimensions mismatch: country=%q/%d scenario=%q", stats.TopCountry, stats.TopCountryCount, stats.TopScenario)
	}
	if len(stats.Networks) != 3 || stats.Networks[0].ASNumber != "AS64500" || stats.Networks[0].Count != 2 {
		t.Errorf("network breakdown mismatch: %+v", stats.Networks)
	}
	if len(stats.Sources) != 3 || stats.Sources[0].Source != "1.1.1.1" || stats.Sources[0].Count != 2 {
		t.Errorf("source breakdown mismatch: %+v", stats.Sources)
	}
	if len(stats.ActiveDecisionTypes) != 2 || stats.ActiveDecisionTypes[0].Type != "ban" || stats.ActiveDecisionTypes[0].Count != 1 {
		t.Errorf("decision types mismatch: %+v", stats.ActiveDecisionTypes)
	}
	if len(stats.ActiveDecisionOrigins) != 2 || stats.ActiveDecisions != 2 {
		t.Errorf("decision origins/total mismatch: total=%d origins=%+v", stats.ActiveDecisions, stats.ActiveDecisionOrigins)
	}
}

func TestGetCrowdSecStatsAt_TopBreakdownsReportOther(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	alerts := make([]models.CrowdSecAlert, 0, 12)
	for i := 0; i < 12; i++ {
		alerts = append(alerts, models.CrowdSecAlert{
			AlertID:     fmt.Sprintf("top-%02d", i),
			Scenario:    fmt.Sprintf("scenario/%02d", i),
			SourceValue: fmt.Sprintf("192.0.2.%d", i+1),
			Country:     fmt.Sprintf("X%X", i),
			ASNumber:    fmt.Sprintf("AS%d", 64500+i),
			ASName:      fmt.Sprintf("Network %02d", i),
			CreatedAt:   now.Add(-time.Duration(i+1) * time.Minute).Format(time.RFC3339),
		})
	}
	if _, err := SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatalf("seed alerts: %v", err)
	}

	stats, err := getCrowdSecStatsAt(now)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if len(stats.Countries) != 10 || stats.CountriesOtherCount != 2 {
		t.Errorf("countries top/other = %d/%d, want 10/2", len(stats.Countries), stats.CountriesOtherCount)
	}
	if len(stats.Scenarios) != 10 || stats.ScenariosOtherCount != 2 {
		t.Errorf("scenarios top/other = %d/%d, want 10/2", len(stats.Scenarios), stats.ScenariosOtherCount)
	}
	if len(stats.Networks) != 10 || stats.NetworksOtherCount != 2 {
		t.Errorf("networks top/other = %d/%d, want 10/2", len(stats.Networks), stats.NetworksOtherCount)
	}
	if len(stats.Sources) != 10 || stats.SourcesOtherCount != 2 {
		t.Errorf("sources top/other = %d/%d, want 10/2", len(stats.Sources), stats.SourcesOtherCount)
	}
}

func TestGetCrowdSecStatsAt_EmptyUsesNonNilCollections(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	stats, err := getCrowdSecStatsAt(time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Hourly) != 24 || stats.Countries == nil || stats.Scenarios == nil || stats.Networks == nil || stats.Sources == nil ||
		stats.ActiveDecisionTypes == nil || stats.ActiveDecisionOrigins == nil {
		t.Fatalf("empty stats must expose 24 buckets and non-nil arrays: %+v", stats)
	}
	if stats.DecisionActionRatePercent != 0 {
		t.Errorf("empty decision action rate = %v, want 0", stats.DecisionActionRatePercent)
	}
}

func TestGetCrowdSecAlerts_LimitClamped(t *testing.T) {
	initCrowdSecAlertsTestDB(t)
	now := time.Now().UTC()
	alerts := []models.CrowdSecAlert{
		{AlertID: "x1", Scenario: "s", CreatedAt: now.Format(time.RFC3339)},
		{AlertID: "x2", Scenario: "s", CreatedAt: now.Format(time.RFC3339)},
	}
	if _, err := SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// limit 1 returns one row.
	got, err := GetCrowdSecAlerts(1)
	if err != nil || len(got) != 1 {
		t.Fatalf("limit 1: got %d rows, err %v", len(got), err)
	}
	// limit 0 → default.
	got, err = GetCrowdSecAlerts(0)
	if err != nil || len(got) != 2 {
		t.Fatalf("default limit: got %d rows, err %v", len(got), err)
	}
}
