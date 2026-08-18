package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunDiscoverRequiresReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runDiscover(&stdout, &stderr, nil)
	if err == nil || !strings.Contains(err.Error(), "--report") {
		t.Fatalf("err %v, stderr %s", err, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty, got %s", stdout.Bytes())
	}
}

func TestRunDiscoverPrettyRequiresReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runDiscover(&stdout, &stderr, []string{"--pretty"})
	if err == nil || !strings.Contains(err.Error(), "--report") {
		t.Fatalf("err %v", err)
	}
}

func TestRunDiscoverReportOmitsSerialsAndHomePaths(t *testing.T) {
	sysfs := testdataSysfs(t)
	var stdout, stderr bytes.Buffer
	if err := runDiscover(&stdout, &stderr, []string{"--report", "--sysfs-root", sysfs}); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %s", stderr.Bytes())
	}
	raw := stdout.Bytes()
	if !json.Valid(bytes.TrimSpace(raw)) {
		t.Fatalf("invalid JSON: %s", raw)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		t.Fatal("expected trailing newline")
	}
	s := string(raw)
	for _, forbidden := range []string{
		"TEST-BAT-001",
		"/home/",
		"chat_id",
		`"hostname"`,
		`"timestamp"`,
		`"start_path"`,
		`"end_path"`,
	} {
		if strings.Contains(s, forbidden) {
			t.Errorf("report contains %q\n%s", forbidden, s)
		}
	}
	for _, want := range []string{
		`"manufacturer":"LGC"`,
		`"model":"FixturePack"`,
		`"name":"BAT0"`,
		`"naming_convention":"energy"`,
		`"schema_version":"1"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("report missing %s\n%s", want, s)
		}
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["available_tools"]; !ok {
		t.Fatal("expected available_tools")
	}
	if _, ok := parsed["kernel_modules"]; !ok {
		t.Fatal("expected kernel_modules")
	}
	if _, ok := parsed["features"]; !ok {
		t.Fatal("expected features")
	}
}

func TestRunDiscoverPretty(t *testing.T) {
	sysfs := testdataSysfs(t)
	var stdout bytes.Buffer
	if err := runDiscover(&stdout, ioDiscard(), []string{"--report", "--pretty", "--sysfs-root", sysfs}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("\n  \"schema_version\"")) {
		t.Fatalf("expected indented JSON, got %s", stdout.Bytes())
	}
	if strings.Contains(stdout.String(), "TEST-BAT-001") {
		t.Fatal("pretty report leaked serial")
	}
}

func TestRunDiscoverHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runDiscover(&stdout, &stderr, []string{"-h"}); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("help should not print JSON: %s", stdout.Bytes())
	}
	if !strings.Contains(stderr.String(), "--report") && !strings.Contains(stderr.String(), "-report") {
		t.Fatalf("help %s", stderr.String())
	}
}

func testdataSysfs(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "sysfs")
}

type discard struct{}

func ioDiscard() *discard { return &discard{} }

func (*discard) Write(p []byte) (int, error) { return len(p), nil }
