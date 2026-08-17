package discovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMockLaptops(t *testing.T) {
	cases := []struct {
		name            string
		setup           func(t *testing.T, root string) Options
		wantMethod      string
		wantModules     []string
		wantTLP         bool
		wantTLPCanSet   bool
		wantCycle       bool
		wantPowerNow    bool
		wantCurrentVolt bool
		wantNaming      string
		whyContains     string
	}{
		{
			name: "thinkpad",
			setup: func(t *testing.T, root string) Options {
				writeBattery(t, root, "BAT0", map[string]string{
					"type":                           "Battery",
					"present":                        "1",
					"status":                         "Charging",
					"capacity":                       "62",
					"energy_now":                     "30000000",
					"energy_full":                    "48000000",
					"energy_full_design":             "51000000",
					"power_now":                      "12000000",
					"voltage_now":                    "12200000",
					"current_now":                    "980000",
					"cycle_count":                    "241",
					"manufacturer":                   "SMP",
					"model_name":                     "5B10W13900",
					"serial_number":                  "TP-BAT-1",
					"technology":                     "Li-ion",
					"charge_control_start_threshold": "75",
					"charge_control_end_threshold":   "80",
				})
				writeSupply(t, root, "AC", map[string]string{"type": "Mains", "online": "1"})
				writeModules(t, root, "thinkpad_acpi", "acpi_call")
				writePlatform(t, root, "thinkpad_acpi", map[string]string{
					"hotkey_enable": "1",
				})
				return optsFor(t, root, fakeRunner{
					bins:    map[string]string{"tlp": "/usr/bin/tlp", "tlp-stat": "/usr/bin/tlp-stat", "upower": "/usr/bin/upower"},
					version: "1.6.1",
					stat: `--- TLP 1.6.1 --------------------------------------------
+++ Charge Thresholds
NATACPI    = active (start = 75, stop = 80)
`,
				})
			},
			wantMethod:      MethodSysfs,
			wantModules:     []string{"thinkpad_acpi", "acpi_call"},
			wantTLP:         true,
			wantTLPCanSet:   true,
			wantCycle:       true,
			wantPowerNow:    true,
			wantCurrentVolt: true,
			wantNaming:      "energy",
		},
		{
			name: "dell-xps",
			setup: func(t *testing.T, root string) Options {
				writeBattery(t, root, "BAT0", map[string]string{
					"type":                           "Battery",
					"present":                        "1",
					"status":                         "Discharging",
					"capacity":                       "54",
					"charge_now":                     "2200000",
					"charge_full":                    "3400000",
					"charge_full_design":             "4000000",
					"voltage_now":                    "11500000",
					"current_now":                    "-1100000",
					"cycle_count":                    "119",
					"manufacturer":                   "SMP",
					"model_name":                     "DELL G8VCF",
					"serial_number":                  "XPS-BAT-1",
					"technology":                     "Li-ion",
					"charge_control_start_threshold": "50",
					"charge_control_end_threshold":   "80",
				})
				writeSupply(t, root, "ACAD", map[string]string{"type": "Mains", "online": "0"})
				writeModules(t, root, "dell_wmi", "dell_smbios")
				return optsFor(t, root, fakeRunner{
					bins:    map[string]string{"tlp": "/usr/sbin/tlp", "upower": "/usr/bin/upower"},
					version: "1.6.1",
					stat: `--- TLP 1.6.1 --------------------------------------------
+++ Charge Thresholds
NATACPI    = active (start = 50, stop = 80)
`,
				})
			},
			wantMethod:      MethodSysfs,
			wantModules:     []string{"dell_wmi", "dell_smbios"},
			wantTLP:         true,
			wantTLPCanSet:   true,
			wantCycle:       true,
			wantPowerNow:    false,
			wantCurrentVolt: true,
			wantNaming:      "charge",
		},
		{
			name: "fujitsu",
			setup: func(t *testing.T, root string) Options {
				writeBattery(t, root, "BAT1", map[string]string{
					"type":               "Battery",
					"present":            "1",
					"status":             "Discharging",
					"capacity":           "76",
					"energy_now":         "32000000",
					"energy_full":        "42100000",
					"energy_full_design": "50000000",
					"voltage_now":        "11412000",
					"current_now":        "-1234000",
					"cycle_count":        "312",
					"manufacturer":       "Fujitsu",
					"model_name":         "CP788370-01",
					"serial_number":      "FUJ-BAT-1",
					"technology":         "Li-ion",
					"temp":               "298",
				})
				writeSupply(t, root, "ADP1", map[string]string{"type": "Mains", "online": "1"})
				writeModules(t, root, "fujitsu_laptop")
				return optsFor(t, root, fakeRunner{
					bins:    map[string]string{"tlp": "/usr/sbin/tlp", "acpi": "/usr/bin/acpi"},
					version: "1.8.0",
					stat: `--- TLP 1.8.0 --------------------------------------------
+++ Charge Thresholds
NATACPI    = inactive (unsupported hardware)
tpacpi-bat = inactive (kernel module 'acpi_call' not installed)
tp-smapi   = inactive (unsupported hardware)
`,
				})
			},
			wantMethod:      MethodNone,
			wantModules:     []string{"fujitsu_laptop"},
			wantTLP:         true,
			wantTLPCanSet:   false,
			wantCycle:       true,
			wantPowerNow:    false,
			wantCurrentVolt: true,
			wantNaming:      "energy",
			whyContains:     "fujitsu_laptop",
		},
		{
			name: "asus",
			setup: func(t *testing.T, root string) Options {
				writeBattery(t, root, "BAT0", map[string]string{
					"type":                         "Battery",
					"present":                      "1",
					"status":                       "Full",
					"capacity":                     "98",
					"charge_now":                   "4800000",
					"charge_full":                  "4900000",
					"charge_full_design":           "5000000",
					"voltage_now":                  "12600000",
					"current_now":                  "0",
					"manufacturer":                 "ASUSTeK",
					"model_name":                   "C31N1842",
					"serial_number":                "ASUS-BAT-1",
					"technology":                   "Li-ion",
					"charge_control_end_threshold": "80",
				})
				writeSupply(t, root, "AC0", map[string]string{"type": "Mains", "online": "1"})
				writeModules(t, root, "asus_wmi", "asus_nb_wmi")
				return optsFor(t, root, fakeRunner{bins: map[string]string{"upower": "/usr/bin/upower"}})
			},
			wantMethod:      MethodSysfs,
			wantModules:     []string{"asus_wmi", "asus_nb_wmi"},
			wantTLP:         false,
			wantTLPCanSet:   false,
			wantCycle:       false,
			wantPowerNow:    false,
			wantCurrentVolt: true,
			wantNaming:      "charge",
		},
		{
			name: "generic",
			setup: func(t *testing.T, root string) Options {
				writeBattery(t, root, "BAT0", map[string]string{
					"type":               "Battery",
					"present":            "1",
					"status":             "Discharging",
					"capacity":           "41",
					"energy_now":         "18000000",
					"energy_full":        "42000000",
					"energy_full_design": "45000000",
					"voltage_now":        "11000000",
					"current_now":        "-900000",
					"manufacturer":       "Generic",
					"model_name":         "ACPI-BAT",
					"serial_number":      "GEN-001",
					"technology":         "Li-ion",
				})
				writeSupply(t, root, "AC", map[string]string{"type": "Mains", "online": "0"})
				writeModules(t, root)
				return optsFor(t, root, fakeRunner{})
			},
			wantMethod:      MethodNone,
			wantModules:     nil,
			wantTLP:         false,
			wantTLPCanSet:   false,
			wantCycle:       false,
			wantPowerNow:    false,
			wantCurrentVolt: true,
			wantNaming:      "energy",
			whyContains:     "No charge_control",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			opts := tc.setup(t, root)
			report, err := Run(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if report.Features.ChargeThresholds != tc.wantMethod {
				t.Fatalf("method %q, want %q (why=%q)", report.Features.ChargeThresholds, tc.wantMethod, report.Thresholds.WhyNot)
			}
			if report.Thresholds.Method != tc.wantMethod {
				t.Fatalf("plan method %q", report.Thresholds.Method)
			}
			for _, m := range tc.wantModules {
				if !moduleLoaded(report.KernelModules, m) {
					t.Fatalf("expected module %s in %v", m, report.KernelModules)
				}
			}
			if report.AvailableTools.TLP != tc.wantTLP {
				t.Fatalf("tlp=%v, want %v", report.AvailableTools.TLP, tc.wantTLP)
			}
			if report.AvailableTools.TLPCanSet != tc.wantTLPCanSet {
				t.Fatalf("tlp_can_set=%v, want %v", report.AvailableTools.TLPCanSet, tc.wantTLPCanSet)
			}
			if report.Features.CycleCount != tc.wantCycle {
				t.Fatalf("cycle_count=%v", report.Features.CycleCount)
			}
			if report.Features.PowerNow != tc.wantPowerNow {
				t.Fatalf("power_now=%v", report.Features.PowerNow)
			}
			if report.Features.RawPowerNowSupported != tc.wantPowerNow {
				t.Fatalf("raw_power_now_supported=%v", report.Features.RawPowerNowSupported)
			}
			if report.Features.DerivedPowerSupported != tc.wantCurrentVolt {
				t.Fatalf("derived_power_supported=%v", report.Features.DerivedPowerSupported)
			}
			if report.Features.CurrentVoltage != tc.wantCurrentVolt {
				t.Fatalf("current_voltage=%v", report.Features.CurrentVoltage)
			}
			if report.NamingConvention != tc.wantNaming {
				t.Fatalf("naming %q, want %q", report.NamingConvention, tc.wantNaming)
			}
			if !report.Battery.Present {
				t.Fatal("expected a present battery")
			}
			if tc.wantMethod == MethodNone && tc.whyContains != "" {
				if !strings.Contains(report.Thresholds.WhyNot, tc.whyContains) {
					t.Fatalf("why_not %q, want substring %q", report.Thresholds.WhyNot, tc.whyContains)
				}
			}
			if tc.name == "fujitsu" {
				if report.Features.Temperature != true {
					t.Fatal("fujitsu fixture should expose temp")
				}
				if report.PowerCalculation != "current_voltage" {
					t.Fatalf("fujitsu power method %q, want current_voltage", report.PowerCalculation)
				}
				foundNote := false
				for _, n := range report.Notes {
					if strings.Contains(n, "Unable to register battery charge control") {
						foundNote = true
					}
				}
				if !foundNote {
					t.Fatalf("expected fujitsu charge-control note, got %v", report.Notes)
				}
			}
			if tc.name == "thinkpad" && report.Thresholds.DetectionMethod != "sysfs_native" {
				t.Fatalf("thinkpad detection_method %q", report.Thresholds.DetectionMethod)
			}
			if tc.name == "asus" && report.Thresholds.EndPath == "" {
				t.Fatal("asus should detect charge_control_end_threshold")
			}
			feats := featureStatuses(report)
			if len(feats) == 0 {
				t.Fatal("expected UI feature statuses")
			}
			for _, f := range feats {
				if f.Key == "charge_thresholds" {
					if f.Enabled != (tc.wantMethod != MethodNone) {
						t.Fatalf("feature enabled=%v method=%s", f.Enabled, tc.wantMethod)
					}
					if tc.wantMethod == MethodNone && f.WhyNot == "" {
						t.Fatal("unsupported charge thresholds must include why_not")
					}
				}
				if f.Key == "raw_power_now" && f.Enabled != tc.wantPowerNow {
					t.Fatalf("raw_power_now enabled=%v, want %v", f.Enabled, tc.wantPowerNow)
				}
				if f.Key == "derived_power" && f.Enabled != tc.wantCurrentVolt {
					t.Fatalf("derived_power enabled=%v, want %v", f.Enabled, tc.wantCurrentVolt)
				}
			}
		})
	}
}

