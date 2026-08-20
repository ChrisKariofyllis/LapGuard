package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"lapguard/internal/actions/testfake"
	"lapguard/internal/config"
	"lapguard/internal/power"
	"lapguard/internal/storage"
)

func TestReleaseReadyActionDefaults(t *testing.T) {
	srv := newConfigServer(t, nil)
	cfg := srv.currentConfig()
	if cfg.Actions.RealEnabled {
		t.Fatal("actions.real_enabled must default false")
	}
	if !cfg.Safety.DryRun {
		t.Fatal("safety.dry_run must default true")
	}
	if cfg.Docker.StopEnabled {
		t.Fatal("docker.stop_enabled must default false")
	}

	h := srv.Handler()
	cfgRec := httptest.NewRecorder()
	h.ServeHTTP(cfgRec, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if cfgRec.Code != http.StatusOK {
		t.Fatalf("config %d %s", cfgRec.Code, cfgRec.Body.String())
	}
	var view config.APIConfig
	if err := json.Unmarshal(cfgRec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Actions.RealEnabled || view.Actions.Ready || !view.Safety.DryRun || view.Docker.StopEnabled {
		t.Fatalf("config view %+v", view)
	}

	statusRec := httptest.NewRecorder()
	h.ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/api/v1/actions/status", nil))
	var status actionStatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.RealEnabled || !status.SafetyDryRun || status.CommandsExecuted || status.AutomaticShutdown {
		t.Fatalf("status %+v", status)
	}
	if status.Executor != executorRecording {
		t.Fatalf("executor %q", status.Executor)
	}

	safetyRec := httptest.NewRecorder()
	h.ServeHTTP(safetyRec, httptest.NewRequest(http.MethodGet, "/api/v1/safety", nil))
	var snap struct {
		CommandsExecuted bool `json:"commands_executed"`
		DryRun           bool `json:"dry_run"`
	}
	if err := json.Unmarshal(safetyRec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.CommandsExecuted || !snap.DryRun {
		t.Fatalf("safety %+v", snap)
	}
}

func TestManualActionGates(t *testing.T) {
	tokenFor := func(t *testing.T, srv *Server) string {
		t.Helper()
		return enableAuth(t, srv)
	}
	type setup func(t *testing.T, srv *Server) *http.Request
	cases := []struct {
		name   string
		status int
		err    string
		prep   setup
	}{
		{
			name:   "authentication",
			status: http.StatusUnauthorized,
			prep: func(t *testing.T, srv *Server) *http.Request {
				enableManualReady(t, srv)
				tokenFor(t, srv)
				armHostState(srv, power.SourceBattery, "Discharging", 40)
				return remoteJSONRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`)
			},
		},
		{
			name:   "confirmation",
			status: http.StatusBadRequest,
			err:    errConfirm,
			prep: func(t *testing.T, srv *Server) *http.Request {
				enableManualReady(t, srv)
				armHostState(srv, power.SourceBattery, "Discharging", 40)
				return jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"poweroff"}`)
			},
		},
		{
			name:   "real_enabled",
			status: http.StatusConflict,
			err:    errRealDisabled,
			prep: func(t *testing.T, srv *Server) *http.Request {
				armHostState(srv, power.SourceBattery, "Discharging", 40)
				return jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`)
			},
		},
		{
			name:   "dry_run",
			status: http.StatusConflict,
			err:    errDryRun,
			prep: func(t *testing.T, srv *Server) *http.Request {
				enableRealActions(t, srv, true)
				armHostState(srv, power.SourceBattery, "Discharging", 40)
				return jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`)
			},
		},
		{
			name:   "known_ac_state",
			status: http.StatusConflict,
			err:    errACUnknown,
			prep: func(t *testing.T, srv *Server) *http.Request {
				enableManualReady(t, srv)
				armHostState(srv, power.SourceUnknown, "Discharging", 40)
				return jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`)
			},
		},
		{
			name:   "ac_disconnected",
			status: http.StatusConflict,
			err:    errACConnected,
			prep: func(t *testing.T, srv *Server) *http.Request {
				enableManualReady(t, srv)
				armHostState(srv, power.SourceAC, "Discharging", 40)
				return jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`)
			},
		},
		{
			name:   "battery_discharging",
			status: http.StatusConflict,
			err:    errNotDischarging,
			prep: func(t *testing.T, srv *Server) *http.Request {
				enableManualReady(t, srv)
				armHostState(srv, power.SourceBattery, "Not charging", 40)
				return jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newConfigServer(t, nil)
			var ran bool
			srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
				ran = true
				return nil, nil
			}
			req := tc.prep(t, srv)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d want %d body %s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.err != "" && !strings.Contains(rec.Body.String(), tc.err) {
				t.Fatalf("body %s want %q", rec.Body.String(), tc.err)
			}
			if rec.Code == http.StatusUnauthorized {
				assertUnauthorized(t, rec)
			}
			if strings.Contains(rec.Body.String(), `"commands_executed":true`) {
				t.Fatal("gate failure must not execute")
			}
			if ran {
				t.Fatal("real runner must not be called")
			}
			assertNoCommandLeak(t, rec.Body.String())
		})
	}
}

