package config

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	NotifyProviderNone     = "none"
	NotifyProviderNtfy     = "ntfy"
	NotifyProviderTelegram = "telegram"
	NotifyProviderDiscord  = "discord"
	NotifyProviderWebhook  = "webhook"

	ExecutionStoredOnly   = "stored_only"
	ExecutionUnconfigured = "unconfigured"
	ExecutionDisabled     = "disabled"
	ExecutionDryRun       = "dry_run"
	ExecutionReady        = "ready"
	RedactedSecret        = "[redacted]"

	maxWebhookURLLen = 2048
	maxChatIDLen     = 128
	maxDockerTimeout = 3600
)

// NotificationsConfig is the persisted notification settings. Secrets stay in
// the config file and are never copied into API responses or logs.
type NotificationsConfig struct {
	Provider   string `json:"provider"`
	Enabled    bool   `json:"enabled"`
	DryRun     bool   `json:"dry_run"`
	WebhookURL string `json:"webhook_url"`
	ChatID     string `json:"chat_id"`
}

// NotificationsView is the HTTP representation. Webhook URLs, bot tokens, and
// chat IDs are omitted; *_configured flags tell the UI that a secret is stored.
type NotificationsView struct {
	Provider          string `json:"provider"`
	Enabled           bool   `json:"enabled"`
	DryRun            bool   `json:"dry_run"`
	WebhookURL        string `json:"webhook_url"`
	ChatID            string `json:"chat_id"`
	WebhookConfigured bool   `json:"webhook_configured"`
	ChatIDConfigured  bool   `json:"chat_id_configured"`
}

// ShutdownConfig is persisted only. Automatic host shutdown is not executed in this alpha.
type ShutdownConfig struct {
	Enabled           bool `json:"enabled"`
	WarningThreshold  int  `json:"warning_threshold"`
	CriticalThreshold int  `json:"critical_threshold"`
}

// DockerConfig is persisted only. Automatic container stop is not executed.
// Manual drain requires actions.real_enabled and safety.dry_run=false.
type DockerConfig struct {
	StopEnabled    bool `json:"stop_enabled"`
	TimeoutSeconds int  `json:"timeout_seconds"`
}

// SafetyConfig gates the battery safety controller. dry_run and require_ac_loss
// default to true. Automatic low-battery shutdown is never executed in this
// alpha even if dry_run is later set to false.
type SafetyConfig struct {
	DryRun                bool `json:"dry_run"`
	RequireACLoss         bool `json:"require_ac_loss"`
	MinimumBatteryPercent int  `json:"minimum_battery_percent"`
	CooldownSeconds       int  `json:"cooldown_seconds"`
}

type ExecutionStatus struct {
	Notifications string `json:"notifications"`
	Shutdown      string `json:"shutdown"`
	Docker        string `json:"docker"`
}

// APIConfig is the HTTP view of user-managed settings.
type APIConfig struct {
	Notifications   NotificationsView `json:"notifications"`
	Shutdown        ShutdownConfig    `json:"shutdown"`
	Docker          DockerConfig      `json:"docker"`
	Safety          SafetyConfig      `json:"safety"`
	AuthEnabled     bool              `json:"auth_enabled"`
	TokenConfigured bool              `json:"token_configured"`
	TokenCreatedAt  string            `json:"token_created_at,omitempty"`
	LastRotatedAt   string            `json:"last_rotated_at,omitempty"`
	Actions         ActionsView       `json:"actions"`
	Execution       ExecutionStatus   `json:"execution"`
	Notes           []string          `json:"notes,omitempty"`
}

func DefaultNotifications() NotificationsConfig {
	return NotificationsConfig{Provider: DefaultNotifyProvider}
}

func DefaultShutdown() ShutdownConfig {
	return ShutdownConfig{
		WarningThreshold:  DefaultWarningThreshold,
		CriticalThreshold: DefaultCriticalThreshold,
	}
}

func DefaultDocker() DockerConfig {
	return DockerConfig{TimeoutSeconds: DefaultDockerTimeoutSecs}
}

func DefaultSafety() SafetyConfig {
	return SafetyConfig{
		DryRun:                true,
		RequireACLoss:         true,
		MinimumBatteryPercent: 0,
		CooldownSeconds:       DefaultSafetyCooldown,
	}
}

func StoredOnlyExecution() ExecutionStatus {
	return ExecutionStatus{
		Notifications: ExecutionStoredOnly,
		Shutdown:      ExecutionStoredOnly,
		Docker:        ExecutionStoredOnly,
	}
}

