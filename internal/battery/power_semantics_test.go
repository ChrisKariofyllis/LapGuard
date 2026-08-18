package battery

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPowerSemanticsTable(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		voltageV    *float64
		currentA    *float64
		powerNowW   *float64
		wantPowerW  *float64
		wantMag     *float64
		wantDir     string
		wantLabel   string
		wantRuntime bool
	}{
		{
			name:        "charging derived current times voltage",
			status:      "Charging",
			voltageV:    ptr(11.4),
			currentA:    ptr(3.158),
			wantPowerW:  ptr(11.4 * 3.158),
			wantMag:     ptr(11.4 * 3.158),
			wantDir:     DirectionCharge,
			wantLabel:   LabelChargingPower,
			wantRuntime: false,
		},
		{
			name:        "discharging derived negative current",
			status:      "Discharging",
			voltageV:    ptr(11.412),
			currentA:    ptr(-1.234),
			wantPowerW:  ptr(11.412 * -1.234),
			wantMag:     ptr(math.Abs(11.412 * -1.234)),
			wantDir:     DirectionDischarge,
			wantLabel:   LabelDischargePower,
			wantRuntime: true,
		},
		{
			name:        "not charging zero current",
			status:      "Not charging",
			voltageV:    ptr(11.412),
			currentA:    ptr(0.0),
			wantPowerW:  ptr(0.0),
			wantMag:     ptr(0.0),
			wantDir:     DirectionIdle,
			wantLabel:   LabelIdle,
			wantRuntime: false,
		},
		{
			name:        "full zero current",
			status:      "Full",
			voltageV:    ptr(12.6),
			currentA:    ptr(0.0),
			wantPowerW:  ptr(0.0),
			wantMag:     ptr(0.0),
			wantDir:     DirectionIdle,
			wantLabel:   LabelIdle,
			wantRuntime: false,
		},
		{
			name:        "missing current and voltage",
			status:      "Discharging",
			wantPowerW:  nil,
			wantMag:     nil,
			wantDir:     DirectionDischarge,
			wantLabel:   LabelDischargePower,
			wantRuntime: false,
		},
		{
			name:        "missing sensors unknown status",
			status:      "",
			wantPowerW:  nil,
			wantMag:     nil,
			wantDir:     DirectionUnknown,
			wantLabel:   LabelPowerUnavailable,
			wantRuntime: false,
		},
		{
			name:        "negative discharging current with power_now",
			status:      "Discharging",
			currentA:    ptr(-1.5),
			powerNowW:   ptr(18.0),
			wantPowerW:  ptr(-18.0),
			wantMag:     ptr(18.0),
			wantDir:     DirectionDischarge,
			wantLabel:   LabelDischargePower,
			wantRuntime: true,
		},
		{
			name:        "charging indicated by positive current without status",
			status:      "",
			voltageV:    ptr(11.5),
			currentA:    ptr(2.0),
			wantPowerW:  ptr(23.0),
			wantMag:     ptr(23.0),
			wantDir:     DirectionCharge,
			wantLabel:   LabelChargingPower,
			wantRuntime: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			powerW := PowerWatts(tc.powerNowW, tc.voltageV, tc.currentA, tc.status)
			assertOptionalFloat(t, "power_w", powerW, tc.wantPowerW)
			mag := PowerMagnitude(powerW)
			assertOptionalFloat(t, "battery_power_w", mag, tc.wantMag)
			dir, label := ClassifyPowerDirection(tc.status, tc.currentA, powerW)
			if dir != tc.wantDir {
				t.Fatalf("power_direction %q, want %q", dir, tc.wantDir)
			}
			if label != tc.wantLabel {
				t.Fatalf("power_label %q, want %q", label, tc.wantLabel)
			}
			energy := 32.0
			est := EstimateRuntime(true, tc.status, &energy, powerW)
			if est.Available != tc.wantRuntime {
				t.Fatalf("runtime available=%v, want %v (%s)", est.Available, tc.wantRuntime, est.Reason)
			}
			if tc.status == "Charging" || tc.status == "Full" || tc.status == "Not charging" {
				if est.Available {
					t.Fatal("runtime must stay unavailable on AC / idle")
				}
			}
		})
	}
}

