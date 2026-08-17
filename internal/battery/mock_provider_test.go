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
	probe, err := p.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !probe.BatteryPresent || len(probe.AvailableFields) != len(TrackedFields) {
		t.Fatalf("probe %+v", probe)
	}
}
