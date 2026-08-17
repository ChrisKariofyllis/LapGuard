package config

import (
	"flag"
	"fmt"
	"net"
	"strings"
)

const (
	AppName = "lapguard"
	Version = "0.1.0"

	DefaultListen    = "127.0.0.1:8585"
	DefaultProvider  = "auto"
	DefaultSysfsRoot = "/sys/class/power_supply"
	DefaultWebDir    = "web/dist"
)

// Config holds process-wide settings. Flags are enough for milestone 1;
// a config file can be added later without changing call sites.
type Config struct {
	Listen      string
	Provider    string
	SysfsRoot   string
	BatteryName string
	WebDir      string
	LogJSON     bool
}

func Parse(args []string) (Config, error) {
	cfg := Config{
		Listen:    DefaultListen,
		Provider:  DefaultProvider,
		SysfsRoot: DefaultSysfsRoot,
		WebDir:    DefaultWebDir,
	}

	fs := flag.NewFlagSet(AppName, flag.ContinueOnError)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "HTTP bind address (loopback by default)")
	fs.StringVar(&cfg.Provider, "provider", cfg.Provider, "battery provider: auto, sysfs, or mock")
	fs.StringVar(&cfg.SysfsRoot, "sysfs-root", cfg.SysfsRoot, "power_supply sysfs root (overridable for tests and fixtures)")
	fs.StringVar(&cfg.BatteryName, "battery", cfg.BatteryName, "battery name under sysfs-root (empty = auto-detect BAT*)")
	fs.StringVar(&cfg.WebDir, "web-dir", cfg.WebDir, "directory of built Svelte assets; empty disables static serving")
	fs.BoolVar(&cfg.LogJSON, "log-json", false, "write logs as JSON")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch cfg.Provider {
	case "auto", "sysfs", "mock":
	default:
		return Config{}, fmt.Errorf("unknown provider %q (want auto, sysfs, or mock)", cfg.Provider)
	}

	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		return Config{}, fmt.Errorf("invalid listen address %q: %w", cfg.Listen, err)
	}

	return cfg, nil
}

func (c Config) Loopback() bool {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
