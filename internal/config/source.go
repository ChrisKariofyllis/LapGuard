package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	ConfigSourceDefault = "default"
	ConfigSourceFile    = "file"
	ConfigSourceCLI     = "cli"

	ConfigReloadRestartRequired = "restart_required_for_disk_edits"

	// DiskEditRestartMessage is the operator-facing rule: the process does not
	// watch config.json. Disk edits apply on the next start.
	DiskEditRestartMessage = "Editing config.json on disk does not change the running process. Restart LapGuard after a disk edit, or change settings through PUT /api/v1/config."
)

// ConfigRuntimeView is the public, non-secret description of which config the
// process is using and how it is reloaded.
type ConfigRuntimeView struct {
	Source                     string `json:"source"`
	Path                       string `json:"path"`
	Reload                     string `json:"reload"`
	DiskEditsRequireRestart    bool   `json:"disk_edits_require_restart"`
	APIUpdatesApplyImmediately bool   `json:"api_updates_apply_immediately"`
}

func (c Config) ConfigSource() string {
	switch c.Source {
	case ConfigSourceFile, ConfigSourceCLI, ConfigSourceDefault:
		return c.Source
	default:
		return ConfigSourceDefault
	}
}

func (c Config) RuntimeView() ConfigRuntimeView {
	return ConfigRuntimeView{
		Source:                     c.ConfigSource(),
		Path:                       SafeDisplayPath(c.ConfigPath),
		Reload:                     ConfigReloadRestartRequired,
		DiskEditsRequireRestart:    true,
		APIUpdatesApplyImmediately: true,
	}
}

func (c Config) SafeConfigPath() string {
	return SafeDisplayPath(c.ConfigPath)
}

func (c Config) SafeEventsDBPath() string {
	return SafeDisplayPath(c.EventsDBPath())
}

// SafeDisplayPath redacts home directories and keeps only enough of an
// explicit path to identify the file. It never includes secrets.
func SafeDisplayPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if home, err := os.UserHomeDir(); err == nil {
		home = filepath.Clean(home)
		if home != "" && home != string(filepath.Separator) {
			if path == home {
				return "~"
			}
			prefix := home + string(filepath.Separator)
			if strings.HasPrefix(path, prefix) {
				return "~" + path[len(home):]
			}
		}
	}
	if path == "/etc/lapguard/config.json" || strings.HasPrefix(path, "/etc/lapguard/") {
		return path
	}
	base := filepath.Base(path)
	dir := filepath.Base(filepath.Dir(path))
	if dir == "." || dir == string(filepath.Separator) || dir == "" {
		return "…/" + base
	}
	return "…/" + dir + "/" + base
}

func (c Config) actionExecutorKind() string {
	if c.ManualActionsReady() {
		return "real"
	}
	return "recording"
}

// LogStartup writes a secret-free summary of action gates and config source.
func (c Config) LogStartup(log *slog.Logger) {
	if log == nil {
		return
	}
	gates := c.ActionGates()
	if gates == nil {
		gates = []string{}
	}
	log.Info("action configuration",
		"source", c.ConfigSource(),
		"path", c.SafeConfigPath(),
		"reload", ConfigReloadRestartRequired,
		"real_enabled", c.Actions.RealEnabled,
		"safety_dry_run", c.Safety.DryRun,
		"require_ac_loss", c.Safety.RequireACLoss,
		"docker_stop_enabled", c.Docker.StopEnabled,
		"auto_drain_enabled", c.AutoDrain.Enabled,
		"auth_enabled", c.Auth.Enabled,
		"allow_loopback_no_token", c.Auth.AllowLoopbackNoToken,
		"executor", c.actionExecutorKind(),
		"gates", gates,
	)
}
