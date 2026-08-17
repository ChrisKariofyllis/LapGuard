package discovery

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"
)

// Reporter is the API-facing discovery cache.
type Reporter interface {
	Last() CapabilityReport
	Refresh(ctx context.Context) (CapabilityReport, error)
}

// Service runs discovery and remembers the last report.
type Service struct {
	mu   sync.RWMutex
	opts Options
	last CapabilityReport
}

func NewService(opts Options) *Service {
	return &Service{opts: opts.withDefaults()}
}

func (s *Service) Last() CapabilityReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.last
}

func (s *Service) Refresh(ctx context.Context) (CapabilityReport, error) {
	if err := ctx.Err(); err != nil {
		return CapabilityReport{}, err
	}
	report, err := Run(ctx, s.opts)
	if err != nil {
		return CapabilityReport{}, err
	}
	s.mu.Lock()
	s.last = report
	s.mu.Unlock()
	return report, nil
}

// Static is a fixed report, used by tests and the mock-laptop suite.
type Static struct {
	Report CapabilityReport
}

func (s Static) Last() CapabilityReport { return s.Report }

func (s Static) Refresh(context.Context) (CapabilityReport, error) {
	return s.Report, nil
}

// Run inspects sysfs, kernel modules and userspace tools. It never fails the
// process because a field is missing: unsupported hardware is recorded as
// method "none" with a why_not explanation.
func Run(ctx context.Context, opts Options) (CapabilityReport, error) {
	if err := ctx.Err(); err != nil {
		return CapabilityReport{}, err
	}
	opts = opts.withDefaults()

	hw := scanHardware(opts)
	loaded, details := detectModules(opts)
	tools := detectTools(opts)
	plan := detectThresholds(opts, hw, loaded, tools)

	docker := exists(opts.DockerSocket)
	if !docker {
		if path, err := opts.Runner.LookPath("docker"); err == nil && path != "" {
			docker = true
		}
	}

	report := CapabilityReport{
		Timestamp:        opts.Now().UTC(),
		Hostname:         hostname(opts),
		OS:               readOS(opts.OSRelease),
		Kernel:           readKernel(opts.KernelRelease),
		Battery:          hw.Primary,
		Batteries:        hw.Batteries,
		Adapters:         hw.Adapters,
		AvailableFields:  hw.AvailableFields,
		NamingConvention: hw.NamingConvention,
		PowerCalculation: hw.PowerCalculation,
		Features: Features{
			ChargeThresholds: plan.Method,
			CycleCount:       hw.HasCycleCount,
			PowerNow:         hw.HasPowerNow,
			CurrentVoltage:   hw.HasCurrent && hw.HasVoltage,
			Temperature:      hw.HasTemp,
			AlarmControl:     hw.HasAlarm,
			DockerShutdown:   docker,
		},
		AvailableTools: tools.Tools,
		KernelModules:  loaded,
		ModuleDetails:  details,
		Thresholds:     plan,
	}

	if moduleLoaded(loaded, "fujitsu_laptop") && plan.Method == MethodNone {
		report.Notes = append(report.Notes, "fujitsu_laptop loaded without charge_control sysfs — typical when the driver logs \"Unable to register battery charge control\".")
	}
	if report.AvailableFields == nil {
		report.AvailableFields = []string{}
	}
	if report.KernelModules == nil {
		report.KernelModules = []string{}
	}
	return report, nil
}

func hostname(opts Options) string {
	if opts.Hostname != "" {
		return opts.Hostname
	}
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

func readOS(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "linux"
	}
	defer f.Close()
	pretty := ""
	name := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.Trim(val, `"'`)
		switch key {
		case "PRETTY_NAME":
			pretty = val
		case "NAME":
			name = val
		}
	}
	if pretty != "" {
		return pretty
	}
	if name != "" {
		return name
	}
	return "linux"
}

func readKernel(path string) string {
	s, err := readTrimmed(path)
	if err != nil || s == "" {
		return ""
	}
	return s
}
