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
	"lapguard/internal/discovery"
)

type Server struct {
	provider battery.Provider
	cfg      config.Config
	log      *slog.Logger
	disc     discovery.Reporter
}

func New(provider battery.Provider, cfg config.Config, log *slog.Logger, disc discovery.Reporter) *Server {
	if log == nil {
		log = slog.Default()
	}
	if disc == nil {
		disc = discovery.Static{}
	}
	return &Server{
		provider: provider,
		cfg:      cfg,
		log:      log,
		disc:     disc,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/telemetry", s.handleTelemetry)
	mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /api/v1/discover", s.handleDiscover)
	mux.HandleFunc("GET /api/v1/healthz", s.handleHealthz)
	mux.Handle("/", s.staticHandler())
	return withMiddleware(mux, s.log)
}

type capabilitiesResponse struct {
	App              string                    `json:"app"`
	Version          string                    `json:"version"`
	Provider         string                    `json:"provider"`
	Listen           string                    `json:"listen"`
	BatteryPresent   bool                      `json:"battery_present"`
	BatteryName      string                    `json:"battery_name,omitempty"`
	SysfsRoot        string                    `json:"sysfs_root,omitempty"`
	Hostname         string                    `json:"hostname,omitempty"`
	OS               string                    `json:"os,omitempty"`
	Kernel           string                    `json:"kernel,omitempty"`
	AvailableFields  []string                  `json:"available_fields"`
	NamingConvention string                    `json:"naming_convention,omitempty"`
	PowerCalculation string                    `json:"power_calculation,omitempty"`
	ThresholdMethod  string                    `json:"threshold_method"`
	Features         []discovery.FeatureStatus `json:"features"`
	FeatureFlags     discovery.Features        `json:"feature_flags"`
	Tools            discovery.Tools           `json:"tools"`
	KernelModules    []string                  `json:"kernel_modules"`
	Battery          discovery.BatteryIdentity `json:"battery"`
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

	report := s.applyConfig(s.disc.Last())
	fields := probe.AvailableFields
	if len(fields) == 0 {
		fields = report.AvailableFields
	}
	if fields == nil {
		fields = []string{}
	}

	present := probe.BatteryPresent
	name := probe.BatteryName
	if !present && report.Battery.Present {
		present = true
		if name == "" {
			name = report.Battery.Name
		}
	}

	resp := capabilitiesResponse{
		App:              config.AppName,
		Version:          config.Version,
		Provider:         s.provider.Kind(),
		Listen:           s.cfg.Listen,
		BatteryPresent:   present,
		BatteryName:      name,
		SysfsRoot:        firstNonEmpty(probe.SysfsRoot, s.cfg.SysfsRoot),
		Hostname:         report.Hostname,
		OS:               report.OS,
		Kernel:           report.Kernel,
		AvailableFields:  fields,
		NamingConvention: firstNonEmpty(probe.NamingConvention, report.NamingConvention),
		PowerCalculation: firstNonEmpty(probe.PowerCalculation, report.PowerCalculation),
		ThresholdMethod:  report.Features.ChargeThresholds,
		Features:         report.FeatureStatuses(),
		FeatureFlags:     report.Features,
		Tools:            report.AvailableTools,
		KernelModules:    report.KernelModules,
		Battery:          report.Battery,
	}
	if resp.KernelModules == nil {
		resp.KernelModules = []string{}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	report, err := s.disc.Refresh(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to run hardware discovery", err)
		return
	}
	s.writeJSON(w, http.StatusOK, s.applyConfig(report))
}

func (s *Server) applyConfig(report discovery.CapabilityReport) discovery.CapabilityReport {
	detected := report.Features.ChargeThresholds
	if detected == "" {
		detected = discovery.MethodNone
	}
	method, warn := config.ResolveThresholdMethod(s.cfg.ThresholdMethod, detected)
	report.Features.ChargeThresholds = method
	report.Thresholds.Method = method
	if report.Thresholds.DetectionMethod == "" {
		report.Thresholds.DetectionMethod = "sysfs+tlp+thinkpad_acpi"
	}
	if warn != "" {
		report.Notes = append(report.Notes, warn)
		report.Thresholds.WhyNot = firstNonEmpty(report.Thresholds.WhyNot, warn)
	}
	if method == discovery.MethodNone && report.Thresholds.Recommendation == "" {
		report.Thresholds.Recommendation = "Charge start/stop limits are not available on this hardware."
	}
	return report
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
			"discover":     "/api/v1/discover",
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
