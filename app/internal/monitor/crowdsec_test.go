package monitor

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"status/app/internal/crypto"
	"status/app/internal/database"
	"status/app/internal/models"
)

// initCrowdSecMonitorDB wires the global DB with the crowdsec tables and
// a fixed crypto key for secret roundtrips.
func initCrowdSecMonitorDB(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatalf("init db: %v", err)
	}
	crypto.SetKey([]byte("test-encryption-key-for-crowdsec"))
}

// newLAPIServer serves a fake LAPI with the given alerts handler behavior.
// Login always succeeds; decisions return an empty set (the test focuses on
// the alerts feed path).
func newLAPIServer(t *testing.T, alertsHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/watchers/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"token":"test-token","expire":"2099-01-01T00:00:00Z"}`)
	})
	mux.HandleFunc("/v1/decisions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	})
	mux.HandleFunc("/v1/alerts", alertsHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestPollCrowdSec_AlertsFailureSurfacesInState(t *testing.T) {
	initCrowdSecMonitorDB(t)
	// The alerts endpoint answers 200 with a truncated body — the real-world
	// failure mode observed against LAPIs that cannot serve large pages.
	srv := newLAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1`) // truncated JSON
	})

	if err := database.SaveCrowdSecConfig(testCrowdSecConfig(srv.URL)); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// The poll must NOT fail: decisions still sync (resilience path)…
	if err := PollCrowdSec(context.Background()); err != nil {
		t.Fatalf("poll should survive an alerts failure: %v", err)
	}

	// …but the alerts error MUST be visible in sync state, not erased by
	// the successful decisions save that follows it.
	state, err := database.GetCrowdSecState()
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state == nil {
		t.Fatal("expected state row, got nil")
	}
	if state.LastError == "" {
		t.Fatal("alerts failure must surface in last_error, got empty (masked by decisions success)")
	}
}

func TestPollCrowdSec_AlertsSuccessStoresFeed(t *testing.T) {
	initCrowdSecMonitorDB(t)
	srv := newLAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "100" {
			// Guard: the production page size must stay at the LAPI-safe 100.
			t.Errorf("alerts limit = %q, want 100 (large single-page limits break real LAPIs)", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1001,"created_at":"2026-09-20T10:00:00Z","scenario":"crowdsecurity/ssh-bf",`+
			`"message":"ssh-bf from 1.2.3.4","events_count":7,"source":{"scope":"Ip","value":"1.2.3.4","cn":"CN","latitude":32.06,"longitude":118.78},`+
			`"decisions":[{"duration":"3h","id":5001,"origin":"crowdsec","scenario":"crowdsecurity/ssh-bf","scope":"Ip","type":"ban","value":"1.2.3.4"}]}]`)
	})

	if err := database.SaveCrowdSecConfig(testCrowdSecConfig(srv.URL)); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := PollCrowdSec(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	alerts, err := database.GetCrowdSecAlerts(10)
	if err != nil {
		t.Fatalf("alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("stored alerts = %d, want 1", len(alerts))
	}
	a := alerts[0]
	if a.Scenario != "crowdsecurity/ssh-bf" || a.Country != "CN" || !a.HasDecision {
		t.Errorf("alert roundtrip mismatch: %+v", a)
	}
	if a.Latitude == nil || a.Longitude == nil {
		t.Fatalf("alert geo lost: %+v", a)
	}
	// float32 → float64 widens with rounding: compare by tolerance.
	if math.Abs(*a.Latitude-32.06) > 1e-4 || math.Abs(*a.Longitude-118.78) > 1e-4 {
		t.Errorf("alert geo mismatch: (%v, %v)", *a.Latitude, *a.Longitude)
	}

	// And the sync state must be clean.
	state, err := database.GetCrowdSecState()
	if err != nil || state == nil {
		t.Fatalf("state: %v %v", state, err)
	}
	if state.LastError != "" {
		t.Errorf("unexpected sync error: %q", state.LastError)
	}
}

// testCrowdSecConfig builds an enabled config with machine credentials and
// a bouncer key pointed at the test server.
func testCrowdSecConfig(url string) *models.CrowdSecConfig {
	return &models.CrowdSecConfig{
		Enabled:         true,
		LAPIURL:         url,
		MachineID:       "testmachine",
		MachinePassword: "test-pass",
		BouncerAPIKey:   "test-bouncer-key",
		PollIntervalS:   30,
	}
}
