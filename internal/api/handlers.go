package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lapguard/internal/battery"
	"lapguard/internal/config"
)

type Server struct {
	provider battery.Provider
	cfg      config.Config
	log      *slog.Logger
}

func New(provider battery.Provider, cfg config.Config, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		provider: provider,
		cfg:      cfg,
		log:      log,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/telemetry", s.handleTelemetry)
	mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /api/v1/healthz", s.handleHealthz)
	mux.Handle("/", s.staticHandler())
	return withMiddleware(mux, s.log)
}

type capabilitiesResponse struct {
	App             string   `json:"app"`
	Version         string   `json:"version"`
	Provider        string   `json:"provider"`
	Listen          string   `json:"listen"`
	BatteryPresent  bool     `json:"battery_present"`
	BatteryName     string   `json:"battery_name,omitempty"`
	SysfsRoot       string   `json:"sysfs_root,omitempty"`
	AvailableFields []string `json:"available_fields"`
	Features        features `json:"features"`
}

type features struct {
	Shutdown         bool `json:"shutdown"`
	Docker           bool `json:"docker"`
	ChargeThresholds bool `json:"charge_thresholds"`
	Notifications    bool `json:"notifications"`
	Authentication   bool `json:"authentication"`
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	snap, err := s.provider.Snapshot(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to read battery telemetry", err)
		return
	}
	s.writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	probe, err := s.provider.Probe(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to probe battery capabilities", err)
		return
	}

	fields := probe.AvailableFields
	if fields == nil {
		fields = []string{}
	}

	resp := capabilitiesResponse{
		App:             config.AppName,
		Version:         config.Version,
		Provider:        s.provider.Kind(),
		Listen:          s.cfg.Listen,
		BatteryPresent:  probe.BatteryPresent,
		BatteryName:     probe.BatteryName,
		SysfsRoot:       probe.SysfsRoot,
		AvailableFields: fields,
		Features: features{
			Shutdown:         false,
			Docker:           false,
			ChargeThresholds: false,
			Notifications:    false,
			Authentication:   false,
		},
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"app":     config.AppName,
		"version": config.Version,
	})
}

func (s *Server) staticHandler() http.Handler {
	webDir := strings.TrimSpace(s.cfg.WebDir)
	if webDir == "" {
		return http.HandlerFunc(s.handleAPIOnlyRoot)
	}
	info, err := os.Stat(webDir)
	if err != nil || !info.IsDir() {
		return http.HandlerFunc(s.handleAPIOnlyRoot)
	}
	fileServer := http.FileServer(http.Dir(webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
		if rel != "." && rel != "" {
			if _, err := os.Stat(filepath.Join(webDir, rel)); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})
}

func (s *Server) handleAPIOnlyRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"app":     config.AppName,
		"version": config.Version,
		"message": "frontend not built; use the Vite dev server or run npm run build in web/",
		"api": map[string]string{
			"telemetry":    "/api/v1/telemetry",
			"capabilities": "/api/v1/capabilities",
		},
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(payload); err != nil {
		s.log.Error("encode json", "err", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string, err error) {
	s.log.Error(message, "err", err, "status", status)
	s.writeJSON(w, status, map[string]string{
		"error":  message,
		"detail": err.Error(),
	})
}

func withMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if r.Method == http.MethodOptions {
			setCORS(w, r)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		setCORS(w, r)
		next.ServeHTTP(w, r)
		if r.URL.Path == "/api/v1/telemetry" || r.URL.Path == "/api/v1/healthz" {
			return
		}
		log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	switch origin {
	case "http://127.0.0.1:5173", "http://localhost:5173":
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type")
	}
}
