package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"lapguard/internal/config"
	"lapguard/internal/storage"
)

const maxConfigBody = 64 << 10

type settingsRequest struct {
	Notifications *config.NotificationsPatch `json:"notifications"`
	Shutdown      *config.ShutdownPatch      `json:"shutdown"`
	Docker        *config.DockerPatch        `json:"docker"`
	Safety        *config.SafetyPatch        `json:"safety"`
	Actions       *config.ActionsPatch       `json:"actions"`
	Execution     json.RawMessage            `json:"execution"`
	Notes         json.RawMessage            `json:"notes"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.currentConfig().APIView())
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := decodeObject(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
		return
	}
	view, err := s.updateSettings(func(cfg *config.Config) error {
		if req.Notifications != nil {
			next, err := cfg.Notifications.Apply(*req.Notifications)
			if err != nil {
				return err
			}
			cfg.Notifications = next
		}
		if req.Shutdown != nil {
			next, err := cfg.Shutdown.Apply(*req.Shutdown)
			if err != nil {
				return err
			}
			cfg.Shutdown = next
		}
		if req.Docker != nil {
			next, err := cfg.Docker.Apply(*req.Docker)
			if err != nil {
				return err
			}
			cfg.Docker = next
		}
		if req.Safety != nil {
			next, err := cfg.Safety.Apply(*req.Safety)
			if err != nil {
				return err
			}
			cfg.Safety = next
		}
		if req.Actions != nil {
			next, err := cfg.Actions.Apply(*req.Actions)
			if err != nil {
				return err
			}
			cfg.Actions = next
		}
		return nil
	})
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	s.log.Info("config updated",
		config.SafeNotifications(view.Notifications),
		"shutdown_enabled", view.Shutdown.Enabled,
		"warning_threshold", view.Shutdown.WarningThreshold,
		"critical_threshold", view.Shutdown.CriticalThreshold,
		"docker_stop_enabled", view.Docker.StopEnabled,
		"docker_timeout_seconds", view.Docker.TimeoutSeconds,
		"auth_enabled", view.Auth.Enabled,
		"actions_real_enabled", view.Actions.RealEnabled,
		"safety_dry_run", view.Safety.DryRun,
	)
	s.audit(r, storage.AuditConfigUpdate, true, "config updated")
	s.writeJSON(w, http.StatusOK, view.APIView())
}

func (s *Server) handlePostNotifications(w http.ResponseWriter, r *http.Request) {
	var patch config.NotificationsPatch
	if err := decodeObject(r, &patch); err != nil {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
		return
	}
	view, err := s.updateSettings(func(cfg *config.Config) error {
		next, err := cfg.Notifications.Apply(patch)
		if err != nil {
			return err
		}
		cfg.Notifications = next
		return nil
	})
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	s.log.Info("notifications config stored", config.SafeNotifications(view.Notifications))
	s.audit(r, storage.AuditConfigUpdate, true, "notifications updated")
	s.writeJSON(w, http.StatusOK, view.APIView())
}

func (s *Server) handlePostShutdown(w http.ResponseWriter, r *http.Request) {
	var patch config.ShutdownPatch
	if err := decodeObject(r, &patch); err != nil {
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
		return
	}
	view, err := s.updateSettings(func(cfg *config.Config) error {
		next, err := cfg.Shutdown.Apply(patch)
		if err != nil {
			return err
		}
		cfg.Shutdown = next
		return nil
	})
	if err != nil {
		s.writeSettingsError(w, err)
		return
	}
	s.log.Info("shutdown config stored",
		"enabled", view.Shutdown.Enabled,
		"warning_threshold", view.Shutdown.WarningThreshold,
		"critical_threshold", view.Shutdown.CriticalThreshold,
	)
	s.audit(r, storage.AuditConfigUpdate, true, "shutdown settings stored")
	s.writeJSON(w, http.StatusOK, view.APIView())
}

func (s *Server) writeSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, config.ErrMalformedJSON):
		s.writeError(w, http.StatusBadRequest, "malformed JSON", err)
	case errors.Is(err, config.ErrInvalidConfig):
		s.writeError(w, http.StatusBadRequest, "invalid config", err)
	default:
		s.writeError(w, http.StatusInternalServerError, "failed to persist config", err)
	}
}

func (s *Server) updateSettings(mutate func(*config.Config) error) (config.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.cfg
	if err := mutate(&next); err != nil {
		return config.Config{}, err
	}
	if err := next.Normalize(); err != nil {
		return config.Config{}, err
	}
	path := strings.TrimSpace(next.ConfigPath)
	if path == "" {
		def, err := config.DefaultPath()
		if err != nil {
			return config.Config{}, err
		}
		path = def
		next.ConfigPath = path
	}
	if err := next.Save(path); err != nil {
		return config.Config{}, err
	}
	s.cfg = next
	return next, nil
}

func decodeObject(r *http.Request, dest any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxConfigBody+1))
	if err != nil {
		return fmt.Errorf("%w: unable to read body", config.ErrMalformedJSON)
	}
	if len(data) > maxConfigBody {
		return fmt.Errorf("%w: request body too large", config.ErrMalformedJSON)
	}
	trim := bytes.TrimSpace(data)
	return decodeObjectBytes(trim, dest)
}

func decodeObjectBytes(trim []byte, dest any) error {
	if len(trim) == 0 || trim[0] != '{' {
		return config.ErrMalformedJSON
	}
	dec := json.NewDecoder(bytes.NewReader(trim))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("%w: %s", config.ErrMalformedJSON, jsonDecodeHint(err))
	}
	if dec.More() {
		return config.ErrMalformedJSON
	}
	return nil
}

func jsonDecodeHint(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "unknown field") {
		return msg
	}
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		return "syntax error"
	}
	var typ *json.UnmarshalTypeError
	if errors.As(err, &typ) {
		if typ.Field != "" {
			return "invalid type for " + typ.Field
		}
		return "invalid type"
	}
	return "unable to parse object"
}
