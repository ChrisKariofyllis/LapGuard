package config

import (
	"strings"
	"time"

	"lapguard/internal/auth"
)

// AuthConfig is API token authentication. The plaintext token is never stored;
// only SHA-256 is persisted (mode 0600 with the rest of config).
//
// Defaults (v0.9.6-alpha): Enabled=true and AllowLoopbackNoToken=true.
// Loopback PUT/POST may omit the Bearer token; remote clients must send one.
// GET routes stay readable without a token.
type AuthConfig struct {
	Enabled              bool   `json:"enabled"`
	AllowLoopbackNoToken bool   `json:"allow_loopback_no_token"`
	TokenHash            string `json:"token_hash,omitempty"`
	CreatedAt            string `json:"created_at,omitempty"`
	LastRotatedAt        string `json:"last_rotated_at,omitempty"`
}

// AuthView is the public HTTP/CLI representation. It never includes hashes
// or the plaintext token.
type AuthView struct {
	AuthEnabled          bool   `json:"auth_enabled"`
	TokenConfigured      bool   `json:"token_configured"`
	AllowLoopbackNoToken bool   `json:"allow_loopback_no_token"`
	TokenCreatedAt       string `json:"token_created_at,omitempty"`
	LastRotatedAt        string `json:"last_rotated_at,omitempty"`
	ProtectGET           bool   `json:"protect_get"`
}

func DefaultAuth() AuthConfig {
	return AuthConfig{
		Enabled:              true,
		AllowLoopbackNoToken: true,
	}
}

func (a AuthConfig) View() AuthView {
	return AuthView{
		AuthEnabled:          a.Enabled,
		TokenConfigured:      strings.TrimSpace(a.TokenHash) != "",
		AllowLoopbackNoToken: a.AllowLoopbackNoToken,
		TokenCreatedAt:       a.CreatedAt,
		LastRotatedAt:        a.LastRotatedAt,
		ProtectGET:           false,
	}
}

func (a AuthConfig) Warning() string {
	if !a.Enabled {
		return "authentication is disabled; state-changing HTTP routes are unauthenticated"
	}
	if a.AllowLoopbackNoToken {
		return "loopback PUT/POST may omit the Bearer token; remote clients must send Authorization"
	}
	return ""
}

func (a *AuthConfig) normalize() error {
	a.TokenHash = strings.TrimSpace(a.TokenHash)
	a.CreatedAt = strings.TrimSpace(a.CreatedAt)
	a.LastRotatedAt = strings.TrimSpace(a.LastRotatedAt)
	return nil
}

func (a AuthConfig) VerifyToken(token string) bool {
	if !a.Enabled {
		return false
	}
	return auth.Verify(a.TokenHash, token)
}

// GenerateToken creates a new bearer token, stores only its hash, and enables auth.
// The plaintext is returned for one-time CLI display.
func (c *Config) GenerateToken(now time.Time) (string, error) {
	token, err := auth.Generate()
	if err != nil {
		return "", err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ts := now.UTC().Format(time.RFC3339)
	c.Auth.TokenHash = auth.Hash(token)
	c.Auth.Enabled = true
	if c.Auth.CreatedAt == "" {
		c.Auth.CreatedAt = ts
	}
	c.Auth.LastRotatedAt = ts
	if err := c.Auth.normalize(); err != nil {
		return "", err
	}
	return token, nil
}

// RotateToken replaces the hash and invalidates the previous token.
func (c *Config) RotateToken(now time.Time) (string, error) {
	if strings.TrimSpace(c.Auth.TokenHash) == "" && !c.Auth.Enabled {
		return c.GenerateToken(now)
	}
	token, err := auth.Generate()
	if err != nil {
		return "", err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ts := now.UTC().Format(time.RFC3339)
	c.Auth.TokenHash = auth.Hash(token)
	c.Auth.Enabled = true
	if c.Auth.CreatedAt == "" {
		c.Auth.CreatedAt = ts
	}
	c.Auth.LastRotatedAt = ts
	return token, nil
}

func (c *Config) DisableAuth() {
	c.Auth.Enabled = false
}

// EnsureAuthToken mints a bearer token when auth is enabled and no hash exists.
// The plaintext is returned only when a new token is created. Callers must
// print it once to the operator (stderr) and must not attach it to slog.
func (c *Config) EnsureAuthToken(now time.Time) (token string, minted bool, err error) {
	if !c.Auth.Enabled {
		return "", false, nil
	}
	if strings.TrimSpace(c.Auth.TokenHash) != "" {
		return "", false, nil
	}
	token, err = c.GenerateToken(now)
	if err != nil {
		return "", false, err
	}
	return token, true, nil
}
