package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCompatibilityFromStripsSecretsAndPaths(t *testing.T) {
	start, end := 40, 80
	online := true
	src := CapabilityReport{
		Timestamp: time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Hostname:  "alice-laptop",
		OS:        "Ubuntu 26.04 LTS",
		Kernel:    "6.17.0-generic",
		Battery: BatteryIdentity{
			Path:         "/home/alice/.local/sys/class/power_supply/BAT1",
			Name:         "BAT1",
			Present:      true,
			Manufacturer: "Fujitsu",
			Model:        "A3510-pack",
			Serial:       "SN-SECRET-99999",
			Technology:   "Li-ion",
		},
		Batteries: []Supply{{
			Name:            "BAT1",
			Path:            "/home/alice/.local/sys/class/power_supply/BAT1",
			Type:            "Battery",
			Present:         true,
			AvailableFields: []string{"voltage_now", "serial_number", "charge_now", "capacity"},
			Manufacturer:    "Fujitsu",
			Model:           "A3510-pack",
			Serial:          "SN-SECRET-99999",
			Technology:      "Li-ion",
		}},
		Adapters: []Supply{{
			Name:   "ADP1",
			Path:   "/home/alice/.local/sys/class/power_supply/ADP1",
			Type:   "Mains",
			Online: &online,
			Serial: "ADP-SERIAL-777",
		}},
		AvailableFields:  []string{"voltage_now", "serial_number", "charge_now", "capacity"},
		NamingConvention: "charge",
		PowerCalculation: "current_voltage",
		Features: Features{
			ChargeThresholds:      MethodSysfs,
			DerivedPowerSupported: true,
			CurrentVoltage:        true,
			PowerLossDetection:    true,
		},
		AvailableTools: Tools{TLP: true, TLPVersion: "1.8.0", TLPCanSet: false, UPower: true},
		KernelModules:  []string{"fujitsu_laptop"},
		ModuleDetails:  []ModuleDetail{{Name: "fujitsu_laptop", Loaded: true, Enables: "Fujitsu extras"}},
		Thresholds: ThresholdPlan{
			Method:          MethodSysfs,
			DetectionMethod: "sysfs_native",
			StartPath:       "/home/alice/.local/sys/class/power_supply/BAT1/charge_control_start_threshold",
			EndPath:         "/home/alice/.local/sys/class/power_supply/BAT1/charge_control_end_threshold",
			BatteryName:     "BAT1",
			Start:           &start,
			End:             &end,
			Writable:        false,
			WhyNot:          "see user alice at 192.0.2.10 mac 00:1a:2b:3c:4d:5e uuid 550e8400-e29b-41d4-a716-446655440000",
			Recommendation:  "open https://ntfy.sh/alice-secret-topic token=ghp_notarealtoken password=hunter2 chat_id=778899001",
		},
		Notes: []string{
			"webhook_url=https://discord.com/api/webhooks/123/token",
			"contact alice at 2001:db8::1",
			"serial SN-SECRET-99999 lives at /home/alice/config.json",
		},
	}

	got := CompatibilityFrom(src, "dev")
	if src.Battery.Serial != "SN-SECRET-99999" {
		t.Fatal("CompatibilityFrom must not mutate the telemetry report")
	}
	if src.Hostname != "alice-laptop" || src.Battery.Path == "" {
		t.Fatal("input hostname and paths must remain on the telemetry report")
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	assertNoLeaks(t, raw, []string{
		"SN-SECRET-99999",
		"ADP-SERIAL-777",
		"alice",
		"/home/",
		"alice-laptop",
		"192.0.2.10",
		"00:1a:2b:3c:4d:5e",
		"550e8400-e29b-41d4-a716-446655440000",
		"https://ntfy.sh/alice-secret-topic",
		"ghp_notarealtoken",
		"hunter2",
		"778899001",
		"discord.com",
		"2001:db8::1",
		"config.json",
	})

	if got.SchemaVersion != CompatibilitySchemaVersion {
		t.Fatalf("schema_version %q", got.SchemaVersion)
	}
	if got.LapGuardVersion != "dev" {
		t.Fatalf("version %q", got.LapGuardVersion)
	}
	if got.Manufacturer != "Fujitsu" || got.Model != "A3510-pack" {
		t.Fatalf("manufacturer/model %q %q", got.Manufacturer, got.Model)
	}
	if got.OS != "Ubuntu 26.04 LTS" || got.Kernel != "6.17.0-generic" {
		t.Fatalf("os/kernel %q %q", got.OS, got.Kernel)
	}
	if got.NamingConvention != "charge" || got.PowerCalculation != "current_voltage" {
		t.Fatalf("naming/power %q %q", got.NamingConvention, got.PowerCalculation)
	}
	if got.ChargeThresholdMethod != MethodSysfs || got.Thresholds.Method != MethodSysfs {
		t.Fatalf("threshold method %q / %q", got.ChargeThresholdMethod, got.Thresholds.Method)
	}
	if got.Thresholds.BatteryName != "BAT1" {
		t.Fatalf("battery_name %q", got.Thresholds.BatteryName)
	}
	if len(got.Batteries) != 1 || got.Batteries[0].Name != "BAT1" || !got.Batteries[0].Present {
		t.Fatalf("batteries %+v", got.Batteries)
	}
	if len(got.Adapters) != 1 || got.Adapters[0].Name != "ADP1" {
		t.Fatalf("adapters %+v", got.Adapters)
	}
	if !got.AvailableTools.TLP || !got.AvailableTools.UPower {
		t.Fatalf("tools %+v", got.AvailableTools)
	}
	if len(got.KernelModules) != 1 || got.KernelModules[0] != "fujitsu_laptop" {
		t.Fatalf("modules %v", got.KernelModules)
	}
	if !got.Features.DerivedPowerSupported || !got.Features.PowerLossDetection {
		t.Fatalf("features %+v", got.Features)
	}
	if len(got.FeatureDetails) == 0 {
		t.Fatal("expected feature_details")
	}
	wantAttrs := []string{"charge_control_end_threshold", "charge_control_start_threshold"}
	if strings.Join(got.Thresholds.Attributes, ",") != strings.Join(wantAttrs, ",") {
		t.Fatalf("attributes %v", got.Thresholds.Attributes)
	}
	if strings.Join(got.AvailableFields, ",") != "capacity,charge_now,serial_number,voltage_now" {
		t.Fatalf("available_fields not sorted: %v", got.AvailableFields)
	}
	if got.Thresholds.Start == nil || *got.Thresholds.Start != 40 {
		t.Fatalf("start %+v", got.Thresholds.Start)
	}

	var round map[string]any
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hostname", "timestamp", "serial", "path", "start_path", "end_path", "webhook_url", "chat_id", "token", "password"} {
		if _, ok := round[key]; ok {
			t.Errorf("export JSON must not include key %q", key)
		}
	}
}

