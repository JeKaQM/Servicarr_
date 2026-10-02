package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"status/app/internal/database"
	"status/app/internal/models"
	"status/app/internal/stats"
)

func postBackup(t *testing.T, handler http.HandlerFunc, export DatabaseExport) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(export)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("backup", "servicarr-backup.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/settings/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler(recorder, request)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		// A helper that bypasses the transaction would wait forever for the
		// single pooled connection.
		t.Fatal("import did not finish; probable deadlock on the single DB connection")
	}
	return recorder
}

func initBackupTestDB(t *testing.T) {
	t.Helper()
	initCrowdSecHandlerDB(t)
	// main() creates the stats tables before serving; imports clear them.
	if err := stats.EnsureStatsSchema(); err != nil {
		t.Fatal(err)
	}
}

func TestImportBackupIsAtomic(t *testing.T) {
	initBackupTestDB(t)
	if _, err := database.CreateService(&models.ServiceConfig{
		Key: "existing", Name: "Existing", URL: "http://existing.local", CheckType: "http",
		CheckInterval: 60, Timeout: 5, ExpectedMin: 200, ExpectedMax: 399, Visible: true,
	}); err != nil {
		t.Fatal(err)
	}
	database.InsertSample(time.Now(), "existing", true, 200, nil)

	// The duplicate key fails the second insert after services were cleared.
	recorder := postBackup(t, HandleImportDatabase(), DatabaseExport{
		Version: "1.0",
		Services: []exportService{
			{Key: "dup", Name: "First", URL: "http://a.local", CheckType: "http"},
			{Key: "dup", Name: "Second", URL: "http://b.local", CheckType: "http"},
		},
		Samples: []exportSample{{TakenAt: time.Now().UTC().Format(time.RFC3339), ServiceKey: "dup", OK: true}},
	})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", recorder.Code, recorder.Body.String())
	}
	if _, err := importBackup(&DatabaseExport{Version: "1.0", Services: []exportService{{Key: "dup", Name: "A", URL: "http://a.local"}, {Key: "dup", Name: "B", URL: "http://b.local"}}}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("import error = %v, want a UNIQUE constraint failure", err)
	}

	services, err := database.GetAllServices()
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].Key != "existing" {
		t.Fatalf("services after failed import = %+v, want the original service only", services)
	}
	var samples int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM samples WHERE service_key = 'existing'`).Scan(&samples); err != nil {
		t.Fatal(err)
	}
	if samples != 1 {
		t.Fatalf("existing samples after failed import = %d, want 1", samples)
	}
}

func TestImportBackupNormalizesSampleTimes(t *testing.T) {
	initBackupTestDB(t)
	recorder := postBackup(t, HandleImportDatabase(), DatabaseExport{
		Version:  "1.0",
		Services: []exportService{{Key: "svc", Name: "Svc", URL: "http://svc.local", CheckType: "http"}},
		Samples: []exportSample{
			{TakenAt: "2026-09-01T12:30:00+02:00", ServiceKey: "svc", OK: true},
			{TakenAt: "2026-09-01T10:31:00.250Z", ServiceKey: "svc", OK: false},
			{TakenAt: "yesterday", ServiceKey: "svc", OK: true},
			{TakenAt: "2026-09-01T10:32:00Z", ServiceKey: "", OK: true},
		},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["samples_imported"] != float64(2) || response["samples_skipped"] != float64(2) {
		t.Fatalf("response = %v, want 2 imported and 2 skipped", response)
	}

	rows, err := database.DB.Query(`SELECT taken_at FROM samples ORDER BY taken_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var takenAt string
		if err := rows.Scan(&takenAt); err != nil {
			t.Fatal(err)
		}
		got = append(got, takenAt)
	}
	want := []string{"2026-09-01T10:30:00Z", "2026-09-01T10:31:00Z"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("taken_at = %v, want %v", got, want)
	}
}
