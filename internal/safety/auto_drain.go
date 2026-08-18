package safety

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/power"
)

var (
	ErrNoPendingAutoDrain     = errors.New("no pending auto-drain prompt")
	ErrInvalidAutoDrainAction = errors.New("action must be yes or no")
)

const autoDrainPlanMessage = "Intended sequence: stop_docker → sync → poweroff"

type AutoDrainSnapshot struct {
	Enabled                bool       `json:"enabled"`
	State                  string     `json:"state"`
	BatteryPercent         *int       `json:"battery_percent,omitempty"`
	Discharging            bool       `json:"discharging"`
	ThresholdPercent       int        `json:"battery_threshold_percent"`
	PreNotificationMinutes int        `json:"pre_notification_minutes"`
	ResponseTimeoutMinutes int        `json:"response_timeout_minutes"`
	NotificationServices   []string   `json:"notification_services"`
	OnUserNo               string     `json:"on_user_no"`
	Notified               bool       `json:"notified"`
	AwaitingResponse       bool       `json:"awaiting_response"`
	SecondsRemaining       int        `json:"seconds_remaining"`
	DryRun                 bool       `json:"dry_run"`
	DockerStopEnabled      bool       `json:"docker_stop_enabled"`
	RealEnabled            bool       `json:"real_enabled"`
	CommandsExecuted       bool       `json:"commands_executed"`
	Plan                   []string   `json:"plan"`
	Gates                  []string   `json:"gates"`
	Reason                 string     `json:"reason,omitempty"`
	LastResponse           string     `json:"last_response,omitempty"`
	NotifiedAt             *time.Time `json:"notified_at,omitempty"`
	Message                string     `json:"message,omitempty"`
	Running                bool       `json:"running"`
}

type AutoDrainOptions struct {
	Interval time.Duration
	Config   func() config.Config
	Read     func(ctx context.Context) (Sample, error)
	Notify   func(ctx context.Context, event notify.NotificationEvent) error
	Executor ActionExecutor
	Live     func() ActionExecutor
	Logger   *slog.Logger
	Now      func() time.Time
}

// AutoDrain is the smart drain state machine. It never executes host commands
// unless a notification was delivered and the user said YES or the wait timed out.
type AutoDrain struct {
	interval time.Duration
	config   func() config.Config
	read     func(ctx context.Context) (Sample, error)
	notify   func(ctx context.Context, event notify.NotificationEvent) error
	executor ActionExecutor
	live     func() ActionExecutor
	rec      *RecordingExecutor
	log      *slog.Logger
	now      func() time.Time

	mu               sync.Mutex
	running          bool
	state            string
	reason           string
	lastSample       Sample
	notified         bool
	notifiedAt       time.Time
	deadline         time.Time
	lastResponse     string
	commandsExecuted bool
	lastPlan         []string
	executeTrigger   string
}

func NewAutoDrain(opts AutoDrainOptions) *AutoDrain {
	if opts.Config == nil {
		opts.Config = func() config.Config {
			return config.Config{
				AutoDrain:     config.DefaultAutoDrain(),
				Docker:        config.DefaultDocker(),
				Safety:        config.DefaultSafety(),
				Actions:       config.DefaultActions(),
				Notifications: config.DefaultNotifications(),
			}
		}
	}
	if opts.Interval <= 0 {
		opts.Interval = config.DefaultPowerPoll
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	rec := NewRecordingExecutor()
	exec := opts.Executor
	if exec == nil {
		exec = rec
	}
	return &AutoDrain{
		interval: opts.Interval,
		config:   opts.Config,
		read:     opts.Read,
		notify:   opts.Notify,
		executor: exec,
		live:     opts.Live,
		rec:      rec,
		log:      opts.Logger,
		now:      opts.Now,
		state:    AutoDrainIdle,
	}
}

func (a *AutoDrain) Interval() time.Duration { return a.interval }

func (a *AutoDrain) Run(ctx context.Context) {
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()

	a.Poll(ctx)
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.Poll(ctx)
		}
	}
}

