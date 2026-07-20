package database

import (
	"status/app/internal/buildinfo"
	"testing"
)

func TestRecordSoftwareDeploymentTracksRestartsAndVersions(t *testing.T) {
	initTestDB(t)
	first := buildinfo.Info{
		Version:   "1.0.0",
		Commit:    "abc123",
		BuildTime: "2026-07-20T10:00:00Z",
		StartedAt: "2026-07-20T11:00:00Z",
	}
	if err := RecordSoftwareDeployment(first); err != nil {
		t.Fatal(err)
	}
	first.StartedAt = "2026-07-20T12:00:00Z"
	if err := RecordSoftwareDeployment(first); err != nil {
		t.Fatal(err)
	}
	if err := RecordSoftwareDeployment(buildinfo.Info{
		Version:   "1.1.0",
		Commit:    "def456",
		BuildTime: "2026-07-21T09:00:00Z",
		StartedAt: "2026-07-21T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	deployments, err := GetSoftwareDeployments(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 2 {
		t.Fatalf("got %d deployments, want 2", len(deployments))
	}
	if deployments[0].Version != "1.1.0" || deployments[0].StartupCount != 1 {
		t.Fatalf("unexpected latest deployment: %+v", deployments[0])
	}
	if deployments[1].Version != "1.0.0" || deployments[1].StartupCount != 2 {
		t.Fatalf("restart was not accumulated: %+v", deployments[1])
	}
	if deployments[1].FirstStartedAt != "2026-07-20T11:00:00Z" || deployments[1].LastStartedAt != "2026-07-20T12:00:00Z" {
		t.Fatalf("deployment timestamps changed incorrectly: %+v", deployments[1])
	}

	var currentVersion, schemaVersion string
	if err := DB.QueryRow(`SELECT value FROM app_metadata WHERE key = 'software_version'`).Scan(&currentVersion); err != nil {
		t.Fatal(err)
	}
	if err := DB.QueryRow(`SELECT value FROM app_metadata WHERE key = 'database_schema_version'`).Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if currentVersion != "1.1.0" || schemaVersion != "1" {
		t.Fatalf("metadata version=%q schema=%q", currentVersion, schemaVersion)
	}
}

func TestRecordSoftwareDeploymentNormalizesMissingValues(t *testing.T) {
	initTestDB(t)
	if err := RecordSoftwareDeployment(buildinfo.Info{StartedAt: "invalid"}); err != nil {
		t.Fatal(err)
	}
	deployments, err := GetSoftwareDeployments(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 1 || deployments[0].Version != "dev" || deployments[0].Commit != "unknown" {
		t.Fatalf("unexpected normalized deployment: %+v", deployments)
	}
	if version, err := SQLiteVersion(); err != nil || version == "" {
		t.Fatalf("SQLiteVersion() = %q, %v", version, err)
	}
}
