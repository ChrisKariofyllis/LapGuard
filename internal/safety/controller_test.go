package safety

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/power"
)

func testConfig(enabled bool) config.Config {
	return config.Config{
		Shutdown: config.ShutdownConfig{
			Enabled:           enabled,
			WarningThreshold:  20,
			CriticalThreshold: 10,
		},
		Docker: config.DockerConfig{StopEnabled: true, TimeoutSeconds: 30},
		Safety: config.SafetyConfig{
			DryRun:                true,
			RequireACLoss:         true,
			MinimumBatteryPercent: 0,
			CooldownSeconds:       0,
		},
	}
}

func pct(n int) *int { return &n }

func discharging(n int) Sample {
	return Sample{
		Now:         time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC),
		Present:     true,
		Percent:     pct(n),
		Status:      "Discharging",
		Discharging: true,
		Source:      power.SourceBattery,
	}
}

func TestACConnectedAtCriticalDoesNotTriggerAction(t *testing.T) {
	exec := NewRecordingExecutor()
	var notes atomic.Int32
	c := New(Options{
		Config:   func() config.Config { return testConfig(true) },
		Executor: exec,
		Notify: func(context.Context, notify.NotificationEvent) error {
			notes.Add(1)
			return nil
		},
	})
	sample := discharging(5)
	sample.Source = power.SourceAC
	sample.Discharging = false
	sample.Status = "Not charging"
	snap := c.Tick(context.Background(), sample)
	if snap.State != StateACConnected {
		t.Fatalf("state %s", snap.State)
	}
	if exec.Len() != 0 || notes.Load() != 0 {
		t.Fatalf("AC must not notify or execute: calls %v notes %d", exec.Calls(), notes.Load())
	}
	if len(snap.IntendedActions) != 0 {
		t.Fatalf("plan %v", snap.IntendedActions)
	}
	if snap.CommandsExecuted {
		t.Fatal("commands_executed must be false")
	}
}

func TestUnknownACStateDoesNotTriggerAction(t *testing.T) {
	exec := NewRecordingExecutor()
	c := New(Options{
		Config:   func() config.Config { return testConfig(true) },
		Executor: exec,
	})
	sample := discharging(5)
	sample.Source = power.SourceUnknown
	snap := c.Tick(context.Background(), sample)
	if snap.State != StateUnknown {
		t.Fatalf("state %s", snap.State)
	}
	if exec.Len() != 0 {
		t.Fatalf("unknown AC executed %v", exec.Calls())
	}
}

func TestDischargingBelowWarningEmitsOneWarning(t *testing.T) {
	var kinds []string
	c := New(Options{
		Config: func() config.Config { return testConfig(true) },
		Notify: func(_ context.Context, event notify.NotificationEvent) error {
			kinds = append(kinds, event.Type)
			return nil
		},
	})
	snap := c.Tick(context.Background(), discharging(15))
	if snap.State != StateWarning {
		t.Fatalf("state %s", snap.State)
	}
	if len(kinds) != 1 || kinds[0] != EventBatteryWarning {
		t.Fatalf("events %v", kinds)
	}
	c.Tick(context.Background(), discharging(15))
	c.Tick(context.Background(), discharging(14))
	if len(kinds) != 1 {
		t.Fatalf("repeated polls must not duplicate warning: %v", kinds)
	}
}

func TestDischargingBelowCriticalEmitsOneCritical(t *testing.T) {
	exec := NewRecordingExecutor()
	var kinds []string
	c := New(Options{
		Config:   func() config.Config { return testConfig(true) },
		Executor: exec,
		Notify: func(_ context.Context, event notify.NotificationEvent) error {
			kinds = append(kinds, event.Type)
			return nil
		},
	})
	snap := c.Tick(context.Background(), discharging(8))
	if snap.State != StateShutdownPending {
		t.Fatalf("state %s", snap.State)
	}
	if len(kinds) != 1 || kinds[0] != EventBatteryCritical {
		t.Fatalf("events %v", kinds)
	}
	if got := snap.IntendedActions; len(got) != 3 {
		t.Fatalf("plan %v", got)
	}
	c.Tick(context.Background(), discharging(7))
	if len(kinds) != 1 {
		t.Fatalf("duplicate critical: %v", kinds)
	}
	if exec.Len() != 0 {
		t.Fatalf("dry-run invoked executor: %v", exec.Calls())
	}
}

