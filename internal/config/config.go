package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppName = "lapguard"
	Version = "0.2.0"

	DefaultListen          = "127.0.0.1:8585"
	DefaultProvider        = "auto"
	DefaultSysfsRoot       = "/sys/class/power_supply"
	DefaultWebDir          = "web/dist"
	DefaultThresholdMethod = "auto"
)

// Config holds process-wide settings. Flags overlay a JSON file when present.
type Config struct {
	Listen          string `json:"listen"`
	Provider        string `json:"provider"`
	SysfsRoot       string `json:"sysfs_root"`
	BatteryName     string `json:"battery"`
	WebDir          string `json:"web_dir"`
	LogJSON         bool   `json:"log_json"`
	ThresholdMethod string `json:"threshold_method"`
	ConfigPath      string `json:"-"`

	setFlags       map[string]bool `json:"-"`
	writeIfMissing bool            `json:"-"`
}

func defaults() Config {
	return Config{
		Listen:          DefaultListen,
		Provider:        DefaultProvider,
		SysfsRoot:       DefaultSysfsRoot,
		WebDir:          DefaultWebDir,
		ThresholdMethod: DefaultThresholdMethod,
		setFlags:        map[string]bool{},
	}
}

func Parse(args []string) (Config, error) {
	parsed, err := parseFlags(args, defaults())
	if err != nil {
		return Config{}, err
	}
	return parsed, nil
}

// Load parses flags, then overlays ~/.config/lapguard/config.json unless
// -config pointed at another file. A missing file is not an error: the first
// successful start writes one so settings persist.
func Load(args []string) (Config, error) {
	cfg, err := Parse(args)
	if err != nil {
		return Config{}, err
	}
	return ApplyPersistentConfig(cfg)
}

func ApplyPersistentConfig(cfg Config) (Config, error) {
	path := cfg.ConfigPath
	explicit := cfg.flagSet("config")
	if path == "" && !explicit {
		if def, err := DefaultPath(); err == nil {
			path = def
			cfg.ConfigPath = path
		}
	}
	if path == "" {
		return cfg, nil
	}

	file, err := LoadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg.writeIfMissing = true
			return cfg, nil
		}
		return Config{}, err
	}

	merged := overlayFlags(file, cfg)
	merged.ConfigPath = path
	merged.setFlags = cfg.setFlags
	return merged, nil
}

func (c Config) flagSet(name string) bool {
	if c.setFlags == nil {
		return false
	}
	return c.setFlags[name]
}

func (c Config) ShouldWrite() bool { return c.writeIfMissing && c.ConfigPath != "" }

func overlayFlags(file, flags Config) Config {
	out := file
	if out.Listen == "" {
		out.Listen = DefaultListen
	}
	if out.Provider == "" {
		out.Provider = DefaultProvider
	}
	if out.SysfsRoot == "" {
		out.SysfsRoot = DefaultSysfsRoot
	}
	if out.WebDir == "" {
		out.WebDir = DefaultWebDir
	}
	if out.ThresholdMethod == "" {
		out.ThresholdMethod = DefaultThresholdMethod
	}
	if flags.flagSet("listen") {
		out.Listen = flags.Listen
	}
	if flags.flagSet("provider") {
		out.Provider = flags.Provider
	}
	if flags.flagSet("sysfs-root") {
		out.SysfsRoot = flags.SysfsRoot
	}
	if flags.flagSet("battery") {
		out.BatteryName = flags.BatteryName
	}
	if flags.flagSet("web-dir") {
		out.WebDir = flags.WebDir
	}
	if flags.flagSet("log-json") {
		out.LogJSON = flags.LogJSON
	}
	if flags.flagSet("threshold-method") {
		out.ThresholdMethod = flags.ThresholdMethod
	}
	return out
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, AppName, "config.json"), nil
}

func LoadFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := defaults()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.normalize(); err != nil {
		return Config{}, err
	}
	cfg.ConfigPath = path
	return cfg, nil
}

