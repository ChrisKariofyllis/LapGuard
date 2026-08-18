package safety

import "context"

const (
	StateNormal          = "NORMAL"
	StateWarning         = "WARNING"
	StateCritical        = "CRITICAL"
	StateShutdownPending = "SHUTDOWN_PENDING"
	StateACConnected     = "AC_CONNECTED"
	StateUnknown         = "UNKNOWN"

	ActionStopDocker = "stop_docker"
	ActionSync       = "sync"
	ActionPowerOff   = "poweroff"

	EventBatteryWarning  = "BATTERY_WARNING"
	EventBatteryCritical = "BATTERY_CRITICAL"

	DryRunMessage = "Dry run — no commands will be executed"

	recoveryMarginPercent = 2
)

// ActionExecutor is the host-side shutdown plan. The default implementation
// only records intended calls. A real executor is used only for explicit
// manual API actions when actions.real_enabled is true and safety.dry_run is false.
type ActionExecutor interface {
	StopDocker(ctx context.Context) error
	Sync(ctx context.Context) error
	PowerOff(ctx context.Context) error
}
