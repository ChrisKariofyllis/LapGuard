package discovery

import "lapguard/internal/battery"

func (r CapabilityReport) FeatureStatuses() []FeatureStatus {
	return featureStatuses(r)
}

func featureStatuses(report CapabilityReport) []FeatureStatus {
	th := report.Thresholds
	charge := FeatureStatus{
		Key:             "charge_thresholds",
		Label:           "Charge thresholds",
		Enabled:         th.Method != "" && th.Method != MethodNone,
		DetectionMethod: th.DetectionMethod,
		Recommendation:  th.Recommendation,
		WhyNot:          th.WhyNot,
		Method:          th.Method,
	}

	cycle := FeatureStatus{
		Key:             "cycle_count",
		Label:           "Cycle count",
		Enabled:         report.Features.CycleCount,
		DetectionMethod: "sysfs:" + battery.FieldCycleCount,
	}
	if cycle.Enabled {
		cycle.Recommendation = "Cycle count is exposed by the battery driver."
	} else {
		cycle.WhyNot = "sysfs cycle_count is not present on this pack"
		cycle.Recommendation = "Wear can still be estimated from energy_full / energy_full_design when those exist."
	}

	raw := report.Features.RawPowerNowSupported || report.Features.PowerNow
	derived := report.Features.DerivedPowerSupported || report.Features.CurrentVoltage

	rawPower := FeatureStatus{
		Key:             "raw_power_now",
		Label:           "Raw power_now",
		Enabled:         raw,
		DetectionMethod: "sysfs:" + battery.FieldPowerNow,
	}
	if rawPower.Enabled {
		rawPower.Recommendation = "Instantaneous power is read from the power_now sysfs file."
	} else {
		rawPower.WhyNot = "sysfs power_now is not present"
		if derived {
			rawPower.Recommendation = "This is expected on packs like the Fujitsu A3510. Use the derived power estimate instead."
		} else {
			rawPower.Recommendation = "No power_now file and no current/voltage pair to estimate watts."
		}
	}

	derivedPower := FeatureStatus{
		Key:             "derived_power",
		Label:           "Derived power",
		Enabled:         derived,
		DetectionMethod: "derived:current_now*voltage_now",
	}
	if derivedPower.Enabled {
		derivedPower.Recommendation = "Power estimate is calculated from current_now × voltage_now. 0 W is valid when current_now is zero."
	} else if raw {
		derivedPower.WhyNot = "current_now and/or voltage_now is missing"
		derivedPower.Recommendation = "Not required; sysfs power_now supplies instantaneous power."
	} else {
		derivedPower.WhyNot = "current_now and/or voltage_now is missing"
		derivedPower.Recommendation = "Power (W) cannot be estimated on this hardware."
	}

	cv := FeatureStatus{
		Key:             "current_voltage",
		Label:           "Current × voltage sensors",
		Enabled:         derived,
		DetectionMethod: "sysfs:current_now+voltage_now",
	}
	if cv.Enabled {
		cv.Recommendation = "current_now and voltage_now are present and used for the derived power estimate."
	} else {
		cv.WhyNot = "current_now and/or voltage_now is missing"
		cv.Recommendation = "Voltage/current gauges are unavailable; rely on power_now or capacity."
	}

	temp := FeatureStatus{
		Key:             "temperature",
		Label:           "Temperature",
		Enabled:         report.Features.Temperature,
		DetectionMethod: "sysfs:" + battery.FieldTemp,
	}
	if temp.Enabled {
		temp.Recommendation = "Pack temperature is read from sysfs temp (tenths of °C)."
	} else {
		temp.WhyNot = "sysfs temp is not present"
		temp.Recommendation = "Temperature-based policies cannot run on this pack."
	}

	alarm := FeatureStatus{
		Key:             "alarm_control",
		Label:           "Alarm control",
		Enabled:         report.Features.AlarmControl,
		DetectionMethod: "sysfs:" + battery.FieldAlarm,
	}
	if alarm.Enabled {
		alarm.Recommendation = "The alarm attribute is present and can be monitored."
	} else {
		alarm.WhyNot = "sysfs alarm is not present"
		alarm.Recommendation = "Capacity alarm writes are not available."
	}

	docker := FeatureStatus{
		Key:             "docker_shutdown",
		Label:           "Docker shutdown",
		Enabled:         report.Features.DockerShutdown,
		DetectionMethod: "docker.sock+docker CLI",
	}
	if docker.Enabled {
		docker.Recommendation = "Docker is available for coordinated pre-shutdown container stops (control API comes in a later milestone)."
	} else {
		docker.WhyNot = "Docker CLI and /var/run/docker.sock were not found"
		docker.Recommendation = "Host shutdown can still be added later without Docker drain."
	}

	return []FeatureStatus{charge, cycle, rawPower, derivedPower, cv, temp, alarm, docker}
}