func (a *AutoDrain) Poll(ctx context.Context) {
	if a.read == nil {
		return
	}
	sample, err := a.read(ctx)
	if err != nil {
		a.log.Warn("auto-drain poll failed", "err", err)
		return
	}
	a.Tick(ctx, sample)
}

func (a *AutoDrain) Snapshot() AutoDrainSnapshot {
	cfg := a.config()
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked(cfg)
}

func (a *AutoDrain) Reset(reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if reason == "" {
		reason = "auto-drain reset"
	}
	a.resetToIdle(reason)
}

func (a *AutoDrain) snapshotLocked(cfg config.Config) AutoDrainSnapshot {
	ad := cfg.AutoDrain
	services := append([]string{}, ad.NotificationServices...)
	if services == nil {
		services = []string{}
	}
	plan := append([]string{}, a.lastPlan...)
	if plan == nil {
		plan = autoDrainPlan()
	}
	var notifiedAt *time.Time
	if !a.notifiedAt.IsZero() {
		t := a.notifiedAt
		notifiedAt = &t
	}
	awaiting := a.state == AutoDrainWarningSent || a.state == AutoDrainAwaitingResponse
	remaining := 0
	if awaiting && !a.deadline.IsZero() {
		remaining = int(a.deadline.Sub(a.now()).Seconds())
		if remaining < 0 {
			remaining = 0
		}
	}
	msg := autoDrainMessage(cfg, a.state)
	return AutoDrainSnapshot{
		Enabled:                ad.Enabled,
		State:                  a.state,
		BatteryPercent:         a.lastSample.Percent,
		Discharging:            a.lastSample.Discharging,
		ThresholdPercent:       ad.BatteryThresholdPercent,
		PreNotificationMinutes: ad.PreNotificationMinutes,
		ResponseTimeoutMinutes: ad.ResponseTimeoutMinutes,
		NotificationServices:   services,
		OnUserNo:               ad.OnUserNo,
		Notified:               a.notified,
		AwaitingResponse:       awaiting,
		SecondsRemaining:       remaining,
		DryRun:                 cfg.Safety.DryRun,
		DockerStopEnabled:      cfg.Docker.StopEnabled,
		RealEnabled:            cfg.Actions.RealEnabled,
		CommandsExecuted:       a.commandsExecuted,
		Plan:                   plan,
		Gates:                  autoDrainGates(cfg, a.lastSample),
		Reason:                 a.reason,
		LastResponse:           a.lastResponse,
		NotifiedAt:             notifiedAt,
		Message:                msg,
		Running:                a.running,
	}
}

