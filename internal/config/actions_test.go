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

func TestActionsRejectsUnsafePaths(t *testing.T) {
	a := DefaultActions()
	rel := "systemctl"
	if _, err := a.Apply(ActionsPatch{PowerOffPath: &rel}); err == nil {
		t.Fatal("relative poweroff_path should fail")
	}
	shell := "/usr/bin/systemctl;reboot"
	if _, err := a.Apply(ActionsPatch{PowerOffPath: &shell}); err == nil {
		t.Fatal("metacharacters should fail")
	}
	bad := "/usr/bin/reboot"
	if _, err := a.Apply(ActionsPatch{PowerOffPath: &bad}); err == nil {
		t.Fatal("reboot basename should fail")
	}
	ok := "/usr/bin/systemctl"
	out, err := a.Apply(ActionsPatch{PowerOffPath: &ok})
	if err != nil {
		t.Fatal(err)
	}
	if out.PowerOffPath != ok {
		t.Fatalf("path %q", out.PowerOffPath)
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
