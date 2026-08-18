package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"lapguard/internal/battery"
	"lapguard/internal/config"
	"lapguard/internal/notify"
	"lapguard/internal/power"
)

func TestTestNotificationRequiresEnabled(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPost, "/api/v1/actions/test-notification", "{}")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "://") {
		t.Fatalf("leaked URL: %s", rec.Body.String())
	}
}

func TestTestNotificationSuccessAndRedaction(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	app := newConfigServer(t, nil)
	secret := upstream.URL + "/topic-secret-token"
	app.cfg.Notifications = config.NotificationsConfig{
		Provider:   "ntfy",
		Enabled:    true,
		WebhookURL: secret,
	}
	app.AttachNotifier(notify.New(notify.Options{
		Config:      func() config.NotificationsConfig { return app.currentConfig().Notifications },
		Client:      upstream.Client(),
		MaxAttempts: 1,
		Cooldown:    time.Hour,
		HTTPTimeout: time.Second,
	}))

	req := jsonRequest(http.MethodPost, "/api/v1/actions/test-notification", "{}")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body testNotificationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Provider != "ntfy" {
		t.Fatalf("%+v", body)
	}
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "topic-secret-token") {
		t.Fatalf("secret in response: %s", rec.Body.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("hits %d", hits.Load())
	}
}

func TestTestNotificationDryRunZeroHTTP(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	app := newConfigServer(t, nil)
	app.cfg.Notifications = config.NotificationsConfig{
		Provider:   "ntfy",
		Enabled:    true,
		DryRun:     true,
		WebhookURL: upstream.URL,
	}
	req := jsonRequest(http.MethodPost, "/api/v1/actions/test-notification", "{}")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if hits.Load() != 0 {
		t.Fatalf("dry-run hits %d", hits.Load())
	}
}

func TestCapabilitiesNotificationsRequiresProvider(t *testing.T) {
	app := newConfigServer(t, nil)
	app.cfg.Notifications = config.NotificationsConfig{
		Provider:   "none",
		WebhookURL: "https://hooks.example.invalid/only-a-url",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	var body capabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.FeatureFlags.Notifications {
		t.Fatal("webhook URL with provider none must not enable notifications")
	}

	app.cfg.Notifications = config.NotificationsConfig{
		Provider:   "ntfy",
		Enabled:    false,
		WebhookURL: "https://ntfy.example.invalid/lapguard",
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	rec = httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.FeatureFlags.Notifications {
		t.Fatal("valid ntfy provider should enable the capability even when disabled")
	}
}

func TestPowerEventDoesNotNotifyWhenDisabled(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstream.Close)

	app := New(battery.NewMockProvider(), config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	app.cfg.Notifications = config.NotificationsConfig{
		Provider:   "ntfy",
		Enabled:    false,
		WebhookURL: upstream.URL,
	}
	n := notify.New(notify.Options{
		Config:      func() config.NotificationsConfig { return app.currentConfig().Notifications },
		Client:      upstream.Client(),
		MaxAttempts: 1,
		Cooldown:    time.Hour,
	})
	if err := n.HandlePower(context.Background(), power.Transition{Type: power.EventACDisconnected}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("disabled default must not HTTP: %d", hits.Load())
	}
}
