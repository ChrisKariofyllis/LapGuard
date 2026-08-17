package power

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Transition is a debounced AC state change. The initial baseline is never a transition.
type Transition struct {
	Type       string    `json:"type"`
	At         time.Time `json:"timestamp"`
	Source     string    `json:"source"`
	From       Source    `json:"from"`
	To         Source    `json:"to"`
	DurationMs *int64    `json:"duration_ms,omitempty"`
	Snapshot   Snapshot  `json:"-"`
}

type Options struct {
	SysfsRoot string
	Interval  time.Duration
	Debounce  time.Duration
	Now       func() time.Time
	Scan      func(sysfsRoot string) Snapshot
	Logger    *slog.Logger
	OnEvent   func(Transition)
}

// Watcher polls mains adapters and emits transitions after debounce.
type Watcher struct {
	sysfsRoot string
	interval  time.Duration
	debounce  time.Duration
	now       func() time.Time
	scan      func(string) Snapshot
	log       *slog.Logger
	onEvent   func(Transition)

	mu           sync.RWMutex
	running      bool
	baselined    bool
	committed    Source
	pending      Source
	pendingSince time.Time
	lastPoll     time.Time
	lastSnap     Snapshot
	lastOffAC    time.Time
}

func NewWatcher(opts Options) *Watcher {
	if opts.Interval <= 0 {
		opts.Interval = DefaultPollInterval
	}
	if opts.Debounce <= 0 {
		opts.Debounce = DefaultDebounce
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	if opts.Scan == nil {
		opts.Scan = Scan
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Watcher{
		sysfsRoot: opts.SysfsRoot,
		interval:  opts.Interval,
		debounce:  opts.Debounce,
		now:       opts.Now,
		scan:      opts.Scan,
		log:       opts.Logger,
		onEvent:   opts.OnEvent,
	}
}

func (w *Watcher) Interval() time.Duration { return w.interval }
func (w *Watcher) Debounce() time.Duration { return w.debounce }

func (w *Watcher) Snapshot() Snapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.lastSnap.Timestamp.IsZero() {
		return w.lastSnap
	}
	return w.scan(w.sysfsRoot)
}

type WatcherStatus struct {
	Running          bool      `json:"running"`
	IntervalSeconds  float64   `json:"interval_seconds"`
	DebounceSeconds  float64   `json:"debounce_seconds"`
	LastPoll         time.Time `json:"last_poll,omitempty"`
	BaselineRecorded bool      `json:"baseline_recorded"`
	PendingSource    string    `json:"pending_source,omitempty"`
}

func (w *Watcher) Status() WatcherStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	st := WatcherStatus{
		Running:          w.running,
		IntervalSeconds:  w.interval.Seconds(),
		DebounceSeconds:  w.debounce.Seconds(),
		LastPoll:         w.lastPoll,
		BaselineRecorded: w.baselined,
	}
	if w.pending != "" && w.pending != w.committed {
		st.PendingSource = string(w.pending)
	}
	return st
}

// Run polls until ctx is cancelled. Notification delivery is handled by the
// process OnEvent hook, not by the watcher itself. It never executes shutdown
// or Docker commands.
func (w *Watcher) Run(ctx context.Context) {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	w.Tick()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Tick()
		}
	}
}

// Tick reads adapters once and maybe emits a debounced transition.
func (w *Watcher) Tick() *Transition {
	now := w.now()
	snap := w.scan(w.sysfsRoot)
	snap.Timestamp = now

	w.mu.Lock()
	tr := w.evaluateLocked(now, snap)
	w.mu.Unlock()

	if tr != nil && w.onEvent != nil {
		w.onEvent(*tr)
	}
	return tr
}

func (w *Watcher) evaluateLocked(now time.Time, snap Snapshot) *Transition {
	w.lastPoll = now
	w.lastSnap = snap
	src := snap.Source

	if !w.baselined {
		w.committed = src
		w.baselined = true
		w.pending = ""
		if src != SourceAC {
			w.lastOffAC = now
		}
		w.log.Info("power watcher baseline", "source", string(src), "adapters", len(snap.Adapters))
		return nil
	}

	if src == w.committed {
		w.pending = ""
		w.pendingSince = time.Time{}
		return nil
	}

	if w.pending != src {
		w.pending = src
		w.pendingSince = now
		if w.debounce > 0 {
			return nil
		}
	}

	if w.debounce > 0 && now.Sub(w.pendingSince) < w.debounce {
		return nil
	}

	tr := &Transition{
		Type:     src.EventType(),
		At:       now,
		Source:   DetectionSource,
		From:     w.committed,
		To:       src,
		Snapshot: snap,
	}
	if w.committed != SourceAC && src == SourceAC && !w.lastOffAC.IsZero() {
		ms := now.Sub(w.lastOffAC).Milliseconds()
		if ms < 0 {
			ms = 0
		}
		tr.DurationMs = &ms
	}
	if src != SourceAC {
		w.lastOffAC = now
	}

	w.log.Info("power event",
		"type", tr.Type,
		"from", string(tr.From),
		"to", string(tr.To),
		"source", tr.Source,
	)
	w.committed = src
	w.pending = ""
	w.pendingSince = time.Time{}
	return tr
}
