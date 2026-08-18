package actions

import (
	"context"
	"strings"
	"testing"
	"time"

	"lapguard/internal/actions/testfake"
	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/power"
	"lapguard/internal/safety"
)

func TestAutoDrainRealExecutorUsesTestFakes(t *testing.T) {
	h := testfake.New(t)
	h.Stdout = "aaaaaaaaaaaa\n"
	cfg := readyAutoDrainConfig()
	cfg.Safety.DryRun = false
	cfg.Actions.RealEnabled = true
	ex := &RealExecutor{
		DockerPath:   h.Path("docker"),
		PowerOffPath: h.Path("systemctl"),
		SyncPath:     h.Path("sync"),
		DockerTO:     time.Second,
		PowerOffTO:   time.Second,
		SyncTO:       time.Second,
		LookPath:     h.LookPath,
		Run:          h.Runner(),
	}
	rec := safety.NewRecordingExecutor()
	ad := safety.NewAutoDrain(safety.AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Live:     func() safety.ActionExecutor { return ex },
		Notify:   func(context.Context, notify.NotificationEvent) error { return nil },
	})
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ad.Tick(context.Background(), autoDrainSample(now, 5))
	snap, err := ad.Respond(context.Background(), "yes")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.CommandsExecuted {
		t.Fatalf("expected commands_executed: %+v", snap)
	}
	joined := strings.Join(h.Joined(), ";")
	if !strings.Contains(joined, "docker ps") || !strings.Contains(joined, "sync") || !strings.Contains(joined, "systemctl") {
		t.Fatalf("fake argv %v", h.Joined())
	}
}

func TestAutoDrainDoesNotInvokeLiveWhenDryRun(t *testing.T) {
	h := testfake.New(t)
	cfg := readyAutoDrainConfig()
	cfg.Safety.DryRun = true
	cfg.Actions.RealEnabled = true
	ex := &RealExecutor{
		DockerPath:   h.Path("docker"),
		PowerOffPath: h.Path("poweroff"),
		SyncPath:     h.Path("sync"),
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Error("live executor must not run while dry_run is true")
			return nil, nil
		},
	}
	rec := safety.NewRecordingExecutor()
	ad := safety.NewAutoDrain(safety.AutoDrainOptions{
		Config:   func() config.Config { return cfg },
		Executor: rec,
		Live:     func() safety.ActionExecutor { return ex },
		Notify:   func(context.Context, notify.NotificationEvent) error { return nil },
	})
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	ad.Tick(context.Background(), autoDrainSample(now, 5))
	if _, err := ad.Respond(context.Background(), "yes"); err != nil {
		t.Fatal(err)
	}
	if len(h.Calls()) != 0 {
		t.Fatalf("fakes invoked: %v", h.Calls())
	}
	if rec.Len() != 3 {
		t.Fatalf("recording calls %v", rec.Calls())
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

func autoDrainSample(now time.Time, pct int) safety.Sample {
	p := pct
	return safety.Sample{
		Now:         now,
		Present:     true,
		Percent:     &p,
		Status:      "Discharging",
		Discharging: true,
		Source:      power.SourceBattery,
	}
}
