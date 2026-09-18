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
	// countries/scenarios slices marshal as null when absent — the UI must
	// treat that as empty (JS `stats.countries || []` handles it).
	if !strings.Contains(body, `"active_decisions":0`) {
		t.Errorf("unexpected empty stats shape: %s", body)
	}
}