func TestCompatibilityReportDeterministic(t *testing.T) {
	src := CapabilityReport{
		OS:               "Ubuntu Test",
		Kernel:           "6.17.0-test",
		AvailableFields:  []string{"voltage_now", "capacity", "charge_now"},
		NamingConvention: "charge",
		PowerCalculation: "current_voltage",
		Batteries: []Supply{
			{Name: "BAT1", Present: true, Manufacturer: "Fujitsu", Model: "FixturePack"},
			{Name: "BAT0", Present: true, Manufacturer: "LGC", Model: "Pack0"},
		},
		Adapters: []Supply{
			{Name: "ADP1"},
			{Name: "AC"},
		},
		KernelModules: []string{"asus_wmi", "fujitsu_laptop"},
		Features:      Features{ChargeThresholds: MethodNone, DerivedPowerSupported: true},
		Thresholds:    ThresholdPlan{Method: MethodNone, DetectionMethod: "sysfs+tlp+thinkpad_acpi"},
	}
	a, err := json.Marshal(CompatibilityFrom(src, "dev"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(CompatibilityFrom(src, "dev"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("marshal mismatch:\n%s\n%s", a, b)
	}
	got := CompatibilityFrom(src, "dev")
	if strings.Join(got.AvailableFields, ",") != "capacity,charge_now,voltage_now" {
		t.Fatalf("fields %v", got.AvailableFields)
	}
	if got.Batteries[0].Name != "BAT0" || got.Batteries[1].Name != "BAT1" {
		t.Fatalf("battery order %v %v", got.Batteries[0].Name, got.Batteries[1].Name)
	}
	if got.Adapters[0].Name != "AC" || got.Adapters[1].Name != "ADP1" {
		t.Fatalf("adapter order %v %v", got.Adapters[0].Name, got.Adapters[1].Name)
	}
	if strings.Join(got.KernelModules, ",") != "asus_wmi,fujitsu_laptop" {
		t.Fatalf("modules %v", got.KernelModules)
	}
}

func TestCompatibilityFromLiveFixtureOmitsSerialAndHomePath(t *testing.T) {
	user := "alice"
	root := filepath.Join(t.TempDir(), "home", user, "lapguard")
	writeBattery(t, root, "BAT0", map[string]string{
		"type":          "Battery",
		"present":       "1",
		"status":        "Discharging",
		"capacity":      "50",
		"charge_now":    "1000",
		"charge_full":   "2000",
		"voltage_now":   "11000000",
		"current_now":   "500000",
		"manufacturer":  "LGC",
		"model_name":    "FixturePack",
		"serial_number": "REAL-SERIAL-SHOULD-VANISH",
		"technology":    "Li-ion",
	})
	writeSupply(t, root, "AC", map[string]string{"type": "Mains", "online": "1"})
	writeModules(t, root)
	opts := optsFor(t, root, fakeRunner{})
	opts.Hostname = user + "-pc"

	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Battery.Serial != "REAL-SERIAL-SHOULD-VANISH" {
		t.Fatalf("telemetry should still see the serial, got %q", report.Battery.Serial)
	}
	if !strings.Contains(report.Battery.Path, filepath.Join("home", user)) {
		t.Fatalf("telemetry path should include the fixture home path, got %q", report.Battery.Path)
	}

	var buf bytes.Buffer
	if err := WriteCompatibilityReport(&buf, CompatibilityFrom(report, "dev"), false); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	assertNoLeaks(t, raw, []string{
		"REAL-SERIAL-SHOULD-VANISH",
		user + "-pc",
		"/home/",
		"home/" + user,
	})
	if !bytes.Contains(raw, []byte(`"name":"BAT0"`)) {
		t.Fatalf("expected BAT0 in export: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"manufacturer":"LGC"`)) {
		t.Fatalf("expected manufacturer: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"model":"FixturePack"`)) {
		t.Fatalf("expected model: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"naming_convention":"charge"`)) {
		t.Fatalf("expected naming convention: %s", raw)
	}
	if !json.Valid(bytes.TrimSpace(raw)) {
		t.Fatal("export is not valid JSON")
	}
}

func TestWriteCompatibilityReportPretty(t *testing.T) {
	rep := CompatibilityFrom(CapabilityReport{
		OS:         "linux",
		Kernel:     "6.17.0-test",
		Features:   Features{ChargeThresholds: MethodNone},
		Thresholds: ThresholdPlan{Method: MethodNone},
	}, "dev")
	var compact, pretty bytes.Buffer
	if err := WriteCompatibilityReport(&compact, rep, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteCompatibilityReport(&pretty, rep, true); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(compact.Bytes(), []byte("\n")) || !bytes.HasSuffix(pretty.Bytes(), []byte("\n")) {
		t.Fatal("stdout JSON must end with a newline")
	}
	if bytes.Contains(compact.Bytes(), []byte("\n  ")) {
		t.Fatal("compact JSON should not be indented")
	}
	if !bytes.Contains(pretty.Bytes(), []byte("\n  \"schema_version\"")) && !bytes.Contains(pretty.Bytes(), []byte("\n  \"os\"")) {
		t.Fatalf("pretty JSON should be indented:\n%s", pretty.Bytes())
	}
	var a, b map[string]any
	if err := json.Unmarshal(compact.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pretty.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizeTextPreservesChargeAdvice(t *testing.T) {
	in := "Keep the pack between ~20–80% with a smart plug or by unplugging; firmware charge limits are not available."
	if got := sanitizeText(in); got != in {
		t.Fatalf("sanitizer changed hardware advice:\n got %q\nwant %q", got, in)
	}
}

func TestSanitizeTextForbiddenPatterns(t *testing.T) {
	cases := []struct {
		in   string
		deny []string
	}{
		{in: "user /home/alice/.config/lapguard", deny: []string{"/home/", "alice"}},
		{in: "ip 192.0.2.10 and 2001:db8::ff", deny: []string{"192.0.2.10", "2001:db8::ff"}},
		{in: "mac aa:bb:cc:dd:ee:ff", deny: []string{"aa:bb:cc:dd:ee:ff"}},
		{in: "uuid 550e8400-e29b-41d4-a716-446655440000", deny: []string{"550e8400-e29b-41d4-a716-446655440000"}},
		{in: "https://ntfy.sh/secret-topic", deny: []string{"https://", "ntfy.sh", "secret-topic"}},
		{in: "token=abc123 password=hunter2 chat_id=42", deny: []string{"abc123", "hunter2", "42"}},
		{in: "~/secret/file and ~alice/bin", deny: []string{"~/secret", "~alice"}},
	}
	for _, tc := range cases {
		got := sanitizeText(tc.in)
		for _, d := range tc.deny {
			if strings.Contains(got, d) {
				t.Errorf("sanitizeText(%q) = %q still contains %q", tc.in, got, d)
			}
		}
		if !strings.Contains(got, redacted) && got == tc.in {
			t.Errorf("sanitizeText(%q) made no change", tc.in)
		}
	}
}

func TestCompatibilityJSONHasNoForbiddenPatterns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home", "bob", "sys")
	writeBattery(t, root, "BAT1", map[string]string{
		"type":                           "Battery",
		"present":                        "1",
		"status":                         "Full",
		"capacity":                       "100",
		"energy_now":                     "1",
		"energy_full":                    "2",
		"power_now":                      "0",
		"voltage_now":                    "12000000",
		"manufacturer":                   "SMP",
		"model_name":                     "ThinkPad",
		"serial_number":                  "TP-SERIAL-PRIVATE",
		"charge_control_start_threshold": "50",
		"charge_control_end_threshold":   "80",
	})
	writeSupply(t, root, "AC", map[string]string{"type": "Mains", "online": "1"})
	writeModules(t, root, "thinkpad_acpi")
	opts := optsFor(t, root, fakeRunner{
		bins: map[string]string{"tlp": "/home/bob/.local/bin/tlp", "upower": "/usr/bin/upower"},
	})
	opts.Hostname = "bob"

	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteCompatibilityReport(&buf, CompatibilityFrom(report, "0.7.0-alpha"), true); err != nil {
		t.Fatal(err)
	}
	assertNoLeaks(t, buf.Bytes(), []string{
		"TP-SERIAL-PRIVATE",
		"/home/",
		"bob",
	})
	assertNoSecretShapes(t, buf.Bytes())
}

func assertNoLeaks(t *testing.T, raw []byte, forbidden []string) {
	t.Helper()
	s := string(raw)
	for _, f := range forbidden {
		if f == "" {
			continue
		}
		if strings.Contains(s, f) {
			t.Errorf("compatibility JSON contains forbidden %q\n%s", f, s)
		}
	}
}

func assertNoSecretShapes(t *testing.T, raw []byte) {
	t.Helper()
	s := string(raw)
	patterns := []*regexp.Regexp{
		ipv4Re,
		macRe,
		uuidRe,
		urlRe,
		regexp.MustCompile(`/home/`),
		regexp.MustCompile(`(?i)webhook_url`),
		regexp.MustCompile(`(?i)chat_id`),
		regexp.MustCompile(`(?i)"serial"\s*:`),
		regexp.MustCompile(`(?i)"hostname"\s*:`),
		regexp.MustCompile(`(?i)"path"\s*:`),
		regexp.MustCompile(`(?i)"start_path"\s*:`),
		regexp.MustCompile(`(?i)"end_path"\s*:`),
		regexp.MustCompile(`(?i)"timestamp"\s*:`),
	}
	for _, re := range patterns {
		if re.FindStringIndex(s) != nil {
			t.Errorf("compatibility JSON matched forbidden pattern %s\n%s", re, s)
		}
	}
}

func TestCompatibilityEmptyHardware(t *testing.T) {
	report, err := Run(context.Background(), Options{
		SysfsRoot:     filepath.Join(t.TempDir(), "missing"),
		ProcModules:   filepath.Join(t.TempDir(), "modules"),
		PlatformRoot:  t.TempDir(),
		OSRelease:     filepath.Join(t.TempDir(), "os-release"),
		KernelRelease: filepath.Join(t.TempDir(), "osrelease"),
		DockerSocket:  filepath.Join(t.TempDir(), "docker.sock"),
		Hostname:      "testhost",
		Now:           func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) },
		Runner:        fakeRunner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := CompatibilityFrom(report, "dev")
	if got.ChargeThresholdMethod != MethodNone {
		t.Fatalf("method %q", got.ChargeThresholdMethod)
	}
	if got.Batteries == nil || got.Adapters == nil || got.AvailableFields == nil || got.KernelModules == nil {
		t.Fatal("slices must be empty, not nil")
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	assertNoLeaks(t, raw, []string{"testhost"})
}
