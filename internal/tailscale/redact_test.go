package tailscale

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	in := strings.Join([]string{
		"Logged in as alice@example.com",
		"operator alice",
		"Log in at: https://login.tailscale.com/a/SECRETTOKEN",
		"Auth: tskey-auth-abc123XYZ",
		"nodekey:nodekeyABCDEF",
		"https://my-laptop.tailnet.ts.net",
		"100.64.1.2  my-laptop  bob@github  linux",
		"proxy http://127.0.0.1:8585",
	}, "\n")
	out := Redact(in)
	for _, forbidden := range []string{
		"alice@example.com",
		"SECRETTOKEN",
		"tskey-auth-abc123XYZ",
		"nodekeyABCDEF",
		"login.tailscale.com",
		"my-laptop.tailnet.ts.net",
		"bob@github",
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("leaked %q in:\n%s", forbidden, out)
		}
	}
	if !strings.Contains(out, "100.64.1.2") {
		t.Fatalf("should keep Tailscale IPv4, got:\n%s", out)
	}
	if !strings.Contains(out, "127.0.0.1:8585") {
		t.Fatalf("should keep loopback endpoint, got:\n%s", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Fatal("expected redaction marker")
	}
}
