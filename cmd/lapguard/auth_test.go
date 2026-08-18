package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lapguard/internal/config"
)

func TestRunAuthStatusDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := mustParseConfig(t)
	cfg.ConfigPath = path
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := runAuth(&stdout, &stderr, []string{"status", "-config", path}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "auth_enabled=false") {
		t.Fatalf("%s", out)
	}
	if strings.Contains(out, "token_hash") || strings.Contains(strings.ToLower(out), "lg_") {
		t.Fatalf("status leaked secret: %s", out)
	}
}

func TestRunAuthGenerateRotateDisable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := mustParseConfig(t)
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	logbuf := bytes.Buffer{}
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(config.NewRedactingHandler(slog.NewTextHandler(&logbuf, nil))))

	if err := runAuth(&stdout, &stderr, []string{"generate", "-config", path}); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(token, "lg_") {
		t.Fatalf("token %q", token)
	}
	if strings.Contains(logbuf.String(), token) {
		t.Fatalf("token logged:\n%s", logbuf.String())
	}
	loaded, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Auth.Enabled || loaded.Auth.TokenHash == token {
		t.Fatal("must store hash only")
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), token) {
		t.Fatal("plaintext in config file")
	}

	stdout.Reset()
	stderr.Reset()
	if err := runAuth(&stdout, &stderr, []string{"generate", "-config", path}); err == nil {
		t.Fatal("second generate should fail; use rotate")
	}

	stdout.Reset()
	if err := runAuth(&stdout, &stderr, []string{"rotate", "-config", path}); err != nil {
		t.Fatal(err)
	}
	next := strings.TrimSpace(stdout.String())
	if next == token {
		t.Fatal("rotate must change token")
	}
	loaded, err = config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.VerifyToken(token) {
		t.Fatal("old token still valid")
	}
	if !loaded.Auth.VerifyToken(next) {
		t.Fatal("new token not valid")
	}

	stdout.Reset()
	if err := runAuth(&stdout, &stderr, []string{"disable", "-config", path}); err != nil {
		t.Fatal(err)
	}
	loaded, err = config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.Enabled {
		t.Fatal("still enabled")
	}
}

func TestRunAuthHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runAuth(&stdout, &stderr, []string{"-h"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "generate") {
		t.Fatalf("%s", stderr.String())
	}
}

func mustParseConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
