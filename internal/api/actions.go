package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"lapguard/internal/actions"
	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/power"
	"lapguard/internal/safety"
	"lapguard/internal/storage"
)

const (
	errRealDisabled     = "real actions are disabled"
	errDryRun           = "safety.dry_run is enabled"
	errConfirm          = "confirmation required"
	errACConnected      = "refusing action while AC is connected"
	errACUnknown        = "refusing action while AC state is unknown"
	errNotDischarging   = "refusing action while battery is not discharging"
	errCooldown         = "action cooldown active"
	errInFlight         = "an action is already in progress"
	errDuplicateKey     = "duplicate idempotency key"
	errExecutorUnavail  = "action executor is unavailable"
	errBadIdempotency   = "invalid idempotency key"
	maxIdempotencyBytes = 128
	executorRecording   = "recording"
	executorReal        = "real"
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

type actionStatusResponse struct {
	RealEnabled       bool                     `json:"real_enabled"`
	SafetyDryRun      bool                     `json:"safety_dry_run"`
	RequireACLoss     bool                     `json:"require_ac_loss"`
	ACState           string                   `json:"ac_state"`
	BatteryStatus     string                   `json:"battery_status"`
	BatteryPercent    *int                     `json:"battery_percent"`
	Discharging       bool                     `json:"discharging"`
	Cooldown          cooldownStatus           `json:"cooldown"`
	Executor          string                   `json:"executor"`
	CommandsExecuted  bool                     `json:"commands_executed"`
	Ready             bool                     `json:"ready"`
	Gates             []string                 `json:"gates"`
	Warnings          []string                 `json:"warnings"`
	AutomaticShutdown bool                     `json:"automatic_shutdown_executed"`
	Plan              []string                 `json:"plan"`
	Config            config.ConfigRuntimeView `json:"config"`
	RestartRequired   string                   `json:"restart_required_for_disk_edits,omitempty"`
}

type actionPreflightResponse struct {
	RealEnabled       bool                     `json:"real_enabled"`
	SafetyDryRun      bool                     `json:"safety_dry_run"`
	Executor          string                   `json:"executor"`
	ACState           string                   `json:"ac_state"`
	BatteryStatus     string                   `json:"battery_status"`
	BatteryPercent    *int                     `json:"battery_percent"`
	Discharging       bool                     `json:"discharging"`
	Ready             bool                     `json:"ready"`
	Gates             []string                 `json:"gates"`
	CommandsExecuted  bool                     `json:"commands_executed"`
	AutomaticShutdown bool                     `json:"automatic_shutdown_executed"`
	Config            config.ConfigRuntimeView `json:"config"`
	Explanation       string                   `json:"explanation"`
}

type cooldownStatus struct {
	Active           bool `json:"active"`
	InProgress       bool `json:"in_progress"`
	SecondsRemaining int  `json:"seconds_remaining"`
}

type actionGuard struct {
	mu         sync.Mutex
	inflight   bool
	last       time.Time
	lastKey    string
	pendingKey string
}

func (g *actionGuard) begin(cooldown time.Duration, now time.Time, keyHash string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inflight {
		if keyHash != "" && keyHash == g.pendingKey {
			return errors.New(errDuplicateKey)
		}
		return errors.New(errInFlight)
	}
	inCooldown := cooldown > 0 && !g.last.IsZero() && now.Before(g.last.Add(cooldown))
	if inCooldown {
		if keyHash != "" && keyHash == g.lastKey {
			return errors.New(errDuplicateKey)
		}
		return errors.New(errCooldown)
	}
	g.inflight = true
	g.pendingKey = keyHash
	return nil
}

func (g *actionGuard) end(now time.Time, ran bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inflight = false
	if ran {
		g.last = now
		g.lastKey = g.pendingKey
	}
	g.pendingKey = ""
}

func (g *actionGuard) snapshot(now time.Time, cooldown time.Duration) cooldownStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := cooldownStatus{InProgress: g.inflight}
	if cooldown > 0 && !g.last.IsZero() {
		until := g.last.Add(cooldown)
		if now.Before(until) {
			out.Active = true
			sec := int(until.Sub(now).Seconds())
			if sec < 1 {
				sec = 1
			}
			out.SecondsRemaining = sec
		}
	}
	return out
}

func (s *Server) handleActionStatus(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.actionStatus())
}

func (s *Server) handleActionPreflight(w http.ResponseWriter, _ *http.Request) {
	st := s.actionStatus()
	s.writeJSON(w, http.StatusOK, actionPreflightResponse{
		RealEnabled:       st.RealEnabled,
		SafetyDryRun:      st.SafetyDryRun,
		Executor:          st.Executor,
		ACState:           st.ACState,
		BatteryStatus:     st.BatteryStatus,
		BatteryPercent:    st.BatteryPercent,
		Discharging:       st.Discharging,
		Ready:             st.Ready,
		Gates:             st.Gates,
		CommandsExecuted:  false,
		AutomaticShutdown: false,
		Config:            st.Config,
		Explanation:       config.DiskEditRestartMessage,
	})
}