func (c Config) APIView() APIConfig {
	view := c.Auth.View()
	hostExec := c.hostExecutionState()
	return APIConfig{
		Notifications:   c.Notifications.Public(),
		Shutdown:        c.Shutdown,
		Docker:          c.Docker,
		Safety:          c.Safety,
		AuthEnabled:     view.AuthEnabled,
		TokenConfigured: view.TokenConfigured,
		TokenCreatedAt:  view.TokenCreatedAt,
		LastRotatedAt:   view.LastRotatedAt,
		Actions:         c.Actions.View(c.IntendedPlan(), c.ActionGates(), c.ManualActionsReady()),
		Execution: ExecutionStatus{
			Notifications: c.Notifications.ExecutionState(),
			Shutdown:      hostExec,
			Docker:        hostExec,
		},
		Notes: []string{
			"Notification delivery runs only when a provider is configured and enabled.",
			"Automatic low-battery shutdown is not executed in this alpha. The safety controller remains a recorder.",
			"Manual Docker drain and poweroff are experimental, disabled by default, and require actions.real_enabled=true, safety.dry_run=false, and explicit confirmation. Do not enable them on an important machine.",
			"GET telemetry, capabilities, discover, power, events, safety, healthz, and auth/status stay readable without a token in this alpha. POST/PUT require a Bearer token when auth.enabled is true.",
		},
	}
}

func (n NotificationsConfig) Public() NotificationsView {
	return NotificationsView{
		Provider:          n.Provider,
		Enabled:           n.Enabled,
		DryRun:            n.DryRun,
		WebhookURL:        "",
		ChatID:            "",
		WebhookConfigured: strings.TrimSpace(n.WebhookURL) != "",
		ChatIDConfigured:  strings.TrimSpace(n.ChatID) != "",
	}
}

func (n NotificationsConfig) ExecutionState() string {
	if !n.ProviderConfigured() {
		return ExecutionUnconfigured
	}
	if !n.Enabled {
		return ExecutionDisabled
	}
	if n.DryRun {
		return ExecutionDryRun
	}
	return ExecutionReady
}

// ProviderConfigured is true when a real provider has the fields needed to send
// a message. A webhook URL with provider "none" is not enough.
func (n NotificationsConfig) ProviderConfigured() bool {
	url := strings.TrimSpace(n.WebhookURL)
	if url == "" || validateHTTPURL(url) != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(n.Provider)) {
	case NotifyProviderNtfy, NotifyProviderDiscord, NotifyProviderWebhook:
		return true
	case NotifyProviderTelegram:
		return strings.TrimSpace(n.ChatID) != ""
	default:
		return false
	}
}

func (n *NotificationsConfig) normalize() error {
	n.Provider = strings.ToLower(strings.TrimSpace(n.Provider))
	if n.Provider == "" {
		n.Provider = NotifyProviderNone
	}
	switch n.Provider {
	case NotifyProviderNone, NotifyProviderNtfy, NotifyProviderTelegram, NotifyProviderDiscord, NotifyProviderWebhook:
	default:
		return invalidConfig("unknown notification provider (want none, ntfy, telegram, discord, or webhook)")
	}

	n.WebhookURL = strings.TrimSpace(n.WebhookURL)
	n.ChatID = strings.TrimSpace(n.ChatID)
	if len(n.WebhookURL) > maxWebhookURLLen {
		return invalidConfig("webhook_url is too long")
	}
	if len(n.ChatID) > maxChatIDLen {
		return invalidConfig("chat_id is too long")
	}
	if n.WebhookURL != "" {
		if err := validateHTTPURL(n.WebhookURL); err != nil {
			return err
		}
	}
	if n.Enabled {
		if n.Provider == NotifyProviderNone {
			return invalidConfig("notifications.enabled requires a provider other than none")
		}
		if n.WebhookURL == "" {
			return invalidConfig("webhook_url is required when notifications are enabled")
		}
		if n.Provider == NotifyProviderTelegram && n.ChatID == "" {
			return invalidConfig("chat_id is required for telegram")
		}
	}
	return nil
}

func (s *ShutdownConfig) normalize() error {
	if err := validatePercent("warning_threshold", s.WarningThreshold); err != nil {
		return err
	}
	if err := validatePercent("critical_threshold", s.CriticalThreshold); err != nil {
		return err
	}
	if s.CriticalThreshold >= s.WarningThreshold {
		return invalidConfig("critical_threshold must be lower than warning_threshold")
	}
	return nil
}

