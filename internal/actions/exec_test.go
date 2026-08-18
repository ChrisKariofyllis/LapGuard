package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateArgvRejectsShell(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "sh", args: []string{"-c", "poweroff"}},
		{name: "/bin/sh", args: []string{"-c", "docker stop $(docker ps -q)"}},
		{name: "/usr/bin/docker", args: []string{"stop", "$(docker ps -q)"}},
		{name: "/usr/bin/docker", args: []string{"stop", "abc; rm -rf /"}},
		{name: "docker", args: []string{"ps", "-q"}},
		{name: "/usr/bin/systemctl", args: []string{"reboot"}},
		{name: "/usr/bin/systemctl", args: []string{"poweroff;reboot"}},
	}
	for _, tc := range cases {
		if err := validateArgv(tc.name, tc.args); err == nil {
			t.Errorf("expected reject %s %v", tc.name, tc.args)
		}
	}
	if err := validateArgv("/usr/bin/systemctl", []string{"poweroff"}); err != nil {
		t.Fatal(err)
	}
	if err := validateArgv("/usr/bin/docker", []string{"ps", "-q"}); err != nil {
		t.Fatal(err)
	}
}

func TestParseContainerIDs(t *testing.T) {
	got := parseContainerIDs([]byte("abc123def456\nnot valid!\n0123456789ab\n"))
	if len(got) != 2 || got[0] != "abc123def456" || got[1] != "0123456789ab" {
		t.Fatalf("%v", got)
	}
}

func TestRealExecutorUsesArgvNotShell(t *testing.T) {
	var calls [][]string
	ex := &RealExecutor{
		DockerPath:   "/usr/bin/docker",
		PowerOffPath: "/usr/bin/systemctl",
		SyncPath:     "/bin/sync",
		DockerTO:     time.Second,
		PowerOffTO:   time.Second,
		SyncTO:       time.Second,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls = append(calls, append([]string{name}, args...))
			if name == "/usr/bin/docker" && len(args) >= 1 && args[0] == "ps" {
				return []byte("aaaaaaaaaaaa\nbbbbbbbbbbbb\n"), nil
			}
			return nil, nil
		},
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
	joined := fmtCalls(calls)
	for _, bad := range []string{"sh", "-c", "$(", "docker ps -q", ";"} {
		if strings.Contains(joined, bad) && bad != "docker ps -q" {
			t.Fatalf("shell interpolation in %s", joined)
		}
	}
	if strings.Contains(joined, "sh -c") || strings.Contains(joined, "$(") {
		t.Fatalf("shell: %s", joined)
	}
	wantSeq := []string{
		"/usr/bin/docker ps -q",
		"/usr/bin/docker stop aaaaaaaaaaaa",
		"/usr/bin/docker stop bbbbbbbbbbbb",
		"/bin/sync",
		"/usr/bin/systemctl poweroff",
	}
	if len(calls) != len(wantSeq) {
		t.Fatalf("calls %v", calls)
	}
	for i, want := range wantSeq {
		got := strings.Join(calls[i], " ")
		if got != want {
			t.Fatalf("call %d %q want %q", i, got, want)
		}
	}
}

func TestDefaultRunRefusedDuringTests(t *testing.T) {
	ex := &RealExecutor{PowerOffPath: "/usr/bin/systemctl", PowerOffTO: time.Millisecond}
	err := ex.PowerOff(context.Background())
	if !errors.Is(err, ErrRefusedInTest) {
		t.Fatalf("err %v", err)
	}
}

func fmtCalls(calls [][]string) string {
	var b strings.Builder
	for _, c := range calls {
		b.WriteString(strings.Join(c, " "))
		b.WriteByte('\n')
	}
	return b.String()
}