func (s *Server) actionStatus() actionStatusResponse {
	cfg := s.currentConfig()
	src := s.currentPowerSource()
	bat := s.currentBattery()
	now := s.nowUTC()
	cool := s.guard.snapshot(now, time.Duration(cfg.Actions.CooldownSeconds)*time.Second)
	gates := liveActionGates(cfg, src, bat, cool)
	return actionStatusResponse{
		RealEnabled:       cfg.Actions.RealEnabled,
		SafetyDryRun:      cfg.Safety.DryRun,
		RequireACLoss:     cfg.Safety.RequireACLoss,
		ACState:           string(src),
		BatteryStatus:     bat.status,
		BatteryPercent:    bat.percent,
		Discharging:       bat.discharging,
		Cooldown:          cool,
		Executor:          s.executorKind(cfg),
		CommandsExecuted:  false,
		Ready:             len(gates) == 0 && cfg.ManualActionsReady() && src == power.SourceBattery && bat.discharging && !cool.Active && !cool.InProgress,
		Gates:             gates,
		Warnings:          actionWarnings(cfg, gates),
		AutomaticShutdown: false,
		Plan:              cfg.IntendedPlan(),
		Config:            cfg.RuntimeView(),
		RestartRequired:   config.ConfigReloadRestartRequired,
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
	s.runManualAction(w, r, config.ConfirmPowerOff, storage.AuditPowerOffAttempt, storage.AuditPowerOffResult, func(ctx context.Context, exec safety.ActionExecutor, cfg config.Config) error {
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
	s.runManualAction(w, r, config.ConfirmStopDocker, storage.AuditDockerAttempt, storage.AuditDockerResult, func(ctx context.Context, exec safety.ActionExecutor, _ config.Config) error {
		return exec.StopDocker(ctx)
	})
}

func (s *Server) runManualAction(w http.ResponseWriter, r *http.Request, wantConfirm, attemptType, resultType string, run func(context.Context, safety.ActionExecutor, config.Config) error) {
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
	keyHash, err := idempotencyHash(r.Header.Get("Idempotency-Key"))
	if err != nil {
		resp.Error = errBadIdempotency
		s.writeJSON(w, http.StatusBadRequest, resp)
		return
	}

	if blocked, msg := safetyGate(cfg, s.currentPowerSource(), s.currentBattery()); blocked {
		resp.Error = msg
		s.writeJSON(w, http.StatusConflict, resp)
		return
	}

	now := s.nowUTC()
	cooldown := time.Duration(cfg.Actions.CooldownSeconds) * time.Second
	if err := s.guard.begin(cooldown, now, keyHash); err != nil {
		resp.Error = err.Error()
		s.writeJSON(w, http.StatusConflict, resp)
		return
	}
	ran := false
	defer func() { s.guard.end(now, ran) }()

	s.audit(r, attemptType, false, "attempt")
	exec := s.manualExecutor()
	if exec == nil {
		s.audit(r, resultType, false, "unavailable")
		resp.Error = errExecutorUnavail
		s.writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	timeout := manualTimeout(cfg, wantConfirm)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	err = run(ctx, exec, cfg)
	if err != nil {
		s.audit(r, resultType, false, "failed")
		s.log.Error("manual action failed")
		resp.Error = errExecutorUnavail
		s.writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	ran = true
	s.audit(r, resultType, true, "ok")
	resp.OK = true
	resp.CommandsExecuted = true
	s.writeJSON(w, http.StatusOK, resp)
}

func safetyGate(cfg config.Config, src power.Source, bat batteryReading) (bool, string) {
	if !cfg.Actions.RealEnabled {
		return true, errRealDisabled
	}
	if cfg.Safety.DryRun {
		return true, errDryRun
	}
	switch src {
	case power.SourceAC:
		return true, errACConnected
	case power.SourceUnknown, "":
		return true, errACUnknown
	}
	if !bat.discharging {
		return true, errNotDischarging
	}
	return false, ""
}

func liveActionGates(cfg config.Config, src power.Source, bat batteryReading, cool cooldownStatus) []string {
	gates := append([]string{}, cfg.ActionGates()...)
	switch src {
	case power.SourceAC:
		gates = append(gates, "ac_connected")
	case power.SourceUnknown, "":
		gates = append(gates, "ac_unknown")
	}
	if !bat.discharging {
		gates = append(gates, "battery_not_discharging")
	}
	if cool.Active {
		gates = append(gates, "cooldown_active")
	}
	if cool.InProgress {
		gates = append(gates, "action_in_progress")
	}
	return gates
}

func actionWarnings(cfg config.Config, gates []string) []string {
	out := []string{
		"Real actions are experimental and are not safe for production yet.",
		"Automatic low-battery shutdown is not implemented.",
		"Do not enable real actions on an important machine.",
		config.DiskEditRestartMessage,
	}
	for _, g := range gates {
		switch g {
		case "real_actions_disabled":
			out = append(out, "Real actions are disabled.")
		case "safety_dry_run":
			out = append(out, "safety.dry_run is enabled; host commands will not run.")
		case "ac_connected":
			out = append(out, "AC is connected.")
		case "ac_unknown":
			out = append(out, "AC state is unknown.")
		case "battery_not_discharging":
			out = append(out, "Battery is not discharging.")
		}
	}
	return out
}

type batteryReading struct {
	status      string
	percent     *int
	discharging bool
}

func (s *Server) currentBattery() batteryReading {
	s.mu.RLock()
	override := s.testBattery
	provider := s.provider
	s.mu.RUnlock()
	if override != nil {
		return *override
	}
	if provider == nil {
		return batteryReading{}
	}
	snap, err := provider.Snapshot(context.Background())
	if err != nil {
		return batteryReading{}
	}
	status := strings.TrimSpace(snap.Battery.Status)
	return batteryReading{
		status:      status,
		percent:     snap.Battery.CapacityPercent,
		discharging: notify.IsDischarging(status),
	}
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

func (s *Server) nowUTC() time.Time {
	s.mu.RLock()
	fn := s.nowFn
	s.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return time.Now().UTC()
}

func (s *Server) executorKind(cfg config.Config) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.testActor != nil {
		return executorRecording
	}
	if cfg.ManualActionsReady() {
		return executorReal
	}
	return executorRecording
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

func idempotencyHash(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > maxIdempotencyBytes {
		return "", errors.New(errBadIdempotency)
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", errors.New(errBadIdempotency)
		}
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}
