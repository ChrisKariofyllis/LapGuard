package actions

import (
	"context"

	"lapguard/internal/safety"
)

// DrainSyncPowerOff runs docker drain, sync, then poweroff. Callers must already
// have passed safety gates. Tests inject fake executors; production uses the
// gated real executor.
func DrainSyncPowerOff(ctx context.Context, exec safety.ActionExecutor) error {
	if exec == nil {
		return ErrUnavailable
	}
	if err := exec.StopDocker(ctx); err != nil {
		return err
	}
	if err := exec.Sync(ctx); err != nil {
		return err
	}
	return exec.PowerOff(ctx)
}
