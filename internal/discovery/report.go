package discovery

import "time"

// Charge-threshold methods. sysfs is preferred when both sysfs and TLP work.
const (
	MethodSysfs = "sysfs"
	MethodTLP   = "tlp"
	MethodNone  = "none"
)

// CapabilityReport is the full auto-discovery result for one machine.
type CapabilityReport struct {
	Timestamp time.Time `json:"timestamp"`
	Hostname  string    `json:"hostname"`
	OS        string    `json:"os"`
	Kernel    string    `json:"kernel"`

	Battery BatteryIdentity `json:"battery"`

	Batteries []Supply `json:"batteries,omitempty"`
	Adapters  []Supply `json:"adapters,omitempty"`

	AvailableFields  []string `json:"available_fields"`
	NamingConvention string   `json:"naming_convention,omitempty"`
	PowerCalculation string   `json:"power_calculation,omitempty"`

	Features Features `json:"features"`

	AvailableTools Tools `json:"available_tools"`

	KernelModules []string       `json:"kernel_modules"`
	ModuleDetails []ModuleDetail `json:"module_details,omitempty"`

	Thresholds ThresholdPlan `json:"thresholds"`
	Notes      []string      `json:"notes,omitempty"`
}

// BatteryIdentity is the primary pack summarized on the report.
type BatteryIdentity struct {
	Path         string `json:"path"`
	Name         string `json:"name,omitempty"`
	Present      bool   `json:"present"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	Technology   string `json:"technology"`
}

// Supply is one power_supply class device (battery or AC adapter).
type Supply struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Type            string   `json:"type"`
	Present         bool     `json:"present"`
	Online          *bool    `json:"online,omitempty"`
	AvailableFields []string `json:"available_fields,omitempty"`
	Manufacturer    string   `json:"manufacturer,omitempty"`
	Model           string   `json:"model,omitempty"`
	Serial          string   `json:"serial,omitempty"`
	Technology      string   `json:"technology,omitempty"`
}

// Features is the compact capability bitmap used by GET /api/v1/discover.
type Features struct {
	ChargeThresholds string `json:"charge_thresholds"` // "sysfs" | "tlp" | "none"
	CycleCount       bool   `json:"cycle_count"`
	// PowerNow is true only when sysfs power_now exists (same as RawPowerNowSupported).
	PowerNow bool `json:"power_now"`
	// RawPowerNowSupported is true only when the power_now sysfs file exists.
	RawPowerNowSupported bool `json:"raw_power_now_supported"`
	// DerivedPowerSupported is true when current_now and voltage_now can be multiplied.
	DerivedPowerSupported bool `json:"derived_power_supported"`
	CurrentVoltage        bool `json:"current_voltage"`
	Temperature           bool `json:"temperature"`
	AlarmControl          bool `json:"alarm_control"`
	DockerShutdown        bool `json:"docker_shutdown"`
}

// Tools lists userspace helpers that LapGuard can detect.
type Tools struct {
	TLP        bool   `json:"tlp"`
	TLPVersion string `json:"tlp_version,omitempty"`
	TLPCanSet  bool   `json:"tlp_can_set_thresholds"`
	UPower     bool   `json:"upower"`
	ACPI       bool   `json:"acpi"`
	TPSMAPI    bool   `json:"tp_smapi"`
	I8kutils   bool   `json:"i8kutils"`
}

// ModuleDetail records a known vendor module and what it enables.
type ModuleDetail struct {
	Name    string `json:"name"`
	Loaded  bool   `json:"loaded"`
	Enables string `json:"enables"`
}

// ThresholdPlan is the detected write path for charge start/stop limits.
type ThresholdPlan struct {
	Method          string `json:"method"` // sysfs | tlp | none
	DetectionMethod string `json:"detection_method"`
	StartPath       string `json:"start_path,omitempty"`
	EndPath         string `json:"end_path,omitempty"`
	BatteryName     string `json:"battery_name,omitempty"`
	Start           *int   `json:"start,omitempty"`
	End             *int   `json:"end,omitempty"`
	Writable        bool   `json:"writable"`
	WhyNot          string `json:"why_not,omitempty"`
	Recommendation  string `json:"recommendation,omitempty"`
}

// FeatureStatus is the UI-oriented view of one capability.
type FeatureStatus struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Enabled         bool   `json:"enabled"`
	DetectionMethod string `json:"detection_method"`
	Recommendation  string `json:"recommendation"`
	WhyNot          string `json:"why_not,omitempty"`
	Method          string `json:"method,omitempty"`
}
