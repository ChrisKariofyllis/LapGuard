package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"lapguard/internal/config"
)

func deliver(ctx context.Context, client *http.Client, timeout time.Duration, cfg config.NotificationsConfig, event NotificationEvent) error {
	req, err := buildRequest(ctx, cfg, event)
	if err != nil {
		return err
	}
	callCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	req = req.WithContext(callCtx)

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	if retryableStatus(res.StatusCode) {
		return statusError{code: res.StatusCode}
	}
	return fmt.Errorf("provider rejected the request")
}

func buildRequest(ctx context.Context, cfg config.NotificationsConfig, event NotificationEvent) (*http.Request, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case config.NotifyProviderNtfy:
		return ntfyRequest(ctx, cfg.WebhookURL, event)
	case config.NotifyProviderTelegram:
		return telegramRequest(ctx, cfg.WebhookURL, cfg.ChatID, event)
	case config.NotifyProviderDiscord:
		return discordRequest(ctx, cfg.WebhookURL, event)
	case config.NotifyProviderWebhook:
		return webhookRequest(ctx, cfg.WebhookURL, event)
	default:
		return nil, ErrNotConfigured
	}
}

func ntfyRequest(ctx context.Context, url string, event NotificationEvent) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(event.Message))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", event.Title)
	req.Header.Set("User-Agent", "LapGuard/"+config.Version)
	switch event.Type {
	case EventBatteryCritical, EventACDisconnected:
		req.Header.Set("Priority", "high")
		req.Header.Set("Tags", "warning")
	case EventBatteryWarning:
		req.Header.Set("Priority", "default")
		req.Header.Set("Tags", "battery")
	default:
		req.Header.Set("Priority", "default")
	}
	return req, nil
}

func telegramRequest(ctx context.Context, url, chatID string, event NotificationEvent) (*http.Request, error) {
	body, err := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     event.Title + "\n" + event.Message,
		"disable_web_page_preview": true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LapGuard/"+config.Version)
	return req, nil
}

func discordRequest(ctx context.Context, url string, event NotificationEvent) (*http.Request, error) {
	body, err := json.Marshal(map[string]any{
		"content": event.Title + "\n" + event.Message,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LapGuard/"+config.Version)
	return req, nil
}

func webhookRequest(ctx context.Context, url string, event NotificationEvent) (*http.Request, error) {
	body, err := json.Marshal(map[string]any{
		"event":     event.Type,
		"title":     event.Title,
		"message":   event.Message,
		"timestamp": event.Timestamp.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LapGuard/"+config.Version)
	return req, nil
}
