package battery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPowerWattsFromPowerNow(t *testing.T) {
	power := 14.082
	current := -1.234
	got := PowerWatts(&power, nil, &current, "Discharging")
	if got == nil {
		t.Fatal("expected power")
	}
	if *got != -14.082 {
		t.Fatalf("got %v, want -14.082", *got)
	}
}

func TestPowerWattsFromVoltageAndCurrent(t *testing.T) {
	v := 11.412
	i := -1.234
	got := PowerWatts(nil, &v, &i, "")
	if got == nil {
		t.Fatal("expected power")
	}
	want := v * i
	if math.Abs(*got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", *got, want)
	}
}

func TestPowerWattsMissingInputs(t *testing.T) {
	if PowerWatts(nil, nil, nil, "Discharging") != nil {
		t.Fatal("expected nil when no measurements exist")
	}
	v := 11.0
	if PowerWatts(nil, &v, nil, "") != nil {
		t.Fatal("expected nil when current is missing")
	}
}

func TestHealthPercent(t *testing.T) {
	full := 42.1
	design := 50.0
	got := HealthPercent(&full, &design)
	if got == nil {
		t.Fatal("expected health")
	}
	if math.Abs(*got-84.2) > 1e-9 {
		t.Fatalf("got %v, want 84.2", *got)
	}
}

func TestHealthPercentZeroDesign(t *testing.T) {
	full := 42.1
	design := 0.0
	if HealthPercent(&full, &design) != nil {
		t.Fatal("expected nil health when design energy is 0")
	}
	if HealthPercent(&full, nil) != nil {
		t.Fatal("expected nil health when design energy is missing")
	}
}

func TestSysfsProviderFullFixture(t *testing.T) {
	p := NewSysfsProvider(sysfsFixture(t), "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Battery.Present {
		t.Fatal("expected battery present")
	}
	if snap.Battery.Name != "BAT0" {
		t.Fatalf("name %q", snap.Battery.Name)
	}
	if snap.Battery.Status != "Discharging" {
		t.Fatalf("status %q", snap.Battery.Status)
	}
	assertInt(t, "capacity", snap.Battery.CapacityPercent, 76)
	assertFloat(t, "voltage", snap.Battery.VoltageNowV, 11.412)
	assertFloat(t, "current", snap.Battery.CurrentNowA, -1.234)
	assertFloat(t, "power_now", snap.Battery.PowerNowW, 14.082)
	assertFloat(t, "energy_full", snap.Battery.EnergyFullWh, 42.1)
	assertFloat(t, "energy_full_design", snap.Battery.EnergyFullDesignWh, 50.0)
	assertInt(t, "cycle_count", snap.Battery.CycleCount, 312)
	assertFloat(t, "power_w", snap.Battery.PowerW, -14.082)
	assertFloat(t, "health", snap.Battery.HealthPercent, 84.2)
	if snap.MissingFields == nil {
		t.Fatal("missing_fields should be an empty slice, not nil")
	}
	if len(snap.MissingFields) != 0 {
		t.Fatalf("unexpected missing fields: %v", snap.MissingFields)
	}
}

func TestSysfsProviderAutoDetectsBAT0(t *testing.T) {
	p := NewSysfsProvider(sysfsFixture(t), "")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Battery.Name != "BAT0" {
		t.Fatalf("auto-detect name %q", snap.Battery.Name)
	}
}

func TestSysfsProviderMissingOptionalFiles(t *testing.T) {
	root := cloneFixture(t, "power_now", "cycle_count")
	p := NewSysfsProvider(root, "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Battery.PowerNowW != nil {
		t.Fatal("power_now should be missing")
	}
	if snap.Battery.CycleCount != nil {
		t.Fatal("cycle_count should be missing")
	}
	if snap.Battery.PowerW == nil {
		t.Fatal("power_w should be derived from voltage * current")
	}
	want := 11.412 * -1.234
	assertFloat(t, "derived power", snap.Battery.PowerW, want)
	if !contains(snap.MissingFields, FieldPowerNow) || !contains(snap.MissingFields, FieldCycleCount) {
		t.Fatalf("missing_fields = %v", snap.MissingFields)
	}
}

func TestSysfsProviderMissingBattery(t *testing.T) {
	p := NewSysfsProvider(t.TempDir(), "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Battery.Present {
		t.Fatal("expected present=false when BAT0 is absent")
	}
	if len(snap.Warnings) == 0 {
		t.Fatal("expected a warning about the missing directory")
	}
}

func TestSysfsProviderMissingRoot(t *testing.T) {
	p := NewSysfsProvider(filepath.Join(t.TempDir(), "does-not-exist"), "")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Battery.Present {
		t.Fatal("expected present=false when sysfs root is missing")
	}
}

func TestSysfsProviderNotPresentFile(t *testing.T) {
	root := cloneFixture(t)
	if err := os.WriteFile(filepath.Join(root, "BAT0", "present"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewSysfsProvider(root, "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Battery.Present {
		t.Fatal("expected present=false")
	}
}

func TestSysfsProviderParseError(t *testing.T) {
	root := cloneFixture(t)
	if err := os.WriteFile(filepath.Join(root, "BAT0", "capacity"), []byte("not-a-number\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewSysfsProvider(root, "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Battery.CapacityPercent != nil {
		t.Fatal("capacity should be omitted on parse error")
	}
	if len(snap.Warnings) == 0 {
		t.Fatal("expected parse warning")
	}
}

func TestSysfsProviderProbe(t *testing.T) {
	p := NewSysfsProvider(sysfsFixture(t), "BAT0")
	probe, err := p.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !probe.BatteryPresent || probe.BatteryName != "BAT0" {
		t.Fatalf("probe = %+v", probe)
	}
	if len(probe.AvailableFields) != len(TrackedFields) {
		t.Fatalf("available fields %v", probe.AvailableFields)
	}
}

func TestOpenAutoFallsBackToMock(t *testing.T) {
	p, err := Open(context.Background(), OpenOptions{
		Kind:      "auto",
		SysfsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind() != "mock" {
		t.Fatalf("kind %q, want mock", p.Kind())
	}
}

func TestOpenAutoUsesSysfsWhenPresent(t *testing.T) {
	p, err := Open(context.Background(), OpenOptions{
		Kind:      "auto",
		SysfsRoot: sysfsFixture(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind() != "sysfs" {
		t.Fatalf("kind %q, want sysfs", p.Kind())
	}
}

func sysfsFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(moduleRoot(t), "testdata", "sysfs")
	if _, err := os.Stat(filepath.Join(root, "BAT0", "status")); err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
	return root
}

func cloneFixture(t *testing.T, skip ...string) string {
	t.Helper()
	src := filepath.Join(sysfsFixture(t), "BAT0")
	root := t.TempDir()
	dst := filepath.Join(root, "BAT0")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	skipSet := make(map[string]struct{}, len(skip))
	for _, name := range skip {
		skipSet[name] = struct{}{}
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if _, ok := skipSet[entry.Name()]; ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, entry.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func assertInt(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: nil, want %d", name, want)
	}
	if *got != want {
		t.Fatalf("%s: got %d, want %d", name, *got, want)
	}
}

func assertFloat(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: nil, want %v", name, want)
	}
	if math.Abs(*got-want) > 1e-9 {
		t.Fatalf("%s: got %v, want %v", name, *got, want)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
