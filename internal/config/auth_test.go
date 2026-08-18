package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthDefaultDisabled(t *testing.T) {
	cfg := defaults()
	if cfg.Auth.Enabled || cfg.Auth.TokenHash != "" {
		t.Fatalf("%+v", cfg.Auth)
	}
	view := cfg.APIView()
	if view.AuthEnabled || view.TokenConfigured {
		t.Fatalf("API view leaked auth: %+v", view)
	}
	if strings.Contains(strings.ToLower(fmtAPI(view)), "token_hash") {
		t.Fatal("API view included token_hash")
	}
}

func fmtAPI(v APIConfig) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestGenerateRotateDisable(t *testing.T) {
	cfg := defaults()
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	token, err := cfg.GenerateToken(now)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.Enabled || cfg.Auth.TokenHash == "" {
		t.Fatal("generate should enable and hash")
	}
	if token == cfg.Auth.TokenHash || strings.Contains(cfg.Auth.TokenHash, token) {
		t.Fatal("plaintext must not be stored")
	}
	if !cfg.Auth.VerifyToken(token) {
		t.Fatal("generated token should verify")
	}

	old := token
	next, err := cfg.RotateToken(now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if next == old {
		t.Fatal("rotate must mint a new token")
	}
	if cfg.Auth.VerifyToken(old) {
		t.Fatal("old token must be invalid after rotate")
	}
	if !cfg.Auth.VerifyToken(next) {
		t.Fatal("new token should verify")
	}

	cfg.DisableAuth()
	if cfg.Auth.Enabled {
		t.Fatal("disable should clear enabled")
	}
	if cfg.Auth.VerifyToken(next) {
		t.Fatal("disabled auth must not verify")
	}
}

func TestAuthPersistedWithoutPlaintext(t *testing.T) {
	cfg := defaults()
	token, err := cfg.GenerateToken(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatal("plaintext token written to config")
	}
	if !strings.Contains(string(raw), `"token_hash"`) {
		t.Fatal("expected hash in file")
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.VerifyToken(token) == false || !loaded.Auth.Enabled {
		t.Fatalf("loaded %+v", loaded.Auth)
	}
	view := loaded.APIView()
	if strings.Contains(fmtAPI(view), loaded.Auth.TokenHash) || strings.Contains(fmtAPI(view), token) {
		t.Fatal("API view leaked hash or token")
	}
}
