package thresholds

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"lapguard/internal/discovery"
)

func TestSysfsSetAndGet(t *testing.T) {
	dir := t.TempDir()
	startPath := filepath.Join(dir, "charge_control_start_threshold")
	endPath := filepath.Join(dir, "charge_control_end_threshold")
	if err := os.WriteFile(startPath, []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(endPath, []byte("100\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := New(discovery.ThresholdPlan{
		Method:    discovery.MethodSysfs,
		StartPath: startPath,
		EndPath:   endPath,
	}, nil)

	if err := c.Set(context.Background(), 40, 80); err != nil {
		t.Fatal(err)
	}
	start, end, err := c.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if start != 40 || end != 80 {
		t.Fatalf("got %d/%d", start, end)
	}
}

func TestTLPSetInvokesSetcharge(t *testing.T) {
	r := &recordRunner{}
	c := New(discovery.ThresholdPlan{
		Method:      discovery.MethodTLP,
		BatteryName: "BAT0",
	}, r)
	if err := c.Set(context.Background(), 50, 80); err != nil {
		t.Fatal(err)
	}
	if r.name != "tlp" {
		t.Fatalf("name %q", r.name)
	}
	want := []string{"setcharge", "50", "80", "BAT0"}
	if len(r.args) != len(want) {
		t.Fatalf("args %v", r.args)
	}
	for i := range want {
		if r.args[i] != want[i] {
			t.Fatalf("args %v, want %v", r.args, want)
		}
	}
}

func TestNoneSetDisabled(t *testing.T) {
	c := New(discovery.ThresholdPlan{Method: discovery.MethodNone}, nil)
	if err := c.Set(context.Background(), 40, 80); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetRejectsInvertedRange(t *testing.T) {
	c := New(discovery.ThresholdPlan{
		Method:    discovery.MethodSysfs,
		StartPath: filepath.Join(t.TempDir(), "start"),
		EndPath:   filepath.Join(t.TempDir(), "end"),
	}, nil)
	if err := c.Set(context.Background(), 80, 40); err == nil {
		t.Fatal("expected error")
	}
}

type recordRunner struct {
	name string
	args []string
}

func (r *recordRunner) LookPath(file string) (string, error) { return "/usr/bin/" + file, nil }

func (r *recordRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = append([]string{}, args...)
	return []byte("OK\n"), nil
}
