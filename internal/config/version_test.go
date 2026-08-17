package config

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultVersion(t *testing.T) {
	if Version != "dev" {
		t.Fatalf("untagged go test / go run Version = %q, want %q", Version, "dev")
	}
}

func TestVersionScriptNormalizes(t *testing.T) {
	script := versionScript(t)
	cases := []struct {
		in, want string
	}{
		{"v0.6.0-alpha", "0.6.0-alpha"},
		{"v0.6.0-alpha-dirty", "0.6.0-alpha"},
		{"0.5.0-alpha-dirty", "0.5.0-alpha"},
		{"dev", "dev"},
		{"v1.2.3", "1.2.3"},
	}
	for _, tc := range cases {
		out, err := exec.Command("sh", script, tc.in).Output()
		if err != nil {
			t.Fatalf("version.sh %q: %v", tc.in, err)
		}
		got := strings.TrimSpace(string(out))
		if got != tc.want {
			t.Fatalf("version.sh %q = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestVersionScriptDefaultHasNoDirty(t *testing.T) {
	out, err := exec.Command("sh", versionScript(t)).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(out))
	if got == "" {
		t.Fatal("version.sh with no args returned empty")
	}
	if strings.HasSuffix(got, "-dirty") {
		t.Fatalf("version.sh default includes -dirty: %q", got)
	}
}

func versionScript(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	script := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "version.sh")
	return script
}
