package battery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type OpenOptions struct {
	Kind        string
	SysfsRoot   string
	BatteryName string
	Logger      *slog.Logger
}

// Open selects a provider. "auto" tries sysfs and falls back to mock when no
// battery is present — that is the development default on the ProDesk.
func Open(ctx context.Context, opts OpenOptions) (Provider, error) {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}

	kind := strings.ToLower(strings.TrimSpace(opts.Kind))
	if kind == "" {
		kind = "auto"
	}

	switch kind {
	case "mock":
		log.Info("using mock battery provider")
		return NewMockProvider(), nil
	case "sysfs":
		p := NewSysfsProvider(opts.SysfsRoot, opts.BatteryName)
		log.Info("using sysfs battery provider", "root", opts.SysfsRoot, "battery", opts.BatteryName)
		return p, nil
	case "auto":
		p := NewSysfsProvider(opts.SysfsRoot, opts.BatteryName)
		probe, err := p.Probe(ctx)
		if err != nil {
			return nil, fmt.Errorf("auto-detect sysfs battery: %w", err)
		}
		if probe.BatteryPresent {
			log.Info("auto-detected sysfs battery", "name", probe.BatteryName, "root", opts.SysfsRoot)
			return p, nil
		}
		log.Info("no sysfs battery found; falling back to mock provider", "root", opts.SysfsRoot)
		return NewMockProvider(), nil
	default:
		return nil, fmt.Errorf("unknown battery provider %q", opts.Kind)
	}
}
