package config

import "testing"

func TestDefaultActionsNeverReady(t *testing.T) {
	cfg := defaults()
	if cfg.Actions.RealEnabled {
		t.Fatal("real_enabled must default false")
	}
	if !cfg.Safety.DryRun {
		t.Fatal("dry_run must default true")
	}
	if !cfg.Safety.RequireACLoss {
		t.Fatal("require_ac_loss must default true")
	}
	if cfg.ManualActionsReady() {
		t.Fatal("manual actions must not be ready by default")
	}
	if cfg.hostExecutionState() != ExecutionDisabled {
		t.Fatalf("execution %s", cfg.hostExecutionState())
	}
	plan := cfg.IntendedPlan()
	if len(plan) != 2 || plan[0] != "sync" || plan[1] != "poweroff" {
		t.Fatalf("plan %v", plan)
	}
}

func TestRequireConfirmationCannotBeDisabled(t *testing.T) {
	a := DefaultActions()
	off := false
	out, err := a.Apply(ActionsPatch{RequireConfirmation: &off})
	if err != nil {
		t.Fatal(err)
	}
	if !out.RequireConfirmation {
		t.Fatal("require_confirmation must stay forced on")
	}
}

func TestHTTPActionsPatchRejectsExecPaths(t *testing.T) {
	a := DefaultActions()
	ok := "/usr/bin/systemctl"
	if _, err := a.Apply(ActionsPatch{PowerOffPath: &ok}); err == nil {
		t.Fatal("HTTP must not accept poweroff_path")
	}
	docker := "/usr/bin/docker"
	if _, err := a.Apply(ActionsPatch{DockerPath: &docker}); err == nil {
		t.Fatal("HTTP must not accept docker_path")
	}
}

func TestNormalizeValidatesExecPaths(t *testing.T) {
	a := DefaultActions()
	a.PowerOffPath = "systemctl"
	if err := a.normalize(); err == nil {
		t.Fatal("relative poweroff_path should fail")
	}
	a = DefaultActions()
	a.PowerOffPath = "/usr/bin/systemctl;reboot"
	if err := a.normalize(); err == nil {
		t.Fatal("metacharacters should fail")
	}
	a = DefaultActions()
	a.PowerOffPath = "/usr/bin/reboot"
	if err := a.normalize(); err == nil {
		t.Fatal("reboot basename should fail")
	}
	a = DefaultActions()
	a.PowerOffPath = "/usr/bin/systemctl"
	if err := a.normalize(); err != nil {
		t.Fatal(err)
	}
}

func TestManualActionsReadyRequiresBothGates(t *testing.T) {
	cfg := defaults()
	cfg.Actions.RealEnabled = true
	if cfg.ManualActionsReady() {
		t.Fatal("dry_run should still block")
	}
	if cfg.hostExecutionState() != ExecutionDryRun {
		t.Fatalf("state %s", cfg.hostExecutionState())
	}
	cfg.Safety.DryRun = false
	if !cfg.ManualActionsReady() {
		t.Fatal("expected ready")
	}
	if cfg.hostExecutionState() != ExecutionReady {
		t.Fatalf("state %s", cfg.hostExecutionState())
	}
}
