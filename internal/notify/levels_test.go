package notify

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestLevelMonitorWarningThenCritical(t *testing.T) {
	var kinds []string
	percent := 50
	discharging := true
	opts := LevelOptions{
		Read: func(context.Context) (LevelReading, error) {
			return LevelReading{Present: true, Percent: percent, PercentOK: true, Discharging: discharging}, nil
		},
		Thresholds: func() (int, int) { return 20, 10 },
		Send: func(_ context.Context, event NotificationEvent) error {
			kinds = append(kinds, event.Type)
			return nil
		},
	}
	var mon levelMonitor

	mon.tick(context.Background(), opts)
	if len(kinds) != 0 {
		t.Fatalf("no alert at 50%%: %v", kinds)
	}

	percent = 15
	mon.tick(context.Background(), opts)
	if len(kinds) != 1 || kinds[0] != EventBatteryWarning {
		t.Fatalf("warning: %v", kinds)
	}

	mon.tick(context.Background(), opts)
	if len(kinds) != 1 {
		t.Fatalf("duplicate warning: %v", kinds)
	}

	percent = 8
	mon.tick(context.Background(), opts)
	if len(kinds) != 2 || kinds[1] != EventBatteryCritical {
		t.Fatalf("critical: %v", kinds)
	}

	discharging = false
	percent = 8
	mon.tick(context.Background(), opts)
	percent = 8
	discharging = true
	mon.tick(context.Background(), opts)
	if len(kinds) != 3 || kinds[2] != EventBatteryCritical {
		t.Fatalf("should re-arm after charging: %v", kinds)
	}
}

func TestLevelMonitorSkipsWhenCharging(t *testing.T) {
	var hits atomic.Int32
	opts := LevelOptions{
		Read: func(context.Context) (LevelReading, error) {
			return LevelReading{Present: true, Percent: 5, PercentOK: true, Discharging: false}, nil
		},
		Thresholds: func() (int, int) { return 20, 10 },
		Send: func(context.Context, NotificationEvent) error {
			hits.Add(1)
			return nil
		},
	}
	var mon levelMonitor
	mon.tick(context.Background(), opts)
	if hits.Load() != 0 {
		t.Fatal("charging at 5% must not notify")
	}
}

func TestIsDischarging(t *testing.T) {
	if !IsDischarging("Discharging") || IsDischarging("Charging") || IsDischarging("Not charging") {
		t.Fatal("status match")
	}
}
