package monitor

import (
	"status/app/internal/database"
	"status/app/internal/resources"
	"testing"
)

type recordingUPSNotifier struct {
	calls  int
	queued bool
}

func (n *recordingUPSNotifier) NotifyUPSLineLost(_ *resources.UPSInfo) bool {
	n.calls++
	return n.queued
}

func upsReading(powerPresent bool) *resources.UPSInfo {
	return &resources.UPSInfo{Model: "Test UPS", PowerPresent: &powerPresent}
}

func TestProcessUPSPowerReadingNotifiesOncePerOutage(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	notifier := &recordingUPSNotifier{queued: true}

	if err := processUPSPowerReading(notifier, "host/apc", upsReading(false)); err != nil {
		t.Fatal(err)
	}
	if err := processUPSPowerReading(notifier, "host/apc", upsReading(false)); err != nil {
		t.Fatal(err)
	}
	if notifier.calls != 1 {
		t.Fatalf("outage notifications = %d, want 1", notifier.calls)
	}

	if err := processUPSPowerReading(notifier, "host/apc", upsReading(true)); err != nil {
		t.Fatal(err)
	}
	if err := processUPSPowerReading(notifier, "host/apc", upsReading(false)); err != nil {
		t.Fatal(err)
	}
	if notifier.calls != 2 {
		t.Fatalf("notifications after recovery and second outage = %d, want 2", notifier.calls)
	}
}

func TestProcessUPSPowerReadingRetriesWhenEmailNotConfigured(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	notifier := &recordingUPSNotifier{queued: false}

	_ = processUPSPowerReading(notifier, "host/apc", upsReading(false))
	_ = processUPSPowerReading(notifier, "host/apc", upsReading(false))
	if notifier.calls != 2 {
		t.Fatalf("unqueued notification attempts = %d, want 2", notifier.calls)
	}
}

func TestProcessUPSPowerReadingTreatsNewSourceAsNewState(t *testing.T) {
	if err := database.Init(":memory:"); err != nil {
		t.Fatal(err)
	}
	notifier := &recordingUPSNotifier{queued: true}

	_ = processUPSPowerReading(notifier, "host/apc", upsReading(false))
	_ = processUPSPowerReading(notifier, "other/apc", upsReading(false))
	if notifier.calls != 2 {
		t.Fatalf("notifications across sources = %d, want 2", notifier.calls)
	}
}
