package battery

import (
	"math"
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

func microToUnit(v int64) float64 {
	return float64(v) / 1e6
}

// deciToUnit converts tenths (sysfs temp is typically tenths of °C) to units.
func deciToUnit(v int64) float64 {
	return float64(v) / 10
}
