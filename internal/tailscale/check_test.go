package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordedCall struct {
	Name string
	Args []string
}

type recordingRunner struct {
	mu    sync.Mutex
	calls []recordedCall
	run   func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, recordedCall{Name: name, Args: append([]string(nil), args...)})
	r.mu.Unlock()
	if r.run != nil {
		return r.run(ctx, name, args...)
	}
	return nil, errors.New("no runner")
}

func (r *recordingRunner) Calls() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

func TestCheckMissingBinary(t *testing.T) {
	rec := &recordingRunner{}
	report, err := Check(context.Background(), Options{
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		Run:      rec.Run,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.TailscaleInstalled {
		t.Fatal("expected tailscale_installed false")
	}
	if report.LapguardListen != ExpectedListen {
		t.Fatalf("listen %q", report.LapguardListen)
	}
	if report.RecommendedAccess != RecommendedAccess {
		t.Fatalf("recommended_access %q", report.RecommendedAccess)
	}
	if report.TailnetConnected != nil || report.ServeConfigured != nil || report.IPv4 != "" {
		t.Fatalf("undetermined fields should be omitted: %+v", report)
	}
	joined := strings.Join(report.Warnings, "\n")
	if !strings.Contains(joined, "not found") && !strings.Contains(strings.ToLower(joined), "install") {
		t.Fatalf("expected install guidance, got %q", joined)
	}
	if !strings.Contains(joined, "https://tailscale.com/download") {
		t.Fatalf("expected download guidance, got %q", joined)
	}
	if len(rec.Calls()) != 0 {
		t.Fatalf("must not execute commands when binary is missing: %v", rec.Calls())
	}
	assertNoSudo(t, rec.Calls())
	assertJSONRoundTrip(t, report)
}

func TestCheckSuccessfulMockedRunner(t *testing.T) {
	rec := &recordingRunner{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "tailscale" && name != "/usr/bin/tailscale" {
			t.Errorf("unexpected binary %q", name)
		}
		switch strings.Join(args, " ") {
		case "version":
			return []byte("1.84.0\n  tailscale commit: abcdef\n"), nil
		case "status":
			return []byte("100.64.1.2  laptop  alice@github  linux  -\n"), nil
		case "ip -4":
			return []byte("100.64.1.2\n"), nil
		case "serve status":
			return []byte("https://laptop.tailnet.ts.net (tailnet only)\n|-- / proxy http://127.0.0.1:8585\n"), nil
		default:
			return nil, errors.New("unexpected " + strings.Join(args, " "))
		}
	}}
	report, err := Check(context.Background(), Options{
		LookPath: func(string) (string, error) { return "/usr/bin/tailscale", nil },
		Run:      rec.Run,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.TailscaleInstalled {
		t.Fatal("expected installed")
	}
	if report.TailscaleVersion != "1.84.0" {
		t.Fatalf("version %q", report.TailscaleVersion)
	}
	if report.IPv4 != "100.64.1.2" {
		t.Fatalf("ipv4 %q", report.IPv4)
	}
	if report.TailnetConnected == nil || !*report.TailnetConnected {
		t.Fatal("expected tailnet_connected true")
	}
	if report.ServeConfigured == nil || !*report.ServeConfigured {
		t.Fatal("expected serve_configured true")
	}
	if report.LapguardListen != "127.0.0.1:8585" {
		t.Fatalf("listen %q", report.LapguardListen)
	}
	if report.RecommendedAccess != "tailscale_serve" {
		t.Fatalf("access %q", report.RecommendedAccess)
	}
	raw, _ := json.Marshal(report)
	s := string(raw)
	for _, forbidden := range []string{"alice@github", "laptop.tailnet.ts.net"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("JSON leaked %q: %s", forbidden, s)
		}
	}
	assertNoSudo(t, rec.Calls())
	if len(rec.Calls()) != 4 {
		t.Fatalf("expected 4 read-only commands, got %v", rec.Calls())
	}
}

func TestCheckCommandTimeout(t *testing.T) {
	rec := &recordingRunner{run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	start := time.Now()
	report, err := Check(context.Background(), Options{
		LookPath: func(string) (string, error) { return "tailscale", nil },
		Run:      rec.Run,
		Timeout:  25 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("check hung despite per-command timeout")
	}
	if !report.TailscaleInstalled {
		t.Fatal("binary was present")
	}
	joined := strings.ToLower(strings.Join(report.Warnings, "\n"))
	if !strings.Contains(joined, "timed out") {
		t.Fatalf("expected timeout warning, got %q", joined)
	}
	assertNoSudo(t, rec.Calls())
}

func TestCheckRedactsSecretsFromOutput(t *testing.T) {
	rec := &recordingRunner{run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "version":
			return []byte("1.80.0\n"), nil
		case "status":
			return []byte("Logged in as alice@example.com\nLog in at: https://login.tailscale.com/a/SECRETTOKEN\nAuth tskey-auth-abc123\nnodekey:deadbeef\n100.64.9.9  host  alice@github  linux\n"), nil
		case "ip -4":
			return []byte("100.64.9.9\n"), nil
		case "serve status":
			return []byte("https://secret-host.tailnet.ts.net\n|-- / proxy http://127.0.0.1:8585\n"), nil
		default:
			return nil, errors.New("unexpected")
		}
	}}
	report, err := Check(context.Background(), Options{
		LookPath: func(string) (string, error) { return "tailscale", nil },
		Run:      rec.Run,
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteReport(&buf, report, true); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, forbidden := range []string{
		"alice@example.com",
		"SECRETTOKEN",
		"tskey-auth-abc123",
		"deadbeef",
		"login.tailscale.com",
		"secret-host.tailnet.ts.net",
		"alice@github",
	} {
		if strings.Contains(s, forbidden) {
			t.Errorf("leaked %q in JSON:\n%s", forbidden, s)
		}
	}
	if report.IPv4 != "100.64.9.9" {
		t.Fatalf("ipv4 %q", report.IPv4)
	}
	if !strings.Contains(s, `"lapguard_listen": "127.0.0.1:8585"`) {
		t.Fatalf("missing listen:\n%s", s)
	}
	assertNoSudo(t, rec.Calls())
}

func TestCheckNeverExecutesSudo(t *testing.T) {
	rec := &recordingRunner{run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "sudo" || strings.EqualFold(name, "sudo") {
			t.Fatal("sudo executed")
		}
		for _, a := range args {
			if strings.EqualFold(a, "sudo") || a == "--bg" || strings.EqualFold(a, "funnel") {
				t.Fatalf("forbidden arg %q", a)
			}
		}
		return []byte("No serve config\n"), nil
	}}
	if _, err := Check(context.Background(), Options{
		LookPath: func(string) (string, error) { return "tailscale", nil },
		Run:      rec.Run,
	}); err != nil {
		t.Fatal(err)
	}
	assertNoSudo(t, rec.Calls())
	for _, c := range rec.Calls() {
		if err := allow(c.Name, c.Args); err != nil {
			t.Errorf("executed non-allowlisted command %s %v: %v", c.Name, c.Args, err)
		}
	}
}

func TestWriteReportAlwaysListenAndServe(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, Report{}, false); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, `"lapguard_listen":"127.0.0.1:8585"`) {
		t.Fatal(s)
	}
	if !strings.Contains(s, `"recommended_access":"tailscale_serve"`) {
		t.Fatal(s)
	}
	if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
		t.Fatal("expected trailing newline")
	}
}

func assertNoSudo(t *testing.T, calls []recordedCall) {
	t.Helper()
	for _, c := range calls {
		if strings.EqualFold(filepathBase(c.Name), "sudo") {
			t.Fatalf("sudo executed: %#v", c)
		}
		for _, a := range c.Args {
			if strings.EqualFold(a, "sudo") {
				t.Fatalf("sudo argument: %#v", c)
			}
		}
	}
}

func filepathBase(name string) string {
	i := strings.LastIndexAny(name, `/\`)
	if i < 0 {
		return name
	}
	return name[i+1:]
}

func assertJSONRoundTrip(t *testing.T, report Report) {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteReport(&buf, report, false); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("invalid JSON %s", buf.Bytes())
	}
	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["lapguard_listen"] != ExpectedListen {
		t.Fatalf("listen %v", parsed["lapguard_listen"])
	}
	if parsed["recommended_access"] != RecommendedAccess {
		t.Fatalf("access %v", parsed["recommended_access"])
	}
	if _, ok := parsed["warnings"]; !ok {
		t.Fatal("missing warnings")
	}
	if _, ok := parsed["instructions"]; !ok {
		t.Fatal("missing instructions")
	}
}
