package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != DefaultListen {
		t.Fatalf("listen %q", cfg.Listen)
	}
	if cfg.Provider != "auto" {
		t.Fatalf("provider %q", cfg.Provider)
	}
	if !cfg.Loopback() {
		t.Fatal("default listen should be loopback")
	}
}

func TestParseRejectsUnknownProvider(t *testing.T) {
	if _, err := Parse([]string{"-provider", "postgres"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRejectsBadListen(t *testing.T) {
	if _, err := Parse([]string{"-listen", "not-an-addr"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseThresholdMethod(t *testing.T) {
	cfg, err := Parse([]string{"-threshold-method", "tlp"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ThresholdMethod != "tlp" {
		t.Fatalf("got %q", cfg.ThresholdMethod)
	}
}

func TestPersistentConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	src := defaults()
	src.Listen = "127.0.0.1:9999"
	src.ThresholdMethod = "auto"
	if err := src.Save(path); err != nil {
		t.Fatal(err)
	}

	cfg, err := Parse([]string{"-config", path, "-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = ApplyPersistentConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9999" {
		t.Fatalf("listen from file: %q", cfg.Listen)
	}
	if cfg.Provider != "mock" {
		t.Fatalf("flag should overlay file: %q", cfg.Provider)
	}
}

func TestPersistentConfigWritesOnFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lapguard", "config.json")
	cfg, err := Parse([]string{"-config", path})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = ApplyPersistentConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ShouldWrite() {
		t.Fatal("expected first-run write")
	}
	if err := cfg.Save(cfg.ConfigPath); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ThresholdMethod != "auto" {
		t.Fatalf("threshold_method %q", loaded.ThresholdMethod)
	}
}

func TestResolveThresholdMethod(t *testing.T) {
	m, warn := ResolveThresholdMethod("auto", "sysfs")
	if m != "sysfs" || warn != "" {
		t.Fatalf("%s %q", m, warn)
	}
	m, warn = ResolveThresholdMethod("tlp", "sysfs")
	if m != "tlp" {
		t.Fatalf("user tlp override got %s", m)
	}
	m, warn = ResolveThresholdMethod("sysfs", "none")
	if m != "none" || warn == "" {
		t.Fatalf("unavailable sysfs should fall back, got %s %q", m, warn)
	}
}

func TestSaveMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := defaults()
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o, want 0600", info.Mode().Perm())
	}
}

func TestSaveCreatesParentAndIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "lapguard", "config.json")
	cfg := defaults()
	cfg.Shutdown.WarningThreshold = 25
	cfg.Shutdown.CriticalThreshold = 8
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Shutdown.WarningThreshold != 25 || loaded.Shutdown.CriticalThreshold != 8 {
		t.Fatalf("loaded shutdown %+v", loaded.Shutdown)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file %s", e.Name())
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("persisted JSON is not valid: %s", raw)
	}

	cfg.Shutdown.WarningThreshold = 40
	cfg.Shutdown.CriticalThreshold = 15
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Shutdown.WarningThreshold != 40 {
		t.Fatalf("second write did not replace atomically: %+v", reloaded.Shutdown)
	}
}

func TestSaveFailureLeavesOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write into 0555 directories")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "lapguard")
	path := filepath.Join(sub, "config.json")
	original := defaults()
	original.Listen = "127.0.0.1:1111"
	if err := original.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })

	next := defaults()
	next.Listen = "127.0.0.1:2222"
	if err := next.Save(path); err == nil {
		t.Fatal("expected save to fail")
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Listen != "127.0.0.1:1111" {
		t.Fatalf("original config was mutated: %q", loaded.Listen)
	}
}

func TestLoadFileTightensMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"listen":"127.0.0.1:8585","provider":"auto"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o, want 0600 after load", info.Mode().Perm())
	}
}

func TestLoadFileKeepsSettingsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"listen":"127.0.0.1:8585","provider":"auto"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notifications.Provider != "none" {
		t.Fatalf("notifications %+v", cfg.Notifications)
	}
	if cfg.Shutdown.WarningThreshold != 20 || cfg.Shutdown.CriticalThreshold != 10 {
		t.Fatalf("shutdown %+v", cfg.Shutdown)
	}
	if cfg.Docker.TimeoutSeconds != 30 {
		t.Fatalf("docker %+v", cfg.Docker)
	}
}

func TestShutdownThresholdValidation(t *testing.T) {
	s := DefaultShutdown()
	s.WarningThreshold = 101
	if err := s.normalize(); err == nil || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("want invalid warning, got %v", err)
	}
	s = DefaultShutdown()
	s.CriticalThreshold = -1
	if err := s.normalize(); err == nil {
		t.Fatal("want invalid critical")
	}
	s = DefaultShutdown()
	s.WarningThreshold = 10
	s.CriticalThreshold = 10
	if err := s.normalize(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("equal thresholds should fail, got %v", err)
	}
	s = DefaultShutdown()
	s.WarningThreshold = 10
	s.CriticalThreshold = 20
	if err := s.normalize(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("critical above warning should fail, got %v", err)
	}
	s = DefaultShutdown()
	s.WarningThreshold = 15
	s.CriticalThreshold = 5
	if err := s.normalize(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationValidationDoesNotEmbedSecrets(t *testing.T) {
	n := NotificationsConfig{
		Provider:   "webhook",
		Enabled:    true,
		WebhookURL: "https://hooks.example.invalid/secret-token-abc",
	}
	if err := n.normalize(); err != nil {
		t.Fatal(err)
	}
	n.WebhookURL = "not a url"
	err := n.normalize()
	if err == nil {
		t.Fatal("expected invalid webhook")
	}
	if strings.Contains(err.Error(), "not a url") {
		t.Fatalf("validation error leaked webhook value: %v", err)
	}
}

func TestRedactingHandlerStripsWebhookURLs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewRedactingHandler(slog.NewTextHandler(&buf, nil)))
	secret := "https://hooks.example.invalid/secret-token-abc"
	log.Info("updated",
		"webhook_url", secret,
		"token", "abc123",
		"password", "hunter2",
		"path", "/api/v1/config",
	)
	out := buf.String()
	for _, leak := range []string{secret, "secret-token-abc", "abc123", "hunter2"} {
		if strings.Contains(out, leak) {
			t.Fatalf("secret %q appeared in logs: %s", leak, out)
		}
	}
	if !strings.Contains(out, "path=/api/v1/config") {
		t.Fatalf("safe field was redacted: %s", out)
	}
	if !strings.Contains(out, redacted) {
		t.Fatalf("expected redacted marker: %s", out)
	}
}

func TestAtomicWriteFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "file.json")
	if err := atomicWriteFile(path, []byte(`{"ok":true}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
}

func TestConfigFileModeConstant(t *testing.T) {
	if ConfigFileMode != 0o600 {
		t.Fatalf("ConfigFileMode %o", ConfigFileMode)
	}
	if ConfigFileMode != fs.FileMode(0o600) {
		t.Fatal("expected fs.FileMode 0600")
	}
}