func (a *AutoDrain) Tick(ctx context.Context, sample Sample) AutoDrainSnapshot {
	if sample.Now.IsZero() {
		sample.Now = a.now()
	}
	cfg := a.config()

	a.mu.Lock()
	a.lastSample = sample
	prev := a.state
	if !cfg.AutoDrain.Enabled {
		a.resetToIdle("auto_drain is disabled")
		snap := a.snapshotLocked(cfg)
		a.mu.Unlock()
		return snap
	}

	if a.recovered(cfg, sample) {
		if a.state == AutoDrainAborted || a.state == AutoDrainTimeout || a.state == AutoDrainExecuting {
			a.resetToIdle("battery recovered; auto-drain rearmed")
		}
	}

	switch a.state {
	case AutoDrainIdle:
		if reason := a.startBlockReason(cfg, sample); reason != "" {
			a.reason = reason
			snap := a.snapshotLocked(cfg)
			a.mu.Unlock()
			return snap
		}
		a.state = AutoDrainWarningSent
		a.reason = "sending auto-drain warning"
		a.mu.Unlock()
		if err := a.sendWarning(ctx, cfg, sample); err != nil {
			a.mu.Lock()
			a.resetToIdle("notification failed; auto-drain will not execute")
			a.log.Warn("auto-drain notification failed", "err", err)
			snap := a.snapshotLocked(cfg)
			a.mu.Unlock()
			return snap
		}
		a.mu.Lock()
		timeout := time.Duration(cfg.AutoDrain.ResponseTimeoutMinutes) * time.Minute
		if timeout <= 0 {
			timeout = time.Duration(config.DefaultAutoDrainResponseMinutes) * time.Minute
		}
		a.notified = true
		a.notifiedAt = sample.Now
		a.deadline = sample.Now.Add(timeout)
		a.state = AutoDrainAwaitingResponse
		a.reason = "waiting for YES/NO in the dashboard"
		a.log.Info("auto-drain warning sent", "percent", ptrInt(sample.Percent), "timeout_min", cfg.AutoDrain.ResponseTimeoutMinutes)
		snap := a.snapshotLocked(cfg)
		a.mu.Unlock()
		return snap

	case AutoDrainWarningSent:
		a.state = AutoDrainAwaitingResponse
		fallthrough
	case AutoDrainAwaitingResponse:
		if !sample.Discharging || sample.Source == power.SourceAC {
			a.resetToIdle("AC restored before auto-drain executed")
			snap := a.snapshotLocked(cfg)
			a.mu.Unlock()
			return snap
		}
		if !a.notified {
			a.resetToIdle("auto-drain wait without a notification is refused")
			snap := a.snapshotLocked(cfg)
			a.mu.Unlock()
			return snap
		}
		if sample.Now.Before(a.deadline) {
			a.reason = "waiting for YES/NO in the dashboard"
			snap := a.snapshotLocked(cfg)
			a.mu.Unlock()
			return snap
		}
		a.lastResponse = "timeout"
		a.state = AutoDrainTimeout
		a.reason = "no response; proceeding with save and stop"
		a.mu.Unlock()
		a.execute(ctx, cfg, sample, "timeout")
		return a.Snapshot()
	}

	if prev != a.state {
		a.log.Info("auto-drain state", "from", prev, "to", a.state, "percent", ptrInt(sample.Percent))
	}
	snap := a.snapshotLocked(cfg)
	a.mu.Unlock()
	return snap
}

func (a *AutoDrain) Respond(ctx context.Context, action string) (AutoDrainSnapshot, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "yes" && action != "no" {
		return AutoDrainSnapshot{}, ErrInvalidAutoDrainAction
	}
	cfg := a.config()
	a.mu.Lock()
	if a.state != AutoDrainWarningSent && a.state != AutoDrainAwaitingResponse {
		snap := a.snapshotLocked(cfg)
		a.mu.Unlock()
		return snap, ErrNoPendingAutoDrain
	}
	if !a.notified {
		a.resetToIdle("auto-drain wait without a notification is refused")
		snap := a.snapshotLocked(cfg)
		a.mu.Unlock()
		return snap, ErrNoPendingAutoDrain
	}
	sample := a.lastSample
	if sample.Now.IsZero() {
		sample.Now = a.now()
	}
	a.lastResponse = action
	if action == "no" {
		a.state = AutoDrainAborted
		a.reason = "user chose to continue on battery"
		a.log.Info("auto-drain aborted by user")
		snap := a.snapshotLocked(cfg)
		a.mu.Unlock()
		return snap, nil
	}
	a.state = AutoDrainExecuting
	a.reason = "user confirmed save and stop"
	a.mu.Unlock()
	a.execute(ctx, cfg, sample, "yes")
	return a.Snapshot(), nil
}

