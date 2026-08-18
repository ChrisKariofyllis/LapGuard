package tailscale

import "testing"

func TestAllowlistAcceptsReadOnlyCommands(t *testing.T) {
	for _, args := range [][]string{
		{"status"},
		{"ip", "-4"},
		{"serve", "status"},
		{"version"},
	} {
		if err := allow("tailscale", args); err != nil {
			t.Errorf("allow tailscale %v: %v", args, err)
		}
		if err := allow("/usr/bin/tailscale", args); err != nil {
			t.Errorf("allow /usr/bin/tailscale %v: %v", args, err)
		}
	}
}

func TestAllowlistRejectsMutatingAndSudo(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "sudo", args: []string{"tailscale", "status"}},
		{name: "tailscale", args: []string{"up"}},
		{name: "tailscale", args: []string{"down"}},
		{name: "tailscale", args: []string{"serve", "--bg", "http://127.0.0.1:8585"}},
		{name: "tailscale", args: []string{"funnel"}},
		{name: "tailscale", args: []string{"funnel", "--bg", "8585"}},
		{name: "tailscale", args: []string{"serve", "reset"}},
		{name: "tailscale", args: []string{"login"}},
		{name: "tailscale", args: []string{"logout"}},
		{name: "tailscale", args: []string{"status", "--json"}},
	}
	for _, tc := range cases {
		if err := allow(tc.name, tc.args); err == nil {
			t.Errorf("expected reject %s %v", tc.name, tc.args)
		}
	}
}
