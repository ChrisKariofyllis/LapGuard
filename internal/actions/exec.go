package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"
)

var (
	ErrRefusedInTest = errors.New("real host commands are disabled during tests")
	ErrUnsafeArgs    = errors.New("refusing unsafe command arguments")
	ErrUnavailable   = errors.New("action executor is unavailable")
)

// Runner executes one argv. Tests inject fakes. Production uses exec.CommandContext.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// LookPath resolves a binary name to an absolute path.
type LookPath func(file string) (string, error)

// RealExecutor runs Docker drain, sync, and poweroff without a shell.
// Default exec.CommandContext is refused while `go test` is running unless
// a test injects Runner.
type RealExecutor struct {
	DockerPath   string
	PowerOffPath string
	SyncPath     string
	DockerTO     time.Duration
	PowerOffTO   time.Duration
	SyncTO       time.Duration
	LookPath     LookPath
	Run          Runner
}

func defaultLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func defaultRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	if testing.Testing() {
		return nil, ErrRefusedInTest
	}
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func (e *RealExecutor) look(file string) (string, error) {
	lp := e.LookPath
	if lp == nil {
		lp = defaultLookPath
	}
	path, err := lp(file)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrUnavailable, file)
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: resolved path for %s is not absolute", ErrUnavailable, file)
	}
	return path, nil
}

func (e *RealExecutor) run(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if err := validateArgv(name, args); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	run := e.Run
	if run == nil {
		run = defaultRun
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return run(ctx, name, args...)
}

func validateArgv(name string, args []string) error {
	if name == "" || !filepath.IsAbs(name) {
		return ErrUnsafeArgs
	}
	if strings.ContainsAny(name, "|&;<>$`\n\r \t") {
		return ErrUnsafeArgs
	}
	base := filepath.Base(name)
	switch base {
	case "docker", "systemctl", "poweroff", "sync":
	default:
		return ErrUnsafeArgs
	}
	if base == "systemctl" && (len(args) != 1 || args[0] != "poweroff") {
		return ErrUnsafeArgs
	}
	if base == "docker" {
		switch {
		case len(args) == 2 && args[0] == "ps" && args[1] == "-q":
		case len(args) == 2 && args[0] == "stop" && validContainerID(args[1]):
		default:
			return ErrUnsafeArgs
		}
	}
	if base == "sync" && len(args) != 0 {
		return ErrUnsafeArgs
	}
	if base == "poweroff" && len(args) != 0 {
		return ErrUnsafeArgs
	}
	for _, a := range args {
		if a == "-c" || a == "/c" || strings.ContainsAny(a, "|&;<>$`\n\r") {
			return ErrUnsafeArgs
		}
	}
	return nil
}

func validContainerID(id string) bool {
	id = strings.TrimSpace(id)
	if len(id) < 12 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if unicode.Is(unicode.ASCII_Hex_Digit, r) {
			continue
		}
		return false
	}
	return true
}

func redactOutput(raw []byte) string {
	s := string(bytes.TrimSpace(raw))
	if s == "" {
		return ""
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