func TestPowerWBackwardCompatibleJSON(t *testing.T) {
	b := Battery{}
	b.PowerW = PowerWatts(nil, ptr(11.4), ptr(3.158), "Charging")
	b.BatteryPowerW = PowerMagnitude(b.PowerW)
	b.PowerDirection, b.PowerLabel = ClassifyPowerDirection("Charging", ptr(3.158), b.PowerW)
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"power_w":`) {
		t.Fatalf("power_w must remain in JSON: %s", s)
	}
	if !strings.Contains(s, `"battery_power_w":`) {
		t.Fatalf("battery_power_w missing: %s", s)
	}
	if !strings.Contains(s, `"power_direction":"charge"`) {
		t.Fatalf("power_direction missing: %s", s)
	}
	if !strings.Contains(s, `"power_label":"Battery charging power"`) {
		t.Fatalf("power_label missing: %s", s)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	pw, ok := decoded["power_w"].(float64)
	if !ok || math.Abs(pw-11.4*3.158) > 1e-9 {
		t.Fatalf("power_w decoded %v", decoded["power_w"])
	}
}

func TestSysfsChargingAeroSemantics(t *testing.T) {
	// Gigabyte Aero 16 observation: Charging at 5%, current×voltage ≈ 36 W
	// is pack charge power, not system consumption.
	root := writeSysfsBattery(t, map[string]string{
		"type":        "Battery",
		"present":     "1",
		"status":      "Charging",
		"capacity":    "5",
		"voltage_now": "11400000",
		"current_now": "3158000",
		"charge_now":  "200000",
		"charge_full": "4000000",
	})
	snap := mustSnapshot(t, root)
	b := snap.Battery
	if b.Status != "Charging" {
		t.Fatalf("status %q", b.Status)
	}
	want := 11.4 * 3.158
	assertFloat(t, "power_w", b.PowerW, want)
	assertFloat(t, "battery_power_w", b.BatteryPowerW, want)
	if b.PowerDirection != DirectionCharge || b.PowerLabel != LabelChargingPower {
		t.Fatalf("direction %q label %q", b.PowerDirection, b.PowerLabel)
	}
	if b.EstimatedRuntimeAvailable {
		t.Fatal("must not estimate runtime while charging")
	}
}

func TestSysfsDischargingPowerDirection(t *testing.T) {
	p := NewSysfsProvider(sysfsFixture(t), "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b := snap.Battery
	assertFloat(t, "power_w", b.PowerW, -14.082)
	assertFloat(t, "battery_power_w", b.BatteryPowerW, 14.082)
	if b.PowerDirection != DirectionDischarge || b.PowerLabel != LabelDischargePower {
		t.Fatalf("direction %q label %q", b.PowerDirection, b.PowerLabel)
	}
}

func TestSysfsNotChargingIdle(t *testing.T) {
	root := writeSysfsBattery(t, map[string]string{
		"type":        "Battery",
		"present":     "1",
		"status":      "Not charging",
		"capacity":    "80",
		"voltage_now": "11412000",
		"current_now": "0",
	})
	b := mustSnapshot(t, root).Battery
	assertFloat(t, "power_w", b.PowerW, 0)
	assertFloat(t, "battery_power_w", b.BatteryPowerW, 0)
	if b.PowerDirection != DirectionIdle || b.PowerLabel != LabelIdle {
		t.Fatalf("direction %q label %q", b.PowerDirection, b.PowerLabel)
	}
	if b.EstimatedRuntimeAvailable {
		t.Fatal("must not estimate runtime while not charging")
	}
}

func TestSysfsFullIdle(t *testing.T) {
	root := writeSysfsBattery(t, map[string]string{
		"type":        "Battery",
		"present":     "1",
		"status":      "Full",
		"capacity":    "100",
		"voltage_now": "12600000",
		"current_now": "0",
	})
	b := mustSnapshot(t, root).Battery
	assertFloat(t, "power_w", b.PowerW, 0)
	assertFloat(t, "battery_power_w", b.BatteryPowerW, 0)
	if b.PowerDirection != DirectionIdle || b.PowerLabel != LabelIdle {
		t.Fatalf("direction %q label %q", b.PowerDirection, b.PowerLabel)
	}
	if b.EstimatedRuntimeAvailable {
		t.Fatal("must not estimate runtime while full")
	}
}

func TestSysfsMissingCurrentVoltageUnknownPower(t *testing.T) {
	root := writeSysfsBattery(t, map[string]string{
		"type":     "Battery",
		"present":  "1",
		"status":   "Unknown",
		"capacity": "40",
	})
	b := mustSnapshot(t, root).Battery
	if b.PowerW != nil || b.BatteryPowerW != nil {
		t.Fatalf("expected no watts, got power_w=%v battery_power_w=%v", b.PowerW, b.BatteryPowerW)
	}
	if b.PowerDirection != DirectionUnknown || b.PowerLabel != LabelPowerUnavailable {
		t.Fatalf("direction %q label %q", b.PowerDirection, b.PowerLabel)
	}
}

func writeSysfsBattery(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	bat := filepath.Join(root, "BAT0")
	if err := os.MkdirAll(bat, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(bat, name), []byte(body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func mustSnapshot(t *testing.T, root string) Snapshot {
	t.Helper()
	p := NewSysfsProvider(root, "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func assertOptionalFloat(t *testing.T, name string, got, want *float64) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s: got %v, want nil", name, *got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s: nil, want %v", name, *want)
	}
	if math.Abs(*got-*want) > 1e-9 {
		t.Fatalf("%s: got %v, want %v", name, *got, *want)
	}
}
