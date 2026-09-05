package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"status/app/internal/database"
	"status/app/internal/maintenance"
	"status/app/internal/models"
	"status/app/internal/monitor"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func initMaintenanceHandlerDB(t *testing.T) {
	t.Helper()
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
}

func activeScheduleAt(now time.Time) *models.MaintenanceSchedule {
	return &models.MaintenanceSchedule{
		ID: "active", Name: "Active maintenance", Message: "Maintenance in progress", Level: "warning",
		Weekday: int(now.UTC().Weekday()), StartTime: now.UTC().Format("15:04"), DurationMinutes: 2,
		Timezone: "UTC", SuppressMonitoring: true, Enabled: true,
	}
}

func TestMaintenanceSchedulesHandlerListsDefault(t *testing.T) {
	initMaintenanceHandlerDB(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/maintenance-schedules", nil)
	HandleMaintenanceSchedules().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var schedules []models.MaintenanceSchedule
	if err := json.NewDecoder(recorder.Body).Decode(&schedules); err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 1 || schedules[0].ID != "weekly-monday-maintenance" {
		t.Fatalf("unexpected schedules: %+v", schedules)
	}
}

func TestMaintenanceSchedulesHandlerRejectsInvalidTimezone(t *testing.T) {
	initMaintenanceHandlerDB(t)
	body := `{"name":"Bad","message":"Maintenance","level":"warning","weekday":1,"start_time":"03:00","duration_minutes":30,"timezone":"Bad/Zone","enabled":true,"suppress_monitoring":true}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/maintenance-schedules", strings.NewReader(body))
	HandleMaintenanceSchedules().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestPublicStatusAlertsIncludesActiveSchedule(t *testing.T) {
	initMaintenanceHandlerDB(t)
	_, _ = database.DB.Exec(`DELETE FROM maintenance_schedules`)
	now := time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC)
	schedule := &models.MaintenanceSchedule{
		ID: "weekly", Name: "Weekly", Message: "Maintenance in progress", Level: "warning",
		Weekday: int(time.Monday), StartTime: "02:55", DurationMinutes: 30,
		Timezone: "Europe/London", SuppressMonitoring: true, Enabled: true,
	}
	if err := database.SaveMaintenanceSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	alerts, err := getPublicStatusAlerts(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || !alerts[0].Scheduled || alerts[0].ID != "scheduled:weekly" || alerts[0].EndsAt == "" {
		t.Fatalf("unexpected public alerts: %+v", alerts)
	}
}

func TestHandleCheckSkipsTargetDuringMaintenance(t *testing.T) {
	initMaintenanceHandlerDB(t)
	_, _ = database.DB.Exec(`DELETE FROM maintenance_schedules`)
	now := time.Now().UTC()
	if err := database.SaveMaintenanceSchedule(activeScheduleAt(now)); err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	_, err := database.CreateService(&models.ServiceConfig{
		Key: "svc", Name: "Service", URL: target.URL, ServiceType: "custom", CheckType: "http",
		CheckInterval: 60, Timeout: 2, ExpectedMin: 200, ExpectedMax: 299, Visible: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/check", nil)
	HandleCheck(monitor.NewFailureTracker()).ServeHTTP(recorder, request)
	if requests.Load() != 0 {
		t.Fatalf("target received %d checks during maintenance", requests.Load())
	}
	var payload models.LivePayload
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Status["svc"].Maintenance {
		t.Fatalf("service was not marked for maintenance: %+v", payload.Status["svc"])
	}
}

func TestIngestNowWritesNothingDuringMaintenance(t *testing.T) {
	initMaintenanceHandlerDB(t)
	_, _ = database.DB.Exec(`DELETE FROM maintenance_schedules`)
	if err := database.SaveMaintenanceSchedule(activeScheduleAt(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/ingest-now", nil)
	HandleIngestNow(monitor.NewFailureTracker()).ServeHTTP(recorder, request)

	var samples int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&samples); err != nil {
		t.Fatal(err)
	}
	if samples != 0 {
		t.Fatalf("recorded %d samples during maintenance", samples)
	}
}

func TestMaintenanceSchedulesAPIHandlesFlexibleWindows(t *testing.T) {
	initMaintenanceHandlerDB(t)
	for _, body := range []string{
		`{"id":"once","name":"Long upgrade","message":"In progress","level":"warning","schedule_type":"once","starts_at":"2026-09-15T10:00","ends_at":"2027-11-20T10:00","timezone":"Europe/London","enabled":true}`,
		`{"id":"recurring","name":"Multiweek","message":"In progress","level":"info","schedule_type":"weekly","weekdays":[1,3,5],"start_time":"01:00","duration_minutes":40000,"timezone":"UTC","enabled":true}`,
		`{"id":"open","name":"Open","message":"In progress","level":"info","schedule_type":"once","starts_at":"2026-09-15T10:00","timezone":"UTC","enabled":true}`,
	} {
		r := httptest.NewRecorder()
		HandleMaintenanceSchedules().ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/api/admin/maintenance-schedules", strings.NewReader(body)))
		if r.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
		}
		var s models.MaintenanceSchedule
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			t.Fatal(err)
		}
		if s.ScheduleType == "once" && s.StartsAt == "" {
			t.Fatalf("missing start: %+v", s)
		}
		// Backups serialize this same model. Verify it remains valid after JSON export/import.
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var restored models.MaintenanceSchedule
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if err := maintenance.ValidateSchedule(&restored); err != nil {
			t.Fatal(err)
		}
		if err := database.SaveMaintenanceSchedule(&restored); err != nil {
			t.Fatal(err)
		}
	}
	list := httptest.NewRecorder()
	HandleMaintenanceSchedules().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/admin/maintenance-schedules", nil))
	var schedules []models.MaintenanceSchedule
	if err := json.NewDecoder(list.Body).Decode(&schedules); err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 4 {
		t.Fatalf("expected default + 3 new schedules, got %d", len(schedules))
	}
}

func TestMaintenanceSchedulesAPIRejectsCalendarErrors(t *testing.T) {
	initMaintenanceHandlerDB(t)
	for _, dates := range []string{
		`"starts_at":"2026-03-29T01:30"`,
		`"starts_at":"2026-09-12T12:00","ends_at":"2026-09-12T11:00"`,
		`"starts_at":"2026-02-30T12:00"`,
	} {
		body := `{"name":"Invalid","message":"In progress","level":"warning","schedule_type":"once","timezone":"Europe/London",` + dates + `}`
		r := httptest.NewRecorder()
		HandleMaintenanceSchedules().ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/api/admin/maintenance-schedules", strings.NewReader(body)))
		if r.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", r.Code, r.Body.String())
		}
	}
}

func TestOpenEndedPublicBannerOmitsEnd(t *testing.T) {
	initMaintenanceHandlerDB(t)
	_, _ = database.DB.Exec(`DELETE FROM maintenance_schedules`)
	s := &models.MaintenanceSchedule{ID: "open", Name: "Open", Message: "Maintenance", Level: "warning", ScheduleType: "once", StartsAt: "2026-01-01T00:00:00Z", Timezone: "UTC", Enabled: true, SuppressMonitoring: true}
	if err := database.SaveMaintenanceSchedule(s); err != nil {
		t.Fatal(err)
	}
	alerts, err := getPublicStatusAlerts(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(alerts) != 1 || alerts[0].EndsAt != "" || !alerts[0].Scheduled {
		t.Fatalf("alerts = %+v, err = %v", alerts, err)
	}
}
