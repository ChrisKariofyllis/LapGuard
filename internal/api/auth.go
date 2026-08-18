package api

import (
	"net/http"

	"lapguard/internal/config"
	"lapguard/internal/storage"
)

func (s *Server) handleAuthStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, s.currentConfig().Auth.View())
}

func (s *Server) handleAuthRotate(w http.ResponseWriter, r *http.Request) {
	// Tokens are minted only by `lapguard auth rotate` so the plaintext is
	// shown once on stdout and never returned as JSON.
	s.audit(r, storage.AuditAuthRotate, false, "use CLI to mint tokens")
	s.writeJSON(w, http.StatusBadRequest, map[string]string{
		"error":  "use CLI",
		"detail": "Run lapguard auth rotate on this laptop. The new token is printed once on stdout and is never returned over HTTP.",
	})
}

func (s *Server) handleAuthDisable(w http.ResponseWriter, r *http.Request) {
	view, err := s.updateSettings(func(cfg *config.Config) error {
		cfg.DisableAuth()
		return nil
	})
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	s.log.Info("authentication disabled")
	s.audit(r, storage.AuditAuthDisable, true, "authentication disabled")
	s.writeJSON(w, http.StatusOK, view.Auth.View())
}
