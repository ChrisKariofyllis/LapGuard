package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lapguard/internal/battery"
	"lapguard/internal/config"
)

func TestGetConfig(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body config.APIConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Notifications.Provider != "none" {
		t.Fatalf("notifications %+v", body.Notifications)
	}
	if body.Shutdown.WarningThreshold != 20 || body.Shutdown.CriticalThreshold != 10 {
		t.Fatalf("shutdown %+v", body.Shutdown)
	}
	if body.Docker.TimeoutSeconds != 30 {
		t.Fatalf("docker %+v", body.Docker)
	}
	if body.Execution.Shutdown != config.ExecutionStoredOnly {
		t.Fatalf("execution %+v", body.Execution)
	}
	if body.Execution.Docker != config.ExecutionStoredOnly {
		t.Fatalf("docker execution %+v", body.Execution)
	}
	if body.Execution.Notifications != config.ExecutionUnconfigured {
		t.Fatalf("notifications execution %+v", body.Execution)
	}
}

func TestPutConfig(t *testing.T) {
	srv := newConfigServer(t, nil)
	payload := `{
		"notifications": {"provider":"telegram","enabled":false,"webhook_url":"https://api.telegram.org/botTEST/sendMessage","chat_id":"42"},
		"shutdown": {"enabled":true,"warning_threshold":25,"critical_threshold":8},
		"docker": {"stop_enabled":true,"timeout_seconds":45}
	}`
	req := jsonRequest(http.MethodPut, "/api/v1/config", payload)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body config.APIConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Shutdown.WarningThreshold != 25 || !body.Shutdown.Enabled {
		t.Fatalf("shutdown %+v", body.Shutdown)
	}
	if !body.Docker.StopEnabled || body.Docker.TimeoutSeconds != 45 {
		t.Fatalf("docker %+v", body.Docker)
	}
	if body.Execution.Shutdown != config.ExecutionStoredOnly {
		t.Fatal("PUT must not claim shutdown is implemented")
	}
	if body.Execution.Docker != config.ExecutionStoredOnly {
		t.Fatal("PUT must not claim Docker stop is implemented")
	}
	if body.Execution.Notifications != config.ExecutionDisabled {
		t.Fatalf("configured but disabled notifications: %+v", body.Execution)
	}
	if body.Notifications.WebhookURL != "" || body.Notifications.ChatID != "" {
		t.Fatalf("API must not return secrets: %+v", body.Notifications)
	}
	if !body.Notifications.WebhookConfigured || !body.Notifications.ChatIDConfigured {
		t.Fatalf("configured flags %+v", body.Notifications)
	}

	loaded, err := config.LoadFile(srv.currentConfig().ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Shutdown.CriticalThreshold != 8 {
		t.Fatalf("persisted %+v", loaded.Shutdown)
	}
	if loaded.Notifications.ChatID != "42" {
		t.Fatalf("persisted notifications %+v", loaded.Notifications)
	}
}

func TestPutConfigMalformedJSON(t *testing.T) {
	srv := newConfigServer(t, nil)
	cases := []string{"", "{", "[]", "null", "not-json", `{"shutdown":`, `{"shutdown":{"enabled":true}}{}`}
	for _, body := range cases {
		req := jsonRequest(http.MethodPut, "/api/v1/config", body)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q status %d, want 400", body, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "malformed JSON") {
			t.Fatalf("body %q response %s", body, rec.Body.String())
		}
	}
}

func TestPutConfigInvalidThresholds(t *testing.T) {
	srv := newConfigServer(t, nil)
	cases := []string{
		`{"shutdown":{"warning_threshold":101,"critical_threshold":10}}`,
		`{"shutdown":{"warning_threshold":20,"critical_threshold":-1}}`,
		`{"shutdown":{"warning_threshold":20,"critical_threshold":20}}`,
		`{"shutdown":{"warning_threshold":10,"critical_threshold":20}}`,
	}
	for _, body := range cases {
		req := jsonRequest(http.MethodPut, "/api/v1/config", body)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s status %d, want 400", body, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "malformed JSON") {
			t.Fatalf("threshold error should not be malformed JSON: %s", rec.Body.String())
		}
	}
}

func TestPostNotificationsAndShutdown(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPost, "/api/v1/config/notifications",
		`{"provider":"discord","enabled":true,"webhook_url":"https://discord.com/api/webhooks/1/abc"}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("notifications status %d body %s", rec.Code, rec.Body.String())
	}

	req = jsonRequest(http.MethodPost, "/api/v1/config/shutdown",
		`{"enabled":true,"warning_threshold":30,"critical_threshold":12}`)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("shutdown status %d body %s", rec.Code, rec.Body.String())
	}
	var body config.APIConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Notifications.Provider != "discord" || body.Shutdown.CriticalThreshold != 12 {
		t.Fatalf("merged view %+v", body)
	}
	if body.Notifications.WebhookURL != "" {
		t.Fatalf("discord webhook leaked in API: %+v", body.Notifications)
	}
	if body.Execution.Shutdown != config.ExecutionStoredOnly {
		t.Fatal("POST shutdown must not execute")
	}
	if body.Execution.Notifications != config.ExecutionReady {
		t.Fatalf("discord enabled should be ready: %+v", body.Execution)
	}
}

func TestConfigAtomicPersistenceAndMode(t *testing.T) {
	srv := newConfigServer(t, nil)
	path := srv.currentConfig().ConfigPath
	req := jsonRequest(http.MethodPut, "/api/v1/config",
		`{"shutdown":{"enabled":false,"warning_threshold":18,"critical_threshold":7}}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o, want 0600", info.Mode().Perm())
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file %s", e.Name())
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("invalid persisted JSON %s", raw)
	}

	req = jsonRequest(http.MethodPut, "/api/v1/config",
		`{"shutdown":{"warning_threshold":22,"critical_threshold":9}}`)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	loaded, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Shutdown.WarningThreshold != 22 || loaded.Shutdown.CriticalThreshold != 9 {
		t.Fatalf("atomic replace failed: %+v", loaded.Shutdown)
	}
}

func TestConfigSecretsNotLogged(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	srv := newConfigServer(t, log)
	secret := "https://hooks.example.invalid/whsec_TEST_SECRET_TOKEN_9f3a"
	putBody := `{"notifications":{"provider":"webhook","enabled":true,"webhook_url":"` + secret + `"}}`
	req := jsonRequest(http.MethodPut, "/api/v1/config", putBody)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	postBody := `{"provider":"webhook","enabled":true,"webhook_url":"` + secret + `","chat_id":"999"}`
	req = jsonRequest(http.MethodPost, "/api/v1/config/notifications", postBody)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post status %d body %s", rec.Code, rec.Body.String())
	}

	out := buf.String()
	for _, leak := range []string{secret, "whsec_TEST_SECRET_TOKEN_9f3a", "hooks.example.invalid"} {
		if strings.Contains(out, leak) {
			t.Fatalf("secret %q appeared in logs:\n%s", leak, out)
		}
	}
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "whsec_TEST_SECRET_TOKEN_9f3a") {
		t.Fatalf("secret appeared in API response: %s", rec.Body.String())
	}
}

func newConfigServer(t *testing.T, log *slog.Logger) *Server {
	t.Helper()
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConfigPath = filepath.Join(t.TempDir(), "lapguard", "config.json")
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return New(battery.NewMockProvider(), cfg, log, nil)
}
