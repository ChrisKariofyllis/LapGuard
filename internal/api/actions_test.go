package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"
	"path/filepath"

	"lapguard/internal/config"
	"lapguard/internal/power"
	"lapguard/internal/safety"
	"lapguard/internal/storage"
)

func TestDefaultConfigNeverExecutesDockerDrain(t *testing.T) {
	srv := newConfigServer(t, nil)
	var ran bool
	srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
		ran = true
		return nil, nil
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/docker-drain", `{"confirm":"STOP_DOCKER"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if ran || srv.rec.Len() != 0 {
		t.Fatal("default docker drain must not execute")
	}
}

func TestPreviewNeverExecutes(t *testing.T) {
	srv := newConfigServer(t, nil)
	var ran bool
	srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
		ran = true
		return nil, nil
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/preview", `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body actionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CommandsExecuted || !body.OK {
		t.Fatalf("%+v", body)
	}
	if body.RealEnabled || !body.DryRun {
		t.Fatalf("defaults %+v", body)
	}
	if len(body.Plan) < 2 {
		t.Fatalf("plan %v", body.Plan)
	}
	assertNoCommandLeak(t, rec.Body.String())
	if ran || srv.rec.Len() != 0 {
		t.Fatal("preview must not invoke executors")
	}
}

func TestDefaultConfigNeverExecutesPoweroff(t *testing.T) {
	srv := newConfigServer(t, nil)
	var ran bool
	srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
		ran = true
		return nil, nil
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body actionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CommandsExecuted || body.OK {
		t.Fatalf("%+v", body)
	}
	if body.Error != errRealDisabled {
		t.Fatalf("error %q", body.Error)
	}
	assertNoCommandLeak(t, rec.Body.String())
	if ran || srv.rec.Len() != 0 {
		t.Fatal("default poweroff must not execute")
	}
}

func TestDryRunNeverExecutesEvenWhenRealEnabled(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableRealActions(t, srv, true)
	var ran bool
	srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
		ran = true
		return nil, nil
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errDryRun) {
		t.Fatalf("body %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"commands_executed":true`) {
		t.Fatal("dry-run executed")
	}
	if ran || srv.rec.Len() != 0 {
		t.Fatal("dry-run must not call executors")
	}
}

func TestPoweroffRequiresConfirmation(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"yes"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errConfirm) {
		t.Fatalf("body %s", rec.Body.String())
	}
	if srv.rec.Len() != 0 {
		t.Fatal("bad confirm must not execute")
	}
}

func TestDockerDrainRequiresConfirmation(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/docker-drain", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestPoweroffRejectsACConnected(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	setTestSource(srv, power.SourceAC)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errACConnected) {
		t.Fatalf("body %s", rec.Body.String())
	}
	if srv.rec.Len() != 0 {
		t.Fatal("AC connected must not execute")
	}
}

func TestPoweroffRejectsACUnknown(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	setTestSource(srv, power.SourceUnknown)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errACUnknown) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestCooldownPreventsDuplicateAction(t *testing.T) {
	srv := newConfigServer(t, nil)
	actor := safety.NewRecordingExecutor()
	enableManualReady(t, srv)
	setTestActor(srv, actor)
	setTestSource(srv, power.SourceBattery)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("first status %d %s", rec.Code, rec.Body.String())
	}
	var first actionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if !first.CommandsExecuted || !first.OK {
		t.Fatalf("first %+v", first)
	}
	assertNoCommandLeak(t, rec.Body.String())

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second status %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errCooldown) {
		t.Fatalf("body %s", rec.Body.String())
	}
	if actor.Len() != 2 {
		t.Fatalf("cooldown must not re-run executor, calls %v", actor.Calls())
	}
}

func TestRecordingExecutorRecordsManualActions(t *testing.T) {
	srv := newConfigServer(t, nil)
	actor := safety.NewRecordingExecutor()
	enableManualReady(t, srv)
	setTestActor(srv, actor)
	setTestSource(srv, power.SourceBattery)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	got := strings.Join(actor.Calls(), ",")
	if got != "sync,poweroff" {
		t.Fatalf("calls %s", got)
	}
}

func TestDockerDrainUsesRecordingExecutor(t *testing.T) {
	srv := newConfigServer(t, nil)
	actor := safety.NewRecordingExecutor()
	enableManualReady(t, srv)
	setTestActor(srv, actor)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/docker-drain", `{"confirm":"STOP_DOCKER"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(actor.Calls(), ","); got != "stop_docker" {
		t.Fatalf("calls %s", got)
	}
	assertNoCommandLeak(t, rec.Body.String())
}

func TestRealExecutorNotCalledByDefault(t *testing.T) {
	srv := newConfigServer(t, nil)
	var ran bool
	srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
		ran = true
		t.Error("real runner called")
		return nil, nil
	}
	for _, tc := range []struct {
		path, body string
	}{
		{"/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`},
		{"/api/v1/actions/docker-drain", `{"confirm":"STOP_DOCKER"}`},
		{"/api/v1/actions/preview", `{}`},
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, tc.path, tc.body))
		if ran {
			t.Fatalf("%s invoked real executor", tc.path)
		}
	}
}

func TestGatedSuccessWithoutStubRefusesHostCommands(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	setTestSource(srv, power.SourceBattery)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code == http.StatusOK {
		t.Fatal("tests must not succeed a real poweroff")
	}
	if strings.Contains(rec.Body.String(), `"commands_executed":true`) {
		t.Fatal("must not claim commands executed")
	}
	assertNoCommandLeak(t, rec.Body.String())
}

func TestActionAuditEvents(t *testing.T) {
	srv := newConfigServer(t, nil)
	store, err := storage.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv.AttachPower(nil, store)
	actor := safety.NewRecordingExecutor()
	enableManualReady(t, srv)
	setTestActor(srv, actor)
	setTestSource(srv, power.SourceBattery)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	rows, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, row := range rows {
		types = append(types, row.Type)
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, storage.AuditPowerOffAttempt) || !strings.Contains(joined, storage.AuditPowerOffResult) {
		t.Fatalf("audit %v", types)
	}
}

func TestAuthRequiredForActionEndpoints(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableAuth(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	assertUnauthorized(t, rec)
}

func enableRealActions(t *testing.T, srv *Server, dryRun bool) {
	t.Helper()
	body := `{"actions":{"real_enabled":true,"cooldown_seconds":60},"safety":{"dry_run":true}}`
	if !dryRun {
		body = `{"actions":{"real_enabled":true,"cooldown_seconds":60},"safety":{"dry_run":false}}`
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable real actions: %d %s", rec.Code, rec.Body.String())
	}
}

func enableManualReady(t *testing.T, srv *Server) {
	t.Helper()
	enableRealActions(t, srv, false)
}

func setTestActor(srv *Server, actor safety.ActionExecutor) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.testActor = actor
}

func setTestSource(srv *Server, src power.Source) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	cp := src
	srv.testSource = &cp
}

func assertNoCommandLeak(t *testing.T, body string) {
	t.Helper()
	for _, leak := range []string{"systemctl", "docker stop", "$(", "/bin/sh", "-c", "poweroff_path"} {
		if strings.Contains(body, leak) {
			t.Fatalf("response leaked command %q: %s", leak, body)
		}
	}
}

func TestPutConfigActionsStayDisabledByDefault(t *testing.T) {
	srv := newConfigServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", `{"actions":{"real_enabled":false}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body config.APIConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Actions.Ready || body.Execution.Shutdown != config.ExecutionDisabled {
		t.Fatalf("%+v", body)
	}
}
