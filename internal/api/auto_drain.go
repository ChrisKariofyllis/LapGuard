package api

import (
	"errors"
	"net/http"
	"strings"

	"lapguard/internal/config"
	"lapguard/internal/safety"
	"lapguard/internal/storage"
)

type autoDrainRespondRequest struct {
	Action string `json:"action"`
}

func (s *Server) handleGetAutoDrain(w http.ResponseWriter, _ *http.Request) {
	ad := s.AutoDrain()
	if ad == nil {
		cfg := s.currentConfig()
		s.writeJSON(w, http.StatusOK, safety.AutoDrainSnapshot{
			Enabled:                false,
			State:                  safety.AutoDrainIdle,
			ThresholdPercent:       cfg.AutoDrain.BatteryThresholdPercent,
			PreNotificationMinutes: cfg.AutoDrain.PreNotificationMinutes,
			ResponseTimeoutMinutes: cfg.AutoDrain.ResponseTimeoutMinutes,
			NotificationServices:   cfg.AutoDrain.NotificationServices,
			OnUserNo:               cfg.AutoDrain.OnUserNo,
			DryRun:                 cfg.Safety.DryRun,
			DockerStopEnabled:      cfg.Docker.StopEnabled,
			RealEnabled:            cfg.Actions.RealEnabled,
			CommandsExecuted:       false,
			Plan:                   []string{safety.ActionStopDocker, safety.ActionSync, safety.ActionPowerOff},
			Gates:                  []string{"auto_drain_uninitialized"},
			Reason:                 "auto-drain controller is not initialized",
			Message:                "Smart automatic drain is disabled",
		})
		return
	}
	s.writeJSON(w, http.StatusOK, ad.Snapshot())
}

func (s *Server) handlePutAutoDrainConfig(w http.ResponseWriter, r *http.Request) {
	var patch config.AutoDrainPatch
	if err := decodeObject(r, &patch); err != nil {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
		return
	}
	view, err := s.updateSettings(func(cfg *config.Config) error {
		next, err := cfg.AutoDrain.Apply(patch)
		if err != nil {
			return err
		}
		cfg.AutoDrain = next
		return nil
	})
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	if !view.AutoDrain.Enabled {
		if ad := s.AutoDrain(); ad != nil {
			ad.Reset("auto_drain disabled via API")
		}
	}
	s.log.Info("auto-drain config updated",
		"enabled", view.AutoDrain.Enabled,
		"threshold", view.AutoDrain.BatteryThresholdPercent,
		"response_timeout_min", view.AutoDrain.ResponseTimeoutMinutes,
	)
	s.audit(r, storage.AuditConfigUpdate, true, "auto-drain config updated")
	if ad := s.AutoDrain(); ad != nil {
		s.writeJSON(w, http.StatusOK, ad.Snapshot())
		return
	}
	s.writeJSON(w, http.StatusOK, view.APIView())
}

func (s *Server) handleAutoDrainRespond(w http.ResponseWriter, r *http.Request) {
	var req autoDrainRespondRequest
	if err := decodeObject(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
		return
	}
	ad := s.AutoDrain()
	if ad == nil {
		s.writeJSON(w, http.StatusConflict, map[string]any{
			"ok":    false,
			"error": "auto-drain controller is not initialized",
			"state": safety.AutoDrainIdle,
		})
		return
	}
	snap, err := ad.Respond(r.Context(), req.Action)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, safety.ErrNoPendingAutoDrain) {
			status = http.StatusConflict
		}
		s.writeJSON(w, status, map[string]any{
			"ok":      false,
			"error":   err.Error(),
			"state":   snap.State,
			"dry_run": snap.DryRun,
		})
		s.audit(r, storage.AuditAutoDrainRespond, false, err.Error())
		return
	}
	s.audit(r, storage.AuditAutoDrainRespond, true, "auto-drain "+strings.ToLower(strings.TrimSpace(req.Action)))
	s.writeJSON(w, http.StatusOK, snap)
}