func (c Config) Save(path string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	c.ConfigPath = ""
	raw, err := json.MarshalIndent(persistDTO{
		Listen:          c.Listen,
		Provider:        c.Provider,
		SysfsRoot:       c.SysfsRoot,
		BatteryName:     c.BatteryName,
		WebDir:          c.WebDir,
		LogJSON:         c.LogJSON,
		ThresholdMethod: c.ThresholdMethod,
	}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

type persistDTO struct {
	Listen          string `json:"listen"`
	Provider        string `json:"provider"`
	SysfsRoot       string `json:"sysfs_root"`
	BatteryName     string `json:"battery"`
	WebDir          string `json:"web_dir"`
	LogJSON         bool   `json:"log_json"`
	ThresholdMethod string `json:"threshold_method"`
}

func (c *Config) normalize() error {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	switch c.Provider {
	case "auto", "sysfs", "mock":
	default:
		return fmt.Errorf("unknown provider %q (want auto, sysfs, or mock)", c.Provider)
	}

	c.ThresholdMethod = strings.ToLower(strings.TrimSpace(c.ThresholdMethod))
	if c.ThresholdMethod == "" {
		c.ThresholdMethod = DefaultThresholdMethod
	}
	switch c.ThresholdMethod {
	case "auto", "sysfs", "tlp", "none":
	default:
		return fmt.Errorf("unknown threshold-method %q (want auto, sysfs, tlp, or none)", c.ThresholdMethod)
	}
	return nil
}

// ResolveThresholdMethod returns the write method to use. "auto" keeps the
// discovered method. An explicit method is honoured only if discovery still
// supports it (or the user chose none).
func ResolveThresholdMethod(configured, detected string) (method string, warning string) {
	configured = strings.ToLower(strings.TrimSpace(configured))
	if configured == "" || configured == "auto" {
		return detected, ""
	}
	if configured == "none" {
		return "none", ""
	}
	if configured == detected {
		return detected, ""
	}
	if detected == "none" {
		return "none", fmt.Sprintf("configured threshold method %q is unavailable; hardware reports none", configured)
	}
	if configured == "sysfs" && detected != "sysfs" {
		return detected, fmt.Sprintf("configured sysfs thresholds are unavailable; using %s", detected)
	}
	if configured == "tlp" && detected == "sysfs" {
		return "tlp", ""
	}
	if configured == "tlp" && detected != "tlp" {
		return detected, fmt.Sprintf("configured TLP thresholds are unavailable; using %s", detected)
	}
	return detected, ""
}

func parseFlags(args []string, cfg Config) (Config, error) {
	if cfg.setFlags == nil {
		cfg.setFlags = map[string]bool{}
	}

	fs := flag.NewFlagSet(AppName, flag.ContinueOnError)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "HTTP bind address (loopback by default)")
	fs.StringVar(&cfg.Provider, "provider", cfg.Provider, "battery provider: auto, sysfs, or mock")
	fs.StringVar(&cfg.SysfsRoot, "sysfs-root", cfg.SysfsRoot, "power_supply sysfs root (overridable for tests and fixtures)")
	fs.StringVar(&cfg.BatteryName, "battery", cfg.BatteryName, "battery name under sysfs-root (empty = auto-detect BAT*)")
	fs.StringVar(&cfg.WebDir, "web-dir", cfg.WebDir, "directory of built Svelte assets; empty disables static serving")
	fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "JSON config file (default: ~/.config/lapguard/config.json)")
	fs.StringVar(&cfg.ThresholdMethod, "threshold-method", cfg.ThresholdMethod, "charge threshold method: auto, sysfs, tlp, or none")
	fs.BoolVar(&cfg.LogJSON, "log-json", cfg.LogJSON, "write logs as JSON")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	fs.Visit(func(f *flag.Flag) {
		cfg.setFlags[f.Name] = true
	})
	if err := cfg.normalize(); err != nil {
		return Config{}, err
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
