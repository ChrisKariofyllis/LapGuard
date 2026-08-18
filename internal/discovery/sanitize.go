package discovery

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const redacted = "[redacted]"

var (
	urlRe  = regexp.MustCompile(`(?i)\b(?:https?|wss?|ftp|file)://\S+`)
	uuidRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	macRe  = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{2}:){5}[0-9a-f]{2}\b`)
	ipv4Re = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
	// Full or compressed IPv6, including ::1. Applied after MAC/UUID so those
	// colon-separated forms are not treated as addresses.
	ipv6Re = regexp.MustCompile(`(?i)\b(?:(?:[0-9a-f]{1,4}:){1,7}[0-9a-f]{1,4}|(?:[0-9a-f]{1,4}:){1,7}:|:(?::[0-9a-f]{1,4}){1,7}|::1?)\b`)
	// Home directories and ~user / ~/path forms. Do not match "~20–80%".
	homePathRe    = regexp.MustCompile(`(?i)(?:/home/[^/\s]+(?:/\S*)?|/root(?:/\S*)?|~[a-z_][a-z0-9_-]*(?:/\S*)?|~/\S*)`)
	secretKVRe    = regexp.MustCompile(`(?i)\b(?:webhook_url|webhook|token|password|passwd|secret|api[_-]?key|chat[_-]?id|authorization)\s*[=:]\s*\S+`)
	unixAbsPathRe = regexp.MustCompile(`(?i)(?:^|[\s"'=])(/[^\s"']+)+`)
)

// sanitizeText strips identifiers that must never appear in a public report.
// Hardware names such as BAT0, manufacturer strings, and OS/kernel versions
// are left intact when they do not match a forbidden pattern.
func sanitizeText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = urlRe.ReplaceAllString(s, redacted)
	s = uuidRe.ReplaceAllString(s, redacted)
	s = macRe.ReplaceAllString(s, redacted)
	s = ipv4Re.ReplaceAllString(s, redacted)
	s = ipv6Re.ReplaceAllString(s, redacted)
	s = homePathRe.ReplaceAllString(s, redacted)
	s = secretKVRe.ReplaceAllString(s, redacted)
	s = redactEmbeddedPaths(s)
	return strings.TrimSpace(s)
}

func redactEmbeddedPaths(s string) string {
	return unixAbsPathRe.ReplaceAllStringFunc(s, func(m string) string {
		// Keep the leading delimiter; only the path is sensitive.
		i := strings.Index(m, "/")
		if i < 0 {
			return m
		}
		prefix := m[:i]
		path := m[i:]
		if keepSystemPath(path) {
			return m
		}
		return prefix + redacted
	})
}

func keepSystemPath(path string) bool {
	// Feature copy mentions Docker's socket. That is not a home directory
	// and does not identify a person.
	return path == "/var/run/docker.sock" || strings.HasPrefix(path, "/var/run/docker.sock")
}

func scrubText(s string, extras []string) string {
	s = sanitizeText(s)
	for _, id := range extras {
		s = redactIdent(s, id)
	}
	return strings.TrimSpace(s)
}

func redactIdent(s, ident string) string {
	ident = strings.TrimSpace(ident)
	if skipIdent(ident) || ident == "" {
		return s
	}
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(ident) + `\b`)
	return re.ReplaceAllString(s, redacted)
}

func skipIdent(s string) bool {
	if len(s) < 3 {
		return true
	}
	switch strings.ToLower(s) {
	case "linux", "ubuntu", "sysfs", "tlp", "none", "battery", "charge", "energy",
		"generic", "present", "kernel", "true", "false", "auto":
		return true
	}
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "BAT") && len(s) <= 5 {
		return true
	}
	return false
}

func secretValues(report CapabilityReport) []string {
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if skipIdent(s) {
			return
		}
		seen[s] = struct{}{}
	}
	addPath := func(p string) {
		if p == "" {
			return
		}
		add(p)
		if u := homeUser(p); u != "" {
			add(u)
		}
	}

	add(report.Hostname)
	if host, _, ok := strings.Cut(report.Hostname, "."); ok {
		add(host)
	}
	add(report.Battery.Serial)
	addPath(report.Battery.Path)
	for _, b := range report.Batteries {
		add(b.Serial)
		addPath(b.Path)
	}
	for _, a := range report.Adapters {
		add(a.Serial)
		addPath(a.Path)
	}
	addPath(report.Thresholds.StartPath)
	addPath(report.Thresholds.EndPath)

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func homeUser(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := 0; i < len(parts)-1; i++ {
		if strings.EqualFold(parts[i], "home") && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return ""
}
