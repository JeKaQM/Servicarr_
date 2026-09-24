package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"status/app/internal/crowdsec"
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

	// The successful decisions feed must still be retained, while the partial
	// failure is returned so manual Sync Now cannot falsely report success.
	if err := PollCrowdSec(context.Background()); err == nil {
		t.Fatal("poll must report the alerts failure")
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
	if state.LastSync.IsZero() {
		t.Fatal("successful decisions sync must be preserved on partial failure")
	}
}

func TestPollCrowdSec_DecisionsFailureStillStoresAlerts(t *testing.T) {
	initCrowdSecMonitorDB(t)
	createdAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/watchers/login", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"token":"test-token","expire":"2099-01-01T00:00:00Z"}`)
	})
	mux.HandleFunc("/v1/decisions", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"temporary failure"}`, http.StatusInternalServerError)
	})
	mux.HandleFunc("/v1/alerts", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"id":2501,"created_at":%q,"scenario":"crowdsecurity/http-probing","source":{"value":"2.5.0.1"}}]`, createdAt)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := database.SaveCrowdSecConfig(testCrowdSecConfig(srv.URL)); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := PollCrowdSec(context.Background()); err == nil {
		t.Fatal("poll must report the decisions failure")
	}
	alerts, err := database.GetCrowdSecAlerts(10)
	if err != nil {
		t.Fatalf("alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].AlertID != "2501" {
		t.Fatalf("successful alerts feed was discarded: %+v", alerts)
	}
	state, err := database.GetCrowdSecState()
	if err != nil || state == nil || state.LastError == "" {
		t.Fatalf("partial failure was not persisted: state=%+v err=%v", state, err)
	}
}

func TestPollCrowdSec_AlertsSuccessStoresFeed(t *testing.T) {
	initCrowdSecMonitorDB(t)
	createdAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	srv := newLAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "100" {
			// Guard: the production page size must stay at the LAPI-safe 100.
			t.Errorf("alerts limit = %q, want 100 (large single-page limits break real LAPIs)", r.URL.Query().Get("limit"))
		}
		if duration, err := time.ParseDuration(r.URL.Query().Get("since")); err != nil || duration <= 0 {
			t.Errorf("alerts since must be a positive duration: %q", r.URL.Query().Get("since"))
		}
		if r.URL.Query().Get("include_capi") != "false" {
			t.Errorf("alerts include_capi = %q, want false", r.URL.Query().Get("include_capi"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{"id":1001,"created_at":%q,"scenario":"crowdsecurity/ssh-bf",`+
			`"message":"ssh-bf from 1.2.3.4","events_count":7,"source":{"scope":"Ip","value":"1.2.3.4","cn":"CN","latitude":32.06,"longitude":118.78},`+
			`"decisions":[{"duration":"3h","id":5001,"origin":"crowdsec","scenario":"crowdsecurity/ssh-bf","scope":"Ip","type":"ban","value":"1.2.3.4"}]}]`, createdAt)
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

func TestPollCrowdSec_MachineOnlySyncsAlertsAndReusesJWT(t *testing.T) {
	initCrowdSecMonitorDB(t)
	createdAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	var loginCalls atomic.Int64
	var decisionsCalls atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/watchers/login", func(w http.ResponseWriter, r *http.Request) {
		loginCalls.Add(1)
		fmt.Fprint(w, `{"token":"test-token","expire":"2099-01-01T00:00:00Z"}`)
	})
	mux.HandleFunc("/v1/decisions", func(w http.ResponseWriter, r *http.Request) {
		decisionsCalls.Add(1)
		http.Error(w, "machine-only config must not fetch decisions", http.StatusInternalServerError)
	})
	mux.HandleFunc("/v1/alerts", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("since") == "" || r.URL.Query().Get("include_capi") != "false" {
			t.Errorf("unexpected alerts query: %s", r.URL.RawQuery)
		}
		fmt.Fprintf(w, `[{"id":2001,"created_at":%q,"scenario":"crowdsecurity/http-probing","source":{"value":"2.3.4.5"}}]`, createdAt)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := testCrowdSecConfig(srv.URL)
	cfg.BouncerAPIKey = ""
	if err := database.SaveCrowdSecConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := PollCrowdSec(context.Background()); err != nil {
		t.Fatalf("first machine-only poll: %v", err)
	}
	if err := PollCrowdSec(context.Background()); err != nil {
		t.Fatalf("second machine-only poll: %v", err)
	}

	if decisionsCalls.Load() != 0 {
		t.Errorf("machine-only poll made %d decisions requests, want 0", decisionsCalls.Load())
	}
	if loginCalls.Load() != 1 {
		t.Errorf("JWT was not reused across polls: got %d logins, want 1", loginCalls.Load())
	}
	alerts, err := database.GetCrowdSecAlerts(10)
	if err != nil {
		t.Fatalf("alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].AlertID != "2001" {
		t.Fatalf("machine-only alerts were not stored: %+v", alerts)
	}
}

func TestPollCrowdSec_MachineOnlyFailureDoesNotClaimSuccessfulSync(t *testing.T) {
	initCrowdSecMonitorDB(t)
	srv := newLAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":1`) // truncated JSON
	})
	cfg := testCrowdSecConfig(srv.URL)
	cfg.BouncerAPIKey = ""
	if err := database.SaveCrowdSecConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := PollCrowdSec(context.Background()); err == nil {
		t.Fatal("machine-only poll must report the alerts failure")
	}
	state, err := database.GetCrowdSecState()
	if err != nil || state == nil {
		t.Fatalf("state: %v %v", state, err)
	}
	if !state.LastSync.IsZero() {
		t.Errorf("failed machine-only poll claimed successful sync at %v", state.LastSync)
	}
}

