package battery

import (
	"math"
	"strings"
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

// HealthPercent is energy_full / energy_full_design * 100.
// Some packs report slightly over 100% when new; the value is not clamped.
func HealthPercent(energyFullWh, energyFullDesignWh *float64) *float64 {
	if energyFullWh == nil || energyFullDesignWh == nil {
		return nil
	}
	if *energyFullDesignWh == 0 || math.IsNaN(*energyFullDesignWh) || math.IsInf(*energyFullDesignWh, 0) {
		return nil
	}
	h := (*energyFullWh / *energyFullDesignWh) * 100
	if math.IsNaN(h) || math.IsInf(h, 0) {
		return nil
	}
	return &h
}

func microToUnit(v int64) float64 {
	return float64(v) / 1e6
}