func TestTLPOnlyThinkPadUsesTLPMethod(t *testing.T) {
	root := t.TempDir()
	writeBattery(t, root, "BAT0", map[string]string{
		"type":               "Battery",
		"present":            "1",
		"status":             "Charging",
		"capacity":           "70",
		"energy_now":         "1",
		"energy_full":        "2",
		"energy_full_design": "3",
	})
	writeModules(t, root, "thinkpad_acpi")
	opts := optsFor(t, root, fakeRunner{
		bins:    map[string]string{"tlp": "/usr/bin/tlp"},
		version: "1.6.1",
		stat:    "+++ Charge Thresholds\nNATACPI    = active (start = 40, stop = 80)\n",
	})
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Features.ChargeThresholds != MethodTLP {
		t.Fatalf("method %q, want tlp (%s)", report.Features.ChargeThresholds, report.Thresholds.WhyNot)
	}
}

func TestThinkPadPlatformChargeControl(t *testing.T) {
	root := t.TempDir()
	writeBattery(t, root, "BAT0", map[string]string{
		"type":     "Battery",
		"present":  "1",
		"status":   "Full",
		"capacity": "100",
	})
	writeModules(t, root, "thinkpad_acpi")
	writePlatform(t, root, "thinkpad_acpi", map[string]string{
		"charge_control_start_threshold": "40",
		"charge_control_end_threshold":   "80",
	})
	opts := optsFor(t, root, fakeRunner{})
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Features.ChargeThresholds != MethodSysfs {
		t.Fatalf("method %q, want sysfs", report.Features.ChargeThresholds)
	}
}