func (a *AutoDrain) execute(ctx context.Context, cfg config.Config, sample Sample, trigger string) {
	a.mu.Lock()
	if !a.notified {
		a.reason = "refusing execute without a prior notification"
		a.commandsExecuted = false
		a.mu.Unlock()
		return
	}
	a.executeTrigger = trigger
	if trigger == "timeout" {
		a.state = AutoDrainTimeout
	} else {
		a.state = AutoDrainExecuting
	}
	a.lastPlan = autoDrainPlan()
	if reason := a.executeBlockReason(cfg, sample); reason != "" {
		a.reason = reason
		a.commandsExecuted = false
		a.log.Info("auto-drain skipped", "reason", reason, "trigger", trigger)
		a.mu.Unlock()
		return
	}
	if cfg.Safety.DryRun {
		a.reason = "safety.dry_run is true; sequence will be recorded only"
	}
	exec := a.pickExecutor(cfg)
	realRun := cfg.ManualActionsReady() && !cfg.Safety.DryRun
	a.mu.Unlock()

	err := drainSyncPowerOff(ctx, exec)
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.commandsExecuted = false
		a.reason = "auto-drain sequence failed"
		a.log.Warn("auto-drain sequence failed", "trigger", trigger)
		return
	}
	a.commandsExecuted = realRun
	if realRun {
		a.reason = "drain, sync, and poweroff completed"
	} else {
		a.reason = "dry-run or real actions disabled; sequence recorded only"
	}
	a.log.Info("auto-drain sequence finished",
		"trigger", trigger,
		"commands_executed", a.commandsExecuted,
		"dry_run", cfg.Safety.DryRun,
	)
}

func (a *AutoDrain) pickExecutor(cfg config.Config) ActionExecutor {
	if !cfg.ManualActionsReady() {
		if a.executor != nil {
			return a.executor
		}
		if a.rec != nil {
			return a.rec
		}
		return NewRecordingExecutor()
	}
	if a.live != nil {
		if exec := a.live(); exec != nil {
			return exec
		}
	}
	if a.executor != nil {
		return a.executor
	}
	if a.rec != nil {
		return a.rec
	}
	return NewRecordingExecutor()
}

func (a *AutoDrain) sendWarning(ctx context.Context, cfg config.Config, sample Sample) error {
	if a.notify == nil {
		return errors.New("notifier is not configured")
	}
	defer func() {
		if rec := recover(); rec != nil {
			a.log.Error("auto-drain notification panicked")
		}
	}()
	pct := 0
	if sample.Percent != nil {
		pct = *sample.Percent
	}
	pre := cfg.AutoDrain.PreNotificationMinutes
	if pre <= 0 {
		pre = config.DefaultAutoDrainPreNotifyMinutes
	}
	msg := fmt.Sprintf("Battery low (%d%%)! In %dmin: [YES] Save+Stop / [NO] Let run. Confirm in the LapGuard dashboard.", pct, pre)
	nctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return a.notify(nctx, notify.NotificationEvent{
		Type:      notify.EventAutoDrain,
		Title:     "LapGuard: battery low",
		Message:   msg,
		Timestamp: sample.Now,
	})
}

func (a *AutoDrain) startBlockReason(cfg config.Config, sample Sample) string {
	if !cfg.AutoDrain.Enabled {
		return "auto_drain is disabled"
	}
	if a.notify == nil {
		return "notifications are disabled or unconfigured"
	}
	if !cfg.Docker.StopEnabled {
		return "docker.stop_enabled is false"
	}
	if !sample.Present || sample.Percent == nil {
		return "battery percentage is unavailable"
	}
	if !sample.Discharging || sample.Source == power.SourceAC {
		return "battery is not discharging"
	}
	if *sample.Percent > cfg.AutoDrain.BatteryThresholdPercent {
		return "battery is above the auto-drain threshold"
	}
	if reason := notificationReadyReason(cfg); reason != "" {
		return reason
	}
	return ""
}

func (a *AutoDrain) executeBlockReason(cfg config.Config, sample Sample) string {
	if !a.notified {
		return "refusing execute without a prior notification"
	}
	if !cfg.AutoDrain.Enabled {
		return "auto_drain is disabled"
	}
	if !cfg.Docker.StopEnabled {
		return "docker.stop_enabled is false"
	}
	if !sample.Discharging || sample.Source == power.SourceAC {
		return "battery is not discharging"
	}
	if sample.Percent == nil || *sample.Percent > cfg.AutoDrain.BatteryThresholdPercent {
		return "battery is above the auto-drain threshold"
	}
	return ""
}

