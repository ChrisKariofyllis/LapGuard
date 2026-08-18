package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"lapguard/internal/tailscale"
)

func TestRunTailscaleInstructions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runTailscaleOpts(&stdout, &stderr, []string{"instructions"}, tailscale.Options{
		LookPath: func(string) (string, error) { t.Fatal("instructions must not look up tailscale"); return "", nil },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("instructions must not execute commands")
			return nil, errors.New("no")
		},
	}); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %s", stderr.Bytes())
	}
	text := stdout.String()
	if !strings.Contains(text, "127.0.0.1:8585") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "sudo tailscale serve --bg http://127.0.0.1:8585") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "tailscale status") || !strings.Contains(text, "tailscale ip -4") || !strings.Contains(text, "tailscale serve status") {
		t.Fatal(text)
	}
	for _, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "tailscale funnel") || strings.HasPrefix(lower, "sudo tailscale funnel") {
			t.Fatalf("Funnel action: %s", line)
		}
	}
}

func TestRunTailscaleCheckMissingBinary(t *testing.T) {
	var stdout bytes.Buffer
	err := runTailscaleOpts(&stdout, ioDiscard(), []string{"check"}, tailscale.Options{
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("must not execute when missing")
			return nil, errors.New("no")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := stdout.Bytes()
	if !json.Valid(bytes.TrimSpace(raw)) {
		t.Fatalf("invalid JSON: %s", raw)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["tailscale_installed"] != false {
		t.Fatalf("%v", parsed["tailscale_installed"])
	}
	if parsed["lapguard_listen"] != "127.0.0.1:8585" {
		t.Fatalf("listen %v", parsed["lapguard_listen"])
	}
	if parsed["recommended_access"] != "tailscale_serve" {
		t.Fatalf("access %v", parsed["recommended_access"])
	}
}

func TestRunTailscaleCheckPrettyMock(t *testing.T) {
	var stdout bytes.Buffer
	err := runTailscaleOpts(&stdout, ioDiscard(), []string{"check", "--pretty"}, tailscale.Options{
		LookPath: func(string) (string, error) { return "tailscale", nil },
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "sudo" {
				t.Fatal("sudo")
			}
			switch strings.Join(args, " ") {
			case "version":
				return []byte("1.84.0\n"), nil
			case "status":
				return []byte("100.64.2.2  node  user@github  linux\n"), nil
			case "ip -4":
				return []byte("100.64.2.2\n"), nil
			case "serve status":
				return []byte("|-- / proxy http://127.0.0.1:8585\n"), nil
			default:
				t.Fatalf("unexpected %s %v", name, args)
				return nil, errors.New("unexpected")
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("\n  \"lapguard_listen\"")) {
		t.Fatalf("expected indented JSON, got %s", stdout.Bytes())
	}
	if strings.Contains(stdout.String(), "user@github") {
		t.Fatal("leaked login name")
	}
}

func TestRunTailscaleCheckTimeout(t *testing.T) {
	var stdout bytes.Buffer
	err := runTailscaleOpts(&stdout, ioDiscard(), []string{"check"}, tailscale.Options{
		LookPath: func(string) (string, error) { return "tailscale", nil },
		Timeout:  20 * time.Millisecond,
		Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(stdout.String()), "timed out") {
		t.Fatalf("expected timeout in JSON, got %s", stdout.Bytes())
	}
}

func TestRunTailscaleRequiresSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runTailscale(&stdout, &stderr, nil)
	if err == nil || !strings.Contains(err.Error(), "subcommand") {
		t.Fatalf("err %v stderr %s", err, stderr.String())
	}
}

func TestRunTailscaleHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runTailscale(&stdout, &stderr, []string{"-h"}); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("help on stdout: %s", stdout.Bytes())
	}
	if !strings.Contains(stderr.String(), "instructions") || !strings.Contains(stderr.String(), "check") {
		t.Fatalf("help %s", stderr.String())
	}
}

func TestRunTailscaleUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runTailscale(&stdout, &stderr, []string{"funnel"})
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout %s", stdout.Bytes())
	}
}
