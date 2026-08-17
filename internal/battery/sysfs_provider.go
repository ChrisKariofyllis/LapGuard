package battery

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultSysfsRoot = "/sys/class/power_supply"

// skipAttrNames are sysfs entries that are not battery attributes.
var skipAttrNames = map[string]struct{}{
	"device":    {},
	"power":     {},
	"subsystem": {},
	"uevent":    {},
	"hwmon":     {},
}

// SysfsProvider reads Linux power_supply attributes from a configurable root.
// The default root is /sys/class/power_supply; tests point it at testdata/sysfs.
//
// It tries both energy_* and charge_* naming conventions and omits any field
// that is missing rather than failing the snapshot.
type SysfsProvider struct {
	root string
	name string
	now  func() time.Time
}

func NewSysfsProvider(root, batteryName string) *SysfsProvider {
	if strings.TrimSpace(root) == "" {
		root = defaultSysfsRoot
	}
	return &SysfsProvider{
		root: root,
		name: strings.TrimSpace(batteryName),
		now:  time.Now,
	}
}

func (p *SysfsProvider) Kind() string { return "sysfs" }

func (p *SysfsProvider) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}

	snap := Snapshot{
		Timestamp:       p.now().UTC(),
		Provider:        p.Kind(),
		Battery:         Battery{Name: p.name},
		AvailableFields: []string{},
		MissingFields:   []string{},
	}

	dir, name, err := p.resolveBatteryDir()
	if err != nil {
		snap.Warnings = append(snap.Warnings, err.Error())
		snap.MissingFields = append([]string{}, TrackedFields...)
		return snap, nil
	}
	snap.Battery.Name = name

	present, presentOK, w := readPresent(dir)
	if w != "" {
		snap.Warnings = append(snap.Warnings, w)
	}
	if presentOK && !present {
		snap.Battery.Present = false
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("%s reports present=0", name))
		snap.AvailableFields = listAttrNames(dir)
		return snap, nil
	}
	snap.Battery.Present = true
	snap.AvailableFields = listAttrNames(dir)

	status, missing, warn := readStringField(dir, FieldStatus)
	snap.note(FieldStatus, missing, warn)
	snap.Battery.Status = status

	if cap, missing, warn := readIntField(dir, FieldCapacity); missing || warn != "" {
		snap.note(FieldCapacity, missing, warn)
	} else {
		v := int(cap)
		snap.Battery.CapacityPercent = &v
	}

	level, missing, warn := readStringField(dir, FieldCapacityLevel)
	if !missing && warn == "" {
		snap.Battery.CapacityLevel = level
	}

	if v, missing, warn := readIntField(dir, FieldVoltageNow); missing || warn != "" {
		snap.note(FieldVoltageNow, missing, warn)
	} else {
		volts := microToUnit(v)
		snap.Battery.VoltageNowV = &volts
	}

	if v, missing, warn := readIntField(dir, FieldCurrentNow); missing || warn != "" {
		snap.note(FieldCurrentNow, missing, warn)
	} else {
		amps := microToUnit(v)
		snap.Battery.CurrentNowA = &amps
	}

	if v, missing, warn := readIntField(dir, FieldPowerNow); missing || warn != "" {
		snap.note(FieldPowerNow, missing, warn)
	} else {
		watts := microToUnit(v)
		snap.Battery.PowerNowW = &watts
	}

	if v, missing, warn := readIntField(dir, FieldEnergyNow); missing || warn != "" {
		snap.note(FieldEnergyNow, missing, warn)
	} else {
		wh := microToUnit(v)
		snap.Battery.EnergyNowWh = &wh
	}

	if v, missing, warn := readIntField(dir, FieldEnergyFull); missing || warn != "" {
		snap.note(FieldEnergyFull, missing, warn)
	} else {
		wh := microToUnit(v)
		snap.Battery.EnergyFullWh = &wh
	}

	if v, missing, warn := readIntField(dir, FieldEnergyFullDesign); missing || warn != "" {
		snap.note(FieldEnergyFullDesign, missing, warn)
	} else {
		wh := microToUnit(v)
		snap.Battery.EnergyFullDesignWh = &wh
	}

	if v, missing, warn := readIntField(dir, FieldChargeNow); missing || warn != "" {
		snap.note(FieldChargeNow, missing, warn)
	} else {
		ah := microToUnit(v)
		snap.Battery.ChargeNowAh = &ah
	}

	if v, missing, warn := readIntField(dir, FieldChargeFull); missing || warn != "" {
		snap.note(FieldChargeFull, missing, warn)
	} else {
		ah := microToUnit(v)
		snap.Battery.ChargeFullAh = &ah
	}

	if v, missing, warn := readIntField(dir, FieldChargeFullDesign); missing || warn != "" {
		snap.note(FieldChargeFullDesign, missing, warn)
	} else {
		ah := microToUnit(v)
		snap.Battery.ChargeFullDesignAh = &ah
	}

	if v, missing, warn := readIntField(dir, FieldCycleCount); missing || warn != "" {
		snap.note(FieldCycleCount, missing, warn)
	} else {
		n := int(v)
		snap.Battery.CycleCount = &n
	}

	if v, missing, warn := readIntField(dir, FieldTemp); missing || warn != "" {
		snap.note(FieldTemp, missing, warn)
	} else {
		c := deciToUnit(v)
		snap.Battery.TemperatureC = &c
	}

	if v, missing, warn := readIntField(dir, FieldAlarm); missing || warn != "" {
		snap.note(FieldAlarm, missing, warn)
	} else {
		snap.Battery.Alarm = &v
	}

	if s, missing, warn := readStringField(dir, FieldManufacturer); !missing && warn == "" {
		snap.Battery.Manufacturer = s
	}
	if s, missing, warn := readStringField(dir, FieldModelName); !missing && warn == "" {
		snap.Battery.ModelName = s
	}
	if s, missing, warn := readStringField(dir, FieldSerialNumber); !missing && warn == "" {
		snap.Battery.SerialNumber = s
	}
	if s, missing, warn := readStringField(dir, FieldTechnology); !missing && warn == "" {
		snap.Battery.Technology = s
	}

	if n, ok := readThreshold(dir, FieldChargeControlStart, FieldChargeStartThreshold); ok {
		snap.Battery.ChargeStartThreshold = &n
	}
	if n, ok := readThreshold(dir, FieldChargeControlEnd, FieldChargeStopThreshold); ok {
		snap.Battery.ChargeEndThreshold = &n
	}

	snap.enrich()
	return snap, nil
}

