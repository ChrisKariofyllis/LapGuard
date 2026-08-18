package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/safety"
	"lapguard/internal/storage"
)

type safetyTestRequest struct {
	Scenario string `json:"scenario"`
}

func (s *Server) handleGetSafety(w http.ResponseWriter, _ *http.Request) {
	ctrl := s.Safety()
	if ctrl == nil {
		s.writeJSON(w, http.StatusOK, safety.Snapshot{
			State:            safety.StateUnknown,
			DryRun:           true,
			Message:          safety.DryRunMessage,
			IntendedActions:  []string{},
			CommandsExecuted: false,
			Reason:           "safety controller is not initialized",
		})
		return
	}
	s.writeJSON(w, http.StatusOK, ctrl.Snapshot())
}

func (s *Server) handleTestSafety(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("safety test panicked")
			s.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"ok":    false,
				"error": "internal error",
			})
		}
	}()

	ctrl := s.Safety()
	if ctrl == nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": "safety controller is not initialized",
		})
		return
	}

	scenario := "warning"
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxConfigBody+1))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
		return
	}
	if len(raw) > maxConfigBody {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", config.ErrMalformedJSON)
		return
	}
	if trim := bytes.TrimSpace(raw); len(trim) > 0 {
		var req safetyTestRequest
		if err := decodeObjectBytes(trim, &req); err != nil {
			s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
			return
		}
		if strings.TrimSpace(req.Scenario) != "" {
			scenario = strings.ToLower(strings.TrimSpace(req.Scenario))
		}
	}

	snap, err := ctrl.Simulate(r.Context(), scenario)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":      false,
			"error":   "unknown scenario",
			"detail":  "scenario must be warning or critical",
			"dry_run": true,
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"dry_run": true,
		"message": safety.DryRunMessage,
		"safety":  snap,
	})
	s.audit(r, storage.AuditSafetyTest, true, "safety simulation")
}

func (s *Server) safetyRead(ctx context.Context) (safety.Sample, error) {
	snap, err := s.provider.Snapshot(ctx)
	if err != nil {
		return safety.Sample{}, err
	}
	sample := safety.Sample{
		Present:     snap.Battery.Present,
		Status:      snap.Battery.Status,
		Discharging: notify.IsDischarging(snap.Battery.Status),
		Percent:     snap.Battery.CapacityPercent,
	}
	s.mu.RLock()
	w := s.watcher
	s.mu.RUnlock()
	if w != nil {
		sample.Source = w.Snapshot().Source
	}
	return sample, nil
}
