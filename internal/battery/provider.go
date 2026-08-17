package battery

import (
	"context"
	"time"
)

// Known sysfs attribute names. Providers report which of these were present.
const (
	FieldStatus               = "status"
	FieldCapacity             = "capacity"
	FieldCapacityLevel        = "capacity_level"
	FieldVoltageNow           = "voltage_now"
	FieldCurrentNow           = "current_now"
	FieldPowerNow             = "power_now"
	FieldEnergyNow            = "energy_now"
	FieldEnergyFull           = "energy_full"
	FieldEnergyFullDesign     = "energy_full_design"
	FieldChargeNow            = "charge_now"
	FieldChargeFull           = "charge_full"
	FieldChargeFullDesign     = "charge_full_design"
	FieldCycleCount           = "cycle_count"
	FieldTemp                 = "temp"
	FieldAlarm                = "alarm"
	FieldManufacturer         = "manufacturer"
	FieldModelName            = "model_name"
	FieldSerialNumber         = "serial_number"
	FieldTechnology           = "technology"
	FieldChargeControlStart   = "charge_control_start_threshold"
	FieldChargeControlEnd     = "charge_control_end_threshold"
	FieldChargeStartThreshold = "charge_start_threshold"
	FieldChargeStopThreshold  = "charge_stop_threshold"
)

// TrackedFields are measurement attributes. Missing ones are listed on the
// snapshot unless an equivalent (energy_* ↔ charge_*) is present.
var TrackedFields = []string{
	FieldStatus,
	FieldCapacity,
	FieldVoltageNow,
	FieldCurrentNow,
	FieldPowerNow,
	FieldEnergyNow,
	FieldEnergyFull,
	FieldEnergyFullDesign,
	FieldChargeNow,
	FieldChargeFull,
	FieldChargeFullDesign,
	FieldCycleCount,
	FieldTemp,
	FieldAlarm,
}

// equivalentFields lets a charge_* pack omit energy_* (and vice versa) from
// missing_fields, since LapGuard treats them as the same measurement.
var equivalentFields = map[string]string{
	FieldEnergyNow:        FieldChargeNow,
	FieldEnergyFull:       FieldChargeFull,
	FieldEnergyFullDesign: FieldChargeFullDesign,
	FieldChargeNow:        FieldEnergyNow,
	FieldChargeFull:       FieldEnergyFull,
	FieldChargeFullDesign: FieldEnergyFullDesign,
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
	Timestamp       time.Time `json:"timestamp"`
	Provider        string    `json:"provider"`
	Battery         Battery   `json:"battery"`
	AvailableFields []string  `json:"available_fields"`
	MissingFields   []string  `json:"missing_fields"`
	Warnings        []string  `json:"warnings,omitempty"`
}

// Battery holds converted measurements. Pointers are omitted in JSON when the
// corresponding sysfs file is missing or unreadable.
type Battery struct {
	Name                      string   `json:"name"`
	Present                   bool     `json:"present"`
	Status                    string   `json:"status,omitempty"`
	CapacityPercent           *int     `json:"capacity_percent,omitempty"`
	CapacityLevel             string   `json:"capacity_level,omitempty"`
	VoltageNowV               *float64 `json:"voltage_now_v,omitempty"`
	CurrentNowA               *float64 `json:"current_now_a,omitempty"`
	PowerNowW                 *float64 `json:"power_now_w,omitempty"`
	EnergyNowWh               *float64 `json:"energy_now_wh,omitempty"`
	EnergyFullWh              *float64 `json:"energy_full_wh,omitempty"`
	EnergyFullDesignWh        *float64 `json:"energy_full_design_wh,omitempty"`
	ChargeNowAh               *float64 `json:"charge_now_ah,omitempty"`
	ChargeFullAh              *float64 `json:"charge_full_ah,omitempty"`
	ChargeFullDesignAh        *float64 `json:"charge_full_design_ah,omitempty"`
	CycleCount                *int     `json:"cycle_count,omitempty"`
	TemperatureC              *float64 `json:"temperature_c,omitempty"`
	Alarm                     *int64   `json:"alarm,omitempty"`
	Manufacturer              string   `json:"manufacturer,omitempty"`
	ModelName                 string   `json:"model_name,omitempty"`
	SerialNumber              string   `json:"serial_number,omitempty"`
	Technology                string   `json:"technology,omitempty"`
	ChargeStartThreshold      *int     `json:"charge_start_threshold,omitempty"`
	ChargeEndThreshold        *int     `json:"charge_end_threshold,omitempty"`
	PowerW                    *float64 `json:"power_w,omitempty"`
	HealthPercent             *float64 `json:"health_percent,omitempty"`
	NamingConvention          string   `json:"naming_convention,omitempty"`
	PowerCalculation          string   `json:"power_calculation,omitempty"`
	EstimatedRuntimeSeconds   *int     `json:"estimated_runtime_seconds"`
	EstimatedRuntimeHours     *float64 `json:"estimated_runtime_hours"`
	EstimatedRuntimeAvailable bool     `json:"estimated_runtime_available"`
	EstimatedRuntimeReason    *string  `json:"estimated_runtime_reason"`
}

