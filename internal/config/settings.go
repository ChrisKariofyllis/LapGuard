package config

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	NotifyProviderNone     = "none"
	NotifyProviderTelegram = "telegram"
	NotifyProviderDiscord  = "discord"
	NotifyProviderWebhook  = "webhook"

	ExecutionStoredOnly = "stored_only"

	maxWebhookURLLen = 2048
	maxChatIDLen     = 128
	maxDockerTimeout = 3600
)

// NotificationsConfig is persisted only. Delivery is not implemented in this milestone.
type NotificationsConfig struct {
	Provider   string `json:"provider"`
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
	ChatID     string `json:"chat_id"`
}

// ShutdownConfig is persisted only. Host shutdown is not executed in this milestone.
type ShutdownConfig struct {
	Enabled           bool `json:"enabled"`
	WarningThreshold  int  `json:"warning_threshold"`
	CriticalThreshold int  `json:"critical_threshold"`
}

// DockerConfig is persisted only. Containers are not stopped in this milestone.
type DockerConfig struct {
	StopEnabled    bool `json:"stop_enabled"`
	TimeoutSeconds int  `json:"timeout_seconds"`
}

type ExecutionStatus struct {
	Notifications string `json:"notifications"`
	Shutdown      string `json:"shutdown"`
	Docker        string `json:"docker"`
}

// APIConfig is the HTTP view of user-managed settings.
type APIConfig struct {
	Notifications NotificationsConfig `json:"notifications"`
	Shutdown      ShutdownConfig      `json:"shutdown"`
	Docker        DockerConfig        `json:"docker"`
	Execution     ExecutionStatus     `json:"execution"`
	Notes         []string            `json:"notes,omitempty"`
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

func StoredOnlyExecution() ExecutionStatus {
	return ExecutionStatus{
		Notifications: ExecutionStoredOnly,
		Shutdown:      ExecutionStoredOnly,
		Docker:        ExecutionStoredOnly,
	}
}

func (c Config) APIView() APIConfig {
	return APIConfig{
		Notifications: c.Notifications,
		Shutdown:      c.Shutdown,
		Docker:        c.Docker,
		Execution:     StoredOnlyExecution(),
		Notes: []string{
			"Notification delivery, Docker container stop, and shutdown are stored but not executed in this milestone.",
		},
	}
}

func (n *NotificationsConfig) normalize() error {
	n.Provider = strings.ToLower(strings.TrimSpace(n.Provider))
	if n.Provider == "" {
		n.Provider = NotifyProviderNone
	}
	switch n.Provider {
	case NotifyProviderNone, NotifyProviderTelegram, NotifyProviderDiscord, NotifyProviderWebhook:
	default:
		return invalidConfig("unknown notification provider (want none, telegram, discord, or webhook)")
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

func (n NotificationsConfig) Apply(p NotificationsPatch) (NotificationsConfig, error) {
	out := n
	if p.Provider != nil {
		out.Provider = *p.Provider
	}
	if p.Enabled != nil {
		out.Enabled = *p.Enabled
	}
	if p.WebhookURL != nil {
		out.WebhookURL = *p.WebhookURL
	}
	if p.ChatID != nil {
		out.ChatID = *p.ChatID
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

func invalidConfig(msg string) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, msg)
}
