package safety

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/power"
)

var ErrUnknownScenario = errors.New("unknown safety test scenario (want warning or critical)")

// Sample is one controller input. Thresholds use only Percent.
type Sample struct {
	Now         time.Time
	Present     bool
	Percent     *int
	Status      string
	Discharging bool
	Source      power.Source
	Force       bool // simulate path: skip cooldown
}

type Event struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Percent   *int      `json:"percent,omitempty"`
	State     string    `json:"state,omitempty"`
}

type Snapshot struct {
	State                 string     `json:"state"`
	DryRun                bool       `json:"dry_run"`
	Message               string     `json:"message"`
	Running               bool       `json:"running"`
	BatteryPercent        *int       `json:"battery_percent,omitempty"`
	BatteryStatus         string     `json:"battery_status,omitempty"`
	ACSource              string     `json:"ac_source,omitempty"`
	Discharging           bool       `json:"discharging"`
	ShutdownEnabled       bool       `json:"shutdown_enabled"`
	WarningThreshold      int        `json:"warning_threshold"`
	CriticalThreshold     int        `json:"critical_threshold"`
	RequireACLoss         bool       `json:"require_ac_loss"`
	MinimumBatteryPercent int        `json:"minimum_battery_percent"`
	CooldownSeconds       int        `json:"cooldown_seconds"`
	LastEvent             *Event     `json:"last_event,omitempty"`
	LastActionAt          *time.Time `json:"last_action,omitempty"`
	IntendedActions       []string   `json:"intended_actions"`
	CommandsExecuted      bool       `json:"commands_executed"`
	Reason                string     `json:"reason,omitempty"`
}

type Options struct {
	Interval time.Duration
	Config   func() config.Config
	Read     func(ctx context.Context) (Sample, error)
	Notify   func(ctx context.Context, event notify.NotificationEvent) error
	Executor ActionExecutor
	Logger   *slog.Logger
	Now      func() time.Time
}

// Controller is the battery safety state machine. It never executes host commands
// while dry_run is true (the default). The only executor in this milestone is a recorder.
type Controller struct {
	interval time.Duration
	config   func() config.Config
	read     func(ctx context.Context) (Sample, error)
	notify   func(ctx context.Context, event notify.NotificationEvent) error
	executor ActionExecutor
	log      *slog.Logger
	now      func() time.Time

	mu              sync.Mutex
	running         bool
	state           string
	warningLatched  bool
	criticalLatched bool
	shutdownLatched bool
	lastEvent       *Event
	lastPlan        []string
	lastActionAt    time.Time
	lastSample      Sample
	reason          string
}

