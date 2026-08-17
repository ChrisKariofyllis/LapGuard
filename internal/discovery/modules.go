package discovery

import (
	"os"
	"path/filepath"
	"strings"
)

// KnownModules maps a kernel module to the hardware extras it typically enables.
// Names use the underscore form that /proc/modules reports.
var KnownModules = []ModuleDetail{
	{Name: "fujitsu_laptop", Enables: "Fujitsu extras (hotkeys, brightness). Charge control often fails to register."},
	{Name: "thinkpad_acpi", Enables: "ThinkPad ACPI extras; may expose charge_control sysfs via the battery device."},
	{Name: "dell_smbios", Enables: "Dell SMBIOS interface used by some charge and thermal controls."},
	{Name: "dell_wmi", Enables: "Dell WMI extras (hotkeys, thermal, sometimes charge behaviour)."},
	{Name: "asus_wmi", Enables: "ASUS WMI extras; may expose charge_control_end_threshold."},
	{Name: "asus_nb_wmi", Enables: "ASUS notebook WMI; charge mode / threshold on many laptops."},
	{Name: "hp_wmi", Enables: "HP WMI extras (hotkeys, rfkill). Charge limits are uncommon."},
	{Name: "hp_accel", Enables: "HP accelerometer / HDD protection."},
	{Name: "tp_smapi", Enables: "ThinkPad SMAPI; charge start/stop on older ThinkPads (T4x–T6x, X6x)."},
	{Name: "acpi_call", Enables: "Raw ACPI method calls; used by TLP/tpacpi-bat on some ThinkPads."},
}

func detectModules(opts Options) (loaded []string, details []ModuleDetail) {
	present := parseProcModules(opts.ProcModules)
	details = make([]ModuleDetail, 0, len(KnownModules))
	for _, m := range KnownModules {
		d := m
		d.Loaded = present[m.Name] || present[strings.ReplaceAll(m.Name, "_", "-")]
		if d.Loaded {
			loaded = append(loaded, d.Name)
		}
		details = append(details, d)
	}
	if loaded == nil {
		loaded = []string{}
	}
	return loaded, details
}

func parseProcModules(path string) map[string]bool {
	out := map[string]bool{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, _, _ := strings.Cut(line, " ")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = true
		out[strings.ReplaceAll(name, "-", "_")] = true
	}
	return out
}

func moduleLoaded(loaded []string, name string) bool {
	want := strings.ReplaceAll(name, "-", "_")
	for _, m := range loaded {
		if strings.ReplaceAll(m, "-", "_") == want {
			return true
		}
	}
	return false
}

func thinkpadACPIChargePaths(platformRoot string) []string {
	if platformRoot == "" {
		return nil
	}
	patterns := []string{
		filepath.Join(platformRoot, "thinkpad_acpi", "charge_control*"),
		filepath.Join(platformRoot, "thinkpad_acpi", "*", "charge_control*"),
		filepath.Join(platformRoot, "thinkpad_acpi", "charge_start_threshold"),
		filepath.Join(platformRoot, "thinkpad_acpi", "charge_stop_threshold"),
	}
	var found []string
	for _, pat := range patterns {
		matches, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		found = append(found, matches...)
	}
	return found
}
