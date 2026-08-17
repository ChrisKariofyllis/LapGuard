package config

import (
	"path/filepath"
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