func (d *DockerConfig) normalize() error {
	if d.TimeoutSeconds < 0 || d.TimeoutSeconds > maxDockerTimeout {
		return invalidConfig("docker timeout_seconds must be between 0 and 3600")
	}
	return nil
}

func (s *SafetyConfig) normalize() error {
	if err := validatePercent("minimum_battery_percent", s.MinimumBatteryPercent); err != nil {
		return err
	}
	if s.CooldownSeconds < 0 || s.CooldownSeconds > 86400 {
		return invalidConfig("safety cooldown_seconds must be between 0 and 86400")
	}
	return nil
}

func validatePercent(name string, value int) error {
	if value < 0 || value > 100 {
		return invalidConfig(name + " must be between 0 and 100")
	}
	return nil
}

func validateHTTPURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.Host == "" {
		return invalidConfig("webhook_url is invalid")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return invalidConfig("webhook_url is invalid")
	}
	return nil
}

type NotificationsPatch struct {
	Provider   *string `json:"provider"`
	Enabled    *bool   `json:"enabled"`
	DryRun     *bool   `json:"dry_run"`
	WebhookURL *string `json:"webhook_url"`
	ChatID     *string `json:"chat_id"`
}

type ShutdownPatch struct {
	Enabled           *bool `json:"enabled"`
	WarningThreshold  *int  `json:"warning_threshold"`
	CriticalThreshold *int  `json:"critical_threshold"`
}

type DockerPatch struct {
	StopEnabled    *bool `json:"stop_enabled"`
	TimeoutSeconds *int  `json:"timeout_seconds"`
}

type SafetyPatch struct {
	DryRun                *bool `json:"dry_run"`
	RequireACLoss         *bool `json:"require_ac_loss"`
	MinimumBatteryPercent *int  `json:"minimum_battery_percent"`
	CooldownSeconds       *int  `json:"cooldown_seconds"`
}

func (n NotificationsConfig) Apply(p NotificationsPatch) (NotificationsConfig, error) {
	out := n
	if p.Provider != nil {
		out.Provider = *p.Provider
	}
	if p.Enabled != nil {
		out.Enabled = *p.Enabled
	}
	if p.DryRun != nil {
		out.DryRun = *p.DryRun
	}
	if p.WebhookURL != nil {
		if v := strings.TrimSpace(*p.WebhookURL); v != "" && v != RedactedSecret {
			out.WebhookURL = v
		}
	}
	if p.ChatID != nil {
		if v := strings.TrimSpace(*p.ChatID); v != "" && v != RedactedSecret {
			out.ChatID = v
		}
	}
	if err := out.normalize(); err != nil {
		return NotificationsConfig{}, err
	}
	return out, nil
}

func (s ShutdownConfig) Apply(p ShutdownPatch) (ShutdownConfig, error) {
	out := s
	if p.Enabled != nil {
		out.Enabled = *p.Enabled
	}
	if p.WarningThreshold != nil {
		out.WarningThreshold = *p.WarningThreshold
	}
	if p.CriticalThreshold != nil {
		out.CriticalThreshold = *p.CriticalThreshold
	}
	if err := out.normalize(); err != nil {
		return ShutdownConfig{}, err
	}
	return out, nil
}

func (d DockerConfig) Apply(p DockerPatch) (DockerConfig, error) {
	out := d
	if p.StopEnabled != nil {
		out.StopEnabled = *p.StopEnabled
	}
	if p.TimeoutSeconds != nil {
		out.TimeoutSeconds = *p.TimeoutSeconds
	}
	if err := out.normalize(); err != nil {
		return DockerConfig{}, err
	}
	return out, nil
}

func (s SafetyConfig) Apply(p SafetyPatch) (SafetyConfig, error) {
	out := s
	if p.DryRun != nil {
		out.DryRun = *p.DryRun
	}
	if p.RequireACLoss != nil {
		out.RequireACLoss = *p.RequireACLoss
	}
	if p.MinimumBatteryPercent != nil {
		out.MinimumBatteryPercent = *p.MinimumBatteryPercent
	}
	if p.CooldownSeconds != nil {
		out.CooldownSeconds = *p.CooldownSeconds
	}
	if err := out.normalize(); err != nil {
		return SafetyConfig{}, err
	}
	return out, nil
}

func invalidConfig(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, msg)
}
