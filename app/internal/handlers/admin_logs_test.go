package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"status/app/internal/database"
)

func countLogs(t *testing.T) int {
	t.Helper()
	var n int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM system_logs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestGetLogsRejectsInvalidPaging(t *testing.T) {
	initMaintenanceHandlerDB(t)
	for _, query := range []string{"limit=-1", "limit=abc", "limit=0", "offset=-5", "offset=x"} {
		recorder := httptest.NewRecorder()
		HandleGetLogs()(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/logs?"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	HandleGetLogs()(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/logs?limit=9999&offset=0", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid paging: status = %d", recorder.Code)
	}
}

func TestClearLogsRequiresExplicitDays(t *testing.T) {
	initMaintenanceHandlerDB(t)
	_ = database.InsertLog(database.LogLevelInfo, database.LogCategoryAudit, "", "Login succeeded", "")

	for _, body := range []string{"", "not json", `{}`, `{"days": -1}`} {
		recorder := httptest.NewRecorder()
		HandleClearLogs()(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/logs", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, recorder.Code)
		}
	}
	if n := countLogs(t); n == 0 {
		t.Fatal("a rejected request cleared the logs")
	}

	recorder := httptest.NewRecorder()
	HandleClearLogs()(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/logs", strings.NewReader(`{"days": 0}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("explicit clear: status = %d", recorder.Code)
	}
	if n := countLogs(t); n != 0 {
		t.Fatalf("logs after explicit clear = %d, want 0", n)
	}
}
