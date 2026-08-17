package battery

import (
	"math"
	"strconv"
	"strings"
)

const (
	PowerMethodPowerNow       = "power_now"
	PowerMethodCurrentVoltage = "current_voltage"
	PowerMethodNone           = "none"

	NamingEnergy = "energy"
	NamingCharge = "charge"
	NamingBoth   = "both"
	NamingNone   = "none"
)

// PowerWatts returns instantaneous battery power in watts.
//
// Prefer power_now when present. Linux reports it as an unsigned magnitude on
// many drivers, so the sign is taken from current_now (negative = discharging)
// or from status. When power_now is missing, power is voltage_now * current_now.
//
// Sign convention: negative means discharging, positive means charging.
func PowerWatts(powerNowW, voltageV, currentA *float64, status string) *float64 {
	var watts float64
	switch {
	case powerNowW != nil:
		watts = math.Abs(*powerNowW)
		watts = applyPowerSign(watts, currentA, status)
	case voltageV != nil && currentA != nil:
		watts = *voltageV * *currentA
	default:
		return nil
	}
	return &watts
}

func applyPowerSign(absWatts float64, currentA *float64, status string) float64 {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "discharging":
		return -absWatts
	case "charging":
		return absWatts
	}
	if currentA != nil {
		switch {
		case *currentA < 0:
			return -absWatts
		case *currentA > 0:
			return absWatts
		}
	}
	return absWatts
}

// HealthPercent is full / design * 100. It works for both energy (Wh) and
// charge (Ah) pairs. Some packs report slightly over 100% when new; the value
// is not clamped.
func HealthPercent(full, design *float64) *float64 {
	if full == nil || design == nil {
		return nil
	}
	if *design == 0 || math.IsNaN(*design) || math.IsInf(*design, 0) {
		return nil
	}
	h := (*full / *design) * 100
	if math.IsNaN(h) || math.IsInf(h, 0) {
		return nil
	}
	return &h
}

// ChargeToEnergyWh converts amp-hours × volts into watt-hours.
func ChargeToEnergyWh(chargeAh, voltageV *float64) *float64 {
	if chargeAh == nil || voltageV == nil {
		return nil
	}
	wh := *chargeAh * *voltageV
	if math.IsNaN(wh) || math.IsInf(wh, 0) {
		return nil
	}
	return &wh
}

func PowerCalculationMethod(hasPowerNow, hasCurrentVoltage bool) string {
	switch {
	case hasPowerNow:
		return PowerMethodPowerNow
	case hasCurrentVoltage:
		return PowerMethodCurrentVoltage
	default:
		return PowerMethodNone
	}
}

func NamingConvention(hasEnergy, hasCharge bool) string {
	switch {
	case hasEnergy && hasCharge:
		return NamingBoth
	case hasEnergy:
		return NamingEnergy
	case hasCharge:
		return NamingCharge
	default:
		return NamingNone
	}
}

const (
	RuntimeReasonDischarging = "available while discharging"
	RuntimeReasonOnAC        = "not available while connected to AC"
	RuntimeReasonZeroPower   = "battery discharge power is zero"
	RuntimeReasonNoPower     = "battery discharge power is unavailable"
	RuntimeReasonNoEnergy    = "battery energy data is unavailable"
	RuntimeReasonNoBattery   = "battery is not present"
)

// RuntimeEstimate is time remaining at the current discharge rate. It is never
// Infinity/NaN; unavailable cases leave Seconds and Hours nil.
type RuntimeEstimate struct {
	Available bool
	Seconds   *int
	Hours     *float64
	Reason    string
}

// EstimateRuntime computes energy_now_wh / abs(power_w) in seconds. It is only
// available while the pack is discharging with usable energy and non-zero power.
func EstimateRuntime(present bool, status string, energyNowWh, powerW *float64) RuntimeEstimate {
	if !present {
		return RuntimeEstimate{Reason: RuntimeReasonNoBattery}
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "charging", "full", "not charging":
		return RuntimeEstimate{Reason: RuntimeReasonOnAC}
	case "discharging":
	default:
		return RuntimeEstimate{Reason: RuntimeReasonDischarging}
	}
	if energyNowWh == nil || *energyNowWh <= 0 || math.IsNaN(*energyNowWh) || math.IsInf(*energyNowWh, 0) {
		return RuntimeEstimate{Reason: RuntimeReasonNoEnergy}
	}
	if powerW == nil || math.IsNaN(*powerW) || math.IsInf(*powerW, 0) {
		return RuntimeEstimate{Reason: RuntimeReasonNoPower}
	}
	watts := math.Abs(*powerW)
	if watts == 0 {
		return RuntimeEstimate{Reason: RuntimeReasonZeroPower}
	}
	sec := (*energyNowWh / watts) * 3600
	if math.IsNaN(sec) || math.IsInf(sec, 0) || sec <= 0 {
		return RuntimeEstimate{Reason: RuntimeReasonNoPower}
	}
	rounded := int(math.Round(sec))
	if rounded < 1 {
		rounded = 1
	}
	hours := float64(rounded) / 3600
	return RuntimeEstimate{
		Available: true,
		Seconds:   &rounded,
		Hours:     &hours,
		Reason:    RuntimeReasonDischarging,
	}
}

// FormatEstimatedRuntime is the dashboard label for estimated_runtime_seconds.
func FormatEstimatedRuntime(seconds int, available bool) string {
	if !available || seconds <= 0 {
		return "—"
	}
	if seconds < 3600 {
		min := int(math.Round(float64(seconds) / 60))
		if min < 1 {
			min = 1
		}
		return strconv.Itoa(min) + " min"
	}
	if seconds < 86400 {
		h := seconds / 3600
		m := int(math.Round(float64(seconds%3600) / 60))
		if m == 60 {
			return strconv.Itoa(h+1) + "h"
		}
		if m == 0 {
			return strconv.Itoa(h) + "h"
		}
		return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
	}
	d := seconds / 86400
	h := int(math.Round(float64(seconds%86400) / 3600))
	if h == 24 {
		return strconv.Itoa(d+1) + "d"
	}
	if h == 0 {
		return strconv.Itoa(d) + "d"
	}
	return strconv.Itoa(d) + "d " + strconv.Itoa(h) + "h"
}

func microToUnit(v int64) float64 {
	return float64(v) / 1e6
}

// deciToUnit converts tenths (sysfs temp is typically tenths of °C) to units.
func deciToUnit(v int64) float64 {
	return float64(v) / 10
}
