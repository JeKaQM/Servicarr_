package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"status/app/internal/auth"
	"status/app/internal/database"
)

func testAuditAuth(t *testing.T) *auth.Auth {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return auth.NewAuth("admin", hash, []byte("audit-test-secret-with-enough-bytes"), true, 3600)
}

func TestAuditAdminActionsRecordsMutationActorAndOutcome(t *testing.T) {
	initAPIOutageTest(t)
	authMgr := testAuditAuth(t)
	cookieRecorder := httptest.NewRecorder()
	if err := authMgr.MakeSessionCookie(cookieRecorder, "admin", authMgr.SessionMaxAge()); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/check", nil)
	for _, cookie := range cookieRecorder.Result().Cookies() {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	AuditAdminActions(authMgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(recorder, request)

	logs, err := database.GetLogs(10, "", database.LogCategoryAudit, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Message != "Service card refreshed" {
		t.Fatalf("unexpected audit logs: %+v", logs)
	}
	if !strings.Contains(logs[0].Details, `"actor":"admin"`) || !strings.Contains(logs[0].Details, `"outcome":"success"`) {
		t.Fatalf("audit context missing: %s", logs[0].Details)
	}
}

func TestAuditAdminActionsSkipsBackgroundReadsButRecordsExplicitRefresh(t *testing.T) {
	initAPIOutageTest(t)
	authMgr := testAuditAuth(t)
	handler := AuditAdminActions(authMgr, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/admin/logs", nil))
	request := httptest.NewRequest(http.MethodGet, "/api/admin/logs", nil)
	request.Header.Set("X-Audit-Action", "logs.refresh")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	logs, err := database.GetLogs(10, "", database.LogCategoryAudit, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Message != "Logs refreshed" {
		t.Fatalf("unexpected read audit logs: %+v", logs)
	}
}

func TestLoginHandlerAuditsFailedAndSuccessfulAttempts(t *testing.T) {
	initAPIOutageTest(t)
	authMgr := testAuditAuth(t)

	failed := httptest.NewRecorder()
	failedRequest := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	HandleLogin(authMgr).ServeHTTP(failed, failedRequest)
	if failed.Code != http.StatusUnauthorized {
		t.Fatalf("failed login status = %d", failed.Code)
	}

	succeeded := httptest.NewRecorder()
	successRequest := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"admin","password":"correct-password"}`))
	HandleLogin(authMgr).ServeHTTP(succeeded, successRequest)
	if succeeded.Code != http.StatusOK {
		t.Fatalf("successful login status = %d", succeeded.Code)
	}

	logs, err := database.GetLogs(10, "", database.LogCategoryAudit, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Message != "Login succeeded" || logs[1].Message != "Login failed" {
		t.Fatalf("unexpected login audit logs: %+v", logs)
	}
}
