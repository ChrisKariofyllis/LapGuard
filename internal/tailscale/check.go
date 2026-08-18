package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	defaultCommandTimeout = 4 * time.Second
	installHint           = "The tailscale executable was not found on PATH. Install Tailscale separately from https://tailscale.com/download and authenticate this machine, then configure Serve."
)

var (
	ipv4LineRe    = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
	versionLineRe = regexp.MustCompile(`(?i)(?:^|\b)(\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?)`)
	noServeRe     = regexp.MustCompile(`(?i)no serve config|serve is not enabled|not serving`)
	funnelHintRe  = regexp.MustCompile(`(?i)\bfunnel\b|public internet|funnel on`)
	loggedOutRe   = regexp.MustCompile(`(?i)logged out|not logged in|needs login`)
	stoppedRe     = regexp.MustCompile(`(?i)tailscale is stopped|\bstopped\.\s*$`)
	proxyLocalRe  = regexp.MustCompile(`(?i)127\.0\.0\.1:8585|localhost:8585`)
	serveActiveRe = regexp.MustCompile(`(?i)\bproxy\b|https://|\|--`)
)

// Report is the JSON payload of `lapguard tailscale check`.
type Report struct {
	TailscaleInstalled bool     `json:"tailscale_installed"`
	TailscaleVersion   string   `json:"tailscale_version,omitempty"`
	TailnetConnected   *bool    `json:"tailnet_connected,omitempty"`
	IPv4               string   `json:"ipv4,omitempty"`
	ServeConfigured    *bool    `json:"serve_configured,omitempty"`
	LapguardListen     string   `json:"lapguard_listen"`
	RecommendedAccess  string   `json:"recommended_access"`
	Warnings           []string `json:"warnings"`
	Instructions       []string `json:"instructions"`
}

// Options inject LookPath / command execution for tests. Empty fields use
// the real host PATH and exec.CommandContext. Callers never need Tailscale
// libraries; only the tailscale CLI is probed.
type Options struct {
	LookPath func(file string) (string, error)
	Run      func(ctx context.Context, name string, args ...string) ([]byte, error)
	Timeout  time.Duration
}

func (o Options) withDefaults() Options {
	if o.LookPath == nil {
		o.LookPath = lookPathTailscale
	}
	if o.Run == nil {
		o.Run = runCommand
	}
	if o.Timeout <= 0 {
		o.Timeout = defaultCommandTimeout
	}
	return o
}

func lookPathTailscale(file string) (string, error) {
	if file != "tailscale" {
		return "", errors.New("refusing PATH lookup of a binary other than tailscale")
	}
	return exec.LookPath(file)
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func newReport() Report {
	return Report{
		LapguardListen:    ExpectedListen,
		RecommendedAccess: RecommendedAccess,
		Warnings:          []string{},
		Instructions:      InstructionLines(),
	}
}

func boolPtr(v bool) *bool { return &v }

// Check runs read-only Tailscale diagnostics. It never executes sudo, Funnel,
// Serve configuration, or any command outside the allowlist.
func Check(ctx context.Context, opts Options) (Report, error) {
	opts = opts.withDefaults()
	r := newReport()

	path, err := opts.LookPath("tailscale")
	if err != nil || strings.TrimSpace(path) == "" {
		r.TailscaleInstalled = false
		r.Warnings = append(r.Warnings, installHint)
		r.Warnings = redactWarnings(r.Warnings)
		return r, nil
	}
	r.TailscaleInstalled = true

	versionOut, versionErr := runAllowed(ctx, opts, path, "version")
	if versionErr != nil {
		r.Warnings = append(r.Warnings, commandWarn("version", versionErr))
	} else {
		r.TailscaleVersion = parseVersion(versionOut)
	}

	statusOut, statusErr := runAllowed(ctx, opts, path, "status")
	if statusErr != nil {
		r.Warnings = append(r.Warnings, commandWarn("status", statusErr))
	}

	ipOut, ipErr := runAllowed(ctx, opts, path, "ip", "-4")
	if ipErr != nil {
		r.Warnings = append(r.Warnings, commandWarn("ip -4", ipErr))
	} else {
		r.IPv4 = firstIPv4(ipOut)
	}

	serveOut, serveErr := runAllowed(ctx, opts, path, "serve", "status")
	if serveErr != nil {
		r.Warnings = append(r.Warnings, commandWarn("serve status", serveErr))
	} else {
		configured, known := parseServe(serveOut)
		if known {
			r.ServeConfigured = boolPtr(configured)
		}
		if funnelHintRe.MatchString(serveOut) {
			r.Warnings = append(r.Warnings, "Tailscale Funnel appears to be enabled. Disable Funnel. LapGuard has no application-level authentication and must not be exposed to the public Internet.")
		}
		if known && !configured {
			r.Warnings = append(r.Warnings, "Tailscale Serve does not appear to be configured for this node. Run "+serveCommand+" yourself (LapGuard will not).")
		} else if known && configured && !proxyLocalRe.MatchString(serveOut) {
			r.Warnings = append(r.Warnings, "Serve is configured but does not mention 127.0.0.1:8585. Point Serve at http://127.0.0.1:8585.")
		}
	}

	if connected, known := parseConnected(statusOut, statusErr, r.IPv4, ipErr); known {
		r.TailnetConnected = boolPtr(connected)
		if !connected {
			r.Warnings = append(r.Warnings, "Tailscale does not appear to be connected to a tailnet. Authenticate this machine with Tailscale before using Serve.")
		}
	}

	r.Warnings = redactWarnings(r.Warnings)
	return r, nil
}

func runAllowed(ctx context.Context, opts Options, path string, args ...string) (string, error) {
	if err := allow(path, args); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmdCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	out, err := opts.Run(cmdCtx, path, args...)
	text := Redact(string(out))
	if err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return text, fmtTimeout(args)
		}
		if cmdCtx.Err() != nil {
			return text, cmdCtx.Err()
		}
		if text != "" {
			return text, err
		}
		return "", err
	}
	return text, nil
}