func TestACRestoreResetsState(t *testing.T) {
	var kinds []string
	c := New(Options{
		Config: func() config.Config { return testConfig(true) },
		Notify: func(_ context.Context, event notify.NotificationEvent) error {
			kinds = append(kinds, event.Type)
			return nil
		},
	})
	c.Tick(context.Background(), discharging(15))
	ac := discharging(15)
	ac.Source = power.SourceAC
	ac.Discharging = false
	ac.Status = "Charging"
	snap := c.Tick(context.Background(), ac)
	if snap.State != StateACConnected {
		t.Fatalf("state %s", snap.State)
	}
	c.Tick(context.Background(), discharging(15))
	if len(kinds) != 2 {
		t.Fatalf("AC restore should re-arm warning: %v", kinds)
	}
}

func TestDryRunNeverInvokesExecutor(t *testing.T) {
	exec := NewRecordingExecutor()
	c := New(Options{
		Config:   func() config.Config { return testConfig(true) },
		Executor: exec,
	})
	c.Tick(context.Background(), discharging(4))
	if exec.Len() != 0 {
		t.Fatalf("executor calls %v", exec.Calls())
	}
	if _, err := c.Simulate(context.Background(), "critical"); err != nil {
		t.Fatal(err)
	}
	if exec.Len() != 0 {
		t.Fatalf("simulate invoked executor: %v", exec.Calls())
	}
}

func TestNotificationFailureDoesNotCrash(t *testing.T) {
	c := New(Options{
		Config: func() config.Config { return testConfig(false) },
		Notify: func(context.Context, notify.NotificationEvent) error {
			return errors.New("boom")
		},
	})
	snap := c.Tick(context.Background(), discharging(15))
	if snap.State != StateWarning {
		t.Fatalf("state %s", snap.State)
	}
	c.Tick(context.Background(), discharging(8))
}

func TestNotifyPanicDoesNotCrash(t *testing.T) {
	c := New(Options{
		Config: func() config.Config { return testConfig(false) },
		Notify: func(context.Context, notify.NotificationEvent) error {
			panic("notifier exploded")
		},
	})
	c.Tick(context.Background(), discharging(12))
}

func TestRecordingExecutorDoesNotShellOut(t *testing.T) {
	exec := NewRecordingExecutor()
	if err := exec.StopDocker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := exec.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := exec.PowerOff(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(exec.Calls(), ",")
	if got != "stop_docker,sync,poweroff" {
		t.Fatalf("calls %s", got)
	}
}

func TestHysteresisIgnoresSmallFluctuations(t *testing.T) {
	var kinds []string
	c := New(Options{
		Config: func() config.Config { return testConfig(false) },
		Notify: func(_ context.Context, event notify.NotificationEvent) error {
			kinds = append(kinds, event.Type)
			return nil
		},
	})
	c.Tick(context.Background(), discharging(19))
	c.Tick(context.Background(), discharging(21)) // below warning+2 recovery margin
	c.Tick(context.Background(), discharging(19))
	if len(kinds) != 1 {
		t.Fatalf("fluctuation around warning re-fired: %v", kinds)
	}
}

func TestSimulateUnknownScenario(t *testing.T) {
	c := New(Options{Config: func() config.Config { return testConfig(true) }})
	if _, err := c.Simulate(context.Background(), "explode"); !errors.Is(err, ErrUnknownScenario) {
		t.Fatalf("got %v", err)
	}
}

func TestMissingPercentIsUnknown(t *testing.T) {
	c := New(Options{Config: func() config.Config { return testConfig(true) }})
	snap := c.Tick(context.Background(), Sample{Present: true, Discharging: true, Source: power.SourceBattery})
	if snap.State != StateUnknown {
		t.Fatalf("state %s", snap.State)
	}
}
