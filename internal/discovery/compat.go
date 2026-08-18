package discovery

import (
	"encoding/json"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// CompatibilitySchemaVersion is the JSON schema for `lapguard discover --report`.
const CompatibilitySchemaVersion = "1"

// CompatibilityReport is a privacy-safe, deterministic snapshot of hardware
// discovery. It is meant to be attached to a GitHub compatibility issue.
// Serial numbers, hostnames, home paths, IPs, MACs, UUIDs, and secrets are
// omitted. Telemetry and GET /api/v1/discover still use CapabilityReport.
type CompatibilityReport struct {
	SchemaVersion    string `json:"schema_version"`
	LapGuardVersion  string `json:"lapguard_version"`
	OS               string `json:"os"`
	Kernel           string `json:"kernel"`
	Manufacturer     string `json:"manufacturer"`
	Model            string `json:"model"`
	NamingConvention string `json:"naming_convention"`
	PowerCalculation string `json:"power_calculation"`

	Batteries []CompatBattery `json:"batteries"`
	Adapters  []CompatAdapter `json:"adapters"`

	AvailableFields []string `json:"available_fields"`

	ChargeThresholdMethod string           `json:"charge_threshold_method"`
	Thresholds            CompatThresholds `json:"thresholds"`
	AvailableTools        Tools            `json:"available_tools"`
	KernelModules         []string         `json:"kernel_modules"`
	ModuleDetails         []ModuleDetail   `json:"module_details"`
	Features              Features         `json:"features"`
	FeatureDetails        []CompatFeature  `json:"feature_details"`
	Notes                 []string         `json:"notes,omitempty"`
}

// CompatBattery is one pack: sysfs name (BAT0/BAT1) and public identity only.
type CompatBattery struct {
	Name            string   `json:"name"`
	Present         bool     `json:"present"`
	Manufacturer    string   `json:"manufacturer,omitempty"`
	Model           string   `json:"model,omitempty"`
	Technology      string   `json:"technology,omitempty"`
	AvailableFields []string `json:"available_fields,omitempty"`
}

// CompatAdapter is a mains supply identified by discovered name, not a hardcoded list.
type CompatAdapter struct {
	Name   string `json:"name"`
	Online *bool  `json:"online,omitempty"`
}

// CompatThresholds is the charge-limit plan without sysfs paths.
type CompatThresholds struct {
	Method          string   `json:"method"`
	DetectionMethod string   `json:"detection_method"`
	BatteryName     string   `json:"battery_name,omitempty"`
	Attributes      []string `json:"attributes,omitempty"`
	Writable        bool     `json:"writable"`
	Start           *int     `json:"start,omitempty"`
	End             *int     `json:"end,omitempty"`
	WhyNot          string   `json:"why_not,omitempty"`
	Recommendation  string   `json:"recommendation,omitempty"`
}

// CompatFeature is one capability row after PII sanitization.
type CompatFeature struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Enabled         bool   `json:"enabled"`
	DetectionMethod string `json:"detection_method"`
	Recommendation  string `json:"recommendation"`
	WhyNot          string `json:"why_not,omitempty"`
	Method          string `json:"method,omitempty"`
}