func fmtTimeout(args []string) error {
	return fmt.Errorf("tailscale %s timed out", strings.Join(args, " "))
}

func commandWarn(cmd string, err error) string {
	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(msg), "timed out") {
		return "tailscale " + cmd + " timed out (read-only probe; no state was changed)"
	}
	return "tailscale " + cmd + " failed: " + Redact(msg)
}

func parseVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(Redact(line))
		if line == "" || strings.HasPrefix(line, "[redacted]") {
			continue
		}
		if m := versionLineRe.FindStringSubmatch(line); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func firstIPv4(out string) string {
	for _, m := range ipv4LineRe.FindAllString(out, -1) {
		ip := net.ParseIP(m)
		if ip == nil || ip.To4() == nil {
			continue
		}
		return m
	}
	return ""
}

func parseServe(out string) (configured bool, known bool) {
	text := strings.TrimSpace(out)
	if text == "" {
		return false, true
	}
	if noServeRe.MatchString(text) {
		return false, true
	}
	if serveActiveRe.MatchString(text) || proxyLocalRe.MatchString(text) {
		return true, true
	}
	return false, true
}

func parseConnected(status string, statusErr error, ipv4 string, ipErr error) (connected bool, known bool) {
	if ipErr == nil && ipv4 != "" {
		return true, true
	}
	if statusErr != nil && strings.TrimSpace(status) == "" {
		return false, false
	}
	if loggedOutRe.MatchString(status) || stoppedRe.MatchString(status) {
		return false, true
	}
	if ipv4LineRe.MatchString(status) {
		return true, true
	}
	if strings.TrimSpace(status) == "" {
		return false, false
	}
	return false, false
}

func redactWarnings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, w := range in {
		w = Redact(w)
		if w == "" {
			continue
		}
		out = append(out, w)
	}
	return out
}

// WriteReport encodes Report as JSON on w. pretty indents with two spaces.
func WriteReport(w io.Writer, report Report, pretty bool) error {
	if report.Warnings == nil {
		report.Warnings = []string{}
	}
	if report.Instructions == nil {
		report.Instructions = InstructionLines()
	}
	if report.LapguardListen == "" {
		report.LapguardListen = ExpectedListen
	}
	if report.RecommendedAccess == "" {
		report.RecommendedAccess = RecommendedAccess
	}
	var (
		raw []byte
		err error
	)
	if pretty {
		raw, err = json.MarshalIndent(report, "", "  ")
	} else {
		raw, err = json.Marshal(report)
	}
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}
