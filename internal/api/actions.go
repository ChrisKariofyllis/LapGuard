package api

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"lapguard/internal/actions"
	"lapguard/internal/config"
	"lapguard/internal/power"
	"lapguard/internal/safety"
	"lapguard/internal/storage"
)

const (
	errRealDisabled = "real actions are disabled"
	errDryRun       = "safety.dry_run is enabled"
	errConfirm      = "confirmation required"
	errACConnected  = "refusing poweroff while AC is connected"
	errACUnknown    = "refusing poweroff while AC state is unknown"
	errCooldown     = "action cooldown active"
	errInFlight     = "an action is already in progress"
)

type actionConfirm struct {
	Confirm string `json:"confirm"`
}

type actionResponse struct {
	OK                bool     `json:"ok"`
	DryRun            bool     `json:"dry_run"`
	RealEnabled       bool     `json:"real_enabled"`
	CommandsExecuted  bool     `json:"commands_executed"`
	Plan              []string `json:"plan"`
	Gates             []string `json:"gates,omitempty"`
	Error             string   `json:"error,omitempty"`
	Detail            string   `json:"detail,omitempty"`
	AutomaticShutdown bool     `json:"automatic_shutdown_executed"`
}

type actionGuard struct {
	mu       sync.Mutex
	inflight bool
	last     time.Time
}

func (g *actionGuard) begin(cooldown time.Duration, now time.Time) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inflight {
		return errors.New(errInFlight)
	}
	if cooldown > 0 && !g.last.IsZero() && now.Before(g.last.Add(cooldown)) {
		return errors.New(errCooldown)
	}
	g.inflight = true
	return nil
}

func (g *actionGuard) end(now time.Time, ran bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inflight = false
	if ran {
		g.last = now
	}
}

func (s *Server) handleActionPreview(w http.ResponseWriter, _ *http.Request) {
	cfg := s.currentConfig()
	s.writeJSON(w, http.StatusOK, actionResponse{
		OK:                true,
		DryRun:            cfg.Safety.DryRun,
		RealEnabled:       cfg.Actions.RealEnabled,
		CommandsExecuted:  false,
		Plan:              cfg.IntendedPlan(),
		Gates:             cfg.ActionGates(),
		AutomaticShutdown: false,
	})
}

func (s *Server) handleActionPowerOff(w http.ResponseWriter, r *http.Request) {
	s.runManualAction(w, r, config.ConfirmPowerOff, storage.AuditPowerOffAttempt, storage.AuditPowerOffResult, true, func(ctx context.Context, exec safety.ActionExecutor, cfg config.Config) error {
		if cfg.Docker.StopEnabled {
			if err := exec.StopDocker(ctx); err != nil {
				return err
			}
		}
		if err := exec.Sync(ctx); err != nil {
			return err
		}
		return exec.PowerOff(ctx)
	})
}

func (s *Server) handleActionDockerDrain(w http.ResponseWriter, r *http.Request) {
	s.runManualAction(w, r, config.ConfirmStopDocker, storage.AuditDockerAttempt, storage.AuditDockerResult, false, func(ctx context.Context, exec safety.ActionExecutor, _ config.Config) error {
		return exec.StopDocker(ctx)
	})
}

func (s *Server) runManualAction(w http.ResponseWriter, r *http.Request, wantConfirm, attemptType, resultType string, checkAC bool, run func(context.Context, safety.ActionExecutor, config.Config) error) {
	cfg := s.currentConfig()
	resp := actionResponse{
		DryRun:            cfg.Safety.DryRun,
		RealEnabled:       cfg.Actions.RealEnabled,
		CommandsExecuted:  false,
		Plan:              cfg.IntendedPlan(),
		Gates:             cfg.ActionGates(),
		AutomaticShutdown: false,
	}

	var req actionConfirm
	if err := decodeObject(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
		return
	}
	if req.Confirm != wantConfirm {
		resp.Error = errConfirm
		resp.Detail = "confirm must be " + wantConfirm
		s.writeJSON(w, http.StatusBadRequest, resp)
		return
	}
	if !cfg.Actions.RealEnabled {
		resp.Error = errRealDisabled
		s.writeJSON(w, http.StatusConflict, resp)
		return
	}
	if cfg.Safety.DryRun {
		resp.Error = errDryRun
		s.writeJSON(w, http.StatusConflict, resp)
		return
	}
	if checkAC {
		src := s.currentPowerSource()
		switch src {
		case power.SourceAC:
			resp.Error = errACConnected
			s.writeJSON(w, http.StatusConflict, resp)
			return
		case power.SourceUnknown, "":
			resp.Error = errACUnknown
			s.writeJSON(w, http.StatusConflict, resp)
			return
		}
	}

	now := time.Now().UTC()
	cooldown := time.Duration(cfg.Actions.CooldownSeconds) * time.Second
	if err := s.guard.begin(cooldown, now); err != nil {
		resp.Error = err.Error()
		s.writeJSON(w, http.StatusConflict, resp)
		return
	}
	ran := false
	defer func() { s.guard.end(now, ran) }()

	s.audit(r, attemptType, false, "attempt")
	exec := s.manualExecutor()
	timeout := manualTimeout(cfg, wantConfirm)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	err := run(ctx, exec, cfg)
	if err != nil {
		s.audit(r, resultType, false, "failed")
		s.log.Error("manual action failed", "err", err)
		resp.Error = "action failed"
		s.writeJSON(w, http.StatusBadGateway, resp)
		return
	}
	ran = true
	s.audit(r, resultType, true, "ok")
	resp.OK = true
	resp.CommandsExecuted = true
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) currentPowerSource() power.Source {
	s.mu.RLock()
	override := s.testSource
	w := s.watcher
	root := s.cfg.SysfsRoot
	s.mu.RUnlock()
	if override != nil {
		return *override
	}
	if w != nil {
		return w.Snapshot().Source
	}
	return power.Scan(root).Source
}

func (s *Server) manualExecutor() safety.ActionExecutor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.testActor != nil {
		return s.testActor
	}
	cfg := s.cfg
	if cfg.ManualActionsReady() {
		return s.liveRealLocked(cfg)
	}
	if s.rec != nil {
		return s.rec
	}
	return safety.NewRecordingExecutor()
}

func manualTimeout(cfg config.Config, confirm string) time.Duration {
	if confirm == config.ConfirmStopDocker {
		timeout := time.Duration(cfg.Docker.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			return 30 * time.Second
		}
		return timeout
	}
	timeout := time.Duration(cfg.Actions.PowerOffTimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timeout += 15 * time.Second
	if cfg.Docker.StopEnabled {
		dt := time.Duration(cfg.Docker.TimeoutSeconds) * time.Second
		if dt <= 0 {
			dt = 30 * time.Second
		}
		timeout += dt
	}
	return timeout
}

func (s *Server) liveRealLocked(cfg config.Config) *actions.RealExecutor {
	to := time.Duration(cfg.Docker.TimeoutSeconds) * time.Second
	po := time.Duration(cfg.Actions.PowerOffTimeoutSecs) * time.Second
	return &actions.RealExecutor{
		DockerPath:   cfg.Actions.DockerPath,
		PowerOffPath: cfg.Actions.PowerOffPath,
		SyncPath:     cfg.Actions.SyncPath,
		DockerTO:     to,
		PowerOffTO:   po,
		SyncTO:       15 * time.Second,
		Run:          s.realRun,
	}
}
