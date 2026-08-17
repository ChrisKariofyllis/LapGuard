package discovery

import (
	"fmt"
	"path/filepath"
	"strings"
)

func detectThresholds(opts Options, hw hardwareScan, mods []string, tools toolScan) ThresholdPlan {
	plan := ThresholdPlan{
		Method:          MethodNone,
		DetectionMethod: "sysfs+tlp+thinkpad_acpi",
		BatteryName:     hw.Primary.Name,
	}

	sysfsOK := hw.ChargeStartPath != "" || hw.ChargeEndPath != ""
	if sysfsOK {
		plan.StartPath = hw.ChargeStartPath
		plan.EndPath = hw.ChargeEndPath
		plan.Start = hw.ChargeStart
		plan.End = hw.ChargeEnd
		plan.Writable = fileWritable(firstNonEmpty(hw.ChargeStartPath, hw.ChargeEndPath))
		plan.Method = MethodSysfs
		plan.DetectionMethod = "sysfs_native"
		plan.Recommendation = "Use sysfs charge_control attributes (best available method)."
		if !plan.Writable {
			plan.Recommendation += " Reads succeed; writes may need root or a udev rule."
		}
		return plan
	}

	if tpPaths := thinkpadACPIChargePaths(opts.PlatformRoot); len(tpPaths) > 0 {
		start, end := splitThinkPadThresholds(tpPaths)
		plan.StartPath = start
		plan.EndPath = end
		plan.Writable = fileWritable(firstNonEmpty(start, end))
		plan.Method = MethodSysfs
		plan.DetectionMethod = "sysfs_native"
		plan.Recommendation = "Use thinkpad_acpi platform charge_control sysfs (best available method)."
		if v, _, ok := readThresholdValue(start); ok {
			plan.Start = &v
		}
		if v, _, ok := readThresholdValue(end); ok {
			plan.End = &v
		}
		return plan
	}

	if tools.Tools.TLP && tools.TLPCanSet {
		plan.Method = MethodTLP
		plan.DetectionMethod = "tlp"
		plan.Recommendation = "Use TLP setcharge; this machine has no charge_control sysfs files."
		plan.Writable = true
		return plan
	}

	plan.WhyNot = thresholdWhyNot(mods, tools)
	plan.Recommendation = "Keep the pack between ~20–80% with a smart plug or by unplugging; firmware charge limits are not available."
	return plan
}

func thresholdWhyNot(mods []string, tools toolScan) string {
	var parts []string
	parts = append(parts, "No charge_control sysfs attributes")
	if moduleLoaded(mods, "fujitsu_laptop") {
		parts = append(parts, "fujitsu_laptop is loaded but did not register charge control (no charge_control sysfs files)")
	}
	if moduleLoaded(mods, "thinkpad_acpi") {
		parts = append(parts, "thinkpad_acpi is loaded but charge_control sysfs is absent")
	}
	if moduleLoaded(mods, "asus_wmi") || moduleLoaded(mods, "asus_nb_wmi") {
		parts = append(parts, "ASUS WMI modules are loaded but charge_control sysfs is absent")
	}
	if moduleLoaded(mods, "dell_wmi") || moduleLoaded(mods, "dell_smbios") {
		parts = append(parts, "Dell WMI/SMBIOS modules are loaded but charge_control sysfs is absent")
	}
	if tools.Tools.TLP {
		if tools.TLPCanSet {
			parts = append(parts, "TLP reports threshold support")
		} else {
			parts = append(parts, "TLP is installed but cannot control this hardware")
		}
	} else {
		parts = append(parts, "TLP is not installed")
	}
	if len(parts) == 0 {
		return "No charge_control sysfs attributes and no TLP threshold backend"
	}
	return strings.Join(parts, "; ") + "."
}

func splitThinkPadThresholds(paths []string) (start, end string) {
	for _, p := range paths {
		base := filepath.Base(p)
		switch {
		case strings.Contains(base, "start"):
			start = p
		case strings.Contains(base, "end") || strings.Contains(base, "stop"):
			end = p
		default:
			if start == "" {
				start = p
			} else if end == "" {
				end = p
			}
		}
	}
	return start, end
}

func readThresholdValue(path string) (int, string, bool) {
	if path == "" {
		return 0, "", false
	}
	raw, err := readTrimmed(path)
	if err != nil {
		return 0, "", false
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
		return 0, raw, false
	}
	return n, raw, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
