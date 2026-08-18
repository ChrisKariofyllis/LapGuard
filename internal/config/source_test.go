package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeDisplayPathRedactsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	got := SafeDisplayPath(filepath.Join(home, ".config", "lapguard", "config.json"))
	if got != "~/.config/lapguard/config.json" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, home) {
		t.Fatal("home directory leaked")
	}
}

func TestSafeDisplayPathKeepsSystemConfig(t *testing.T) {
	got := SafeDisplayPath("/etc/lapguard/config.json")
	if got != "/etc/lapguard/config.json" {
		t.Fatalf("got %q", got)
	}
}

func TestSafeDisplayPathExplicitIsTruncated(t *testing.T) {
	got := SafeDisplayPath("/tmp/lg-preflight-test/config.json")
	if got != "…/lg-preflight-test/config.json" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "/tmp/") {
		t.Fatal("full explicit path leaked")
	}
}

func TestLoadClassifiesConfigSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := defaults()
	cfg.Actions.RealEnabled = false
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load([]string{"-config", path, "-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ConfigSource() != ConfigSourceCLI {
		t.Fatalf("source %q", loaded.ConfigSource())
	}
	if loaded.SafeConfigPath() != "…/"+filepath.Base(dir)+"/config.json" {
		t.Fatalf("path %q", loaded.SafeConfigPath())
	}

	missing := filepath.Join(dir, "missing", "config.json")
	first, err := Load([]string{"-config", missing, "-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ConfigSource() != ConfigSourceCLI {
		t.Fatalf("missing explicit file source %q", first.ConfigSource())
	}

	parsed, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ConfigSource() != ConfigSourceDefault {
		t.Fatalf("parse source %q", parsed.ConfigSource())
	}

	parsed.ConfigPath = path
	fromDefault, err := ApplyPersistentConfig(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if fromDefault.ConfigSource() != ConfigSourceFile {
		t.Fatalf("default-path file source %q", fromDefault.ConfigSource())
	}
}

func TestDiskEditDoesNotChangeLoadedConfigUntilReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	initial := defaults()
	initial.Provider = "mock"
	if err := initial.Save(path); err != nil {
		t.Fatal(err)
	}
	runtime, err := Load([]string{"-config", path, "-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Actions.RealEnabled || !runtime.Safety.DryRun {
		t.Fatalf("runtime %+v %+v", runtime.Actions, runtime.Safety)
	}

	onDisk, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	onDisk.Actions.RealEnabled = true
	onDisk.Safety.DryRun = false
	if err := onDisk.Save(path); err != nil {
		t.Fatal(err)
	}

	if runtime.Actions.RealEnabled || !runtime.Safety.DryRun {
		t.Fatal("in-memory config must ignore a later disk edit")
	}
	disk, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !disk.Actions.RealEnabled || disk.Safety.DryRun {
		t.Fatal("disk file should hold the edited values")
	}

	restarted, err := Load([]string{"-config", path, "-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.Actions.RealEnabled || restarted.Safety.DryRun {
		t.Fatalf("restart did not load disk edit: %+v %+v", restarted.Actions, restarted.Safety)
	}
}

func TestLogStartupOmitsSecretsAndRawHomePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	cfg := defaults()
	cfg.ConfigPath = filepath.Join(home, ".config", "lapguard", "config.json")
	cfg.Source = ConfigSourceFile
	cfg.Notifications.Provider = "ntfy"
	cfg.Notifications.WebhookURL = "https://ntfy.example.invalid/secret-topic"
	cfg.Notifications.ChatID = "chat-secret-99"
	cfg.Auth.TokenHash = "deadbeef"

	var buf bytes.Buffer
	log := slog.New(NewRedactingHandler(slog.NewTextHandler(&buf, nil)))
	cfg.LogStartup(log)
	out := buf.String()
	if !strings.Contains(out, "action configuration") {
		t.Fatalf("missing action configuration log: %s", out)
	}
	if !strings.Contains(out, "source=file") || !strings.Contains(out, "real_enabled=false") || !strings.Contains(out, "safety_dry_run=true") {
		t.Fatalf("gates missing: %s", out)
	}
	if !strings.Contains(out, ConfigReloadRestartRequired) {
		t.Fatalf("reload status missing: %s", out)
	}
	for _, leak := range []string{
		home,
		"secret-topic",
		"chat-secret-99",
		"deadbeef",
		"https://ntfy.example.invalid",
		"poweroff_path",
		"/usr/bin/",
		"token_hash",
	} {
		if strings.Contains(out, leak) {
			t.Fatalf("startup log leaked %q: %s", leak, out)
		}
	}
}
