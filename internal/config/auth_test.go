package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthDefaultEnabled(t *testing.T) {
	cfg := defaults()
	if !cfg.Auth.Enabled {
		t.Fatal("auth.enabled must default true")
	}
	if !cfg.Auth.AllowLoopbackNoToken {
		t.Fatal("allow_loopback_no_token must default true")
	}
	if cfg.Auth.TokenHash != "" {
		t.Fatal("no token hash until minted")
	}
	view := cfg.APIView()
	if !view.AuthEnabled || view.TokenConfigured {
		t.Fatalf("API view %+v", view)
	}
	if !view.AllowLoopbackNoToken {
		t.Fatal("API view missing loopback flag")
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

func TestAuthMigrationMissingSectionEnables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"listen":"127.0.0.1:8585"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Auth.Enabled || !loaded.Auth.AllowLoopbackNoToken {
		t.Fatalf("missing auth section should take defaults: %+v", loaded.Auth)
	}
}

func TestAuthMigrationKeepsExplicitOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"auth":{"enabled":false}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Auth.Enabled {
		t.Fatal("explicit auth.enabled=false must be kept")
	}
	if !loaded.Auth.AllowLoopbackNoToken {
		t.Fatal("missing allow_loopback_no_token should default true")
	}
}

func TestEnsureAuthTokenMintsOnce(t *testing.T) {
	cfg := defaults()
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	token, minted, err := cfg.EnsureAuthToken(now)
	if err != nil {
		t.Fatal(err)
	}
	if !minted || token == "" || !strings.HasPrefix(token, "lg_") {
		t.Fatalf("minted=%t token %q", minted, token)
	}
	if token == cfg.Auth.TokenHash || strings.Contains(cfg.Auth.TokenHash, token) {
		t.Fatal("plaintext must not be stored")
	}
	again, minted2, err := cfg.EnsureAuthToken(now)
	if err != nil {
		t.Fatal(err)
	}
	if minted2 || again != "" {
		t.Fatal("second ensure must not mint")
	}
	if !cfg.Auth.VerifyToken(token) {
		t.Fatal("first token should still verify")
	}

	cfg.DisableAuth()
	none, minted3, err := cfg.EnsureAuthToken(now)
	if err != nil || minted3 || none != "" {
		t.Fatalf("disabled auth must not mint: minted=%t err=%v", minted3, err)
	}
}
