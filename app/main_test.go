package main

import (
	"status/app/internal/buildinfo"
	"status/app/internal/database"
	"status/app/internal/models"
	"testing"
	"time"
)

func TestRecordSoftwareStartupPersistsDeploymentAndLog(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	build := buildinfo.Info{
		Version:   "1.4.0",
		Commit:    "abc123",
		BuildTime: "2026-07-20T10:00:00Z",
		StartedAt: "2026-07-20T11:00:00Z",
		GoVersion: "go1.25.3",
	}
	if err := recordSoftwareStartup(build); err != nil {
		t.Fatal(err)
	}

	deployments, err := database.GetSoftwareDeployments(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 1 || deployments[0].Version != "1.4.0" {
		t.Fatalf("unexpected deployments: %+v", deployments)
	}
	logs, err := database.GetLogs(10, database.LogLevelInfo, database.LogCategorySystem, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Message != "Application started" || logs[0].Details == "" {
		t.Fatalf("unexpected startup logs: %+v", logs)
	}
}

func TestCheckResultIsConfirmed(t *testing.T) {
	tests := []struct {
		name                string
		checkOK             bool
		consecutiveFailures int
		want                bool
	}{
		{name: "successful check", checkOK: true, want: true},
		{name: "first failure is debounced", consecutiveFailures: 1, want: false},
		{name: "second failure is confirmed", consecutiveFailures: 2, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := checkResultIsConfirmed(test.checkOK, test.consecutiveFailures); got != test.want {
				t.Fatalf("checkResultIsConfirmed(%t, %d) = %t, want %t", test.checkOK, test.consecutiveFailures, got, test.want)
			}
		})
	}
}

func TestRecordConfirmedServiceState(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateService(&models.ServiceConfig{
		Key: "svc", Name: "Server", URL: "http://example.test", ServiceType: "custom", CheckType: "http",
		CheckInterval: 60, Timeout: 2, ExpectedMin: 200, ExpectedMax: 299, Visible: true,
	}); err != nil {
		t.Fatal(err)
	}
	notifier := &testServiceAlertNotifier{queued: true}
	now := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)

	if err := recordConfirmedServiceState(notifier, "svc", "Server", false, 1, false, now); err != nil {
		t.Fatal(err)
	}
	states, err := database.GetVisibleServiceOutageStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 || notifier.calls != 0 {
		t.Fatalf("first failure created state=%+v calls=%d", states, notifier.calls)
	}

	if err := recordConfirmedServiceState(notifier, "svc", "Server", false, 2, false, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	states, err = database.GetVisibleServiceOutageStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || !states[0].IsDown || !states[0].AlertSent || notifier.calls != 1 {
		t.Fatalf("confirmed failure state=%+v calls=%d", states, notifier.calls)
	}
}

type testServiceAlertNotifier struct {
	queued bool
	calls  int
}

func (n *testServiceAlertNotifier) CheckAndSendAlerts(_ string, _ string, _ bool, _ bool) bool {
	n.calls++
	return n.queued
}
