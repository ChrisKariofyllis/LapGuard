package thresholds

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"lapguard/internal/discovery"
)

// Controller applies charge start/stop limits using the method discovery chose.
type Controller struct {
	method      string
	startPath   string
	endPath     string
	batteryName string
	runner      discovery.Runner
}

func New(plan discovery.ThresholdPlan, runner discovery.Runner) *Controller {
	if runner == nil {
		runner = discovery.ExecRunner()
	}
	return &Controller{
		method:      plan.Method,
		startPath:   plan.StartPath,
		endPath:     plan.EndPath,
		batteryName: plan.BatteryName,
		runner:      runner,
	}
}

func (c *Controller) Method() string {
	if c == nil || c.method == "" {
		return discovery.MethodNone
	}
	return c.method
}

func (c *Controller) Get(ctx context.Context) (start, end int, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	switch c.Method() {
	case discovery.MethodSysfs:
		s, err1 := readIntFile(c.startPath)
		e, err2 := readIntFile(c.endPath)
		if c.startPath != "" && err1 != nil {
			return 0, 0, err1
		}
		if c.endPath != "" && err2 != nil {
			return 0, 0, err2
		}
		return s, e, nil
	case discovery.MethodTLP, discovery.MethodNone:
		return 0, 0, fmt.Errorf("charge thresholds method %s does not support reading via LapGuard", c.Method())
	default:
		return 0, 0, fmt.Errorf("unknown threshold method %q", c.Method())
	}
}

func (c *Controller) Set(ctx context.Context, start, end int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if start < 0 || end < 0 || start > 100 || end > 100 {
		return fmt.Errorf("thresholds must be 0–100, got start=%d end=%d", start, end)
	}
	if start > 0 && end > 0 && start >= end {
		return fmt.Errorf("start threshold (%d) must be below end threshold (%d)", start, end)
	}

	switch c.Method() {
	case discovery.MethodSysfs:
		return c.setSysfs(start, end)
	case discovery.MethodTLP:
		return c.setTLP(start, end)
	case discovery.MethodNone:
		return fmt.Errorf("charge thresholds are not supported on this hardware")
	default:
		return fmt.Errorf("unknown threshold method %q", c.Method())
	}
}

func (c *Controller) setSysfs(start, end int) error {
	// Write start first when both exist: many drivers reject start >= end.
	if c.startPath != "" {
		if err := os.WriteFile(c.startPath, []byte(strconv.Itoa(start)+"\n"), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", c.startPath, err)
		}
	}
	if c.endPath != "" {
		if err := os.WriteFile(c.endPath, []byte(strconv.Itoa(end)+"\n"), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", c.endPath, err)
		}
	}
	if c.startPath == "" && c.endPath == "" {
		return fmt.Errorf("sysfs threshold paths are missing")
	}
	return nil
}

func (c *Controller) setTLP(start, end int) error {
	args := []string{"setcharge", strconv.Itoa(start), strconv.Itoa(end)}
	if strings.TrimSpace(c.batteryName) != "" {
		args = append(args, c.batteryName)
	}
	out, err := c.runner.CombinedOutput("tlp", args...)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("tlp setcharge: %s: %w", msg, err)
		}
		return fmt.Errorf("tlp setcharge: %w", err)
	}
	return nil
}

func readIntFile(path string) (int, error) {
	if path == "" {
		return 0, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	return n, nil
}
