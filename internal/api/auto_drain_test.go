package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/power"
	"lapguard/internal/safety"
)

func TestAutoDrainStatusDefaultOff(t *testing.T) {
	srv := newConfigServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auto-drain/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body safety.AutoDrainSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Enabled || body.State != safety.AutoDrainIdle || body.CommandsExecuted {
		t.Fatalf("%+v", body)
	}
	if body.ThresholdPercent != config.DefaultAutoDrainThreshold {
		t.Fatalf("threshold %d", body.ThresholdPercent)
	}
	assertNoCommandLeak(t, rec.Body.String())
}

func TestAutoDrainWriteRequiresAuth(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableAuth(t, srv)
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodPut, "/api/v1/auto-drain/config", `{"enabled":false}`},
		{http.MethodPost, "/api/v1/auto-drain/respond", `{"action":"no"}`},
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, jsonRequest(tc.method, tc.path, tc.body))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status %d", tc.method, tc.path, rec.Code)
		}
	}
	get := httptest.NewRecorder()
	srv.Handler().ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/auto-drain/status", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status %d", get.Code)
	}
}

func TestAutoDrainPutConfigAndRespond(t *testing.T) {
	srv := newConfigServer(t, nil)
	put := httptest.NewRecorder()
	srv.Handler().ServeHTTP(put, jsonRequest(http.MethodPut, "/api/v1/auto-drain/config", `{
		"enabled":true,
		"battery_threshold_percent":10,
		"pre_notification_minutes":30,
		"response_timeout_minutes":10,
		"notification_services":["ntfy"],
		"on_user_no":"continue_on_battery"
	}`))
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}
	if !srv.currentConfig().AutoDrain.Enabled {
		t.Fatal("enabled not persisted")
	}
	if srv.currentConfig().Actions.RealEnabled {
		t.Fatal("PUT auto-drain must not enable real host actions")
	}

	idle := httptest.NewRecorder()
	srv.Handler().ServeHTTP(idle, jsonRequest(http.MethodPost, "/api/v1/auto-drain/respond", `{"action":"yes"}`))
	if idle.Code != http.StatusConflict {
		t.Fatalf("idle respond %d %s", idle.Code, idle.Body.String())
	}

	cfg := srv.currentConfig()
	cfg.Docker.StopEnabled = true
	cfg.Notifications = config.NotificationsConfig{
		Provider:   config.NotifyProviderNtfy,
		Enabled:    true,
		DryRun:     true,
		WebhookURL: "https://ntfy.example.invalid/lapguard",
	}
	srv.mu.Lock()
	srv.cfg = cfg
	srv.mu.Unlock()

	pct := 6
	snap := srv.AutoDrain().Tick(context.Background(), safety.Sample{
		Now:         time.Date(2026, 8, 18, 13, 0, 0, 0, time.UTC),
		Present:     true,
		Percent:     &pct,
		Status:      "Discharging",
		Discharging: true,
		Source:      power.SourceBattery,
	})
	if snap.State != safety.AutoDrainAwaitingResponse {
		t.Fatalf("tick %+v", snap)
	}

	no := httptest.NewRecorder()
	srv.Handler().ServeHTTP(no, jsonRequest(http.MethodPost, "/api/v1/auto-drain/respond", `{"action":"no"}`))
	if no.Code != http.StatusOK {
		t.Fatalf("respond %d %s", no.Code, no.Body.String())
	}
	var aborted safety.AutoDrainSnapshot
	if err := json.Unmarshal(no.Body.Bytes(), &aborted); err != nil {
		t.Fatal(err)
	}
	if aborted.State != safety.AutoDrainAborted || aborted.CommandsExecuted {
		t.Fatalf("%+v", aborted)
	}
}
