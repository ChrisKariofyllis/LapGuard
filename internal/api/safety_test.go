package api

import (
	"encoding/json"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"lapguard/internal/config"
	"lapguard/internal/safety"
)

func TestGetSafetyDryRun(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/safety", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body safety.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.DryRun || body.CommandsExecuted {
		t.Fatalf("%+v", body)
	}
	if !strings.Contains(body.Message, "no commands will be executed") {
		t.Fatalf("message %q", body.Message)
	}
	if body.WarningThreshold != 20 || body.CriticalThreshold != 10 {
		t.Fatalf("thresholds %+v", body)
	}
}

func TestSafetyTestSimulatesWithoutExecuting(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/safety/test", strings.NewReader(`{"scenario":"warning"}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("warning status %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "systemctl") || strings.Contains(rec.Body.String(), "docker stop") {
		t.Fatalf("response mentioned a host command: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/safety/test", strings.NewReader(`{"scenario":"critical"}`))
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("critical status %d body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"commands_executed":true`) {
		t.Fatal("simulate must not claim commands executed")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/safety/test", strings.NewReader(`{"scenario":"explode"}`))
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad scenario status %d", rec.Code)
	}
}

func TestGetConfigIncludesSafetyDefaults(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var body config.APIConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Safety.DryRun || !body.Safety.RequireACLoss {
		t.Fatalf("safety defaults %+v", body.Safety)
	}
}

func TestPutConfigCannotDisableSafetyDryRun(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(`{"safety":{"dry_run":false}}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body config.APIConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Safety.DryRun {
		t.Fatal("safety.dry_run must stay forced on")
	}
	loaded, err := config.LoadFile(srv.currentConfig().ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Safety.DryRun {
		t.Fatal("persisted safety.dry_run must stay true")
	}
}

func TestCapabilitiesBatterySafetyEnabled(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var body capabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.FeatureFlags.BatterySafety {
		t.Fatal("battery_safety should be true when the controller is initialized")
	}
	if body.FeatureFlags.GracefulShutdown {
		t.Fatal("graceful_shutdown must stay false")
	}
}
