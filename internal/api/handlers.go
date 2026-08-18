package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lapguard/internal/actions"
	"lapguard/internal/battery"
	"lapguard/internal/config"
	"lapguard/internal/discovery"
	"lapguard/internal/notify"
	"lapguard/internal/power"
	"lapguard/internal/safety"
	"lapguard/internal/storage"
	"lapguard/internal/webui"
)

type Server struct {
	provider battery.Provider
	log      *slog.Logger
	disc     discovery.Reporter

	mu  sync.RWMutex
	cfg config.Config

	watcher     *power.Watcher
	events      *storage.Store
	notifier    *notify.Service
	safety      *safety.Controller
	rec         *safety.RecordingExecutor
	realRun     actions.Runner
	testActor   safety.ActionExecutor
	testSource  *power.Source
	testBattery *batteryReading
	nowFn       func() time.Time
	guard       actionGuard
}

func New(provider battery.Provider, cfg config.Config, log *slog.Logger, disc discovery.Reporter) *Server {
	if log == nil {
		log = slog.Default()
	}
	log = slog.New(config.NewRedactingHandler(log.Handler()))
	if disc == nil {
		disc = discovery.Static{}
	}
	s := &Server{
		provider: provider,
		cfg:      cfg,
		log:      log,
		disc:     disc,
	}
	s.notifier = notify.New(notify.Options{
		Config: func() config.NotificationsConfig {
			return s.currentConfig().Notifications
		},
		Logger: log,
	})
	interval := cfg.PowerPoll
	if interval <= 0 {
		interval = config.DefaultPowerPoll
	}
	rec := safety.NewRecordingExecutor()
	s.rec = rec
	s.safety = safety.New(safety.Options{
		Interval: interval,
		Config:   func() config.Config { return s.currentConfig() },
		Read:     s.safetyRead,
		Notify: func(ctx context.Context, event notify.NotificationEvent) error {
			n := s.Notifier()
			if n == nil {
				return nil
			}
			return n.Send(ctx, event)
		},
		Executor: rec,
		Logger:   log,
	})
	return s
}

func (s *Server) Safety() *safety.Controller {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.safety
}

func (s *Server) Notifier() *notify.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notifier
}

func (s *Server) AttachNotifier(n *notify.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n != nil {
		s.notifier = n
	}
}

func (s *Server) AttachPower(w *power.Watcher, store *storage.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watcher = w
	s.events = store
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/telemetry", s.handleTelemetry)
	mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /api/v1/discover", s.handleDiscover)
	mux.HandleFunc("GET /api/v1/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/v1/config", s.secureWrite(s.handlePutConfig))
	mux.HandleFunc("POST /api/v1/config/notifications", s.secureWrite(s.handlePostNotifications))
	mux.HandleFunc("POST /api/v1/config/shutdown", s.secureWrite(s.handlePostShutdown))
	mux.HandleFunc("POST /api/v1/actions/test-notification", s.secureWrite(s.handleTestNotification))
	mux.HandleFunc("POST /api/v1/actions/preview", s.secureWrite(s.handleActionPreview))
	mux.HandleFunc("POST /api/v1/actions/poweroff", s.secureWrite(s.handleActionPowerOff))
	mux.HandleFunc("POST /api/v1/actions/docker-drain", s.secureWrite(s.handleActionDockerDrain))
	mux.HandleFunc("GET /api/v1/actions/status", s.handleActionStatus)
	mux.HandleFunc("GET /api/v1/actions/preflight", s.handleActionPreflight)
	mux.HandleFunc("GET /api/v1/power", s.handlePower)
	mux.HandleFunc("GET /api/v1/events", s.handleEvents)
	mux.HandleFunc("GET /api/v1/safety", s.handleGetSafety)
	mux.HandleFunc("POST /api/v1/safety/test", s.secureWrite(s.handleTestSafety))
	mux.HandleFunc("GET /api/v1/healthz", s.handleHealthz)
	mux.HandleFunc("GET /api/v1/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/v1/auth/rotate", s.secureAuthAdmin(s.handleAuthRotate))
	mux.HandleFunc("POST /api/v1/auth/disable", s.secureAuthAdmin(s.handleAuthDisable))
	mux.Handle("/", s.staticHandler())
	return withMiddleware(mux, s.log)
}

func (s *Server) Config() config.Config {
	return s.currentConfig()
}

