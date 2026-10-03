package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"strings"
	"testing"
	"time"
)

func postBanner(t *testing.T, body string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	HandleCreateStatusAlert().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/status-alerts", strings.NewReader(body)))
	var created struct {
		ID string `json:"id"`
	}
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
	}
	return recorder, created.ID
}

func putBanner(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	HandleUpdateStatusAlert().ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/admin/status-alerts", strings.NewReader(body)))
	return recorder
}

func findAlert(alerts []models.StatusAlert, id string) *models.StatusAlert {
	for i := range alerts {
		if alerts[i].ID == id {
			return &alerts[i]
		}
	}
	return nil
}

func adminAlertAt(t *testing.T, now time.Time, id string) *models.StatusAlert {
	t.Helper()
	alerts, err := getAdminStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	return findAlert(alerts, id)
}

func TestScheduledBannerShowsOnlyWhileLive(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Now().UTC()
	start, end := now.Add(time.Hour), now.Add(2*time.Hour)
	recorder, id := postBanner(t, `{"message":"Later","level":"info","starts_at":"`+start.Format(time.RFC3339)+`","ends_at":"`+end.Format(time.RFC3339)+`"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", recorder.Code, recorder.Body)
	}

	for _, step := range []struct {
		at    time.Time
		state string
	}{
		{now, "scheduled"},
		{now.Add(90 * time.Minute), "live"},
		{now.Add(3 * time.Hour), "ended"},
	} {
		if got := adminAlertAt(t, step.at, id); got == nil || got.State != step.state {
			t.Fatalf("at %v admin sees %+v, want state %s", step.at, got, step.state)
		}
		visible := findAlert(mustPublicStatusAlerts(t, step.at), id) != nil
		if visible != (step.state == "live") {
			t.Fatalf("at %v public visible = %v in state %s", step.at, visible, step.state)
		}
	}
}

func TestBannerValidationExplainsTheProblem(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Now().UTC()
	later := now.Add(time.Hour).Format(time.RFC3339)
	for _, test := range []struct{ body, want string }{
		{`{"message":" ","level":"info"}`, "Write a message"},
		{`{"message":"x","level":"loud"}`, "Type must be"},
		{`{"message":"x","service_keys":["ghost"]}`, `Unknown service "ghost"`},
		{`{"message":"x","starts_at":"tomorrow"}`, "Start: use a date"},
		{`{"message":"x","ends_at":"` + now.Add(-time.Minute).Format(time.RFC3339) + `"}`, "already passed"},
		{`{"message":"x","starts_at":"` + later + `","ends_at":"` + later + `"}`, "end must be after the start"},
	} {
		recorder, _ := postBanner(t, test.body)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), test.want) {
			t.Fatalf("%s: %d %q, want 400 with %q", test.body, recorder.Code, recorder.Body.String(), test.want)
		}
	}
}

func TestBannerOnSeveralServicesShowsOnlyPublicOnes(t *testing.T) {
	initAutomaticBannerTest(t)
	createAutomaticBannerService(t, "plex", "Plex")
	createAutomaticBannerService(t, "sonarr", "Sonarr")
	createAutomaticBannerService(t, "backup", "Backup")
	if err := database.UpdateServiceVisibility(mustServiceID(t, "backup"), false); err != nil {
		t.Fatal(err)
	}
	_, shared := postBanner(t, `{"message":"Media is slow","service_keys":["plex"," sonarr ","plex","backup"]}`)
	_, hiddenOnly := postBanner(t, `{"message":"Backups paused","service_keys":["backup"]}`)

	now := time.Now()
	admin := adminAlertAt(t, now, shared)
	if admin == nil || !reflect.DeepEqual(admin.ServiceKeys, []string{"plex", "sonarr", "backup"}) {
		t.Fatalf("admin banner = %+v", admin)
	}
	public := mustPublicStatusAlerts(t, now)
	got := findAlert(public, shared)
	if got == nil || !reflect.DeepEqual(got.ServiceKeys, []string{"plex", "sonarr"}) || got.ServiceKey != "plex" {
		t.Fatalf("public banner = %+v", got)
	}
	if findAlert(public, hiddenOnly) != nil {
		t.Fatal("a banner for a hidden service only reached visitors")
	}
}

func TestEndingAndClearingBanners(t *testing.T) {
	initAutomaticBannerTest(t)
	_, live := postBanner(t, `{"message":"Now","level":"warning"}`)
	later := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	_, scheduled := postBanner(t, `{"message":"Later","starts_at":"`+later+`"}`)

	if recorder := putBanner(t, `{"id":"`+scheduled+`","message":"Later","end_now":true}`); recorder.Code != http.StatusBadRequest {
		t.Fatalf("ending a banner that hasn't started = %d", recorder.Code)
	}
	if recorder := putBanner(t, `{"id":"`+live+`","message":"Now","level":"warning","end_now":true}`); recorder.Code != http.StatusOK {
		t.Fatalf("end now = %d: %s", recorder.Code, recorder.Body)
	}
	if got := adminAlertAt(t, time.Now().Add(time.Second), live); got == nil || got.State != "ended" {
		t.Fatalf("ended banner = %+v", got)
	}

	recorder := httptest.NewRecorder()
	HandleDeleteStatusAlert().ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/status-alerts?ended=1", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete ended = %d", recorder.Code)
	}
	if adminAlertAt(t, time.Now(), live) != nil || adminAlertAt(t, time.Now(), scheduled) == nil {
		t.Fatal("delete ended removed the wrong banners")
	}
}

func TestOlderClientsKeepABannersSchedule(t *testing.T) {
	initAutomaticBannerTest(t)
	end := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	_, id := postBanner(t, `{"message":"Before","ends_at":"`+end.Format(time.RFC3339)+`"}`)
	// The previous editor sent only the message, level and one service.
	if recorder := putBanner(t, `{"id":"`+id+`","message":"After","level":"error","service_key":""}`); recorder.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", recorder.Code, recorder.Body)
	}
	got := adminAlertAt(t, time.Now(), id)
	if got == nil || got.Message != "After" || got.EndsAt != end.Format(time.RFC3339) {
		t.Fatalf("banner = %+v", got)
	}
}

func TestMaintenanceForChosenServicesPausesOnlyThem(t *testing.T) {
	initAutomaticBannerTest(t)
	for _, key := range []string{"nas", "plex"} {
		if _, err := database.CreateService(&models.ServiceConfig{
			Key: key, Name: strings.ToUpper(key), URL: "http://" + key + ".test", ServiceType: "custom",
			CheckType: "always_up", CheckInterval: 60, Timeout: 2, ExpectedMin: 200, ExpectedMax: 299, Visible: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	window := activeScheduleAt(now)
	window.ServiceKeys = []string{"nas"}
	if err := database.SaveMaintenanceSchedule(window); err != nil {
		t.Fatal(err)
	}

	payload, err := buildPublicLiveStatus(now)
	if err != nil {
		t.Fatal(err)
	}
	if !payload.Status["nas"].Maintenance || payload.Status["plex"].Maintenance || !payload.Status["plex"].OK {
		t.Fatalf("live status = %+v", payload.Status)
	}

	for _, key := range []string{"nas", "plex"} {
		if err := database.RecordServiceOutageState(key, true, true, now.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	outage := onlyAutomaticAlert(t, mustPublicStatusAlerts(t, now))
	if !strings.Contains(outage.Message, "PLEX is currently unavailable") {
		t.Fatalf("the outage banner should leave out the service in maintenance: %q", outage.Message)
	}
}

func TestUpcomingMaintenanceIsAnnounced(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Now().UTC()
	start := now.Add(30 * time.Minute).Truncate(time.Minute)
	schedule := &models.MaintenanceSchedule{
		ID: "disk", Name: "NAS disk replacement", Message: "The NAS is getting a new disk.", Level: "warning",
		ScheduleType: "once", StartsAt: start.Format("2006-01-02T15:04"), EndsAt: start.Add(time.Hour).Format("2006-01-02T15:04"),
		Timezone: "UTC", Enabled: true, SuppressMonitoring: true, NoticeMinutes: 60,
	}
	if err := maintenance.ValidateSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveMaintenanceSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	got := findAlert(mustPublicStatusAlerts(t, now), "upcoming:disk")
	if got == nil || got.Kind != "maintenance_upcoming" || got.Level != "info" ||
		got.StartsAt != start.Format(time.RFC3339) || got.ScheduleID != "disk" ||
		!strings.Contains(got.Message, "NAS disk replacement") {
		t.Fatalf("upcoming banner = %+v", got)
	}

	schedule.NoticeMinutes = 0
	if err := database.SaveMaintenanceSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	if findAlert(mustPublicStatusAlerts(t, now), "upcoming:disk") != nil {
		t.Fatal("announced without a notice period")
	}
}

func TestSavingAWindowResetsItsBannerChanges(t *testing.T) {
	initAutomaticBannerTest(t)
	now := time.Now().UTC()
	window := activeScheduleAt(now)
	if err := database.SaveMaintenanceSchedule(window); err != nil {
		t.Fatal(err)
	}
	banner := findAlert(mustPublicStatusAlerts(t, now), "scheduled:active")
	if banner == nil {
		t.Fatal("maintenance banner missing")
	}
	if err := database.SaveStatusAlertOverride(database.StatusAlertOverride{AlertID: banner.ID, OccurrenceAt: banner.CreatedAt, Hidden: true}); err != nil {
		t.Fatal(err)
	}
	if findAlert(mustPublicStatusAlerts(t, now), "scheduled:active") != nil {
		t.Fatal("hidden banner still public")
	}

	body, _ := json.Marshal(map[string]any{
		"id": "active", "name": window.Name, "message": "Rescheduled work", "level": "warning",
		"schedule_type": "weekly", "weekdays": []int{window.Weekday}, "start_time": window.StartTime,
		"duration_minutes": window.DurationMinutes, "timezone": "UTC", "enabled": true, "suppress_monitoring": true,
	})
	recorder := httptest.NewRecorder()
	HandleMaintenanceSchedules().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/maintenance-schedules", strings.NewReader(string(body))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", recorder.Code, recorder.Body)
	}
	if got := findAlert(mustPublicStatusAlerts(t, now), "scheduled:active"); got == nil || got.Message != "Rescheduled work" {
		t.Fatalf("banner after editing the window = %+v", got)
	}
}

func TestMaintenanceListShowsWhatIsRunningAndNext(t *testing.T) {
	initMaintenanceHandlerDB(t) // keeps the seeded Monday window
	if err := database.SaveMaintenanceSchedule(activeScheduleAt(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	HandleMaintenanceSchedules().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/maintenance-schedules", nil))
	var views []maintenanceScheduleView
	if err := json.Unmarshal(recorder.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	byID := map[string]maintenanceScheduleView{}
	for _, view := range views {
		byID[view.ID] = view
	}
	if running := byID["active"]; !running.Active || running.ActiveEndsAt == "" || running.NextStartsAt == "" {
		t.Fatalf("running window = %+v", running)
	}
	if monday := byID["weekly-monday-maintenance"]; monday.NextStartsAt == "" || monday.Problem != "" {
		t.Fatalf("Monday window = %+v", monday)
	}

	recorder = httptest.NewRecorder()
	HandleMaintenanceSchedules().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/maintenance-schedules",
		strings.NewReader(`{"name":"Ghost","message":"m","level":"info","schedule_type":"daily","start_time":"03:00","duration_minutes":5,"timezone":"UTC","enabled":true,"service_keys":["ghost"]}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `Unknown service "ghost"`) {
		t.Fatalf("unknown service = %d %q", recorder.Code, recorder.Body.String())
	}
}

func mustServiceID(t *testing.T, key string) int {
	t.Helper()
	service, err := database.GetServiceByKey(key)
	if err != nil || service == nil {
		t.Fatalf("service %s: %v", key, err)
	}
	return service.ID
}
