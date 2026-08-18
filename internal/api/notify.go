package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"lapguard/internal/notify"
	"lapguard/internal/storage"
)

type testNotificationResponse struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider,omitempty"`
	DryRun   bool   `json:"dry_run,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("test notification panicked")
			s.writeJSON(w, http.StatusInternalServerError, testNotificationResponse{
				OK:    false,
				Error: "internal error",
			})
		}
	}()

	cfg := s.currentConfig().Notifications
	if !cfg.Enabled {
		s.writeJSON(w, http.StatusBadRequest, testNotificationResponse{
			OK:    false,
			Error: notify.ErrDisabled.Error(),
		})
		return
	}
	if !cfg.ProviderConfigured() {
		s.writeJSON(w, http.StatusBadRequest, testNotificationResponse{
			OK:    false,
			Error: notify.ErrNotConfigured.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), notify.DefaultTestTimeout)
	defer cancel()

	n := s.Notifier()
	if n == nil {
		s.writeJSON(w, http.StatusBadRequest, testNotificationResponse{
			OK:    false,
			Error: notify.ErrNotConfigured.Error(),
		})
		return
	}

	err := n.Send(ctx, notify.TestEvent())
	resp := testNotificationResponse{
		Provider: cfg.Provider,
		DryRun:   cfg.DryRun,
	}
	if err != nil {
		resp.OK = false
		resp.Error = notify.PublicFailure(err)
		if strings.Contains(resp.Error, "://") {
			resp.Error = "delivery failed"
		}
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = http.StatusGatewayTimeout
		}
		s.writeJSON(w, status, resp)
		return
	}
	resp.OK = true
	s.audit(r, storage.AuditNotificationTest, true, "notification test")
	s.writeJSON(w, http.StatusOK, resp)
}
