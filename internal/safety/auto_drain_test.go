package safety

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/power"
)

func TestAutoDrainDefaultDisabled(t *testing.T) {
	rec := NewRecordingExecutor()
	ad := NewAutoDrain(AutoDrainOptions{Executor: rec})
	pct := 5
	snap := ad.Tick(context.Background(), dischargingSample(time.Now().UTC(), pct))
	if snap.Enabled || snap.State != AutoDrainIdle {
		t.Fatalf("default must stay idle: %+v", snap)
	}
	if rec.Len() != 0 {
		t.Fatalf("executor calls %v", rec.Calls())
	}
}

func TestAutoDrainRequiresNotification(t *testing.T) {
	rec := NewRecordingExecutor()
	cfg := readyAutoDrainConfig()
	ad := NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
	})
	snap := ad.Tick(context.Background(), dischargingSample(time.Now().UTC(), 5))
	if snap.State != AutoDrainIdle {
		t.Fatalf("state %s", snap.State)
	}
	if !strings.Contains(snap.Reason, "notification") {
		t.Fatalf("reason %q", snap.Reason)
	}
	if rec.Len() != 0 {
		t.Fatal("must not execute without a notifier")
	}
}

func TestAutoDrainNotifyFailureBlocksExecute(t *testing.T) {
	rec := NewRecordingExecutor()
	cfg := readyAutoDrainConfig()
	ad := NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Notify: func(context.Context, notify.NotificationEvent) error {
			return errors.New("ntfy down")
		},
	})
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ad.Tick(context.Background(), dischargingSample(now, 5))
	snap := ad.Tick(context.Background(), dischargingSample(now.Add(15*time.Minute), 5))
	if snap.State != AutoDrainIdle || snap.Notified {
		t.Fatalf("failed notify must not enter wait: %+v", snap)
	}
	if rec.Len() != 0 {
		t.Fatalf("timeout must not execute without a successful notify: %v", rec.Calls())
	}
}

func TestAutoDrainWarningThenYesRecordsSequence(t *testing.T) {
	rec := NewRecordingExecutor()
	cfg := readyAutoDrainConfig()
	var warned notify.NotificationEvent
	var ad *AutoDrain
	ad = NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Notify: func(_ context.Context, event notify.NotificationEvent) error {
			if ad.Snapshot().State != AutoDrainWarningSent {
				t.Fatalf("notify state %s", ad.Snapshot().State)
			}
			warned = event
			return nil
		},
	})
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	snap := ad.Tick(context.Background(), dischargingSample(now, 9))
	if snap.State != AutoDrainAwaitingResponse || !snap.Notified || !snap.AwaitingResponse {
		t.Fatalf("after notify %+v", snap)
	}
	if warned.Type != notify.EventAutoDrain || !strings.Contains(warned.Message, "[YES] Save+Stop") {
		t.Fatalf("copy %+v", warned)
	}
	if rec.Len() != 0 {
		t.Fatal("must wait for YES or timeout")
	}

	snap, err := ad.Respond(context.Background(), "yes")
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != AutoDrainExecuting {
		t.Fatalf("state %s", snap.State)
	}
	if snap.CommandsExecuted {
		t.Fatal("dry-run/real-off must not set commands_executed")
	}
	if got := strings.Join(rec.Calls(), ","); got != "stop_docker,sync,poweroff" {
		t.Fatalf("recorded %s", got)
	}
}

func TestAutoDrainUserNoContinuesOnBattery(t *testing.T) {
	rec := NewRecordingExecutor()
	cfg := readyAutoDrainConfig()
	ad := NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Notify:   func(context.Context, notify.NotificationEvent) error { return nil },
	})
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ad.Tick(context.Background(), dischargingSample(now, 4))
	snap, err := ad.Respond(context.Background(), "no")
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != AutoDrainAborted || snap.LastResponse != "no" {
		t.Fatalf("%+v", snap)
	}
	if rec.Len() != 0 {
		t.Fatalf("NO must not execute: %v", rec.Calls())
	}
	later := ad.Tick(context.Background(), dischargingSample(now.Add(20*time.Minute), 3))
	if later.State != AutoDrainAborted {
		t.Fatalf("should stay aborted until recover: %s", later.State)
	}
	if rec.Len() != 0 {
		t.Fatal("still must not execute after abort")
	}
}

