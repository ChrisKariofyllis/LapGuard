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

	power := FeatureStatus{
		Key:             "power_now",
		Label:           "Power now",
		Enabled:         report.Features.PowerNow,
		DetectionMethod: "sysfs:" + battery.FieldPowerNow,
	}
	if power.Enabled {
		power.Recommendation = "Instantaneous power is read from power_now."
	} else if report.Features.CurrentVoltage {
		power.WhyNot = "sysfs power_now is absent"
		power.Recommendation = "Power will be computed from current_now × voltage_now."
		power.DetectionMethod = "derived:current_now*voltage_now"
	} else {
		power.WhyNot = "Neither power_now nor current_now+voltage_now is available"
		power.Recommendation = "Power (W) cannot be computed on this hardware."
	}

	cv := FeatureStatus{
		Key:             "current_voltage",
		Label:           "Current × voltage",
		Enabled:         report.Features.CurrentVoltage,
		DetectionMethod: "sysfs:current_now+voltage_now",
	}
	if cv.Enabled {
		cv.Recommendation = "current_now and voltage_now are present."
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

	return []FeatureStatus{charge, cycle, power, cv, temp, alarm, docker}
}
