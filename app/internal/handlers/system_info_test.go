package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"status/app/internal/buildinfo"
	"status/app/internal/database"
	"testing"
)

func TestHandleGetSystemInfo(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	originalVersion, originalCommit, originalBuildTime := buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.BuildTime = originalVersion, originalCommit, originalBuildTime
	})
	buildinfo.Version = "1.2.3"
	buildinfo.Commit = "abcdef1234567890"
	buildinfo.BuildTime = "2026-07-20T12:00:00Z"
	if err := database.RecordSoftwareDeployment(buildinfo.Current()); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/settings/system-info", nil)
	HandleGetSystemInfo()(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("Cache-Control = %q", cache)
	}

	var response systemInfoResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Version != "1.2.3" || response.Summary != "v1.2.3 (abcdef123456)" {
		t.Fatalf("unexpected build response: %+v", response)
	}
	if response.Database.Engine != "SQLite" || response.Database.EngineVersion == "" || response.Database.SchemaVersion != database.SchemaVersion {
		t.Fatalf("unexpected database response: %+v", response.Database)
	}
	if len(response.Deployments) != 1 || response.Deployments[0].Version != "1.2.3" {
		t.Fatalf("unexpected deployments: %+v", response.Deployments)
	}
}

func TestHandleGetSystemInfoRejectsOtherMethods(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/settings/system-info", nil)
	HandleGetSystemInfo()(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestDatabaseExportIncludesSoftwareCompatibilityMetadata(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	originalVersion, originalCommit := buildinfo.Version, buildinfo.Commit
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit = originalVersion, originalCommit
	})
	buildinfo.Version = "1.5.0"
	buildinfo.Commit = "release123"

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/settings/export", nil)
	HandleExportDatabase()(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var export DatabaseExport
	if err := json.Unmarshal(recorder.Body.Bytes(), &export); err != nil {
		t.Fatal(err)
	}
	if export.Version != "1.0" || export.ApplicationVersion != "1.5.0" || export.ApplicationCommit != "release123" {
		t.Fatalf("unexpected export version metadata: %+v", export)
	}
	if export.DatabaseSchema != database.SchemaVersion {
		t.Fatalf("database schema = %d, want %d", export.DatabaseSchema, database.SchemaVersion)
	}
}
