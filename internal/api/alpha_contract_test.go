package api

import (
	"encoding/json"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"lapguard/internal/config"
)

func TestPublicAlphaContract(t *testing.T) {
	srv := newConfigServer(t, nil)
	h := srv.Handler()

	cfg := srv.currentConfig()
	if cfg.Actions.RealEnabled {
		t.Fatal("default actions.real_enabled must be false")
	}
	if !cfg.Safety.DryRun {
		t.Fatal("default safety.dry_run must be true")
	}

	viewRec := httptest.NewRecorder()
	h.ServeHTTP(viewRec, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if viewRec.Code != http.StatusOK {
		t.Fatalf("GET config %d %s", viewRec.Code, viewRec.Body.String())
	}
	var view config.APIConfig
	if err := json.Unmarshal(viewRec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Actions.RealEnabled || view.Actions.Ready {
		t.Fatalf("config actions %+v", view.Actions)
	}
	if !view.Safety.DryRun {
		t.Fatal("config safety.dry_run")
	}
	if view.AuthEnabled || view.TokenConfigured {
		t.Fatal("auth must default off")
	}
	if strings.Contains(viewRec.Body.String(), "token_hash") || strings.Contains(viewRec.Body.String(), "webhook_url\":\"http") {
		t.Fatalf("config leaked secret: %s", viewRec.Body.String())
	}

	for _, path := range []string{
		"/api/v1/healthz",
		"/api/v1/telemetry",
		"/api/v1/capabilities",
		"/api/v1/discover",
		"/api/v1/actions/status",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s %d %s", path, rec.Code, rec.Body.String())
		}
		for _, leak := range []string{"systemctl", "docker stop", "$(", "/bin/sh", "token_hash", "/usr/bin/"} {
			if strings.Contains(rec.Body.String(), leak) {
				t.Fatalf("GET %s leaked %q: %s", path, leak, rec.Body.String())
			}
		}
	}

	statusRec := httptest.NewRecorder()
	h.ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/api/v1/actions/status", nil))
	var status actionStatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.RealEnabled || !status.SafetyDryRun || status.CommandsExecuted || status.AutomaticShutdown || status.Executor != executorRecording {
		t.Fatalf("status %+v", status)
	}
	if cfg.Docker.StopEnabled {
		t.Fatal("docker.stop_enabled must default false")
	}

	preview := httptest.NewRecorder()
	h.ServeHTTP(preview, jsonRequest(http.MethodPost, "/api/v1/actions/preview", `{}`))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview %d %s", preview.Code, preview.Body.String())
	}
	var prev actionResponse
	if err := json.Unmarshal(preview.Body.Bytes(), &prev); err != nil {
		t.Fatal(err)
	}
	if prev.CommandsExecuted || !prev.OK {
		t.Fatalf("preview %+v", prev)
	}
	assertNoCommandLeak(t, preview.Body.String())

	for _, tc := range []struct {
		path, body string
	}{
		{"/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`},
		{"/api/v1/actions/docker-drain", `{"confirm":"STOP_DOCKER"}`},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, jsonRequest(http.MethodPost, tc.path, tc.body))
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s status %d %s", tc.path, rec.Code, rec.Body.String())
		}
		var body actionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.OK || body.CommandsExecuted {
			t.Fatalf("%s executed: %+v", tc.path, body)
		}
		assertNoCommandLeak(t, rec.Body.String())
	}

	token := enableAuth(t, srv)
	h = srv.Handler()
	unauth := httptest.NewRecorder()
	h.ServeHTTP(unauth, jsonRequest(http.MethodPost, "/api/v1/actions/preview", `{}`))
	assertUnauthorized(t, unauth)

	getTel := httptest.NewRecorder()
	h.ServeHTTP(getTel, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil))
	if getTel.Code != http.StatusOK {
		t.Fatalf("GET telemetry with auth enabled %d", getTel.Code)
	}

	authed := jsonRequest(http.MethodPost, "/api/v1/actions/preview", `{}`)
	authed.Header.Set("Authorization", "Bearer "+token)
	okPrev := httptest.NewRecorder()
	h.ServeHTTP(okPrev, authed)
	if okPrev.Code != http.StatusOK {
		t.Fatalf("authed preview %d %s", okPrev.Code, okPrev.Body.String())
	}
	if strings.Contains(okPrev.Body.String(), token) {
		t.Fatal("preview returned token")
	}
}
