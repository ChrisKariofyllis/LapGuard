package actions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"lapguard/internal/safety"
)

func (e *RealExecutor) StopDocker(ctx context.Context) error {
	docker, err := e.dockerBin()
	if err != nil {
		return err
	}
	timeout := e.DockerTO
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	out, err := e.run(ctx, timeout, docker, "ps", "-q")
	if err != nil {
		return wrapCmdErr("docker ps", out, err)
	}
	ids := parseContainerIDs(out)
	for _, id := range ids {
		if out, err := e.run(ctx, timeout, docker, "stop", id); err != nil {
			return wrapCmdErr("docker stop", out, err)
		}
	}
	return nil
}

func (e *RealExecutor) Sync(ctx context.Context) error {
	syncBin, err := e.syncBin()
	if err != nil {
		return err
	}
	timeout := e.SyncTO
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	out, err := e.run(ctx, timeout, syncBin)
	return wrapCmdErr("sync", out, err)
}

func (e *RealExecutor) PowerOff(ctx context.Context) error {
	bin, args, err := e.powerOffArgv()
	if err != nil {
		return err
	}
	timeout := e.PowerOffTO
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	out, err := e.run(ctx, timeout, bin, args...)
	return wrapCmdErr("poweroff", out, err)
}

func wrapCmdErr(op string, out []byte, err error) error {
	if err == nil {
		return nil
	}
	_ = redactOutput(out)
	return fmt.Errorf("%s: %w", op, err)
}

func (e *RealExecutor) dockerBin() (string, error) {
	if e.DockerPath != "" {
		return e.DockerPath, validateArgv(e.DockerPath, []string{"ps", "-q"})
	}
	return e.look("docker")
}

func (e *RealExecutor) syncBin() (string, error) {
	if e.SyncPath != "" {
		return e.SyncPath, validateArgv(e.SyncPath, nil)
	}
	return e.look("sync")
}

func (e *RealExecutor) powerOffArgv() (string, []string, error) {
	if e.PowerOffPath != "" {
		switch baseName(e.PowerOffPath) {
		case "systemctl":
			return e.PowerOffPath, []string{"poweroff"}, validateArgv(e.PowerOffPath, []string{"poweroff"})
		case "poweroff":
			return e.PowerOffPath, nil, validateArgv(e.PowerOffPath, nil)
		default:
			return "", nil, ErrUnsafeArgs
		}
	}
	if path, err := e.look("systemctl"); err == nil {
		return path, []string{"poweroff"}, nil
	}
	if path, err := e.look("poweroff"); err == nil {
		return path, nil, nil
	}
	return "", nil, fmt.Errorf("poweroff executable not found")
}

func baseName(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[i+1:]
}

func parseContainerIDs(out []byte) []string {
	var ids []string
	for _, line := range strings.Split(string(out), "\n") {
		id := strings.TrimSpace(line)
		if id == "" {
			continue
		}
		if !validContainerID(id) {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// Compile-time check.
var _ safety.ActionExecutor = (*RealExecutor)(nil)
