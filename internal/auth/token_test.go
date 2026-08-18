package auth

import (
	"strings"
	"testing"
)

func TestGenerateHashVerify(t *testing.T) {
	a, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("tokens must be unique")
	}
	if !strings.HasPrefix(a, Prefix) {
		t.Fatalf("prefix %q", a)
	}
	hash := Hash(a)
	if hash == a || !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("hash %q", hash)
	}
	if !Verify(hash, a) {
		t.Fatal("valid token must verify")
	}
	if Verify(hash, b) {
		t.Fatal("other token must not verify")
	}
	if Verify(hash, "") || Verify("", a) || Verify("sha256:00", a) {
		t.Fatal("empty/wrong hash must not verify")
	}
}

func TestLooksLikeToken(t *testing.T) {
	tok, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !LooksLikeToken(tok) {
		t.Fatal("generated token should match")
	}
	if LooksLikeToken("lg_short") || LooksLikeToken("password") {
		t.Fatal("false positive")
	}
}