// Probe describes what the provider can see without being a full telemetry read.
type Probe struct {
	Kind             string   `json:"kind"`
	BatteryPresent   bool     `json:"battery_present"`
	BatteryName      string   `json:"battery_name,omitempty"`
	AvailableFields  []string `json:"available_fields"`
	SysfsRoot        string   `json:"sysfs_root,omitempty"`
	NamingConvention string   `json:"naming_convention,omitempty"`
	PowerCalculation string   `json:"power_calculation,omitempty"`
}

func (s *Snapshot) enrich() {
	s.Battery.PowerW = PowerWatts(
		s.Battery.PowerNowW,
		s.Battery.VoltageNowV,
		s.Battery.CurrentNowA,
		s.Battery.Status,
	)
	s.Battery.PowerCalculation = PowerCalculationMethod(
		s.Battery.PowerNowW != nil,
		s.Battery.VoltageNowV != nil && s.Battery.CurrentNowA != nil,
	)
	s.Battery.NamingConvention = NamingConvention(
		s.Battery.EnergyFullWh != nil || s.Battery.EnergyNowWh != nil || s.Battery.EnergyFullDesignWh != nil,
		s.Battery.ChargeFullAh != nil || s.Battery.ChargeNowAh != nil || s.Battery.ChargeFullDesignAh != nil,
	)

	switch {
	case s.Battery.EnergyFullWh != nil && s.Battery.EnergyFullDesignWh != nil:
		s.Battery.HealthPercent = HealthPercent(s.Battery.EnergyFullWh, s.Battery.EnergyFullDesignWh)
	default:
		s.Battery.HealthPercent = HealthPercent(s.Battery.ChargeFullAh, s.Battery.ChargeFullDesignAh)
	}

	if s.Battery.EnergyNowWh == nil {
		s.Battery.EnergyNowWh = ChargeToEnergyWh(s.Battery.ChargeNowAh, s.Battery.VoltageNowV)
	}
	if s.Battery.EnergyFullWh == nil {
		s.Battery.EnergyFullWh = ChargeToEnergyWh(s.Battery.ChargeFullAh, s.Battery.VoltageNowV)
	}
	if s.Battery.EnergyFullDesignWh == nil {
		s.Battery.EnergyFullDesignWh = ChargeToEnergyWh(s.Battery.ChargeFullDesignAh, s.Battery.VoltageNowV)
	}

	est := EstimateRuntime(s.Battery.Present, s.Battery.Status, s.Battery.EnergyNowWh, s.Battery.PowerW)
	s.Battery.EstimatedRuntimeAvailable = est.Available
	s.Battery.EstimatedRuntimeSeconds = est.Seconds
	s.Battery.EstimatedRuntimeHours = est.Hours
	if est.Reason != "" {
		reason := est.Reason
		s.Battery.EstimatedRuntimeReason = &reason
	}

	s.MissingFields = filterEquivalentMissing(s.MissingFields, s.AvailableFields)
}

func filterEquivalentMissing(missing, available []string) []string {
	have := make(map[string]struct{}, len(available))
	for _, f := range available {
		have[f] = struct{}{}
	}
	out := missing[:0]
	for _, f := range missing {
		if alt, ok := equivalentFields[f]; ok {
			if _, present := have[alt]; present {
				continue
			}
		}
		out = append(out, f)
	}
	if out == nil {
		return []string{}
	}
	return out
}