func New(opts Options) *Controller {
	if opts.Config == nil {
		opts.Config = func() config.Config {
			cfg := config.Config{
				Shutdown: config.DefaultShutdown(),
				Docker:   config.DefaultDocker(),
				Safety:   config.DefaultSafety(),
			}
			return cfg
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
	if opts.Executor == nil {
		opts.Executor = NewRecordingExecutor()
	}
	return &Controller{
		interval: opts.Interval,
		config:   opts.Config,
		read:     opts.Read,
		notify:   opts.Notify,
		executor: opts.Executor,
		log:      opts.Logger,
		now:      opts.Now,
		state:    StateUnknown,
	}
}

func (c *Controller) Interval() time.Duration { return c.interval }

func (c *Controller) Run(ctx context.Context) {
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	c.Poll(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Poll(ctx)
		}
	}
}

func (c *Controller) Poll(ctx context.Context) {
	if c.read == nil {
		return
	}
	sample, err := c.read(ctx)
	if err != nil {
		c.log.Warn("safety poll failed", "err", err)
		return
	}
	c.Tick(ctx, sample)
}

func (c *Controller) Snapshot() Snapshot {
	cfg := c.config()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked(cfg)
}

func (c *Controller) snapshotLocked(cfg config.Config) Snapshot {
	plan := append([]string{}, c.lastPlan...)
	if plan == nil {
		plan = []string{}
	}
	var lastAction *time.Time
	if !c.lastActionAt.IsZero() {
		t := c.lastActionAt
		lastAction = &t
	}
	var lastEvent *Event
	if c.lastEvent != nil {
		ev := *c.lastEvent
		lastEvent = &ev
	}
	return Snapshot{
		State:                 c.state,
		DryRun:                true, // this milestone is always dry-run in the API banner
		Message:               DryRunMessage,
		Running:               c.running,
		BatteryPercent:        c.lastSample.Percent,
		BatteryStatus:         c.lastSample.Status,
		ACSource:              string(c.lastSample.Source),
		Discharging:           c.lastSample.Discharging,
		ShutdownEnabled:       cfg.Shutdown.Enabled,
		WarningThreshold:      cfg.Shutdown.WarningThreshold,
		CriticalThreshold:     cfg.Shutdown.CriticalThreshold,
		RequireACLoss:         cfg.Safety.RequireACLoss,
		MinimumBatteryPercent: cfg.Safety.MinimumBatteryPercent,
		CooldownSeconds:       cfg.Safety.CooldownSeconds,
		LastEvent:             lastEvent,
		LastActionAt:          lastAction,
		IntendedActions:       plan,
		CommandsExecuted:      false,
		Reason:                c.reason,
	}
}

// Tick evaluates one sample. It never panics on notification failure.
func (c *Controller) Tick(ctx context.Context, sample Sample) Snapshot {
	if sample.Now.IsZero() {
		sample.Now = c.now()
	}
	cfg := c.config()
	c.mu.Lock()
	c.lastSample = sample
	prev := c.state
	event, plan, reason, state := c.evaluateLocked(cfg, sample)
	c.state = state
	c.reason = reason
	if len(plan) > 0 {
		c.lastPlan = append([]string{}, plan...)
		c.lastActionAt = sample.Now
	}
	if event != nil {
		c.lastEvent = event
	}
	snap := c.snapshotLocked(cfg)
	c.mu.Unlock()

	if event != nil {
		c.emit(ctx, *event, sample)
		if prev != state {
			c.log.Info("safety state", "from", prev, "to", state, "event", event.Type, "percent", ptrInt(sample.Percent))
		}
	} else if prev != state {
		c.log.Info("safety state", "from", prev, "to", state, "percent", ptrInt(sample.Percent))
	}

	if len(plan) > 0 {
		c.log.Info("safety intended action", "event", EventBatteryCritical, "actions", plan, "dry_run", true)
		// dry-run: never invoke the executor
	}

	return snap
}

func (c *Controller) evaluateLocked(cfg config.Config, sample Sample) (event *Event, plan []string, reason, state string) {
	warning := cfg.Shutdown.WarningThreshold
	critical := cfg.Shutdown.CriticalThreshold
	minPct := cfg.Safety.MinimumBatteryPercent

	if !sample.Present || sample.Percent == nil {
		return nil, nil, "battery percentage is unavailable", StateUnknown
	}
	pct := *sample.Percent
	if minPct > 0 && pct < minPct {
		return nil, nil, "battery percentage is below the configured minimum", StateUnknown
	}

	if sample.Source == power.SourceAC {
		c.resetLatches()
		return nil, nil, "AC is connected", StateACConnected
	}

	if cfg.Safety.RequireACLoss && sample.Source != power.SourceBattery {
		return nil, nil, "AC state is unknown; shutdown is not considered", StateUnknown
	}
	if sample.Source == power.SourceUnknown {
		return nil, nil, "AC state is unknown; shutdown is not considered", StateUnknown
	}

	if !sample.Discharging {
		if pct > warning {
			c.unlatchIfRecovered(pct, warning, critical)
		}
		return nil, nil, "battery is not discharging", StateNormal
	}

	c.unlatchIfRecovered(pct, warning, critical)

	switch {
	case pct <= critical:
		state = StateCritical
		if cfg.Shutdown.Enabled {
			state = StateShutdownPending
		}
		if !c.criticalLatched {
			c.criticalLatched = true
			c.warningLatched = true
			ev := Event{Type: EventBatteryCritical, Timestamp: sample.Now, Percent: sample.Percent, State: state}
			if cfg.Shutdown.Enabled && c.actionCooldownElapsed(sample.Now, cfg.Safety.CooldownSeconds, sample.Force) {
				plan = intendedPlan(cfg.Docker)
				c.shutdownLatched = true
			}
			return &ev, plan, "battery is at or below the critical threshold", state
		}
		if c.shutdownLatched && cfg.Shutdown.Enabled {
			return nil, nil, "critical shutdown already planned (dry-run)", StateShutdownPending
		}
		return nil, nil, "battery is at or below the critical threshold", state
	case pct <= warning:
		if !c.warningLatched {
			c.warningLatched = true
			ev := Event{Type: EventBatteryWarning, Timestamp: sample.Now, Percent: sample.Percent, State: StateWarning}
			return &ev, nil, "battery is at or below the warning threshold", StateWarning
		}
		return nil, nil, "battery is at or below the warning threshold", StateWarning
	default:
		return nil, nil, "", StateNormal
	}
}

func (c *Controller) resetLatches() {
	c.warningLatched = false
	c.criticalLatched = false
	c.shutdownLatched = false
}

func (c *Controller) unlatchIfRecovered(pct, warning, critical int) {
	if pct > warning+recoveryMarginPercent {
		c.warningLatched = false
	}
	if pct > critical+recoveryMarginPercent {
		c.criticalLatched = false
		c.shutdownLatched = false
	}
}

func (c *Controller) actionCooldownElapsed(now time.Time, seconds int, force bool) bool {
	if force || seconds <= 0 || c.lastActionAt.IsZero() {
		return true
	}
	return !now.Before(c.lastActionAt.Add(time.Duration(seconds) * time.Second))
}

func intendedPlan(docker config.DockerConfig) []string {
	plan := []string{}
	if docker.StopEnabled {
		plan = append(plan, ActionStopDocker)
	}
	plan = append(plan, ActionSync, ActionPowerOff)
	return plan
}

func (c *Controller) emit(ctx context.Context, event Event, sample Sample) {
	if c.notify == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			c.log.Error("safety notification panicked")
		}
	}()
	nctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	title, msg := notify.DefaultCopy(event.Type)
	if sample.Percent != nil {
		msg = "Battery charge is at " + strconv.Itoa(*sample.Percent) + "%."
	}
	err := c.notify(nctx, notify.NotificationEvent{
		Type:      event.Type,
		Title:     title,
		Message:   msg,
		Timestamp: event.Timestamp,
	})
	if err != nil {
		c.log.Warn("safety notification failed", "event", event.Type)
	}
}

func (c *Controller) Simulate(ctx context.Context, scenario string) (Snapshot, error) {
	cfg := c.config()
	warning := cfg.Shutdown.WarningThreshold
	critical := cfg.Shutdown.CriticalThreshold
	pct := warning
	switch scenario {
	case "warning":
		if pct <= critical {
			pct = critical + 1
		}
	case "critical":
		pct = critical
	default:
		return Snapshot{}, ErrUnknownScenario
	}

	c.mu.Lock()
	if scenario == "warning" {
		c.warningLatched = false
	} else {
		c.warningLatched = false
		c.criticalLatched = false
		c.shutdownLatched = false
	}
	c.mu.Unlock()

	sample := Sample{
		Now:         c.now(),
		Present:     true,
		Percent:     &pct,
		Status:      "Discharging",
		Discharging: true,
		Source:      power.SourceBattery,
		Force:       true,
	}
	return c.Tick(ctx, sample), nil
}

func ptrInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
