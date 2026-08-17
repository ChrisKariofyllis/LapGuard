package config

import "testing"

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
