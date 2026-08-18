package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/power"
)

const (
	EventACDisconnected  = "AC_DISCONNECTED"
	EventACConnected     = "AC_CONNECTED"
	EventBatteryWarning  = "BATTERY_WARNING"
	EventBatteryCritical = "BATTERY_CRITICAL"
	EventTest            = "TEST"
	EventAutoDrain       = "AUTO_DRAIN_WARNING"

	DefaultHTTPTimeout = 5 * time.Second
	DefaultMaxAttempts = 3
	DefaultBackoff     = 200 * time.Millisecond
	DefaultCooldown    = 5 * time.Minute
	DefaultTestTimeout = 12 * time.Second
)

var (
	ErrDisabled      = errors.New("notifications are disabled")
	ErrNotConfigured = errors.New("no notification provider configured")
	ErrUnsupported   = errors.New("unsupported notification event")
	ErrRateLimited   = errors.New("notification rate limited")
)

// Notifier sends a single notification event.
type Notifier interface {
	Send(ctx context.Context, event NotificationEvent) error
}

// NotificationEvent is the payload delivered to ntfy, Telegram, or Discord.
type NotificationEvent struct {
	Type      string
	Title     string
	Message   string
	Timestamp time.Time
}

type Options struct {
	Config      func() config.NotificationsConfig
	Client      *http.Client
	Logger      *slog.Logger
	HTTPTimeout time.Duration
	MaxAttempts int
	Backoff     time.Duration
	Cooldown    time.Duration
	Now         func() time.Time
}

// Service implements Notifier with retries, dry-run, and per-event rate limits.
type Service struct {
	config      func() config.NotificationsConfig
	client      *http.Client
	log         *slog.Logger
	httpTimeout time.Duration
	maxAttempts int
	backoff     time.Duration
	limiter     *Limiter
}

