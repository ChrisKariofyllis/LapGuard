package safety

import (
	"context"
	"sync"
)

// RecordingExecutor stores intended action names. It never looks up Docker,
// systemctl, shutdown, reboot, or sync, and it never starts a process.
type RecordingExecutor struct {
	mu    sync.Mutex
	calls []string
}

func NewRecordingExecutor() *RecordingExecutor {
	return &RecordingExecutor{}
}

func (r *RecordingExecutor) StopDocker(context.Context) error {
	r.record(ActionStopDocker)
	return nil
}

func (r *RecordingExecutor) Sync(context.Context) error {
	r.record(ActionSync)
	return nil
}

func (r *RecordingExecutor) PowerOff(context.Context) error {
	r.record(ActionPowerOff)
	return nil
}

func (r *RecordingExecutor) record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
}

func (r *RecordingExecutor) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func (r *RecordingExecutor) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}
