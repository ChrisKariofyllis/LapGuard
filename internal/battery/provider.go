package battery

import (
	"context"
	"time"
)

// Known sysfs attribute names. Providers report which of these were present.
const (
	FieldStatus           = "status"
	FieldCapacity         = "capacity"
	FieldVoltageNow       = "voltage_now"
	FieldCurrentNow       = "current_now"
	FieldPowerNow         = "power_now"
	FieldEnergyFull       = "energy_full"
	FieldEnergyFullDesign = "energy_full_design"
	FieldCycleCount       = "cycle_count"
)

var TrackedFields = []string{
	FieldStatus,
	FieldCapacity,
	FieldVoltageNow,
	FieldCurrentNow,
	FieldPowerNow,
	FieldEnergyFull,
	FieldEnergyFullDesign,
	FieldCycleCount,
}

// Provider reads a point-in-time battery snapshot. Implementations must not
// require root: Linux exposes these sysfs files to unprivileged users.
type Provider interface {
	Kind() string
	Snapshot(ctx context.Context) (Snapshot, error)
	Probe(ctx context.Context) (Probe, error)
}

// Snapshot is the telemetry payload: raw sysfs values converted to SI units,
// plus derived power (W) and health (%).
type Snapshot struct {
	Timestamp     time.Time `json:"timestamp"`
	Provider      string    `json:"provider"`
	Battery       Battery   `json:"battery"`
	MissingFields []string  `json:"missing_fields"`
	Warnings      []string  `json:"warnings,omitempty"`
}

// Battery holds converted measurements. Pointers are omitted in JSON when the
// corresponding sysfs file is missing or unreadable.
type Battery struct {
	Name               string   `json:"name"`
	Present            bool     `json:"present"`
	Status             string   `json:"status,omitempty"`
	CapacityPercent    *int     `json:"capacity_percent,omitempty"`
	VoltageNowV        *float64 `json:"voltage_now_v,omitempty"`
	CurrentNowA        *float64 `json:"current_now_a,omitempty"`
	PowerNowW          *float64 `json:"power_now_w,omitempty"`
	EnergyFullWh       *float64 `json:"energy_full_wh,omitempty"`
	EnergyFullDesignWh *float64 `json:"energy_full_design_wh,omitempty"`
	CycleCount         *int     `json:"cycle_count,omitempty"`
	PowerW             *float64 `json:"power_w,omitempty"`
	HealthPercent      *float64 `json:"health_percent,omitempty"`
}

// Probe describes what the provider can see without being a full telemetry read.
type Probe struct {
	Kind            string   `json:"kind"`
	BatteryPresent  bool     `json:"battery_present"`
	BatteryName     string   `json:"battery_name,omitempty"`
	AvailableFields []string `json:"available_fields"`
	SysfsRoot       string   `json:"sysfs_root,omitempty"`
}

func (s *Snapshot) enrich() {
	s.Battery.PowerW = PowerWatts(
		s.Battery.PowerNowW,
		s.Battery.VoltageNowV,
		s.Battery.CurrentNowA,
		s.Battery.Status,
	)
	s.Battery.HealthPercent = HealthPercent(s.Battery.EnergyFullWh, s.Battery.EnergyFullDesignWh)
}
