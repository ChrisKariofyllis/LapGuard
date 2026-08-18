package tailscale

import (
	"fmt"
	"path/filepath"
	"strings"
)

// allowedArgs are the only tailscale argv tails diagnostics may run.
// `tailscale version` is read-only and used solely to fill tailscale_version.
var allowedArgs = [][]string{
	{"status"},
	{"ip", "-4"},
	{"serve", "status"},
	{"version"},
}

func allow(name string, args []string) error {
	base := filepath.Base(strings.TrimSpace(name))
	if !strings.EqualFold(base, "tailscale") {
		return fmt.Errorf("refusing to execute %q: only the tailscale binary is allowed", base)
	}
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "sudo", "funnel", "up", "down", "--bg", "login", "logout", "reset", "set":
			return fmt.Errorf("refusing Tailscale argument %q (read-only diagnostics)", a)
		}
	}
	if !argsAllowed(args) {
		return fmt.Errorf("command not in read-only allowlist: tailscale %s", strings.Join(args, " "))
	}
	return nil
}

func argsAllowed(args []string) bool {
	for _, want := range allowedArgs {
		if len(args) != len(want) {
			continue
		}
		ok := true
		for i := range want {
			if args[i] != want[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
