package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"lapguard/internal/battery"
	"lapguard/internal/config"
	"lapguard/internal/power"
)

func TestPreflightIsReadOnlyAndNeverExecutes(t *testing.T) {
	srv := newConfigServer(t, nil)
	var ran bool
	srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
		ran = true
		return nil, nil
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/actions/preflight", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body actionPreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RealEnabled || !body.SafetyDryRun || body.CommandsExecuted || body.Ready {
		t.Fatalf("%+v", body)
	}
	if body.Executor != executorRecording {
		t.Fatalf("executor %q", body.Executor)
	}
	if body.Explanation != config.DiskEditRestartMessage {
		t.Fatalf("explanation %q", body.Explanation)
	}
	if !body.Config.DiskEditsRequireRestart || body.Config.Reload != config.ConfigReloadRestartRequired {
		t.Fatalf("config %+v", body.Config)
	}
	if body.AutomaticShutdown {
		t.Fatal("automatic_shutdown_executed")
	}
	if ran || srv.rec.Len() != 0 {
		t.Fatal("preflight must not execute")
	}
	assertNoCommandLeak(t, rec.Body.String())
}

func TestPreflightOmitsSecrets(t *testing.T) {
	srv := newConfigServer(t, nil)
	secret := "https://ntfy.example.invalid/preflight-secret"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", `{"notifications":{"provider":"ntfy","enabled":true,"webhook_url":"`+secret+`","dry_run":true}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("put %d %s", rec.Code, rec.Body.String())
	}
	pre := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pre, httptest.NewRequest(http.MethodGet, "/api/v1/actions/preflight", nil))
	if pre.Code != http.StatusOK {
		t.Fatalf("preflight %d %s", pre.Code, pre.Body.String())
	}
	raw := pre.Body.String()
	for _, leak := range []string{secret, "token_hash", "preflight-secret", "/usr/bin/", "systemctl"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("preflight leaked %q: %s", leak, raw)
		}
	}
}

func TestOnDiskConfigEditDoesNotChangeRunningServer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	initial := mustSaveTestConfig(t, path, false, true)

	loaded, err := config.Load([]string{"-config", path, "-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ConfigPath != path {
		t.Fatalf("config path %q", loaded.ConfigPath)
	}
	srv := New(battery.NewMockProvider(), loaded, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	status := getActionJSON(t, srv, "/api/v1/actions/status")
	if status.RealEnabled || !status.SafetyDryRun {
		t.Fatalf("status before edit %+v", status)
	}
	if status.Config.Source != config.ConfigSourceCLI {
		t.Fatalf("source %q", status.Config.Source)
	}
	if strings.Contains(status.Config.Path, dir) && status.Config.Path == path {
		t.Fatal("status leaked full config path")
	}

	edited := initial
	edited.Actions.RealEnabled = true
	edited.Safety.DryRun = false
	if err := edited.Save(path); err != nil {
		t.Fatal(err)
	}

	after := getActionJSON(t, srv, "/api/v1/actions/status")
	if after.RealEnabled || !after.SafetyDryRun {
		t.Fatal("running process must keep the startup config after a disk edit")
	}
	pre := getPreflight(t, srv)
	if pre.RealEnabled || !pre.SafetyDryRun || pre.CommandsExecuted {
		t.Fatalf("preflight after disk edit %+v", pre)
	}
	if !strings.Contains(pre.Explanation, "Restart LapGuard") {
		t.Fatalf("explanation %q", pre.Explanation)
	}

	restarted, err := config.Load([]string{"-config", path, "-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.Actions.RealEnabled || restarted.Safety.DryRun {
		t.Fatal("Load after restart must see the disk edit")
	}
	next := New(battery.NewMockProvider(), restarted, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	got := getActionJSON(t, next, "/api/v1/actions/status")
	if !got.RealEnabled || got.SafetyDryRun {
		t.Fatalf("restarted status %+v", got)
	}
}

func TestActionStatusIncludesConfigReload(t *testing.T) {
	srv := newConfigServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/actions/status", nil))
	var body actionStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Config.Source != config.ConfigSourceDefault {
		t.Fatalf("source %q", body.Config.Source)
	}
	if !body.Config.DiskEditsRequireRestart || !body.Config.APIUpdatesApplyImmediately {
		t.Fatalf("reload %+v", body.Config)
	}
	if body.RestartRequired != config.ConfigReloadRestartRequired {
		t.Fatalf("restart field %q", body.RestartRequired)
	}
	if !containsString(body.Warnings, "Restart LapGuard") {
		t.Fatalf("warnings %v", body.Warnings)
	}
}

func TestLogStartupAndPreflightStaySecretFreeTogether(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(config.NewRedactingHandler(slog.NewTextHandler(&buf, nil)))
	srv := newConfigServer(t, log)
	srv.currentConfig().LogStartup(log)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/actions/preflight", nil))
	combined := buf.String() + rec.Body.String()
	for _, leak := range []string{"token_hash", "webhook_url", "lg_", "Bearer "} {
		if strings.Contains(combined, leak) {
			t.Fatalf("leaked %q: %s", leak, combined)
		}
	}
}

func mustSaveTestConfig(t *testing.T, path string, realEnabled, dryRun bool) config.Config {
	t.Helper()
	cfg, err := config.Parse([]string{"-provider", "mock"})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Actions.RealEnabled = realEnabled
	cfg.Safety.DryRun = dryRun
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func getActionJSON(t *testing.T, srv *Server, path string) actionStatusResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
	}
	var body actionStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func getPreflight(t *testing.T, srv *Server) actionPreflightResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/actions/preflight", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preflight %d %s", rec.Code, rec.Body.String())
	}
	var body actionPreflightResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CommandsExecuted {
		t.Fatal("preflight commands_executed")
	}
	return body
}

func TestPreflightReportsLiveACAndBattery(t *testing.T) {
	srv := newConfigServer(t, nil)
	armHostState(srv, power.SourceBattery, "Discharging", 41)
	pre := getPreflight(t, srv)
	if pre.ACState != string(power.SourceBattery) || !pre.Discharging || pre.BatteryStatus != "Discharging" {
		t.Fatalf("%+v", pre)
	}
	if pre.BatteryPercent == nil || *pre.BatteryPercent != 41 {
		t.Fatalf("percent %+v", pre.BatteryPercent)
	}
}
