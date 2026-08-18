package actions

import (
	"context"
	"strings"
	"testing"
	"time"

	"lapguard/internal/actions/testfake"
	"lapguard/internal/config"
	"lapguard/internal/power"
	"lapguard/internal/safety"
)

func TestAutomaticControllerNeverInvokesRealExecutor(t *testing.T) {
	h := testfake.New(t)
	var ran bool
	ex := &RealExecutor{
		DockerPath:   h.Path("docker"),
		PowerOffPath: h.Path("systemctl"),
		SyncPath:     h.Path("sync"),
		DockerTO:     time.Second,
		PowerOffTO:   time.Second,
		SyncTO:       time.Second,
		LookPath:     h.LookPath,
		Run: func(context.Context, string, ...string) ([]byte, error) {
			ran = true
			t.Error("automatic controller invoked the real executor")
			return nil, nil
		},
	}
	cfg := config.Config{
		Shutdown: config.ShutdownConfig{
			Enabled:           true,
			WarningThreshold:  20,
			CriticalThreshold: 10,
		},
		Docker: config.DockerConfig{StopEnabled: true, TimeoutSeconds: 30},
		Safety: config.SafetyConfig{
			DryRun:          false,
			RequireACLoss:   true,
			CooldownSeconds: 0,
		},
		Actions: config.ActionsConfig{RealEnabled: true},
	}
	c := safety.New(safety.Options{
		Config:   func() config.Config { return cfg },
		Executor: ex,
	})
	pct := 5
	snap := c.Tick(context.Background(), safety.Sample{
		Now:         time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC),
		Present:     true,
		Percent:     &pct,
		Status:      "Discharging",
		Discharging: true,
		Source:      power.SourceBattery,
	})
	if ran {
		t.Fatal("real executor Run must not be called")
	}
	if len(h.Calls()) != 0 {
		t.Fatalf("fake argv %v", h.Calls())
	}
	if snap.CommandsExecuted {
		t.Fatal("commands_executed must stay false")
	}
	if snap.State != safety.StateShutdownPending {
		t.Fatalf("state %s", snap.State)
	}
	if len(snap.IntendedActions) == 0 {
		t.Fatal("controller should still record an intended plan")
	}
	plan := strings.Join(snap.IntendedActions, ",")
	if !strings.Contains(plan, safety.ActionPowerOff) || !strings.Contains(plan, safety.ActionSync) {
		t.Fatalf("plan %s", plan)
	}
}

func TestAutomaticControllerDoesNotExecFakes(t *testing.T) {
	h := testfake.New(t)
	ex := &RealExecutor{
		DockerPath:   h.Path("docker"),
		PowerOffPath: h.Path("poweroff"),
		SyncPath:     h.Path("sync"),
		Run:          h.Runner(),
	}
	cfg := config.Config{
		Shutdown: config.ShutdownConfig{
			Enabled:           true,
			WarningThreshold:  20,
			CriticalThreshold: 10,
		},
		Docker: config.DockerConfig{StopEnabled: true},
		Safety: config.SafetyConfig{DryRun: false, RequireACLoss: true},
	}
	c := safety.New(safety.Options{
		Config:   func() config.Config { return cfg },
		Executor: ex,
	})
	pct := 4
	snap := c.Tick(context.Background(), safety.Sample{
		Now:         time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC),
		Present:     true,
		Percent:     &pct,
		Status:      "Discharging",
		Discharging: true,
		Source:      power.SourceBattery,
	})
	if snap.CommandsExecuted {
		t.Fatal("commands_executed")
	}
	if len(h.Calls()) != 0 {
		t.Fatalf("Tick must not exec fakes: %v", h.Calls())
	}
}
