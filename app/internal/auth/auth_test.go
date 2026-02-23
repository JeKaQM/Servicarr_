package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func testAuth(t *testing.T) *Auth {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return NewAuth("admin", hash, []byte("test-secret-key-32bytes-long!!!!"), true, 3600)
}

// --- NewAuth ---

func TestNewAuth(t *testing.T) {
	a := NewAuth("user", []byte("hash"), []byte("secret"), false, 7200)
	if a.User != "user" {
		t.Errorf("expected user %q, got %q", "user", a.User)
	}
	if a.SessionMaxAgeS != 7200 {
		t.Errorf("expected maxAge 7200, got %d", a.SessionMaxAgeS)
	}
	if a.InsecureDev {
		t.Error("expected InsecureDev to be false")
	}
}

// --- CheckCredentials ---

func TestCheckCredentials_Valid(t *testing.T) {
	a := testAuth(t)
	if !a.CheckCredentials("admin", "password123") {
		t.Error("expected valid credentials to pass")
	}
}

func TestCheckCredentials_WrongUser(t *testing.T) {
	a := testAuth(t)
	if a.CheckCredentials("wrong", "password123") {
		t.Error("expected wrong username to fail")
	}
}

func TestCheckCredentials_WrongPassword(t *testing.T) {
	a := testAuth(t)
	if a.CheckCredentials("admin", "wrongpass") {
		t.Error("expected wrong password to fail")
	}
}

func TestCheckCredentials_Empty(t *testing.T) {
	a := testAuth(t)
	if a.CheckCredentials("", "") {
		t.Error("expected empty credentials to fail")
	}
}

// --- Session Cookie Round-Trip ---

func TestMakeAndParseSession(t *testing.T) {
	a := testAuth(t)
	recorder := httptest.NewRecorder()

	err := a.MakeSessionCookie(recorder, "admin", 1*time.Hour)
	if err != nil {
		t.Fatalf("MakeSessionCookie: %v", err)
	}

	// Extract cookie from response
	cookies := recorder.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "sess" {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatal("no sess cookie set")
	}

	// Create request with the session cookie
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessCookie)

	sess, err := a.ParseSession(req)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if sess.U != "admin" {
		t.Errorf("expected user %q, got %q", "admin", sess.U)
	}
}

func TestParseSession_NoSession(t *testing.T) {
	a := testAuth(t)
	req := httptest.NewRequest("GET", "/", nil)
	_, err := a.ParseSession(req)
	if err == nil {
		t.Error("expected error for missing session")
	}
}

func TestParseSession_TamperedSignature(t *testing.T) {
	a := testAuth(t)
	recorder := httptest.NewRecorder()
	_ = a.MakeSessionCookie(recorder, "admin", 1*time.Hour)

	cookies := recorder.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "sess" {
			sessCookie = c
			break
		}
	}

	// Tamper with the cookie value
	sessCookie.Value = sessCookie.Value[:len(sessCookie.Value)-4] + "XXXX"

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessCookie)

	_, err := a.ParseSession(req)
	if err == nil {
		t.Error("expected error for tampered signature")
	}
}

func TestParseSession_ExpiredSession(t *testing.T) {
	a := testAuth(t)
	recorder := httptest.NewRecorder()

	// Create a session that already expired
	err := a.MakeSessionCookie(recorder, "admin", -1*time.Hour)
	if err != nil {
		t.Fatalf("MakeSessionCookie: %v", err)
	}

	cookies := recorder.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "sess" {
			sessCookie = c
			break
		}
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessCookie)

	_, err = a.ParseSession(req)
	if err == nil {
		t.Error("expected error for expired session")
	}
}

func TestParseSession_WrongSecret(t *testing.T) {
	a := testAuth(t)
	recorder := httptest.NewRecorder()
	_ = a.MakeSessionCookie(recorder, "admin", 1*time.Hour)

	cookies := recorder.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "sess" {
			sessCookie = c
			break
		}
	}

	// Use a different auth instance with different secret
	a2 := NewAuth("admin", a.Hash, []byte("different-secret-key-here!!!!!!!"), true, 3600)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessCookie)

	_, err := a2.ParseSession(req)
	if err == nil {
		t.Error("expected error when verifying with a different secret")
	}
}

// --- CSRF ---

func TestVerifyCSRF_Valid(t *testing.T) {
	a := testAuth(t)
	recorder := httptest.NewRecorder()
	csrfVal, err := a.SetCSRFCookie(recorder)
	if err != nil {
		t.Fatalf("SetCSRFCookie: %v", err)
	}

	// Build request with matching cookie and header
	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "csrf", Value: csrfVal})
	req.Header.Set("X-CSRF-Token", csrfVal)

	if !a.VerifyCSRF(req) {
		t.Error("expected CSRF verification to pass")
	}
}