// CompatibilityFrom copies hardware discovery into an export that never
// includes serials, hostnames, filesystem paths, or notification secrets.
// The input report is not modified.
func CompatibilityFrom(report CapabilityReport, version string) CompatibilityReport {
	secrets := secretValues(report)
	scrub := func(s string) string { return scrubText(s, secrets) }

	batteries := make([]CompatBattery, 0, len(report.Batteries))
	for _, b := range report.Batteries {
		batteries = append(batteries, CompatBattery{
			Name:            supplyName(b.Name, secrets),
			Present:         b.Present,
			Manufacturer:    scrub(b.Manufacturer),
			Model:           scrub(b.Model),
			Technology:      scrub(b.Technology),
			AvailableFields: sortedCopy(b.AvailableFields),
		})
	}
	sort.Slice(batteries, func(i, j int) bool { return batteries[i].Name < batteries[j].Name })

	adapters := make([]CompatAdapter, 0, len(report.Adapters))
	for _, a := range report.Adapters {
		item := CompatAdapter{Name: supplyName(a.Name, secrets)}
		if a.Online != nil {
			on := *a.Online
			item.Online = &on
		}
		adapters = append(adapters, item)
	}
	sort.Slice(adapters, func(i, j int) bool { return adapters[i].Name < adapters[j].Name })

	modules := sortedCopy(report.KernelModules)
	details := make([]ModuleDetail, 0, len(report.ModuleDetails))
	for _, d := range report.ModuleDetails {
		details = append(details, ModuleDetail{
			Name:    scrub(d.Name),
			Loaded:  d.Loaded,
			Enables: scrub(d.Enables),
		})
	}

	feats := make([]CompatFeature, 0)
	for _, f := range report.FeatureStatuses() {
		feats = append(feats, CompatFeature{
			Key:             f.Key,
			Label:           f.Label,
			Enabled:         f.Enabled,
			DetectionMethod: scrub(f.DetectionMethod),
			Recommendation:  scrub(f.Recommendation),
			WhyNot:          scrub(f.WhyNot),
			Method:          scrub(f.Method),
		})
	}

	notes := make([]string, 0, len(report.Notes))
	for _, n := range report.Notes {
		if s := scrub(n); s != "" {
			notes = append(notes, s)
		}
	}

	manufacturer := scrub(report.Battery.Manufacturer)
	model := scrub(report.Battery.Model)
	if manufacturer == "" && len(batteries) > 0 {
		manufacturer = batteries[0].Manufacturer
	}
	if model == "" && len(batteries) > 0 {
		model = batteries[0].Model
	}

	out := CompatibilityReport{
		SchemaVersion:         CompatibilitySchemaVersion,
		LapGuardVersion:       strings.TrimSpace(version),
		OS:                    scrub(report.OS),
		Kernel:                scrub(report.Kernel),
		Manufacturer:          manufacturer,
		Model:                 model,
		NamingConvention:      report.NamingConvention,
		PowerCalculation:      report.PowerCalculation,
		Batteries:             batteries,
		Adapters:              adapters,
		AvailableFields:       sortedCopy(report.AvailableFields),
		ChargeThresholdMethod: report.Features.ChargeThresholds,
		Thresholds: CompatThresholds{
			Method:          report.Thresholds.Method,
			DetectionMethod: scrub(report.Thresholds.DetectionMethod),
			BatteryName:     supplyName(report.Thresholds.BatteryName, secrets),
			Attributes:      thresholdAttributes(report.Thresholds),
			Writable:        report.Thresholds.Writable,
			Start:           report.Thresholds.Start,
			End:             report.Thresholds.End,
			WhyNot:          scrub(report.Thresholds.WhyNot),
			Recommendation:  scrub(report.Thresholds.Recommendation),
		},
		AvailableTools: Tools{
			TLP:        report.AvailableTools.TLP,
			TLPVersion: scrub(report.AvailableTools.TLPVersion),
			TLPCanSet:  report.AvailableTools.TLPCanSet,
			UPower:     report.AvailableTools.UPower,
			ACPI:       report.AvailableTools.ACPI,
			TPSMAPI:    report.AvailableTools.TPSMAPI,
			I8kutils:   report.AvailableTools.I8kutils,
		},
		KernelModules:  modules,
		ModuleDetails:  details,
		Features:       report.Features,
		FeatureDetails: feats,
		Notes:          notes,
	}
	if out.Batteries == nil {
		out.Batteries = []CompatBattery{}
	}
	if out.Adapters == nil {
		out.Adapters = []CompatAdapter{}
	}
	if out.AvailableFields == nil {
		out.AvailableFields = []string{}
	}
	if out.KernelModules == nil {
		out.KernelModules = []string{}
	}
	if out.ModuleDetails == nil {
		out.ModuleDetails = []ModuleDetail{}
	}
	if out.FeatureDetails == nil {
		out.FeatureDetails = []CompatFeature{}
	}
	if out.ChargeThresholdMethod == "" {
		out.ChargeThresholdMethod = MethodNone
	}
	if out.Thresholds.Method == "" {
		out.Thresholds.Method = MethodNone
	}
	return out
}

// WriteCompatibilityReport prints compact or pretty JSON plus a trailing newline.
func WriteCompatibilityReport(w io.Writer, report CompatibilityReport, pretty bool) error {
	var (
		raw []byte
		err error
	)
	if pretty {
		raw, err = json.MarshalIndent(report, "", "  ")
	} else {
		raw, err = json.Marshal(report)
	}
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

func supplyName(name string, secrets []string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	base := filepath.Base(name)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return scrubText(base, secrets)
}

func thresholdAttributes(plan ThresholdPlan) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range []string{plan.StartPath, plan.EndPath} {
		if p == "" {
			continue
		}
		base := filepath.Base(p)
		if base == "" || base == "." || strings.Contains(base, string(filepath.Separator)) {
			continue
		}
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
