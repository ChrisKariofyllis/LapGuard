// Package testfake writes argv-recording stand-ins for systemctl, poweroff,
// docker, and sync. Tests use them so the real executor can run without
// touching the host. This package must not be used as a production action.
package testfake

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const script = `#!/bin/sh
# LapGuard test fake. Records argv and exits. Never powers off, never talks
# to Docker, never reboots, never calls the real binaries of the same name.
log=%s
{
  printf '%%s' "$0"
  for a in "$@"; do
    printf '\t%%s' "$a"
  done
  printf '\n'
} >> "$log" || exit 1
if [ -n "$LAPGUARD_TEST_STDOUT" ]; then
  printf '%%s' "$LAPGUARD_TEST_STDOUT"
fi
if [ -n "$LAPGUARD_TEST_SLEEP" ]; then
  exec sleep "$LAPGUARD_TEST_SLEEP"
fi
if [ -n "$LAPGUARD_TEST_EXIT" ]; then
  exit "$LAPGUARD_TEST_EXIT"
fi
exit 0
`

var allowedBase = map[string]struct{}{
	"systemctl": {},
	"poweroff":  {},
	"docker":    {},
	"sync":      {},
}

// Harness is a temp directory of fake executables plus an argv log.
type Harness struct {
	Dir    string
	Log    string
	Bins   map[string]string
	Sleep  string
	Exit   string
	Stdout string
}

// New creates fakes for systemctl, poweroff, docker, and sync under t.TempDir().
func New(t testing.TB) *Harness {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h := &Harness{
		Dir:  dir,
		Log:  logPath,
		Bins: map[string]string{},
	}
	for base := range allowedBase {
		path, err := Write(dir, base, logPath)
		if err != nil {
			t.Fatal(err)
		}
		h.Bins[base] = path
	}
	return h
}

// Write creates an executable named base in dir that appends argv to logPath.
func Write(dir, base, logPath string) (string, error) {
	if _, ok := allowedBase[base]; !ok {
		return "", fmt.Errorf("test fake basename %q is not allowed", base)
	}
	if dir == "" || !filepath.IsAbs(dir) {
		return "", fmt.Errorf("test fake dir must be absolute")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, base)
	body := fmt.Sprintf(script, strconv.Quote(logPath))
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		return "", err
	}
	return path, nil
}

// Path returns the fake executable for an allowed basename.
func (h *Harness) Path(base string) string {
	return h.Bins[base]
}

// LookPath resolves only the fake binaries in this harness.
func (h *Harness) LookPath(file string) (string, error) {
	p, ok := h.Bins[file]
	if !ok {
		return "", fmt.Errorf("testfake: %s not in harness", file)
	}
	return p, nil
}

// Runner execs argv with no shell, and only if name is under the harness dir.
func (h *Harness) Runner() func(ctx context.Context, name string, args ...string) ([]byte, error) {
	root := h.Dir
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if err := Allow(root, name); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = append(os.Environ(),
			"LAPGUARD_TEST_SLEEP="+h.Sleep,
			"LAPGUARD_TEST_EXIT="+h.Exit,
			"LAPGUARD_TEST_STDOUT="+h.Stdout,
		)
		return cmd.CombinedOutput()
	}
}

// Allow reports whether name is an allowed fake under root.
func Allow(root, name string) error {
	if name == "" || !filepath.IsAbs(name) {
		return fmt.Errorf("refusing non-absolute exec")
	}
	base := filepath.Base(name)
	if _, ok := allowedBase[base]; !ok {
		return fmt.Errorf("refusing basename %s", base)
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(name))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing exec outside test dir")
	}
	return nil
}

// Calls returns recorded argv, one slice per invocation.
func (h *Harness) Calls() [][]string {
	raw, err := os.ReadFile(h.Log)
	if err != nil {
		return nil
	}
	var out [][]string
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		out = append(out, strings.Split(line, "\t"))
	}
	return out
}

// Joined is space-joined argv lines for assertions.
func (h *Harness) Joined() []string {
	calls := h.Calls()
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}
