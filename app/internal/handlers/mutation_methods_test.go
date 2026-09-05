package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"status/app/internal/auth"
)

func TestMutationHandlersRejectSafeMethodsBeforeSideEffects(t *testing.T) {
	a := auth.NewAuth("admin", nil, []byte("test-secret"), true, 3600)
	cookies := httptest.NewRecorder()
	if err := a.MakeSessionCookie(cookies, "admin", time.Hour); err != nil {
		t.Fatal(err)
	}
	// These handlers deliberately receive nil dependencies. A rejected method
	// must return before checking a service, sending email, or changing the DB.
	for _, endpoint := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"ingest-now", HandleIngestNow(nil)},
		{"reset-recent", HandleResetRecent()},
		{"check", HandleAdminCheck(nil)},
		{"toggle-monitoring", HandleToggleMonitoring(nil)},
		{"unblock", HandleUnblockIP()},
		{"alerts/save", HandleSaveAlertsConfig(nil)},
		{"alerts/test", HandleTestEmail(nil)},
		{"alerts/test-channel", HandleTestNotification(nil)},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
			t.Run(endpoint.name+"/"+method, func(t *testing.T) {
				r := httptest.NewRequest(method, "/api/admin/"+endpoint.name, strings.NewReader(`{"service":"example","ip":"127.0.0.1","channel":"discord"}`))
				for _, cookie := range cookies.Result().Cookies() {
					r.AddCookie(cookie)
					if cookie.Name == "csrf" {
						r.Header.Set("X-CSRF-Token", cookie.Value)
					}
				}
				w := httptest.NewRecorder()
				a.RequireAuth(endpoint.handler)(w, r)
				if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodPost {
					t.Fatalf("unsafe method accepted: status=%d allow=%q", w.Code, w.Header().Get("Allow"))
				}
			})
		}
		t.Run(endpoint.name+"/POST-without-CSRF", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/admin/"+endpoint.name, strings.NewReader(`{}`))
			for _, cookie := range cookies.Result().Cookies() {
				r.AddCookie(cookie)
			}
			w := httptest.NewRecorder()
			a.RequireAuth(endpoint.handler)(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("POST without CSRF accepted: %d", w.Code)
			}
		})
	}
}
