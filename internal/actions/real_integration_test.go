package actions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lapguard/internal/actions/testfake"
)

func TestRealExecutorRecordsArgvViaFakeBinaries(t *testing.T) {
	h := testfake.New(t)
	h.Stdout = "aaaaaaaaaaaa\nbbbbbbbbbbbb\n"
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
	if err := ex.StopDocker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ex.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := ex.PowerOff(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := h.Joined()
	want := []string{
		h.Path("docker") + " ps -q",
		h.Path("docker") + " stop aaaaaaaaaaaa",
		h.Path("docker") + " stop bbbbbbbbbbbb",
		h.Path("sync"),
		h.Path("systemctl") + " poweroff",
	}
	if len(got) != len(want) {
		t.Fatalf("calls %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call %d %q want %q", i, got[i], want[i])
		}
	}
	blob := strings.Join(got, "\n")
	if strings.Contains(blob, "sh -c") || strings.Contains(blob, "$(") || strings.Contains(blob, "-c ") {
		t.Fatalf("shell interpolation: %s", blob)
	}
}

func TestRealExecutorLookPathStaysInsideHarness(t *testing.T) {
	h := testfake.New(t)
	ex := &RealExecutor{
		LookPath:   h.LookPath,
		PowerOffTO: time.Second,
		Run:        h.Runner(),
	}
	if err := ex.PowerOff(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := h.Joined()
	if len(got) != 1 || got[0] != h.Path("systemctl")+" poweroff" {
		t.Fatalf("calls %v", got)
	}
}

func TestRealExecutorRejectsShellMetacharactersWithoutRunning(t *testing.T) {
	h := testfake.New(t)
	var ran bool
	ex := &RealExecutor{
		PowerOffPath: filepath.Join(h.Dir, "systemctl;reboot"),
		PowerOffTO:   time.Second,
		Run: func(context.Context, string, ...string) ([]byte, error) {
			ran = true
			return nil, nil
		},
	}
	err := ex.PowerOff(context.Background())
	if !errors.Is(err, ErrUnsafeArgs) {
		t.Fatalf("err %v", err)
	}
	if ran {
		t.Fatal("runner must not be called")
	}
	if len(h.Calls()) != 0 {
		t.Fatal("fake must not have been invoked")
	}
}

func TestRealExecutorTimeout(t *testing.T) {
	h := testfake.New(t)
	h.Sleep = "2"
	ex := &RealExecutor{
		PowerOffPath: h.Path("systemctl"),
		PowerOffTO:   200 * time.Millisecond,
		Run:          h.Runner(),
	}
	start := time.Now()
	err := ex.PowerOff(context.Background())
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout took too long: %v", time.Since(start))
	}
	if !errors.Is(err, ErrUnavailable) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
}

func TestRealExecutorContextCancel(t *testing.T) {
	h := testfake.New(t)
	h.Sleep = "2"
	ex := &RealExecutor{
		PowerOffPath: h.Path("systemctl"),
		PowerOffTO:   5 * time.Second,
		Run:          h.Runner(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	err := ex.PowerOff(ctx)
	if err == nil {
		t.Fatal("expected cancel")
	}
	if !errors.Is(err, ErrUnavailable) && !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
}

func TestDockerIDsValidatedBeforeStop(t *testing.T) {
	h := testfake.New(t)
	h.Stdout = "aaaaaaaaaaaa\n$(reboot)\nabc;rm\nnot valid\ncccccccccccc\n"
	ex := &RealExecutor{
		DockerPath: h.Path("docker"),
		DockerTO:   time.Second,
		Run:        h.Runner(),
	}
	if err := ex.StopDocker(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := h.Joined()
	wantStop := map[string]bool{
		h.Path("docker") + " stop aaaaaaaaaaaa": true,
		h.Path("docker") + " stop cccccccccccc": true,
	}
	if len(got) != 3 {
		t.Fatalf("calls %v", got)
	}
	if got[0] != h.Path("docker")+" ps -q" {
		t.Fatalf("first %q", got[0])
	}
	if !wantStop[got[1]] || !wantStop[got[2]] {
		t.Fatalf("stops %v", got[1:])
	}
	blob := strings.Join(got, "\n")
	if strings.Contains(blob, "$(reboot)") || strings.Contains(blob, "abc;rm") || strings.Contains(blob, "not valid") {
		t.Fatalf("invalid id reached stop: %s", blob)
	}
}

func TestRestrictedRunDoesNotTouchHostSync(t *testing.T) {
	h := testfake.New(t)
	run := h.Runner()
	if _, err := run(context.Background(), "/bin/sync"); err == nil {
		t.Fatal("host sync must be refused")
	}
	if len(h.Calls()) != 0 {
		t.Fatalf("host sync recorded: %v", h.Calls())
	}
}

func TestDefaultRunStillRefusesEvenWithFakePath(t *testing.T) {
	h := testfake.New(t)
	ex := &RealExecutor{PowerOffPath: h.Path("systemctl"), PowerOffTO: time.Millisecond}
	err := ex.PowerOff(context.Background())
	if !errors.Is(err, ErrRefusedInTest) {
		t.Fatalf("err %v", err)
	}
	if len(h.Calls()) != 0 {
		t.Fatal("defaultRun must not exec the fake either in tests")
	}
}

func TestLookPathEmptyDoesNotExecHostWhenRunnerIsRestricted(t *testing.T) {
	h := testfake.New(t)
	ex := &RealExecutor{
		PowerOffTO: time.Second,
		Run:        h.Runner(),
	}
	err := ex.PowerOff(context.Background())
	if err == nil {
		t.Fatal("host systemctl lookup must not succeed through restricted runner")
	}
	if len(h.Calls()) != 0 {
		t.Fatalf("recorded %v", h.Calls())
	}
	if _, statErr := os.Stat("/usr/bin/systemctl"); statErr == nil {
		if errors.Is(err, ErrRefusedInTest) {
			t.Fatal("restricted runner should refuse the host path before defaultRun")
		}
	}
}
