package testfake

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteRejectsDisallowedBasename(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "bash", filepath.Join(dir, "log")); err == nil {
		t.Fatal("bash must be rejected")
	}
	if _, err := Write(dir, "sh", filepath.Join(dir, "log")); err == nil {
		t.Fatal("sh must be rejected")
	}
}

func TestAllowRefusesHostBinaries(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"/usr/bin/systemctl", "/sbin/poweroff", "/usr/bin/docker", "/bin/sync", "/bin/sh"} {
		if err := Allow(root, name); err == nil {
			t.Fatalf("must refuse %s", name)
		}
	}
	fake, err := Write(root, "systemctl", filepath.Join(root, "argv.log"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Allow(root, fake); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerNeverExecsHostSystemctl(t *testing.T) {
	h := New(t)
	run := h.Runner()
	if _, err := run(context.Background(), "/usr/bin/systemctl", "poweroff"); err == nil {
		t.Fatal("host systemctl must not run")
	}
	if _, err := os.Stat(h.Log); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(h.Log)
	if strings.Contains(string(raw), "/usr/bin/systemctl") {
		t.Fatal("host argv was recorded; runner must not exec it")
	}
}

func TestFakeRecordsArgvWithoutShell(t *testing.T) {
	h := New(t)
	out, err := h.Runner()(context.Background(), h.Path("systemctl"), "poweroff")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("fake should be silent by default, got %q", out)
	}
	calls := h.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls %v", calls)
	}
	if filepath.Base(calls[0][0]) != "systemctl" || calls[0][1] != "poweroff" {
		t.Fatalf("argv %v", calls[0])
	}
	joined := strings.Join(calls[0], " ")
	if strings.Contains(joined, "-c") || strings.Contains(joined, "sh ") {
		t.Fatalf("shell interpolation: %s", joined)
	}
}
