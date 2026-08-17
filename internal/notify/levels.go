package notify

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

type LevelReading struct {
	Present     bool
	Percent     int
	PercentOK   bool
	Discharging bool
}

type LevelOptions struct {
	Interval   time.Duration
	Read       func(ctx context.Context) (LevelReading, error)
	Thresholds func() (warning, critical int)
	Send       func(ctx context.Context, event NotificationEvent) error
	Logger     *slog.Logger
	Now        func() time.Time
}

// WatchLevels polls battery capacity and emits warning/critical events once per
// discharge cycle. It never triggers host shutdown.
func WatchLevels(ctx context.Context, opts LevelOptions) {
	if opts.Read == nil || opts.Send == nil || opts.Thresholds == nil {
		return
	}
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	var mon levelMonitor
	mon.tick(ctx, opts)
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mon.tick(ctx, opts)
		}
	}
}

type levelMonitor struct {
	warningFired  bool
	criticalFired bool
}

func (m *levelMonitor) tick(ctx context.Context, opts LevelOptions) {
	reading, err := opts.Read(ctx)
	if err != nil || !reading.Present || !reading.PercentOK {
		return
	}
	if !reading.Discharging {
		m.warningFired = false
		m.criticalFired = false
		return
	}
	warning, critical := opts.Thresholds()
	if critical < 0 {
		critical = 0
	}
	if warning < critical {
		warning = critical
	}

	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now()
	}
	switch {
	case reading.Percent <= critical:
		if m.criticalFired {
			return
		}
		m.criticalFired = true
		m.warningFired = true
		_ = opts.Send(ctx, NotificationEvent{
			Type:      EventBatteryCritical,
			Title:     "LapGuard: battery critical",
			Message:   "Battery charge is at " + strconv.Itoa(reading.Percent) + "%.",
			Timestamp: now,
		})
	case reading.Percent <= warning:
		if m.warningFired {
			return
		}
		m.warningFired = true
		_ = opts.Send(ctx, NotificationEvent{
			Type:      EventBatteryWarning,
			Title:     "LapGuard: battery warning",
			Message:   "Battery charge is at " + strconv.Itoa(reading.Percent) + "%.",
			Timestamp: now,
		})
	default:
		m.warningFired = false
		m.criticalFired = false
	}
}

func IsDischarging(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "discharging")
}