func (a *AutoDrain) recovered(cfg config.Config, sample Sample) bool {
	if !sample.Discharging || sample.Source == power.SourceAC {
		return true
	}
	if sample.Percent == nil {
		return false
	}
	return *sample.Percent > cfg.AutoDrain.BatteryThresholdPercent+recoveryMarginPercent
}

func (a *AutoDrain) resetToIdle(reason string) {
	a.state = AutoDrainIdle
	a.reason = reason
	a.notified = false
	a.notifiedAt = time.Time{}
	a.deadline = time.Time{}
	a.lastResponse = ""
	a.commandsExecuted = false
	a.executeTrigger = ""
}

func autoDrainGates(cfg config.Config, sample Sample) []string {
	gates := []string{}
	if !cfg.AutoDrain.Enabled {
		gates = append(gates, "auto_drain_disabled")
	}
	if !cfg.Docker.StopEnabled {
		gates = append(gates, "docker_stop_disabled")
	}
	if cfg.Safety.DryRun {
		gates = append(gates, "safety_dry_run")
	}
	if !cfg.Actions.RealEnabled {
		gates = append(gates, "real_actions_disabled")
	}
	if !sample.Discharging || sample.Source == power.SourceAC {
		gates = append(gates, "battery_not_discharging")
	}
	if sample.Percent == nil || *sample.Percent > cfg.AutoDrain.BatteryThresholdPercent {
		gates = append(gates, "battery_above_threshold")
	}
	if reason := notificationReadyReason(cfg); reason != "" {
		gates = append(gates, "notification_unconfigured")
	}
	return gates
}

func notificationReadyReason(cfg config.Config) string {
	n := cfg.Notifications
	if !n.Enabled || !n.ProviderConfigured() {
		return "notifications are disabled or unconfigured"
	}
	if !cfg.AutoDrain.AllowsProvider(n.Provider) {
		return "notification provider is not in auto_drain.notification_services"
	}
	return ""
}

func autoDrainPlan() []string {
	return []string{ActionStopDocker, ActionSync, ActionPowerOff}
}

func autoDrainMessage(cfg config.Config, state string) string {
	if !cfg.AutoDrain.Enabled {
		return "Smart automatic drain is disabled"
	}
	switch state {
	case AutoDrainAwaitingResponse, AutoDrainWarningSent:
		return "Battery low warning sent. Reply YES or NO in the dashboard."
	case AutoDrainAborted:
		return "User chose to continue on battery"
	case AutoDrainTimeout:
		return "No response; save and stop " + dryOrRecorded(cfg)
	case AutoDrainExecuting:
		return "Save and stop " + dryOrRecorded(cfg)
	default:
		if cfg.Safety.DryRun {
			return DryRunMessage
		}
		return autoDrainPlanMessage
	}
}

func dryOrRecorded(cfg config.Config) string {
	if cfg.Safety.DryRun || !cfg.Actions.RealEnabled {
		return "was recorded only (commands_executed=false)"
	}
	return "is executing"
}

func drainSyncPowerOff(ctx context.Context, exec ActionExecutor) error {
	if exec == nil {
		return errors.New("action executor is unavailable")
	}
	if err := exec.StopDocker(ctx); err != nil {
		return err
	}
	if err := exec.Sync(ctx); err != nil {
		return err
	}
	return exec.PowerOff(ctx)
}

func AutoDrainWarningCopy(percent, preMinutes int) (title, message string) {
	title = "LapGuard: battery low"
	message = "Battery low (" + strconv.Itoa(percent) + "%)! In " + strconv.Itoa(preMinutes) + "min: [YES] Save+Stop / [NO] Let run. Confirm in the LapGuard dashboard."
	return title, message
}