func TestMissingSysfsIsNotAnError(t *testing.T) {
	opts := Options{
		SysfsRoot:     filepath.Join(t.TempDir(), "missing"),
		ProcModules:   filepath.Join(t.TempDir(), "modules"),
		PlatformRoot:  t.TempDir(),
		OSRelease:     filepath.Join(t.TempDir(), "os-release"),
		KernelRelease: filepath.Join(t.TempDir(), "osrelease"),
		DockerSocket:  filepath.Join(t.TempDir(), "docker.sock"),
		Hostname:      "testhost",
		Now:           func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) },
		Runner:        fakeRunner{},
	}
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Battery.Present {
		t.Fatal("expected no battery")
	}
	if report.Features.ChargeThresholds != MethodNone {
		t.Fatalf("method %q", report.Features.ChargeThresholds)
	}
}

func writeBattery(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	writeSupply(t, root, name, files)
}

func writeSupply(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, "sysfs", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeModules(t *testing.T, root string, names ...string) {
	t.Helper()
	dir := filepath.Join(root, "proc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n)
		b.WriteString(" 16384 0 - Live 0x0\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "modules"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePlatform(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, "platform", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func optsFor(t *testing.T, root string, runner Runner) Options {
	t.Helper()
	osRelease := filepath.Join(root, "os-release")
	if err := os.WriteFile(osRelease, []byte("PRETTY_NAME=\"Ubuntu Test\"\nNAME=\"Ubuntu\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kernel := filepath.Join(root, "osrelease")
	if err := os.WriteFile(kernel, []byte("6.17.0-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Options{
		SysfsRoot:     filepath.Join(root, "sysfs"),
		ProcModules:   filepath.Join(root, "proc", "modules"),
		PlatformRoot:  filepath.Join(root, "platform"),
		OSRelease:     osRelease,
		KernelRelease: kernel,
		DockerSocket:  filepath.Join(root, "docker.sock"),
		Hostname:      "lapguard-test",
		Now:           func() time.Time { return time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC) },
		Runner:        runner,
	}
}

type fakeRunner struct {
	bins    map[string]string
	version string
	stat    string
}

func (f fakeRunner) LookPath(file string) (string, error) {
	if p, ok := f.bins[file]; ok {
		return p, nil
	}
	return "", os.ErrNotExist
}

func (f fakeRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	base := filepath.Base(name)
	if base == "tlp" && (len(args) == 0 || args[0] == "--version" || args[0] == "-v") {
		ver := f.version
		if ver == "" {
			ver = "1.0.0"
		}
		return []byte("TLP " + ver + "\n"), nil
	}
	if base == "tlp-stat" {
		return []byte(f.stat), nil
	}
	return nil, os.ErrNotExist
}
