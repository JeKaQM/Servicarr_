package handlers

import (
	"net/http"
	"net/http/httptest"
	"status/app/internal/auth"
	"status/app/internal/cache"
	"status/app/internal/database"
	"status/app/internal/monitor"
	"status/app/internal/stats"
	"strings"
	"testing"
	"time"
)

func TestPublicEndpointsDoNotExposeHiddenServices(t *testing.T) {
	initAPIOutageTest(t)
	cache.StatsCache.Clear()
	t.Cleanup(cache.StatsCache.Clear)
	createAPIOutageService(t, "public-security-visible", "Visible", "http://private-visible.internal")
	createAPIOutageService(t, "public-security-hidden", "Secret Service", "http://private-hidden.internal")
	if _, err := database.DB.Exec(`UPDATE services SET visible=0 WHERE key='public-security-hidden'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`UPDATE services SET depends_on='public-security-hidden', connected_to='public-security-hidden', check_type='always_up' WHERE key='public-security-visible'`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, key := range []string{"public-security-visible", "public-security-hidden"} {
		if _, err := database.DB.Exec(`INSERT INTO samples(taken_at,service_key,ok,http_status) VALUES(?,?,0,503)`, now, key); err != nil {
			t.Fatal(err)
		}
		if _, err := database.DB.Exec(`INSERT INTO heartbeats(service_key,status,time,msg,important) VALUES(?,0,?,'Unavailable',1)`, key, now); err != nil {
			t.Fatal(err)
		}
	}
	cache.StatsCache.Set("all_uptime_stats", map[string]*stats.UptimeStats{"public-security-hidden": {}})
	for _, tc := range []struct {
		url     string
		handler http.HandlerFunc
		status  int
	}{
		{"/api/services?admin=true", HandleGetServices, 200},
		{"/api/check", HandleCheck(monitor.NewFailureTracker()), 200},
		{"/api/metrics?hours=24", HandleMetrics(), 200},
		{"/api/uptime", HandleUptimeStats(), 200},
		{"/api/uptime?service=public-security-hidden", HandleUptimeStats(), 404},
		{"/api/heartbeats?service=public-security-hidden", HandleRecentHeartbeats(), 404},
		{"/api/metrics/day-detail?key=public-security-hidden&date=2026-09-05", HandleDayDetail(), 404},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			for _, secret := range []string{"public-security-hidden", "Secret Service", "private-visible.internal", "private-hidden.internal"} {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatalf("public response leaked %q", secret)
				}
			}
		})
	}
	w := httptest.NewRecorder()
	HandleGetAdminServices(w, httptest.NewRequest(http.MethodGet, "/api/admin/services", nil))
	if !strings.Contains(w.Body.String(), "public-security-hidden") {
		t.Fatal("admin view omitted hidden service")
	}
}

func TestLogoutRequiresPostAndCSRF(t *testing.T) {
	initAPIOutageTest(t)
	a := auth.NewAuth("admin", nil, []byte("test-secret"), true, 3600)
	for _, tc := range []struct {
		method string
		csrf   bool
		want   int
	}{{http.MethodGet, false, 405}, {http.MethodPost, false, 403}, {http.MethodPost, true, 200}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, "/api/logout", nil)
		if tc.csrf {
			r.AddCookie(&http.Cookie{Name: "csrf", Value: "test"})
			r.Header.Set("X-CSRF-Token", "test")
		}
		HandleLogout(a)(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s csrf=%v: status %d, want %d", tc.method, tc.csrf, w.Code, tc.want)
		}
	}
}

func TestSetupAuthUsesPersistedSecretAndCannotRunAgain(t *testing.T) {
	initAPIOutageTest(t)
	a := auth.NewAuth("", nil, []byte("temporary-secret"), true, 3600)
	w := httptest.NewRecorder()
	HandleCompleteSetup(a)(w, httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(`{"username":"admin","password":"test-password"}`)))
	if w.Code != 200 {
		t.Fatalf("setup failed: %s", w.Body.String())
	}
	settings, err := database.LoadAppSettings()
	if err != nil {
		t.Fatal(err)
	}
	restarted := auth.NewAuth(settings.Username, []byte(settings.PasswordHash), []byte(settings.AuthSecret), true, 3600)
	cookieResponse := httptest.NewRecorder()
	if err := a.MakeSessionCookie(cookieResponse, "admin", time.Hour); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range cookieResponse.Result().Cookies() {
		r.AddCookie(cookie)
	}
	if _, err := restarted.ParseSession(r); err != nil {
		t.Fatalf("session failed after restart: %v", err)
	}
	w = httptest.NewRecorder()
	HandleCompleteSetup(a)(w, httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(`{"username":"intruder","password":"test-password"}`)))
	if w.Code != 403 {
		t.Fatalf("repeated setup status = %d", w.Code)
	}
}