func TestVerifyCSRF_Mismatch(t *testing.T) {
	a := testAuth(t)

	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "token-a"})
	req.Header.Set("X-CSRF-Token", "token-b")

	if a.VerifyCSRF(req) {
		t.Error("expected CSRF verification to fail on mismatch")
	}
}

func TestVerifyCSRF_MissingHeader(t *testing.T) {
	a := testAuth(t)

	req := httptest.NewRequest("POST", "/", nil)
	req.AddCookie(&http.Cookie{Name: "csrf", Value: "token-a"})

	if a.VerifyCSRF(req) {
		t.Error("expected CSRF verification to fail when header missing")
	}
}

func TestVerifyCSRF_MissingCookie(t *testing.T) {
	a := testAuth(t)

	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("X-CSRF-Token", "token-a")

	if a.VerifyCSRF(req) {
		t.Error("expected CSRF verification to fail when cookie missing")
	}
}

// --- Reload ---

func TestReload(t *testing.T) {
	a := testAuth(t)
	newHash := []byte("new-hash")
	newSecret := []byte("new-secret")
	a.Reload("newuser", newHash, newSecret)

	if a.User != "newuser" {
		t.Errorf("expected user %q after reload, got %q", "newuser", a.User)
	}
}

// --- RequireAuth Middleware ---

func TestRequireAuth_Unauthenticated(t *testing.T) {
	a := testAuth(t)
	handler := a.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_Authenticated_GET(t *testing.T) {
	a := testAuth(t)

	// Create valid session
	sessRecorder := httptest.NewRecorder()
	_ = a.MakeSessionCookie(sessRecorder, "admin", 1*time.Hour)
	cookies := sessRecorder.Result().Cookies()

	handler := a.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for authenticated GET, got %d", rec.Code)
	}
}

func TestRequireAuth_POST_NoCSRF(t *testing.T) {
	a := testAuth(t)

	// Create valid session
	sessRecorder := httptest.NewRecorder()
	_ = a.MakeSessionCookie(sessRecorder, "admin", 1*time.Hour)
	cookies := sessRecorder.Result().Cookies()

	handler := a.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for POST without CSRF, got %d", rec.Code)
	}
}

// --- SessionMaxAge ---

func TestSessionMaxAge(t *testing.T) {
	a := NewAuth("u", nil, nil, false, 7200)
	d := a.SessionMaxAge()
	if d != 7200*time.Second {
		t.Errorf("expected %v, got %v", 7200*time.Second, d)
	}
}

// --- Cookie Properties ---

func TestSessionCookie_Properties(t *testing.T) {
	a := testAuth(t)
	rec := httptest.NewRecorder()
	_ = a.MakeSessionCookie(rec, "admin", 1*time.Hour)

	cookies := rec.Result().Cookies()
	var sessCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "sess":
			sessCookie = c
		case "csrf":
			csrfCookie = c
		}
	}

	if sessCookie == nil {
		t.Fatal("missing sess cookie")
	}
	if !sessCookie.HttpOnly {
		t.Error("sess cookie should be HttpOnly")
	}

	if csrfCookie == nil {
		t.Fatal("missing csrf cookie")
	}
	if csrfCookie.HttpOnly {
		t.Error("csrf cookie should NOT be HttpOnly (JS needs to read it)")
	}
}

// --- ClearSessionCookie ---

func TestClearSessionCookie(t *testing.T) {
	a := testAuth(t)
	rec := httptest.NewRecorder()
	a.ClearSessionCookie(rec)

	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.MaxAge >= 0 {
			t.Errorf("cookie %s should have negative MaxAge to clear it, got %d", c.Name, c.MaxAge)
		}
	}
}

// --- Session JSON Encoding (hardening test) ---

func TestMakeSessionCookie_SpecialCharsInUsername(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.MinCost)
	a := NewAuth(`admin"test`, hash, []byte("test-secret-key-32bytes-long!!!!"), true, 3600)

	rec := httptest.NewRecorder()
	err := a.MakeSessionCookie(rec, `admin"test`, 1*time.Hour)
	if err != nil {
		t.Fatalf("MakeSessionCookie with special chars: %v", err)
	}

	cookies := rec.Result().Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "sess" {
			sessCookie = c
			break
		}
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(sessCookie)

	sess, err := a.ParseSession(req)
	if err != nil {
		t.Fatalf("ParseSession with special chars: %v", err)
	}
	if sess.U != `admin"test` {
		t.Errorf("expected user %q, got %q", `admin"test`, sess.U)
	}
}
