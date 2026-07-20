package database

import (
	"testing"
	"time"
)

func TestRecordServiceOutageStatePreservesTransitionTimes(t *testing.T) {
	initTestDB(t)
	if _, err := CreateService(sampleService("outage-svc")); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, time.July, 21, 10, 0, 0, 0, time.UTC)
	if err := RecordServiceOutageState("outage-svc", false, false, base); err != nil {
		t.Fatal(err)
	}
	state := onlyOutageState(t)
	if state.IsDown || state.RestoredAt != nil {
		t.Fatalf("initial healthy check created a recovery: %+v", state)
	}

	downAt := base.Add(time.Hour)
	if err := RecordServiceOutageState("outage-svc", true, false, downAt); err != nil {
		t.Fatal(err)
	}
	if err := RecordServiceOutageState("outage-svc", true, true, downAt.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	state = onlyOutageState(t)
	if !state.IsDown || state.DownSince == nil || !state.DownSince.Equal(downAt) || !state.AlertSent {
		t.Fatalf("repeated outage did not preserve its transition: %+v", state)
	}

	restoredAt := downAt.Add(30 * time.Minute)
	if err := RecordServiceOutageState("outage-svc", false, false, restoredAt); err != nil {
		t.Fatal(err)
	}
	if err := RecordServiceOutageState("outage-svc", false, false, restoredAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	state = onlyOutageState(t)
	if state.IsDown || state.DownSince != nil || state.RestoredAt == nil || !state.RestoredAt.Equal(restoredAt) || state.AlertSent {
		t.Fatalf("recovery transition was not preserved: %+v", state)
	}
}

func TestVisibleServiceOutageStatesExcludeDisabledAndHiddenServices(t *testing.T) {
	initTestDB(t)
	visibleID, err := CreateService(sampleService("visible-outage"))
	if err != nil {
		t.Fatal(err)
	}
	if err := RecordServiceOutageState("visible-outage", true, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if states, err := GetVisibleServiceOutageStates(); err != nil || len(states) != 1 {
		t.Fatalf("visible state count=%d err=%v", len(states), err)
	}

	if err := SetServiceDisabledState("visible-outage", true); err != nil {
		t.Fatal(err)
	}
	if states, err := GetVisibleServiceOutageStates(); err != nil || len(states) != 0 {
		t.Fatalf("disabled state count=%d err=%v", len(states), err)
	}

	if err := SetServiceDisabledState("visible-outage", false); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`UPDATE services SET visible = 0 WHERE id = ?`, visibleID); err != nil {
		t.Fatal(err)
	}
	if states, err := GetVisibleServiceOutageStates(); err != nil || len(states) != 0 {
		t.Fatalf("hidden state count=%d err=%v", len(states), err)
	}
}

func TestDeleteServiceOutageState(t *testing.T) {
	initTestDB(t)
	if _, err := CreateService(sampleService("delete-outage")); err != nil {
		t.Fatal(err)
	}
	if err := RecordServiceOutageState("delete-outage", true, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := DeleteServiceOutageState("delete-outage"); err != nil {
		t.Fatal(err)
	}
	if states, err := GetVisibleServiceOutageStates(); err != nil || len(states) != 0 {
		t.Fatalf("deleted state count=%d err=%v", len(states), err)
	}
}

func onlyOutageState(t *testing.T) ServiceOutageState {
	t.Helper()
	states, err := GetVisibleServiceOutageStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Fatalf("got %d outage states, want 1", len(states))
	}
	return states[0]
}
