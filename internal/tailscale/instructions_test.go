package tailscale

import (
	"strings"
	"testing"
)

func TestInstructionLinesRecommendServe(t *testing.T) {
	text := InstructionsText()
	if !strings.Contains(text, "127.0.0.1:8585") {
		t.Fatal("instructions must mention the loopback listen address")
	}
	if !strings.Contains(text, "tailscale status") {
		t.Fatal("missing tailscale status")
	}
	if !strings.Contains(text, "tailscale ip -4") {
		t.Fatal("missing tailscale ip -4")
	}
	if !strings.Contains(text, "tailscale serve status") {
		t.Fatal("missing tailscale serve status")
	}
	if !strings.Contains(text, serveCommand) {
		t.Fatalf("missing Serve command %q", serveCommand)
	}
	if !strings.Contains(text, "no application-level authentication") {
		t.Fatal("missing authentication warning")
	}
	if !strings.Contains(text, "ACLs") {
		t.Fatal("missing ACL security-boundary note")
	}
	if !strings.Contains(strings.ToLower(text), "serve") {
		t.Fatal("instructions must recommend Serve")
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "tailscale funnel") || strings.HasPrefix(lower, "sudo tailscale funnel") {
			t.Fatalf("instructions must not contain Funnel as an action: %s", trimmed)
		}
	}
}

func TestInstructionsTextTrailingNewline(t *testing.T) {
	if !strings.HasSuffix(InstructionsText(), "\n") {
		t.Fatal("expected trailing newline")
	}
}
