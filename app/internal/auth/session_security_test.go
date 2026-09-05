package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPasswordChangeRevokesExistingSessions(t *testing.T) {
	a := testAuth(t)
	recorder := httptest.NewRecorder()
	if err := a.MakeSessionCookie(recorder, "admin", time.Hour); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range recorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
	if _, err := a.ParseSession(req); err != nil {
		t.Fatal(err)
	}
	a.Reload("admin", []byte("new-password-hash"), a.HmacSecret)
	if _, err := a.ParseSession(req); err == nil {
		t.Fatal("old session survived password change")
	}
	updated := httptest.NewRecorder()
	if err := a.MakeSessionCookie(updated, "admin", time.Hour); err != nil {
		t.Fatal(err)
	}
	renewed := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range updated.Result().Cookies() {
		renewed.AddCookie(cookie)
	}
	if _, err := a.ParseSession(renewed); err != nil {
		t.Fatalf("new session rejected: %v", err)
	}
}

func TestSessionRejectsUnexpectedUserAndCurrentExpiry(t *testing.T) {
	for _, tc := range []struct {
		user string
		age  time.Duration
	}{{"other", time.Hour}, {"admin", 0}} {
		a := testAuth(t)
		w := httptest.NewRecorder()
		if err := a.MakeSessionCookie(w, tc.user, tc.age); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, cookie := range w.Result().Cookies() {
			r.AddCookie(cookie)
		}
		if _, err := a.ParseSession(r); err == nil {
			t.Fatalf("accepted invalid session %+v", tc)
		}
	}
}
