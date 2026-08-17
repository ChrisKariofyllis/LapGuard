package discovery

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"lapguard/internal/battery"
)

var skipSupplyNames = map[string]struct{}{
	"device":    {},
	"power":     {},
	"subsystem": {},
	"uevent":    {},
	"hwmon":     {},
}

type hardwareScan struct {
	Batteries        []Supply
	Adapters         []Supply
	Primary          BatteryIdentity
	AvailableFields  []string
	NamingConvention string
	PowerCalculation string
	HasCycleCount    bool
	HasPowerNow      bool
	HasCurrent       bool
	HasVoltage       bool
	HasTemp          bool
	HasAlarm         bool
	HasReadableMains bool
	ChargeStartPath  string
	ChargeEndPath    string
	ChargeStart      *int
	ChargeEnd        *int
}

func scanHardware(opts Options) hardwareScan {
	scan := hardwareScan{
		Batteries:        []Supply{},
		Adapters:         []Supply{},
		AvailableFields:  []string{},
		NamingConvention: battery.NamingNone,
		PowerCalculation: battery.PowerMethodNone,
	}

	entries, err := os.ReadDir(opts.SysfsRoot)
	if err != nil {
		return scan
	}

	for _, entry := range entries {
		name := entry.Name()
		dir := filepath.Join(opts.SysfsRoot, name)
		typ := readSupplyString(dir, "type")
		if typ == "" {
			if strings.HasPrefix(strings.ToUpper(name), "BAT") {
				typ = "Battery"
			} else {
				continue
			}
		}

		fields := listSupplyFields(dir)
		sup := Supply{
			Name:            name,
			Path:            dir,
			Type:            typ,
			AvailableFields: fields,
			Manufacturer:    readSupplyString(dir, battery.FieldManufacturer),
			Model:           readSupplyString(dir, battery.FieldModelName),
			Serial:          readSupplyString(dir, battery.FieldSerialNumber),
			Technology:      readSupplyString(dir, battery.FieldTechnology),
		}

		switch strings.ToLower(typ) {
		case "battery":
			present := true
			if raw := readSupplyString(dir, "present"); raw == "0" {
				present = false
			}
			sup.Present = present
			scan.Batteries = append(scan.Batteries, sup)
		case "mains":
			if raw := readSupplyString(dir, "online"); raw == "1" || raw == "0" {
				on := raw == "1"
				sup.Online = &on
				scan.HasReadableMains = true
			}
			scan.Adapters = append(scan.Adapters, sup)
		}
	}

	primary, ok := pickPrimaryBattery(scan.Batteries)
	if !ok {
		return scan
	}
	scan.Primary = BatteryIdentity{
		Path:         primary.Path,
		Name:         primary.Name,
		Present:      primary.Present,
		Manufacturer: primary.Manufacturer,
		Model:        primary.Model,
		Serial:       primary.Serial,
		Technology:   primary.Technology,
	}
	scan.AvailableFields = primary.AvailableFields
	scan.NamingConvention = namingFromFields(primary.AvailableFields)
	scan.PowerCalculation = powerFromFields(primary.AvailableFields)
	scan.HasCycleCount = hasField(primary.AvailableFields, battery.FieldCycleCount)
	scan.HasPowerNow = hasField(primary.AvailableFields, battery.FieldPowerNow)
	scan.HasCurrent = hasField(primary.AvailableFields, battery.FieldCurrentNow)
	scan.HasVoltage = hasField(primary.AvailableFields, battery.FieldVoltageNow)
	scan.HasTemp = hasField(primary.AvailableFields, battery.FieldTemp)
	scan.HasAlarm = hasField(primary.AvailableFields, battery.FieldAlarm)

	startPath, startVal, startOK := findThresholdFile(primary.Path, battery.FieldChargeControlStart, battery.FieldChargeStartThreshold)
	endPath, endVal, endOK := findThresholdFile(primary.Path, battery.FieldChargeControlEnd, battery.FieldChargeStopThreshold)
	if startOK {
		scan.ChargeStartPath = startPath
		scan.ChargeStart = &startVal
	}
	if endOK {
		scan.ChargeEndPath = endPath
		scan.ChargeEnd = &endVal
	}

	return scan
}

func pickPrimaryBattery(bats []Supply) (Supply, bool) {
	if len(bats) == 0 {
		return Supply{}, false
	}
	for _, b := range bats {
		if b.Name == "BAT0" && b.Present {
			return b, true
		}
	}
	for _, b := range bats {
		if b.Present {
			return b, true
		}
	}
	return bats[0], true
}

func listSupplyFields(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if _, skip := skipSupplyNames[name]; skip {
			continue
		}
		if entry.IsDir() {
			continue
		}
		out = append(out, name)
	}
	return out
}

func readSupplyString(dir, field string) string {
	raw, err := os.ReadFile(filepath.Join(dir, field))
	if err != nil {
		return ""
	}
	return trimNL(string(raw))
}

func findThresholdFile(dir string, names ...string) (path string, value int, ok bool) {
	for _, name := range names {
		p := filepath.Join(dir, name)
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(trimNL(string(raw)))
		if err != nil {
			return p, 0, true
		}
		return p, n, true
	}
	return "", 0, false
}

func hasField(fields []string, name string) bool {
	for _, f := range fields {
		if f == name {
			return true
		}
	}
	return false
}

func namingFromFields(fields []string) string {
	var energy, charge bool
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f, "energy_"):
			energy = true
		case f == battery.FieldChargeNow || f == battery.FieldChargeFull || f == battery.FieldChargeFullDesign:
			charge = true
		}
	}
	return battery.NamingConvention(energy, charge)
}

func powerFromFields(fields []string) string {
	return battery.PowerCalculationMethod(
		hasField(fields, battery.FieldPowerNow),
		hasField(fields, battery.FieldCurrentNow) && hasField(fields, battery.FieldVoltageNow),
	)
}

func fileWritable(path string) bool {
	if path == "" {
		return false
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
