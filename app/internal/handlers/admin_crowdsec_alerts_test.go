package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"status/app/internal/database"
	"status/app/internal/models"
)

func TestGetCrowdSecAlerts_ReturnsNonNilArray(t *testing.T) {
	initCrowdSecHandlerDB(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/admin/crowdsec/alerts", nil)
	HandleGetCrowdSecAlerts().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), ": null") {
		t.Errorf("null alerts array: %s", recorder.Body.String())
	}
}

func TestGetCrowdSecAlerts_SeedsAndLists(t *testing.T) {
	initCrowdSecHandlerDB(t)
	now := time.Now().UTC()
	alerts := []models.CrowdSecAlert{
		{AlertID: "9", Scenario: "crowdsecurity/ssh-bf", SourceValue: "1.2.3.4", Country: "CN", CreatedAt: now.Format(time.RFC3339), HasDecision: true},
	}
	if _, err := database.SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/admin/crowdsec/alerts?limit=10", nil)
	HandleGetCrowdSecAlerts().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	var response struct {
		Alerts []models.CrowdSecAlert `json:"alerts"`
		Count  int                    `json:"count"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Count != 1 || len(response.Alerts) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Alerts[0].Country != "CN" || !response.Alerts[0].HasDecision {
		t.Errorf("alert roundtrip mismatch: %+v", response.Alerts[0])
	}
}

func TestGetCrowdSecAlerts_CompactOmitsUnusedTextAndKeepsDashboardFields(t *testing.T) {
	initCrowdSecHandlerDB(t)
	now := time.Now().UTC()
	lat, lng := 51.5, -0.12
	alert := models.CrowdSecAlert{
		AlertID: "compact-1", Scenario: "crowdsecurity/ssh-bf",
		Message: "long raw LAPI diagnostic text", SourceValue: "1.2.3.4",
		Country: "GB", ASNumber: "AS123", ASName: "Example Network",
		Latitude: &lat, Longitude: &lng, EventsCount: 17,
		StartAt:   now.Add(-time.Minute).Format(time.RFC3339),
		CreatedAt: now.Format(time.RFC3339), HasDecision: true, Simulated: true,
	}
	if _, err := database.SyncCrowdSecAlerts([]models.CrowdSecAlert{alert}, now); err != nil {
		t.Fatal(err)
	}
	read := func(path string) map[string]json.RawMessage {
		t.Helper()
		recorder := httptest.NewRecorder()
		HandleGetCrowdSecAlerts().ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != 200 {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var response struct {
			Alerts []map[string]json.RawMessage `json:"alerts"`
			Count  int                          `json:"count"`
		}
		if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.Count != 1 || len(response.Alerts) != 1 {
			t.Fatalf("unexpected alerts response: %+v", response)
		}
		return response.Alerts[0]
	}
	compact := read("/api/admin/crowdsec/alerts?compact=true")
	for _, field := range []string{"message", "start_at"} {
		if _, ok := compact[field]; ok {
			t.Errorf("compact response included unused %s", field)
		}
	}
	for _, field := range []string{"alert_id", "scenario", "source_value", "country", "as_number", "as_name",
		"latitude", "longitude", "events_count", "created_at", "has_decision", "simulated"} {
		if _, ok := compact[field]; !ok {
			t.Errorf("compact response omitted dashboard field %s", field)
		}
	}
	full := read("/api/admin/crowdsec/alerts")
	if _, ok := full["message"]; !ok {
		t.Error("default response no longer includes message")
	}
	if _, ok := full["start_at"]; !ok {
		t.Error("default response no longer includes start_at")
	}
}

func TestGetCrowdSecStats_EndpointShape(t *testing.T) {
	initCrowdSecHandlerDB(t)
	now := time.Now().UTC()
	alerts := []models.CrowdSecAlert{
		{AlertID: "1", Scenario: "crowdsecurity/ssh-bf", Country: "US", CreatedAt: now.Format(time.RFC3339), HasDecision: true},
		{AlertID: "2", Scenario: "crowdsecurity/ssh-bf", Country: "US", CreatedAt: now.Format(time.RFC3339)},
	}
	if _, err := database.SyncCrowdSecAlerts(alerts, now); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/admin/crowdsec/stats", nil)
	HandleGetCrowdSecStats().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var stats models.CrowdSecStats
	if err := json.NewDecoder(recorder.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.Alerts24h != 2 {
		t.Errorf("alerts_24h = %d, want 2", stats.Alerts24h)
	}
	if stats.TopCountry != "US" {
		t.Errorf("top_country = %q, want US", stats.TopCountry)
	}
	if len(stats.Countries) != 1 || stats.Countries[0].Count != 2 {
		t.Errorf("countries = %+v", stats.Countries)
	}
	if stats.DecisionActionRatePercent != 50 {
		t.Errorf("decision_action_rate_percent = %v, want 50", stats.DecisionActionRatePercent)
	}
	if stats.WindowHours != 24 || len(stats.Hourly) != 24 {
		t.Errorf("rolling window shape = %dh/%d buckets, want 24/24", stats.WindowHours, len(stats.Hourly))
	}
	if stats.Networks == nil || stats.Sources == nil || stats.ActiveDecisionTypes == nil || stats.ActiveDecisionOrigins == nil {
		t.Errorf("new stats arrays must be non-nil: %+v", stats)
	}
}

func TestGetCrowdSecStats_EmptyIsZeroValued(t *testing.T) {
	initCrowdSecHandlerDB(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/admin/crowdsec/stats", nil)
	HandleGetCrowdSecStats().ServeHTTP(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"active_decisions":0`) {
		t.Errorf("unexpected empty stats shape: %s", body)
	}
	for _, field := range []string{"hourly", "countries", "scenarios", "networks", "sources", "active_decision_types", "active_decision_origins"} {
		if strings.Contains(body, `"`+field+`":null`) {
			t.Errorf("%s must marshal as an array, got: %s", field, body)
		}
	}
}
