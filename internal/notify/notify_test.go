package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/power"
)

func TestNtfyDelivery(t *testing.T) {
	var gotTitle, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		gotCT = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := newTestService(t, config.NotificationsConfig{
		Provider:   config.NotifyProviderNtfy,
		Enabled:    true,
		WebhookURL: srv.URL,
	}, srv.Client())
	if err := n.Send(context.Background(), TestEvent()); err != nil {
		t.Fatal(err)
	}
	if gotTitle != "LapGuard: test notification" {
		t.Fatalf("title %q", gotTitle)
	}
	if gotBody != "LapGuard test notification. Delivery is working." {
		t.Fatalf("body %q", gotBody)
	}
	if !strings.Contains(gotCT, "text/plain") {
		t.Fatalf("content-type %q", gotCT)
	}
}

func TestTelegramDelivery(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("json: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := newTestService(t, config.NotificationsConfig{
		Provider:   config.NotifyProviderTelegram,
		Enabled:    true,
		WebhookURL: srv.URL + "/botTESTTOKEN/sendMessage",
		ChatID:     "4242",
	}, srv.Client())
	if err := n.Send(context.Background(), NotificationEvent{Type: EventACDisconnected}); err != nil {
		t.Fatal(err)
	}
	if payload["chat_id"] != "4242" {
		t.Fatalf("payload %+v", payload)
	}
	text, _ := payload["text"].(string)
	if !strings.Contains(text, "AC power lost") {
		t.Fatalf("text %q", text)
	}
}

func TestDiscordDelivery(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("json: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	n := newTestService(t, config.NotificationsConfig{
		Provider:   config.NotifyProviderDiscord,
		Enabled:    true,
		WebhookURL: srv.URL + "/api/webhooks/1/secret-token",
	}, srv.Client())
	if err := n.Send(context.Background(), NotificationEvent{Type: EventACConnected}); err != nil {
		t.Fatal(err)
	}
	content, _ := payload["content"].(string)
	if !strings.Contains(content, "AC power restored") {
		t.Fatalf("content %q", content)
	}
}

func TestProviderValidation(t *testing.T) {
	notConfigured := []config.NotificationsConfig{
		{Provider: "none", Enabled: false, WebhookURL: "https://example.invalid/hook"},
		{Provider: "ntfy", Enabled: true},
		{Provider: "telegram", Enabled: true, WebhookURL: "https://example.invalid/bot"},
		{Provider: "discord", Enabled: false, WebhookURL: ""},
	}
	for _, cfg := range notConfigured {
		if cfg.ProviderConfigured() {
			t.Fatalf("should not be configured: %+v", cfg)
		}
	}

	ok := config.NotificationsConfig{
		Provider:   "ntfy",
		WebhookURL: "https://ntfy.example.invalid/lapguard",
	}
	if !ok.ProviderConfigured() {
		t.Fatal("ntfy with URL should be configured even when disabled")
	}
	tg := config.NotificationsConfig{
		Provider:   "telegram",
		WebhookURL: "https://api.telegram.org/botTEST/sendMessage",
		ChatID:     "1",
	}
	if !tg.ProviderConfigured() {
		t.Fatal("telegram with url+chat should be configured")
	}
}

func TestTimeoutAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := New(Options{
		Config: func() config.NotificationsConfig {
			return config.NotificationsConfig{Provider: "ntfy", Enabled: true, WebhookURL: srv.URL}
		},
		Client:      srv.Client(),
		HTTPTimeout: 30 * time.Millisecond,
		MaxAttempts: 2,
		Backoff:     time.Millisecond,
		Cooldown:    time.Hour,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := n.Send(ctx, TestEvent()); err == nil {
		t.Fatal("expected timeout")
	}

	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(fail.Close)
	n = newTestService(t, config.NotificationsConfig{Provider: "ntfy", Enabled: true, WebhookURL: fail.URL}, fail.Client())
	if err := n.Send(context.Background(), TestEvent()); err == nil {
		t.Fatal("expected provider rejection")
	}
}

func TestRetryOnServerError(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := New(Options{
		Config: func() config.NotificationsConfig {
			return config.NotificationsConfig{Provider: "ntfy", Enabled: true, WebhookURL: srv.URL}
		},
		Client:      srv.Client(),
		MaxAttempts: 3,
		Backoff:     time.Millisecond,
		Cooldown:    time.Hour,
		HTTPTimeout: time.Second,
	})
	if err := n.Send(context.Background(), TestEvent()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestSecretsNotLogged(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(config.NewRedactingHandler(slog.NewTextHandler(&buf, nil)))
	secret := "https://hooks.example.invalid/whsec_TEST_SECRET_TOKEN_9f3a"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := New(Options{
		Config: func() config.NotificationsConfig {
			return config.NotificationsConfig{Provider: "discord", Enabled: true, WebhookURL: srv.URL}
		},
		Client:      srv.Client(),
		Logger:      log,
		MaxAttempts: 1,
		Cooldown:    time.Hour,
	})
	if err := n.Send(context.Background(), TestEvent()); err != nil {
		t.Fatal(err)
	}
	n.log.Info("would send", "webhook_url", secret, "token", "bot12345", "chat_id", "999")
	out := buf.String()
	for _, leak := range []string{secret, "whsec_TEST_SECRET_TOKEN_9f3a", "hooks.example.invalid", "bot12345"} {
		if strings.Contains(out, leak) {
			t.Fatalf("secret %q appeared in logs:\n%s", leak, out)
		}
	}
}

func TestDryRunMakesZeroHTTPRequests(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	n := New(Options{
		Config: func() config.NotificationsConfig {
			return config.NotificationsConfig{
				Provider:   "ntfy",
				Enabled:    true,
				DryRun:     true,
				WebhookURL: srv.URL,
			}
		},
		Client: srv.Client(),
		Logger: log,
	})
	if err := n.Send(context.Background(), NotificationEvent{Type: EventACDisconnected}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("dry-run issued %d HTTP requests", hits.Load())
	}
	out := buf.String()
	if !strings.Contains(out, "AC_DISCONNECTED") || !strings.Contains(out, "ntfy") {
		t.Fatalf("dry-run log should include event and provider: %s", out)
	}
	if strings.Contains(out, srv.URL) {
		t.Fatalf("dry-run log leaked URL: %s", out)
	}
}

func TestACEventNotificationDisabledByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := New(Options{
		Config: func() config.NotificationsConfig {
			return config.NotificationsConfig{
				Provider:   "ntfy",
				Enabled:    false,
				WebhookURL: srv.URL,
			}
		},
		Client: srv.Client(),
	})
	if err := n.HandlePower(context.Background(), power.Transition{Type: power.EventACDisconnected, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("disabled notifier issued %d HTTP requests", hits.Load())
	}
}

func TestDuplicateEventsAreRateLimited(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	n := New(Options{
		Config: func() config.NotificationsConfig {
			return config.NotificationsConfig{Provider: "ntfy", Enabled: true, WebhookURL: srv.URL}
		},
		Client:   srv.Client(),
		Cooldown: time.Minute,
		Backoff:  time.Millisecond,
	})
	ev := NotificationEvent{Type: EventACDisconnected}
	if err := n.Send(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	err := n.Send(context.Background(), ev)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second send: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d, want 1", hits.Load())
	}
}

func TestDisabledSkipsHTTP(t *testing.T) {
	n := newTestService(t, config.DefaultNotifications(), http.DefaultClient)
	if err := n.Send(context.Background(), TestEvent()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("got %v", err)
	}
}

func TestContextCancelDuringRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	n := New(Options{
		Config: func() config.NotificationsConfig {
			return config.NotificationsConfig{Provider: "ntfy", Enabled: true, WebhookURL: srv.URL}
		},
		Client:      srv.Client(),
		MaxAttempts: 3,
		Backoff:     50 * time.Millisecond,
		HTTPTimeout: time.Second,
		Cooldown:    time.Hour,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := n.Send(ctx, TestEvent())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestSanitizeErrorStripsURL(t *testing.T) {
	err := errors.New(`Post "https://hooks.example.invalid/secret": timeout`)
	got := SanitizeError(err)
	if strings.Contains(got, "hooks.example.invalid") || strings.Contains(got, "secret") {
		t.Fatalf("leaked: %q", got)
	}
}

func TestPublicJSONOmitsSecrets(t *testing.T) {
	n := config.NotificationsConfig{
		Provider:   "telegram",
		Enabled:    true,
		WebhookURL: "https://api.telegram.org/botSECRET/sendMessage",
		ChatID:     "999",
	}
	raw, err := json.Marshal(n.Public())
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, leak := range []string{"SECRET", "api.telegram.org", "999"} {
		if strings.Contains(s, leak) {
			t.Fatalf("public view leaked %q: %s", leak, s)
		}
	}
	if !strings.Contains(s, `"webhook_configured":true`) {
		t.Fatalf("expected configured flag: %s", s)
	}
}

func newTestService(t *testing.T, cfg config.NotificationsConfig, client *http.Client) *Service {
	t.Helper()
	return New(Options{
		Config:      func() config.NotificationsConfig { return cfg },
		Client:      client,
		MaxAttempts: 1,
		Cooldown:    time.Hour,
		HTTPTimeout: time.Second,
		Backoff:     time.Millisecond,
	})
}
