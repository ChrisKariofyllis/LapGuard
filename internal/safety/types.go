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

// ActionExecutor is the host-side shutdown plan. Milestone 3D only records
// intended calls. A real executor must not be wired until a later milestone.
type ActionExecutor interface {
	StopDocker(ctx context.Context) error
	Sync(ctx context.Context) error
	PowerOff(ctx context.Context) error
}
