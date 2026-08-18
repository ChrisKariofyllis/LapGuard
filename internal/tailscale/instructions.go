package tailscale

import "strings"

const (
	// ExpectedListen is the loopback address LapGuard is meant to use.
	// Diagnostics always report this value; they do not change the HTTP bind.
	ExpectedListen = "127.0.0.1:8585"

	// RecommendedAccess is the only remote-access method this CLI endorses.
	RecommendedAccess = "tailscale_serve"

	serveCommand = "sudo tailscale serve --bg http://127.0.0.1:8585"
)

// InstructionLines is the safe setup copy printed by
// `lapguard tailscale instructions` and included in check JSON.
// It never executes commands.
func InstructionLines() []string {
	return []string{
		"LapGuard listens only on 127.0.0.1:8585. Tailscale Serve is an external reverse proxy in front of that localhost process.",
		"Do not bind LapGuard to a Tailscale 100.x.y.z address, to 0.0.0.0, or to a public interface.",
		"Install and authenticate Tailscale yourself. LapGuard never runs sudo and never changes Tailscale state.",
		"Start LapGuard locally (for a release binary: ./lapguard -web-dir none) and open http://127.0.0.1:8585 on this machine.",
		"Read-only status commands you can run yourself:",
		"  tailscale status",
		"  tailscale ip -4",
		"  tailscale serve status",
		"To expose the localhost dashboard on your tailnet, configure Tailscale Serve yourself:",
		"  " + serveCommand,
		"If that syntax is rejected, check the installed CLI with: tailscale serve --help",
		"Recommend Tailscale Serve, not Funnel. Do not use Tailscale Funnel or expose port 8585 on the public Internet.",
		"LapGuard has no application-level authentication. Tailscale identity and ACLs are the security boundary. Allow only trusted users and devices.",
	}
}

// InstructionsText is the human-readable form of InstructionLines.
func InstructionsText() string {
	return strings.Join(InstructionLines(), "\n") + "\n"
}