func TestPollCrowdSec_BouncerOnlySyncsDecisionsWithoutMachineLogin(t *testing.T) {
	initCrowdSecMonitorDB(t)
	var machineCalls atomic.Int64
	now := time.Now().UTC()
	if _, err := database.SyncCrowdSecAlerts([]models.CrowdSecAlert{{
		AlertID:   "old-machine-alert",
		CreatedAt: now.Format(time.RFC3339),
	}}, now); err != nil {
		t.Fatalf("seed old alerts feed: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/decisions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-bouncer-key" {
			t.Errorf("missing bouncer authentication header")
		}
		fmt.Fprint(w, `[{"id":3001,"duration":"1h","type":"ban","scope":"Ip","value":"3.4.5.6"}]`)
	})
	mux.HandleFunc("/v1/watchers/login", func(w http.ResponseWriter, r *http.Request) {
		machineCalls.Add(1)
		http.Error(w, "unexpected machine login", http.StatusInternalServerError)
	})
	mux.HandleFunc("/v1/alerts", func(w http.ResponseWriter, r *http.Request) {
		machineCalls.Add(1)
		http.Error(w, "unexpected alerts fetch", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := testCrowdSecConfig(srv.URL)
	cfg.MachineID = ""
	cfg.MachinePassword = ""
	if err := database.SaveCrowdSecConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := PollCrowdSec(context.Background()); err != nil {
		t.Fatalf("bouncer-only poll: %v", err)
	}
	if machineCalls.Load() != 0 {
		t.Errorf("bouncer-only poll made %d machine requests, want 0", machineCalls.Load())
	}
	decisions, err := database.GetCrowdSecDecisions(false, 10)
	if err != nil {
		t.Fatalf("decisions: %v", err)
	}
	if len(decisions) != 1 || decisions[0].DecisionID != "3001" {
		t.Fatalf("bouncer-only decisions were not stored: %+v", decisions)
	}
	alerts, err := database.GetCrowdSecAlerts(10)
	if err != nil {
		t.Fatalf("alerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("bouncer-only transition left stale machine alerts: %+v", alerts)
	}
	var archived int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts`).Scan(&archived); err != nil || archived != 1 {
		t.Fatalf("credential removal erased archived history: count=%d err=%v", archived, err)
	}
}

func TestCrowdSecHistoryBackfillsBeyondFirstPage(t *testing.T) {
	initCrowdSecMonitorDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	remote := make([]crowdsec.Alert, 235)
	for i := range remote {
		id := int64(i + 1)
		// Created order intentionally differs from start order, as real LAPI
		// sorts by creation but applies since/until to scenario start time.
		remote[i] = crowdsec.Alert{ID: &id,
			CreatedAt: crowdsec.Time{Time: now.Add(-time.Duration(i+1) * time.Minute)},
			StartAt:   crowdsec.Time{Time: now.Add(-time.Duration((i*73)%235+1) * 15 * time.Minute)}}
	}
	var calls atomic.Int64
	srv := newLAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("offset") != "" {
			t.Error("alerts do not support offset")
		}
		since, err := time.ParseDuration(r.URL.Query().Get("since"))
		if err != nil {
			t.Error(err)
		}
		until, _ := time.ParseDuration(r.URL.Query().Get("until"))
		anchor := time.Now().UTC()
		rows := []crowdsec.Alert{}
		for _, alert := range remote {
			if !alert.StartAt.Before(anchor.Add(-since)) && !alert.StartAt.After(anchor.Add(-until)) {
				rows = append(rows, alert)
			}
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt.Time) })
		if len(rows) > crowdsecAlertsPageSize {
			rows = rows[:crowdsecAlertsPageSize]
		}
		_ = json.NewEncoder(w).Encode(rows)
	})
	if err := database.SaveCrowdSecConfig(testCrowdSecConfig(srv.URL)); err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 40; cycle++ {
		before := calls.Load()
		if err := PollCrowdSec(context.Background()); err != nil {
			t.Fatal(err)
		}
		if used := calls.Load() - before; used > crowdsecAlertRequestBudget {
			t.Fatalf("request budget exceeded: %d", used)
		}
		state, err := database.GetCrowdSecHistoryState()
		if err != nil {
			t.Fatal(err)
		}
		if state.Complete {
			var count int
			if err := database.DB.QueryRow(`SELECT COUNT(*) FROM crowdsec_alerts`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != len(remote) {
				t.Fatalf("backfill lost records: got %d want %d", count, len(remote))
			}
			if state.Truncated {
				t.Fatal("ordinary pagination falsely marked truncated")
			}
			return
		}
	}
	t.Fatal("backfill failed to converge across bounded polls")
}

func TestCrowdSecDecisionPagination(t *testing.T) {
	for _, mode := range []string{"normal", "repeated", "overlap"} {
		t.Run(mode, func(t *testing.T) {
			var calls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
				if mode == "repeated" {
					offset = 0
				}
				if mode == "overlap" && offset > 0 {
					offset--
				}
				rows := []crowdsec.Decision{}
				for i := offset; i < min(offset+500, 650); i++ {
					id := int64(i + 1)
					rows = append(rows, crowdsec.Decision{ID: &id, Value: fmt.Sprintf("192.0.2.%d", i), Duration: "1h"})
				}
				_ = json.NewEncoder(w).Encode(rows)
			}))
			defer srv.Close()
			rows, truncated, err := fetchDecisionsSnapshot(context.Background(), crowdsec.NewClient(crowdsec.Config{BaseURL: srv.URL, BouncerKey: "test"}))
			want := 650
			if mode == "repeated" {
				want = 500
			}
			if err != nil || len(rows) != want || truncated != (mode != "normal") || calls != 2 {
				t.Fatalf("rows=%d truncated=%v calls=%d err=%v", len(rows), truncated, calls, err)
			}
		})
	}
}

func TestCrowdSecCompletedHistoryStillCollectsLateArrivingAlerts(t *testing.T) {
	initCrowdSecMonitorDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	srv := newLAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		since, _ := time.ParseDuration(r.URL.Query().Get("since"))
		// This alert arrived after the prior poll, but its scenario began an
		// hour earlier. Querying only since the last poll would lose it.
		if since < time.Hour {
			fmt.Fprint(w, `[]`)
			return
		}
		fmt.Fprintf(w, `[{"id":9999,"start_at":%q,"created_at":%q}]`, now.Add(-time.Hour).Format(time.RFC3339), now.Add(-10*time.Second).Format(time.RFC3339))
	})
	cfg := testCrowdSecConfig(srv.URL)
	if err := database.SaveCrowdSecConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveCrowdSecHistoryState(database.CrowdSecHistoryState{
		Complete: true, CursorAt: now.Add(-database.CrowdSecArchiveMaxAge).Format(time.RFC3339), LastLiveAt: now.Add(-time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	if err := PollCrowdSec(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := database.GetCrowdSecAlerts(10)
	if err != nil || len(rows) != 1 || rows[0].AlertID != "9999" {
		t.Fatalf("late alert missing: %+v err=%v", rows, err)
	}
}

func TestCrowdSecDecisionPageFailurePreservesSnapshot(t *testing.T) {
	initCrowdSecMonitorDB(t)
	now := time.Now().UTC()
	_, err := database.SyncCrowdSecDecisions([]models.CrowdSecDecision{{DecisionID: "old", Value: "192.0.2.1", ExpiresAt: now.Add(time.Hour).Format(time.RFC3339)}}, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "" {
			http.Error(w, "unavailable", 500)
			return
		}
		rows := make([]crowdsec.Decision, 500)
		for i := range rows {
			id := int64(i + 1)
			rows[i] = crowdsec.Decision{ID: &id, Value: "192.0.2.2", Duration: "1h"}
		}
		_ = json.NewEncoder(w).Encode(rows)
	}))
	defer srv.Close()
	cfg := testCrowdSecConfig(srv.URL)
	cfg.MachineID, cfg.MachinePassword = "", ""
	if err := database.SaveCrowdSecConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := PollCrowdSec(context.Background()); err == nil {
		t.Fatal("expected page failure")
	}
	rows, err := database.GetCrowdSecDecisions(false, 10)
	if err != nil || len(rows) != 1 || rows[0].DecisionID != "old" {
		t.Fatalf("partial snapshot replaced previous data: %+v, %v", rows, err)
	}
}

func TestPollCrowdSec_ConcurrentCallsAreSerialized(t *testing.T) {
	initCrowdSecMonitorDB(t)
	var inFlight atomic.Int64
	var maxInFlight atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/decisions" {
			http.NotFound(w, r)
			return
		}
		current := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			previous := maxInFlight.Load()
			if current <= previous || maxInFlight.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	cfg := testCrowdSecConfig(srv.URL)
	cfg.MachineID = ""
	cfg.MachinePassword = ""
	if err := database.SaveCrowdSecConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	const callers = 4
	errCh := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- PollCrowdSec(context.Background())
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent poll: %v", err)
		}
	}
	if maxInFlight.Load() != 1 {
		t.Errorf("concurrent LAPI requests = %d, want 1", maxInFlight.Load())
	}
}

func TestCrowdSecConfigChangeNotificationCoalesces(t *testing.T) {
	// Drain a signal that may have been left by another test in this process.
	select {
	case <-CrowdSecConfigChanges():
	default:
	}

	NotifyCrowdSecConfigChanged()
	NotifyCrowdSecConfigChanged()
	select {
	case <-CrowdSecConfigChanges():
	default:
		t.Fatal("missing config-change notification")
	}
	select {
	case <-CrowdSecConfigChanges():
		t.Fatal("duplicate config changes should coalesce")
	default:
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
