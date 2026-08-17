package discovery

import (
	"os"
	"os/exec"
	"time"
)

const (
	defaultSysfsRoot     = "/sys/class/power_supply"
	defaultProcModules   = "/proc/modules"
	defaultPlatformRoot  = "/sys/devices/platform"
	defaultOSRelease     = "/etc/os-release"
	defaultKernelRelease = "/proc/sys/kernel/osrelease"
	defaultDockerSocket  = "/var/run/docker.sock"
)

// Runner executes optional helper binaries (tlp, tlp-stat). Tests inject fakes.
type Runner interface {
	LookPath(file string) (string, error)
	CombinedOutput(name string, args ...string) ([]byte, error)
}

// Options configure discovery. Empty paths fall back to the live Linux locations.
type Options struct {
	SysfsRoot     string
	ProcModules   string
	PlatformRoot  string
	OSRelease     string
	KernelRelease string
	DockerSocket  string
	Hostname      string
	Now           func() time.Time
	Runner        Runner
}

func (o Options) withDefaults() Options {
	if o.SysfsRoot == "" {
		o.SysfsRoot = defaultSysfsRoot
	}
	if o.ProcModules == "" {
		o.ProcModules = defaultProcModules
	}
	if o.PlatformRoot == "" {
		o.PlatformRoot = defaultPlatformRoot
	}
	if o.OSRelease == "" {
		o.OSRelease = defaultOSRelease
	}
	if o.KernelRelease == "" {
		o.KernelRelease = defaultKernelRelease
	}
	if o.DockerSocket == "" {
		o.DockerSocket = defaultDockerSocket
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Runner == nil {
		o.Runner = ExecRunner()
	}
	return o
}

// ExecRunner runs LookPath and commands on the real host.
func ExecRunner() Runner { return execRunner{} }

type execRunner struct{}

func (execRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (execRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func readTrimmed(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return trimNL(string(raw)), nil
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