func (p *SysfsProvider) Probe(ctx context.Context) (Probe, error) {
	if err := ctx.Err(); err != nil {
		return Probe{}, err
	}

	probe := Probe{
		Kind:      p.Kind(),
		SysfsRoot: p.root,
	}

	dir, name, err := p.resolveBatteryDir()
	if err != nil {
		return probe, nil
	}
	probe.BatteryName = name

	present, presentOK, _ := readPresent(dir)
	if presentOK && !present {
		probe.AvailableFields = listAttrNames(dir)
		return probe, nil
	}
	probe.BatteryPresent = true
	probe.AvailableFields = listAttrNames(dir)
	probe.NamingConvention = detectNaming(probe.AvailableFields)
	probe.PowerCalculation = detectPowerMethod(probe.AvailableFields)
	return probe, nil
}

func (p *SysfsProvider) resolveBatteryDir() (string, string, error) {
	info, err := os.Stat(p.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", p.name, fmt.Errorf("sysfs root %s not found (no battery subsystem on this machine)", p.root)
		}
		return "", p.name, fmt.Errorf("sysfs root %s: %w", p.root, err)
	}
	if !info.IsDir() {
		return "", p.name, fmt.Errorf("sysfs root %s is not a directory", p.root)
	}

	if p.name != "" {
		dir := filepath.Join(p.root, p.name)
		st, err := os.Stat(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", p.name, fmt.Errorf("battery %s not found under %s", p.name, p.root)
			}
			return "", p.name, fmt.Errorf("battery %s: %w", p.name, err)
		}
		if !st.IsDir() {
			return "", p.name, fmt.Errorf("battery path %s is not a directory", dir)
		}
		return dir, p.name, nil
	}

	entries, err := os.ReadDir(p.root)
	if err != nil {
		return "", "", fmt.Errorf("read sysfs root %s: %w", p.root, err)
	}

	var found []string
	for _, entry := range entries {
		name := entry.Name()
		dir := filepath.Join(p.root, name)
		if !isBatteryDir(dir, name) {
			continue
		}
		found = append(found, name)
	}
	if len(found) == 0 {
		return "", "", fmt.Errorf("no battery found under %s", p.root)
	}
	// Prefer BAT0 when several packs exist.
	chosen := found[0]
	for _, name := range found {
		if name == "BAT0" {
			chosen = name
			break
		}
	}
	return filepath.Join(p.root, chosen), chosen, nil
}

