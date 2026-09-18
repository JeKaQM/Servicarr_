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
