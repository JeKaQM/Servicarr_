package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/monitor"
	"status/app/internal/stats"
)

func initAPIOutageTest(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	if err := stats.EnsureStatsSchema(); err != nil {
		t.Fatal(err)
	}
}

func createAPIOutageService(t *testing.T, key, name, url string) {
	t.Helper()
	if _, err := database.CreateService(&models.ServiceConfig{
		Key: key, Name: name, URL: url, ServiceType: "custom", CheckType: "http",
		CheckInterval: 60, Timeout: 2, ExpectedMin: 200, ExpectedMax: 299, Visible: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleCheckShowsFirstObservedFailureWithoutChangingAlertDebounce(t *testing.T) {
	initAPIOutageTest(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer target.Close()
	createAPIOutageService(t, "first-failure", "First Failure", target.URL)

	tracker := monitor.NewFailureTracker()
	recorder := httptest.NewRecorder()
	HandleCheck(tracker).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/check", nil))

	var payload models.LivePayload
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status["first-failure"].OK {
		t.Fatalf("first failed observation was reported as up: %+v", payload.Status["first-failure"])
	}
	if failures := tracker.Update("first-failure", false); failures != 1 {
		t.Fatalf("public check mutated alert debounce; next failure count = %d, want 1", failures)
	}
}

func TestRecentIncidentsKeepsOngoingOutageAfter24Hours(t *testing.T) {
	initAPIOutageTest(t)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	createAPIOutageService(t, "long-outage", "Long Outage", "http://example.test")
	if err := database.RecordServiceOutageState("long-outage", true, true, now.Add(-30*time.Hour)); err != nil {
		t.Fatal(err)
	}

	incidents, err := loadRecentIncidents(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 || !incidents[0].Ongoing {
		t.Fatalf("ongoing outage missing: %+v", incidents)
	}
	if incidents[0].DurationSeconds < int64((29 * time.Hour).Seconds()) {
		t.Fatalf("duration = %d, want about 30 hours", incidents[0].DurationSeconds)
	}
}

func TestDayDetailCollapsesAllFailedSamplesIntoOneOutage(t *testing.T) {
	initAPIOutageTest(t)
	createAPIOutageService(t, "all-day", "All Day", "http://example.test")
	for hour := 0; hour < 24; hour++ {
		parsed := time.Date(2026, time.September, 3, hour, 5, 0, 0, time.UTC)
		database.InsertSample(parsed, "all-day", false, 0, nil)
	}
	database.InsertSample(time.Date(2026, time.September, 3, 1, 35, 0, 0, time.UTC), "all-day", false, 0, nil)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/metrics/day-detail?key=all-day&date=2026-09-03", nil)
	HandleDayDetail().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Hours      []dayHourBucket `json:"hours"`
		DownEvents []dayDownEvent  `json:"down_events"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.DownEvents) != 1 || !response.DownEvents[0].AllDay || response.DownEvents[0].FailureCount != 25 {
		t.Fatalf("unexpected all-day events: %+v", response.DownEvents)
	}
	if response.Hours[1].DownChecks != 2 || response.Hours[12].DownChecks != 1 {
		t.Fatalf("failed checks missing from hour buckets: %+v", response.Hours)
	}
}

func TestDayDetailLimitsPartialOutageToOneEventPerHour(t *testing.T) {
	initAPIOutageTest(t)
	createAPIOutageService(t, "partial", "Partial", "http://example.test")
	samples := []struct {
		time string
		ok   bool
	}{
		{"2026-09-03T00:05:00Z", true},
		{"2026-09-03T01:05:00Z", false},
		{"2026-09-03T01:25:00Z", false},
		{"2026-09-03T02:05:00Z", false},
	}
	for _, sample := range samples {
		parsed, err := time.Parse(time.RFC3339, sample.time)
		if err != nil {
			t.Fatal(err)
		}
		database.InsertSample(parsed, "partial", sample.ok, 0, nil)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/metrics/day-detail?key=partial&date=2026-09-03", nil)
	HandleDayDetail().ServeHTTP(recorder, request)
	var response struct {
		DownEvents []dayDownEvent `json:"down_events"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.DownEvents) != 2 || response.DownEvents[0].FailureCount != 2 || response.DownEvents[1].FailureCount != 1 {
		t.Fatalf("events were not grouped hourly: %+v", response.DownEvents)
	}
}
