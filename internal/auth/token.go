package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// Prefix marks LapGuard API tokens so logs can redact them without
	// storing or printing the secret.
	Prefix     = "lg_"
	hashPrefix = "sha256:"
	tokenBytes = 32
)

// Generate returns a high-entropy bearer token. Callers must show it once
// (CLI stdout) and store only Hash(token).
func Generate() (string, error) {
	var raw [tokenBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return Prefix + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// Hash returns a SHA-256 digest suitable for high-entropy bearer tokens.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hashPrefix + hex.EncodeToString(sum[:])
}

// Verify reports whether token matches storedHash using a constant-time compare.
// Missing hashes and empty tokens never match.
func Verify(storedHash, token string) bool {
	token = strings.TrimSpace(token)
	storedHash = strings.TrimSpace(storedHash)
	if token == "" || storedHash == "" {
		return false
	}
	got := Hash(token)
	if len(got) != len(storedHash) {
		// Still compare to keep the timing closer to the success path.
		subtle.ConstantTimeCompare([]byte(got), []byte(got))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}

// LooksLikeToken is used by log redaction. It never validates a live secret.
func LooksLikeToken(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, Prefix) {
		return false
	}
	return len(s) > len(Prefix)+16
}