func (s *Server) currentConfig() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
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
	AuthEnabled      bool                      `json:"auth_enabled"`
	AuthWarning      string                    `json:"auth_warning,omitempty"`
	ProtectGET       bool                      `json:"protect_get"`
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

	cfg := s.currentConfig()
	report := s.applyConfig(s.disc.Last(), cfg)
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
		Listen:           cfg.Listen,
		BatteryPresent:   present,
		BatteryName:      name,
		SysfsRoot:        firstNonEmpty(probe.SysfsRoot, cfg.SysfsRoot),
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
		AuthEnabled:      cfg.Auth.Enabled,
		AuthWarning:      cfg.Auth.Warning(),
		ProtectGET:       false,
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
	s.writeJSON(w, http.StatusOK, s.applyConfig(report, s.currentConfig()))
}

func (s *Server) applyConfig(report discovery.CapabilityReport, cfg config.Config) discovery.CapabilityReport {
	detected := report.Features.ChargeThresholds
	if detected == "" {
		detected = discovery.MethodNone
	}
	method, warn := config.ResolveThresholdMethod(cfg.ThresholdMethod, detected)
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
	report.Features.GracefulShutdown = false
	s.mu.RLock()
	report.Features.OutageEventLog = s.events != nil
	notifierReady := s.notifier != nil
	report.Features.BatterySafety = s.safety != nil
	s.mu.RUnlock()
	report.Features.Notifications = notifierReady && cfg.Notifications.ProviderConfigured()
	return report
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	cfg := s.currentConfig()
	body := map[string]any{
		"status":       "ok",
		"app":          config.AppName,
		"version":      config.Version,
		"auth_enabled": cfg.Auth.Enabled,
		"protect_get":  false,
	}
	if warn := cfg.Auth.Warning(); warn != "" {
		body["auth_warning"] = warn
	}
	s.writeJSON(w, http.StatusOK, body)
}

func (s *Server) staticHandler() http.Handler {
	webDir := strings.TrimSpace(s.currentConfig().WebDir)
	if webDir != "" && webDir != "-" && !strings.EqualFold(webDir, "none") {
		info, err := os.Stat(webDir)
		if err == nil && info.IsDir() {
			return s.staticFromDir(webDir)
		}
	}
	if fsys, ok := webui.FS(); ok {
		return s.staticFromFS(fsys)
	}
	return http.HandlerFunc(s.handleAPIOnlyRoot)
}

func (s *Server) staticFromDir(webDir string) http.Handler {
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

func (s *Server) staticFromFS(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		rel := strings.Trim(path.Clean(r.URL.Path), "/")
		if rel == "" || rel == "." {
			rel = "index.html"
		}
		if f, err := fsys.Open(rel); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r2 := *r
		u := *r.URL
		u.Path = "/index.html"
		r2.URL = &u
		fileServer.ServeHTTP(w, &r2)
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
		"message": "frontend not available; use a release binary, run make build, or start the Vite dev server",
		"api": map[string]string{
			"telemetry":            "/api/v1/telemetry",
			"capabilities":         "/api/v1/capabilities",
			"discover":             "/api/v1/discover",
			"config":               "/api/v1/config",
			"config_notifications": "/api/v1/config/notifications",
			"config_shutdown":      "/api/v1/config/shutdown",
			"test_notification":    "/api/v1/actions/test-notification",
			"power":                "/api/v1/power",
			"events":               "/api/v1/events",
			"safety":               "/api/v1/safety",
			"safety_test":          "/api/v1/safety/test",
			"auth_status":          "/api/v1/auth/status",
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
	if status >= 500 {
		s.log.Error(message, "err", err, "status", status)
	} else {
		s.log.Warn(message, "status", status)
	}
	detail := ""
	if err != nil {
		detail = err.Error()
		if errors.Is(err, config.ErrMalformedJSON) {
			detail = "request body is not valid JSON"
			message = "malformed JSON"
		} else if errors.Is(err, config.ErrInvalidConfig) {
			message = "invalid config"
			detail = strings.TrimPrefix(err.Error(), config.ErrInvalidConfig.Error()+": ")
		}
	}
	body := map[string]string{"error": message}
	if detail != "" {
		body["detail"] = detail
	}
	s.writeJSON(w, status, body)
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
		if r.URL.Path == "/api/v1/telemetry" || r.URL.Path == "/api/v1/healthz" || r.URL.Path == "/api/v1/power" || r.URL.Path == "/api/v1/events" || r.URL.Path == "/api/v1/safety" || r.URL.Path == "/api/v1/auth/status" {
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
	if origin == "" || !originExplicitlyAllowed(origin, r.Host) {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
