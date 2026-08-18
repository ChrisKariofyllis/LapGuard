package actions

import (
	"context"
	"testing"

	"lapguard/internal/safety"
)

func TestDrainSyncPowerOffOrder(t *testing.T) {
	rec := safety.NewRecordingExecutor()
	if err := DrainSyncPowerOff(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	got := rec.Calls()
	want := []string{safety.ActionStopDocker, safety.ActionSync, safety.ActionPowerOff}
	if len(got) != len(want) {
		t.Fatalf("calls %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls %v want %v", got, want)
		}
	}
}

func TestDrainSyncPowerOffNilExecutor(t *testing.T) {
	if err := DrainSyncPowerOff(context.Background(), nil); err != ErrUnavailable {
		t.Fatalf("got %v", err)
	}
}
