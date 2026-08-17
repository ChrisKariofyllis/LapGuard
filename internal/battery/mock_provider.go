package battery

import (
	"context"
	"math"
	"sync"
	"time"
)

// MockProvider synthesizes a realistic laptop battery so LapGuard can be
// developed on machines without a pack (the HP ProDesk) and so the dashboard
// has live-looking numbers without touching sysfs.
type MockProvider struct {
	mu      sync.Mutex
	started time.Time
	now     func() time.Time
	static  *Battery
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		started: time.Now(),
		now:     time.Now,
	}
}

// NewStaticMockProvider returns a frozen reading, intended for tests.
func NewStaticMockProvider(b Battery) *MockProvider {
	cp := b
	return &MockProvider{
		started: time.Now(),
		now:     time.Now,
		static:  &cp,
	}
}

func (p *MockProvider) Kind() string { return "mock" }

func (p *MockProvider) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now().UTC()
	snap := Snapshot{
		Timestamp:     now,
		Provider:      p.Kind(),
		MissingFields: []string{},
	}

	if p.static != nil {
		snap.Battery = *p.static
		snap.enrich()
		return snap, nil
	}

	elapsed := now.Sub(p.started).Seconds()
	// Slow oscillation so the dashboard feels alive without racing.
	capacity := 68 + 6*math.Sin(elapsed/90)
	if capacity < 1 {
		capacity = 1
	}
	if capacity > 100 {
		capacity = 100
	}
	cap := int(math.Round(capacity))

	voltage := 11.42 + 0.08*math.Sin(elapsed/40)
	current := -1.18 + 0.12*math.Sin(elapsed/25)
	powerNow := math.Abs(voltage * current)
	energyFull := 42.1
	energyDesign := 48.4
	cycles := 214
	status := "Discharging"
	if current > 0 {
		status = "Charging"
	}

	snap.Battery = Battery{
		Name:               "BAT0",
		Present:            true,
		Status:             status,
		CapacityPercent:    &cap,
		VoltageNowV:        ptr(round3(voltage)),
		CurrentNowA:        ptr(round3(current)),
		PowerNowW:          ptr(round3(powerNow)),
		EnergyFullWh:       ptr(energyFull),
		EnergyFullDesignWh: ptr(energyDesign),
		CycleCount:         &cycles,
	}
	snap.enrich()
	return snap, nil
}

func (p *MockProvider) Probe(ctx context.Context) (Probe, error) {
	if err := ctx.Err(); err != nil {
		return Probe{}, err
	}
	return Probe{
		Kind:            p.Kind(),
		BatteryPresent:  true,
		BatteryName:     "BAT0",
		AvailableFields: append([]string{}, TrackedFields...),
	}, nil
}

func ptr[T any](v T) *T { return &v }

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}
