package battery

import (
	"context"
	"testing"
)

func TestMockProviderSnapshot(t *testing.T) {
	cap := 80
	p := NewStaticMockProvider(Battery{
		Name:               "BAT0",
		Present:            true,
		Status:             "Charging",
		CapacityPercent:    &cap,
		VoltageNowV:        ptr(12.1),
		CurrentNowA:        ptr(0.5),
		EnergyFullWh:       ptr(40.0),
		EnergyFullDesignWh: ptr(50.0),
	})
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind() != "mock" {
		t.Fatalf("kind %q", p.Kind())
	}
	if !snap.Battery.Present || snap.Battery.Status != "Charging" {
		t.Fatalf("battery %+v", snap.Battery)
	}
	if snap.Battery.PowerW == nil || *snap.Battery.PowerW != 6.05 {
		t.Fatalf("power_w %+v", snap.Battery.PowerW)
	}
	if snap.Battery.EstimatedRuntimeAvailable {
		t.Fatal("charging mock must not estimate runtime")
	}
	if snap.Battery.EstimatedRuntimeSeconds != nil || snap.Battery.EstimatedRuntimeHours != nil {
		t.Fatalf("charging runtime must be null: %+v", snap.Battery)
	}
	if snap.Battery.HealthPercent == nil || *snap.Battery.HealthPercent != 80 {
		t.Fatalf("health %+v", snap.Battery.HealthPercent)
	}
}

func TestLiveMockProviderHasAllFields(t *testing.T) {
	p := NewMockProvider()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b := snap.Battery
	if !b.Present || b.Name != "BAT0" {
		t.Fatalf("battery %+v", b)
	}
	if b.CapacityPercent == nil || b.VoltageNowV == nil || b.CurrentNowA == nil {
		t.Fatalf("missing live fields %+v", b)
	}
	if b.PowerW == nil || b.HealthPercent == nil {
		t.Fatalf("missing derived fields %+v", b)
	}
	if !b.EstimatedRuntimeAvailable || b.EstimatedRuntimeSeconds == nil {
		t.Fatalf("live mock should discharge with a runtime estimate: %+v", b)
	}
	probe, err := p.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !probe.BatteryPresent || len(probe.AvailableFields) != len(TrackedFields) {
		t.Fatalf("probe %+v", probe)
	}
}

func TestStaticMockDischargingRuntime(t *testing.T) {
	cap := 50
	p := NewStaticMockProvider(Battery{
		Name:            "BAT0",
		Present:         true,
		Status:          "Discharging",
		CapacityPercent: &cap,
		EnergyNowWh:     ptr(50.0),
		PowerNowW:       ptr(10.0),
		CurrentNowA:     ptr(-1.0),
	})
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Battery.EstimatedRuntimeAvailable {
		t.Fatalf("discharging mock should estimate runtime: %+v", snap.Battery)
	}
	if snap.Battery.EstimatedRuntimeSeconds == nil || *snap.Battery.EstimatedRuntimeSeconds != 18000 {
		t.Fatalf("seconds %+v", snap.Battery.EstimatedRuntimeSeconds)
	}
}