func isBatteryDir(dir, name string) bool {
	typ, missing, _ := readStringField(dir, "type")
	if missing {
		return strings.HasPrefix(strings.ToUpper(name), "BAT")
	}
	return strings.EqualFold(typ, "Battery")
}

func (s *Snapshot) note(field string, missing bool, warning string) {
	if missing {
		s.MissingFields = append(s.MissingFields, field)
	}
	if warning != "" {
		s.Warnings = append(s.Warnings, warning)
	}
}

func readPresent(dir string) (present bool, ok bool, warning string) {
	raw, err := os.ReadFile(filepath.Join(dir, "present"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Many batteries omit this file; treat as present if the directory exists.
			return true, true, ""
		}
		return false, false, fmt.Sprintf("present: %s", classifyReadError("present", err))
	}
	v := strings.TrimSpace(string(raw))
	switch v {
	case "1":
		return true, true, ""
	case "0":
		return false, true, ""
	default:
		return false, false, fmt.Sprintf("present: unexpected value %q", v)
	}
}

func readStringField(dir, field string) (value string, missing bool, warning string) {
	raw, err := os.ReadFile(filepath.Join(dir, field))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", true, ""
		}
		return "", false, classifyReadError(field, err)
	}
	return strings.TrimSpace(string(raw)), false, ""
}

func readIntField(dir, field string) (value int64, missing bool, warning string) {
	raw, err := os.ReadFile(filepath.Join(dir, field))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, true, ""
		}
		return 0, false, classifyReadError(field, err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return 0, false, fmt.Sprintf("%s: file is empty", field)
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false, fmt.Sprintf("%s: not an integer %q", field, text)
	}
	return n, false, ""
}

func readThreshold(dir string, names ...string) (int, bool) {
	for _, name := range names {
		v, missing, warn := readIntField(dir, name)
		if missing || warn != "" {
			continue
		}
		return int(v), true
	}
	return 0, false
}

func listAttrNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if _, skip := skipAttrNames[name]; skip {
			continue
		}
		if entry.IsDir() {
			continue
		}
		out = append(out, name)
	}
	return out
}

func detectNaming(fields []string) string {
	var energy, charge bool
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f, "energy_"):
			energy = true
		case strings.HasPrefix(f, "charge_") && !strings.HasPrefix(f, "charge_control") && f != FieldChargeStartThreshold && f != FieldChargeStopThreshold:
			charge = true
		case f == FieldChargeNow || f == FieldChargeFull || f == FieldChargeFullDesign:
			charge = true
		}
	}
	return NamingConvention(energy, charge)
}

func detectPowerMethod(fields []string) string {
	var power, current, voltage bool
	for _, f := range fields {
		switch f {
		case FieldPowerNow:
			power = true
		case FieldCurrentNow:
			current = true
		case FieldVoltageNow:
			voltage = true
		}
	}
	return PowerCalculationMethod(power, current && voltage)
}

func classifyReadError(field string, err error) string {
	switch {
	case errors.Is(err, fs.ErrPermission):
		return fmt.Sprintf("%s: permission denied (LapGuard does not require root; check file mode)", field)
	default:
		return fmt.Sprintf("%s: %v", field, err)
	}
}