func TestCooldownAndInFlightGates(t *testing.T) {
	t.Run("cooldown", TestCooldownPreventsDuplicateAction)
	t.Run("in_flight", func(t *testing.T) {
		srv := newConfigServer(t, nil)
		enableManualReady(t, srv)
		dir := t.TempDir()
		setActionExecPaths(srv, filepath.Join(dir, "systemctl"), filepath.Join(dir, "docker"), filepath.Join(dir, "sync"))
		armHostState(srv, power.SourceBattery, "Discharging", 40)

		started := make(chan struct{})
		release := make(chan struct{})
		var once sync.Once
		srv.realRun = func(context.Context, string, ...string) ([]byte, error) {
			once.Do(func() { close(started) })
			<-release
			return nil, nil
		}

		done := make(chan int, 1)
		go func() {
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
			done <- rec.Code
		}()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("first action did not start")
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
		if rec.Code != http.StatusConflict {
			t.Fatalf("second status %d %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), errInFlight) {
			t.Fatalf("body %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"commands_executed":true`) {
			t.Fatal("in-flight retry must not execute")
		}
		close(release)
		if code := <-done; code != http.StatusOK {
			t.Fatalf("first action status %d", code)
		}
	})
}

func TestRealExecutorAPISuccessUsesFakeBinaries(t *testing.T) {
	h := testfake.New(t)
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	setActionExecPaths(srv, h.Path("systemctl"), h.Path("docker"), h.Path("sync"))
	srv.realRun = h.Runner()
	armHostState(srv, power.SourceBattery, "Discharging", 40)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	var body actionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || !body.CommandsExecuted || body.AutomaticShutdown {
		t.Fatalf("%+v", body)
	}
	assertNoCommandLeak(t, rec.Body.String())
	got := h.Joined()
	if len(got) != 2 || got[0] != h.Path("sync") || got[1] != h.Path("systemctl")+" poweroff" {
		t.Fatalf("argv %v", got)
	}
	for _, line := range got {
		if strings.Contains(line, "sh -c") || strings.Contains(line, "$(") {
			t.Fatalf("shell: %s", line)
		}
	}
}

func TestRealExecutorAPIMapsErrorsAndHidesOutput(t *testing.T) {
	h := testfake.New(t)
	h.Exit = "1"
	h.Stdout = "SECRET_OUTPUT_DO_NOT_LEAK systemctl poweroff $(reboot)\n"
	srv := newConfigServer(t, nil)
	store, err := storage.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv.AttachPower(nil, store)
	enableManualReady(t, srv)
	setActionExecPaths(srv, h.Path("systemctl"), h.Path("docker"), h.Path("sync"))
	srv.realRun = h.Runner()
	armHostState(srv, power.SourceBattery, "Discharging", 40)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"commands_executed":true`) {
		t.Fatal("failed exec must not claim success")
	}
	if !strings.Contains(rec.Body.String(), errExecutorUnavail) {
		t.Fatalf("body %s", rec.Body.String())
	}
	leaks := []string{"SECRET_OUTPUT_DO_NOT_LEAK", "$(reboot)", h.Path("systemctl"), h.Path("sync"), "LAPGUARD_TEST"}
	for _, leak := range leaks {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("HTTP leaked %q: %s", leak, rec.Body.String())
		}
	}
	assertNoCommandLeak(t, rec.Body.String())

	rows, err := store.ListAudit(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rows)
	for _, leak := range []string{"SECRET_OUTPUT_DO_NOT_LEAK", "$(reboot)", h.Dir, "systemctl", "/usr/bin/", "token", "Bearer"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("audit leaked %q: %s", leak, raw)
		}
	}
	var types []string
	for _, row := range rows {
		types = append(types, row.Type)
		if row.Type != storage.AuditPowerOffAttempt && row.Type != storage.AuditPowerOffResult {
			continue
		}
		if row.Reason != "attempt" && row.Reason != "failed" && row.Reason != "unavailable" {
			t.Fatalf("unexpected poweroff audit reason %q", row.Reason)
		}
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, storage.AuditPowerOffAttempt) || !strings.Contains(joined, storage.AuditPowerOffResult) {
		t.Fatalf("audit types %v", types)
	}
}

func TestDockerDrainFakeDoesNotStopInvalidIDs(t *testing.T) {
	h := testfake.New(t)
	h.Stdout = "aaaaaaaaaaaa\n$(docker ps)\n"
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	setActionExecPaths(srv, h.Path("systemctl"), h.Path("docker"), h.Path("sync"))
	srv.realRun = h.Runner()
	armHostState(srv, power.SourceBattery, "Discharging", 40)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/docker-drain", `{"confirm":"STOP_DOCKER"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	assertNoCommandLeak(t, rec.Body.String())
	got := h.Joined()
	if len(got) != 2 || got[0] != h.Path("docker")+" ps -q" || got[1] != h.Path("docker")+" stop aaaaaaaaaaaa" {
		t.Fatalf("argv %v", got)
	}
	if strings.Contains(strings.Join(got, "\n"), "$(docker") {
		t.Fatal("invalid docker id was passed to stop")
	}
}

func TestSafetySimulateNeverExecsRealRunner(t *testing.T) {
	h := testfake.New(t)
	srv := newConfigServer(t, nil)
	enableManualReady(t, srv)
	setActionExecPaths(srv, h.Path("systemctl"), h.Path("docker"), h.Path("sync"))
	srv.realRun = h.Runner()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/safety/test", `{"scenario":"critical"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"commands_executed":true`) {
		t.Fatal("safety test must not execute")
	}
	if len(h.Calls()) != 0 {
		t.Fatalf("safety simulate executed fakes: %v", h.Calls())
	}
}

func TestDefaultConfigNeverUsesFakeOrHostRunner(t *testing.T) {
	h := testfake.New(t)
	srv := newConfigServer(t, nil)
	srv.realRun = h.Runner()
	armHostState(srv, power.SourceBattery, "Discharging", 5)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if len(h.Calls()) != 0 {
		t.Fatalf("default config executed fakes: %v", h.Calls())
	}
}

func setActionExecPaths(srv *Server, poweroff, docker, sync string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.cfg.Actions.PowerOffPath = poweroff
	srv.cfg.Actions.DockerPath = docker
	srv.cfg.Actions.SyncPath = sync
}