func TestAutoDrainTimeoutTreatsAsYes(t *testing.T) {
	rec := NewRecordingExecutor()
	cfg := readyAutoDrainConfig()
	cfg.AutoDrain.ResponseTimeoutMinutes = 10
	ad := NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Notify:   func(context.Context, notify.NotificationEvent) error { return nil },
	})
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ad.Tick(context.Background(), dischargingSample(now, 8))
	snap := ad.Tick(context.Background(), dischargingSample(now.Add(10*time.Minute), 7))
	if snap.State != AutoDrainTimeout || snap.LastResponse != "timeout" {
		t.Fatalf("%+v", snap)
	}
	if snap.CommandsExecuted {
		t.Fatal("commands_executed")
	}
	if got := strings.Join(rec.Calls(), ","); got != "stop_docker,sync,poweroff" {
		t.Fatalf("recorded %s", got)
	}
}

func TestAutoDrainGates(t *testing.T) {
	rec := NewRecordingExecutor()
	cfg := readyAutoDrainConfig()
	cfg.Docker.StopEnabled = false
	ad := NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Notify:   func(context.Context, notify.NotificationEvent) error { return nil },
	})
	snap := ad.Tick(context.Background(), dischargingSample(time.Now().UTC(), 5))
	if snap.State != AutoDrainIdle {
		t.Fatalf("state %s", snap.State)
	}
	if rec.Len() != 0 {
		t.Fatal("docker.stop_enabled=false must block")
	}

	cfg = readyAutoDrainConfig()
	ad = NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Notify:   func(context.Context, notify.NotificationEvent) error { return nil },
	})
	charging := dischargingSample(time.Now().UTC(), 5)
	charging.Discharging = false
	charging.Status = "Charging"
	charging.Source = power.SourceAC
	snap = ad.Tick(context.Background(), charging)
	if snap.State != AutoDrainIdle || rec.Len() != 0 {
		t.Fatalf("charging must not start: %+v calls %v", snap, rec.Calls())
	}

	high := dischargingSample(time.Now().UTC(), 40)
	snap = ad.Tick(context.Background(), high)
	if snap.State != AutoDrainIdle || rec.Len() != 0 {
		t.Fatalf("above threshold must not start: %+v", snap)
	}
}

func TestAutoDrainACDuringWaitSkipsExecute(t *testing.T) {
	rec := NewRecordingExecutor()
	cfg := readyAutoDrainConfig()
	ad := NewAutoDrain(AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Notify:   func(context.Context, notify.NotificationEvent) error { return nil },
	})
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ad.Tick(context.Background(), dischargingSample(now, 5))
	ac := dischargingSample(now.Add(time.Minute), 5)
	ac.Discharging = false
	ac.Source = power.SourceAC
	snap := ad.Tick(context.Background(), ac)
	if snap.State != AutoDrainIdle {
		t.Fatalf("state %s", snap.State)
	}
	if rec.Len() != 0 {
		t.Fatalf("AC restore must skip execute: %v", rec.Calls())
	}
}

func TestAutoDrainInvalidRespond(t *testing.T) {
	ad := NewAutoDrain(AutoDrainOptions{
		Notify: func(context.Context, notify.NotificationEvent) error { return nil },
	})
	if _, err := ad.Respond(context.Background(), "yes"); !errors.Is(err, ErrNoPendingAutoDrain) {
		t.Fatalf("idle respond: %v", err)
	}
	if _, err := ad.Respond(context.Background(), "maybe"); !errors.Is(err, ErrInvalidAutoDrainAction) {
		t.Fatalf("invalid: %v", err)
	}
}

func readyAutoDrainConfig() config.Config {
	cfg := config.Config{
		AutoDrain: config.DefaultAutoDrain(),
		Docker:    config.DefaultDocker(),
		Safety:    config.DefaultSafety(),
		Actions:   config.DefaultActions(),
		Notifications: config.NotificationsConfig{
			Provider:   config.NotifyProviderNtfy,
			Enabled:    true,
			DryRun:     true,
			WebhookURL: "https://ntfy.example.invalid/lapguard",
		},
	}
	cfg.AutoDrain.Enabled = true
	cfg.Docker.StopEnabled = true
	return cfg
}

func dischargingSample(now time.Time, pct int) Sample {
	p := pct
	return Sample{
		Now:         now,
		Present:     true,
		Percent:     &p,
		Status:      "Discharging",
		Discharging: true,
		Source:      power.SourceBattery,
	}
}