func New(opts Options) *Service {
	if opts.Config == nil {
		opts.Config = func() config.NotificationsConfig { return config.DefaultNotifications() }
	}
	if opts.HTTPTimeout <= 0 {
		opts.HTTPTimeout = DefaultHTTPTimeout
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = DefaultMaxAttempts
	}
	if opts.Backoff <= 0 {
		opts.Backoff = DefaultBackoff
	}
	if opts.Cooldown <= 0 {
		opts.Cooldown = DefaultCooldown
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{
			Timeout: opts.HTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	} else if client.Timeout == 0 {
		cp := *client
		cp.Timeout = opts.HTTPTimeout
		client = &cp
	}
	return &Service{
		config:      opts.Config,
		client:      client,
		log:         opts.Logger,
		httpTimeout: opts.HTTPTimeout,
		maxAttempts: opts.MaxAttempts,
		backoff:     opts.Backoff,
		limiter:     NewLimiter(opts.Cooldown, opts.Now),
	}
}

func (s *Service) Configured() bool {
	return s.config().ProviderConfigured()
}

func (s *Service) Send(ctx context.Context, event NotificationEvent) error {
	cfg := s.config()
	if !cfg.Enabled {
		return ErrDisabled
	}
	if !cfg.ProviderConfigured() {
		return ErrNotConfigured
	}
	if !AllowedEvent(event.Type) {
		return ErrUnsupported
	}
	event = NormalizeEvent(event)

	if event.Type != EventTest && event.Type != EventAutoDrain && !s.limiter.Allow(event.Type) {
		s.log.Info("notification rate-limited", "event", event.Type, "provider", cfg.Provider)
		return ErrRateLimited
	}

	if cfg.DryRun {
		s.log.Info("notification dry-run", "event", event.Type, "provider", cfg.Provider)
		return nil
	}

	err := withRetry(ctx, s.maxAttempts, s.backoff, func(ctx context.Context) error {
		return deliver(ctx, s.client, s.httpTimeout, cfg, event)
	})
	if err != nil {
		s.log.Warn("notification delivery failed", "event", event.Type, "provider", cfg.Provider, "err", SanitizeError(err))
		return err
	}
	s.log.Info("notification sent", "event", event.Type, "provider", cfg.Provider)
	return nil
}

// HandlePower sends AC connect/disconnect notifications. Unknown AC states and
// disabled/unconfigured providers are silent.
func (s *Service) HandlePower(ctx context.Context, tr power.Transition) error {
	switch tr.Type {
	case power.EventACConnected, power.EventACDisconnected:
	default:
		return nil
	}
	err := s.Send(ctx, EventFromPower(tr))
	if skipNotifyErr(err) {
		return nil
	}
	return err
}

func skipNotifyErr(err error) bool {
	return err == nil || errors.Is(err, ErrDisabled) || errors.Is(err, ErrNotConfigured) || errors.Is(err, ErrRateLimited) || errors.Is(err, ErrUnsupported)
}

func AllowedEvent(kind string) bool {
	switch kind {
	case EventACConnected, EventACDisconnected, EventBatteryWarning, EventBatteryCritical, EventTest, EventAutoDrain:
		return true
	default:
		return false
	}
}

func EventFromPower(tr power.Transition) NotificationEvent {
	switch tr.Type {
	case power.EventACDisconnected:
		return NotificationEvent{
			Type:      EventACDisconnected,
			Title:     "LapGuard: AC power lost",
			Message:   "AC power lost. Running on battery.",
			Timestamp: tr.At,
		}
	case power.EventACConnected:
		msg := "AC power restored."
		if tr.DurationMs != nil {
			msg = fmt.Sprintf("AC power restored after %s.", formatDurationMS(*tr.DurationMs))
		}
		return NotificationEvent{
			Type:      EventACConnected,
			Title:     "LapGuard: AC power restored",
			Message:   msg,
			Timestamp: tr.At,
		}
	default:
		return NotificationEvent{Type: tr.Type, Timestamp: tr.At}
	}
}

func NormalizeEvent(event NotificationEvent) NotificationEvent {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Title == "" || event.Message == "" {
		title, msg := DefaultCopy(event.Type)
		if event.Title == "" {
			event.Title = title
		}
		if event.Message == "" {
			event.Message = msg
		}
	}
	return event
}

func DefaultCopy(kind string) (title, message string) {
	switch kind {
	case EventACDisconnected:
		return "LapGuard: AC power lost", "AC power lost. Running on battery."
	case EventACConnected:
		return "LapGuard: AC power restored", "AC power restored."
	case EventBatteryWarning:
		return "LapGuard: battery warning", "Battery charge has reached the warning threshold."
	case EventBatteryCritical:
		return "LapGuard: battery critical", "Battery charge has reached the critical threshold."
	case EventTest:
		return "LapGuard: test notification", "LapGuard test notification. Delivery is working."
	case EventAutoDrain:
		return "LapGuard: battery low", "Battery low! In 30min: [YES] Save+Stop / [NO] Let run. Confirm in the LapGuard dashboard."
	default:
		return "LapGuard", kind
	}
}

func TestEvent() NotificationEvent {
	return NormalizeEvent(NotificationEvent{Type: EventTest})
}

func formatDurationMS(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Round(time.Second).Seconds()))
	}
	return d.Round(time.Second).String()
}

// SanitizeError returns a log/API-safe error string with URLs and tokens removed.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "delivery timed out"
	}
	msg := err.Error()
	if strings.Contains(msg, "://") || strings.Contains(strings.ToLower(msg), "webhook") || strings.Contains(strings.ToLower(msg), "token") {
		if errors.Is(err, context.DeadlineExceeded) {
			return "delivery timed out"
		}
		return "delivery failed"
	}
	if len(msg) > 160 {
		return "delivery failed"
	}
	return msg
}

func PublicFailure(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrDisabled):
		return ErrDisabled.Error()
	case errors.Is(err, ErrNotConfigured):
		return ErrNotConfigured.Error()
	case errors.Is(err, ErrUnsupported):
		return ErrUnsupported.Error()
	case errors.Is(err, ErrRateLimited):
		return ErrRateLimited.Error()
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "delivery timed out"
	default:
		return "delivery failed"
	}
}
