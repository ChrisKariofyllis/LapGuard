package config

import (
	"path/filepath"
	"strings"
)

const (
	DefaultActionsCooldown     = 60
	DefaultPowerOffTimeoutSecs = 30
	ConfirmPowerOff            = "POWER_OFF"
	ConfirmStopDocker          = "STOP_DOCKER"
)

// ActionsConfig gates experimental manual host/Docker actions. Real execution
// stays off unless real_enabled is true AND safety.dry_run is false.
type ActionsConfig struct {
	RealEnabled         bool   `json:"real_enabled"`
	RequireConfirmation bool   `json:"require_confirmation"`
	CooldownSeconds     int    `json:"cooldown_seconds"`
	PowerOffPath        string `json:"poweroff_path,omitempty"`
	DockerPath          string `json:"docker_path,omitempty"`
	SyncPath            string `json:"sync_path,omitempty"`
	PowerOffTimeoutSecs int    `json:"poweroff_timeout_seconds,omitempty"`
}

// ActionsView is the public HTTP representation. It never includes command lines.
type ActionsView struct {
	RealEnabled         bool     `json:"real_enabled"`
	RequireConfirmation bool     `json:"require_confirmation"`
	CooldownSeconds     int      `json:"cooldown_seconds"`
	PowerOffTimeoutSecs int      `json:"poweroff_timeout_seconds"`
	IntendedPlan        []string `json:"intended_plan"`
	Gates               []string `json:"gates"`
	Ready               bool     `json:"ready"`
}

func DefaultActions() ActionsConfig {
	return ActionsConfig{
		RealEnabled:         false,
		RequireConfirmation: true,
		CooldownSeconds:     DefaultActionsCooldown,
		PowerOffTimeoutSecs: DefaultPowerOffTimeoutSecs,
	}
}

func (a *ActionsConfig) normalize() error {
	a.PowerOffPath = strings.TrimSpace(a.PowerOffPath)
	a.DockerPath = strings.TrimSpace(a.DockerPath)
	a.SyncPath = strings.TrimSpace(a.SyncPath)
	if a.CooldownSeconds <= 0 {
		a.CooldownSeconds = DefaultActionsCooldown
	}
	if a.CooldownSeconds > 86400 {
		return invalidConfig("actions cooldown_seconds must be between 1 and 86400")
	}
	if a.PowerOffTimeoutSecs <= 0 {
		a.PowerOffTimeoutSecs = DefaultPowerOffTimeoutSecs
	}
	if a.PowerOffTimeoutSecs > 600 {
		return invalidConfig("actions poweroff_timeout_seconds must be between 1 and 600")
	}
	a.RequireConfirmation = true
	if err := validateOptionalExecPath("poweroff_path", a.PowerOffPath, []string{"systemctl", "poweroff"}); err != nil {
		return err
	}
	if err := validateOptionalExecPath("docker_path", a.DockerPath, []string{"docker"}); err != nil {
		return err
	}
	if err := validateOptionalExecPath("sync_path", a.SyncPath, []string{"sync"}); err != nil {
		return err
	}
	return nil
}

func validateOptionalExecPath(field, path string, allowedBase []string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		return invalidConfig(field + " must be an absolute path")
	}
	if strings.ContainsAny(path, "|&;<>$`\n\r") {
		return invalidConfig(field + " contains unsafe characters")
	}
	base := filepath.Base(path)
	for _, want := range allowedBase {
		if base == want {
			return nil
		}
	}
	return invalidConfig(field + " must end with " + strings.Join(allowedBase, " or "))
}

func (c Config) IntendedPlan() []string {
	plan := []string{}
	if c.Docker.StopEnabled {
		plan = append(plan, "stop_docker")
	}
	plan = append(plan, "sync", "poweroff")
	return plan
}

func (c Config) ActionGates() []string {
	gates := []string{}
	if !c.Actions.RealEnabled {
		gates = append(gates, "real_actions_disabled")
	}
	if c.Safety.DryRun {
		gates = append(gates, "safety_dry_run")
	}
	if !c.Actions.RequireConfirmation {
		gates = append(gates, "confirmation_required")
	}
	return gates
}

func (c Config) ManualActionsReady() bool {
	return c.Actions.RealEnabled && !c.Safety.DryRun
}

func (a ActionsConfig) View(plan []string, gates []string, ready bool) ActionsView {
	if plan == nil {
		plan = []string{}
	}
	if gates == nil {
		gates = []string{}
	}
	return ActionsView{
		RealEnabled:         a.RealEnabled,
		RequireConfirmation: a.RequireConfirmation,
		CooldownSeconds:     a.CooldownSeconds,
		PowerOffTimeoutSecs: a.PowerOffTimeoutSecs,
		IntendedPlan:        plan,
		Gates:               gates,
		Ready:               ready,
	}
}

type ActionsPatch struct {
	RealEnabled         *bool   `json:"real_enabled"`
	RequireConfirmation *bool   `json:"require_confirmation"`
	CooldownSeconds     *int    `json:"cooldown_seconds"`
	PowerOffPath        *string `json:"poweroff_path"`
	DockerPath          *string `json:"docker_path"`
	SyncPath            *string `json:"sync_path"`
	PowerOffTimeoutSecs *int    `json:"poweroff_timeout_seconds"`
}

func (a ActionsConfig) Apply(p ActionsPatch) (ActionsConfig, error) {
	out := a
	if p.RealEnabled != nil {
		out.RealEnabled = *p.RealEnabled
	}
	if p.RequireConfirmation != nil {
		out.RequireConfirmation = *p.RequireConfirmation
	}
	if p.CooldownSeconds != nil {
		out.CooldownSeconds = *p.CooldownSeconds
	}
	if p.PowerOffPath != nil {
		out.PowerOffPath = *p.PowerOffPath
	}
	if p.DockerPath != nil {
		out.DockerPath = *p.DockerPath
	}
	if p.SyncPath != nil {
		out.SyncPath = *p.SyncPath
	}
	if p.PowerOffTimeoutSecs != nil {
		out.PowerOffTimeoutSecs = *p.PowerOffTimeoutSecs
	}
	if err := out.normalize(); err != nil {
		return ActionsConfig{}, err
	}
	return out, nil
}

func (c Config) hostExecutionState() string {
	if !c.Actions.RealEnabled {
		return ExecutionDisabled
	}
	if c.Safety.DryRun {
		return ExecutionDryRun
	}
	return ExecutionReady
}
