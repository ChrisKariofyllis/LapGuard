package tailscale

import (
	"regexp"
	"strings"
)

const redacted = "[redacted]"

var (
	emailRe = regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9._\-]+(?:\.[A-Za-z]{2,})?\b`)
	urlRe   = regexp.MustCompile(`(?i)\b(?:https?|wss?)://\S+`)
	tskeyRe = regexp.MustCompile(`(?i)\btskey-[a-z0-9\-]+`)
	nkeyRe  = regexp.MustCompile(`(?i)\b(?:mkey|nodekey|nkey|disco|key):[A-Za-z0-9+/=_\-]+`)
	tsnetRe = regexp.MustCompile(`(?i)\b[A-Za-z0-9._\-]+\.ts\.net\b`)
	// Tailscale status "user@github" / "user@passkey" login names.
	loginAtRe = regexp.MustCompile(`(?i)\b[A-Za-z0-9._\-]+@(?:github|google|microsoft|okta|passkey|apple|gitlab|azure)\b`)
)

// Redact strips emails, auth keys, login URLs, machine keys, MagicDNS names,
// and login-provider usernames from command output. Loopback URLs/addresses
// and Tailscale CGNAT IPv4 addresses are left intact so diagnostics stay usable.
func Redact(s string) string {
	s = urlRe.ReplaceAllStringFunc(s, redactURL)
	s = tskeyRe.ReplaceAllString(s, redacted)
	s = nkeyRe.ReplaceAllString(s, redacted)
	s = tsnetRe.ReplaceAllString(s, redacted)
	s = emailRe.ReplaceAllString(s, redacted)
	s = loginAtRe.ReplaceAllString(s, redacted)
	return strings.TrimSpace(s)
}

func redactURL(m string) string {
	lower := strings.ToLower(m)
	if strings.Contains(lower, "127.0.0.1") || strings.Contains(lower, "localhost") || strings.Contains(lower, "[::1]") {
		return m
	}
	// Keep the public download docs link used in install guidance.
	if strings.HasPrefix(lower, "https://tailscale.com/") && !strings.Contains(lower, "/a/") {
		return m
	}
	return redacted
}
