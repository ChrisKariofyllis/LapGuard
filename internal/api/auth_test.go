package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lapguard/internal/config"
	"lapguard/internal/storage"
)

func TestAuthDefaultOnLoopbackAllowsWriteWithoutToken(t *testing.T) {
	srv := newConfigServer(t, nil)
	if !srv.currentConfig().Auth.Enabled || !srv.currentConfig().Auth.AllowLoopbackNoToken {
		t.Fatalf("defaults %+v", srv.currentConfig().Auth)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("loopback PUT %d body %s", rec.Code, rec.Body.String())
	}
}

func TestRemoteWriteRequiresTokenWhenAuthEnabled(t *testing.T) {
	srv := newConfigServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, remoteJSONRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`))
	assertUnauthorized(t, rec)

	token := enableAuth(t, srv)
	ok := remoteJSONRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":22,"critical_threshold":8}}`)
	ok.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, ok)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed remote PUT %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoopbackDetectionHostAndPeer(t *testing.T) {
	cases := []struct {
		name, host, remote string
		want               bool
	}{
		{"ipv4", "127.0.0.1:8585", "127.0.0.1:9", true},
		{"localhost", "localhost:8585", "127.0.0.1:9", true},
		{"ipv6", "[::1]:8585", "[::1]:9", true},
		{"tailscale host from loopback peer", "example.ts.net", "127.0.0.1:9", false},
		{"spoofed host from remote", "127.0.0.1:8585", "192.0.2.10:4444", false},
		{"remote", "example.ts.net", "192.0.2.10:4444", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", nil)
		req.Host = tc.host
		req.RemoteAddr = tc.remote
		if got := isLoopback(req); got != tc.want {
			t.Fatalf("%s: isLoopback=%t want %t", tc.name, got, tc.want)
		}
	}
}

func TestAuthDisabledPreservesLocalDevelopment(t *testing.T) {
	srv := newConfigServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET telemetry %d", rec.Code)
	}
}

func TestAuthMissingAndInvalidToken401(t *testing.T) {
	srv := newConfigServer(t, nil)
	token := enableAuth(t, srv)

	missing := remoteJSONRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, missing)
	assertUnauthorized(t, rec)
	missingBody := rec.Body.String()

	invalid := remoteJSONRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	invalid.Header.Set("Authorization", "Bearer definitely-not-"+token)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, invalid)
	assertUnauthorized(t, rec)
	if rec.Body.String() != missingBody {
		t.Fatalf("missing vs invalid must use the same body:\n%s\n%s", missingBody, rec.Body.String())
	}

	valid := remoteJSONRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	valid.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, valid)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token status %d %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedPOSTAndPUT(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableAuth(t, srv)
	paths := []struct {
		method, path, body string
	}{
		{http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":22,"critical_threshold":8}}`},
		{http.MethodPost, "/api/v1/config/notifications", `{"provider":"none"}`},
		{http.MethodPost, "/api/v1/config/shutdown", `{"enabled":false,"warning_threshold":20,"critical_threshold":10}`},
		{http.MethodPost, "/api/v1/actions/test-notification", `{}`},
		{http.MethodPost, "/api/v1/actions/preview", `{}`},
		{http.MethodPost, "/api/v1/actions/poweroff", `{"confirm":"POWER_OFF"}`},
		{http.MethodPost, "/api/v1/actions/docker-drain", `{"confirm":"STOP_DOCKER"}`},
		{http.MethodPost, "/api/v1/safety/test", `{"scenario":"warning"}`},
		{http.MethodPut, "/api/v1/auto-drain/config", `{"enabled":false}`},
		{http.MethodPost, "/api/v1/auto-drain/respond", `{"action":"no"}`},
	}
	for _, tc := range paths {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, remoteJSONRequest(tc.method, tc.path, tc.body))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestGETTelemetryReadableWhenAuthEnabled(t *testing.T) {
	srv := newConfigServer(t, nil)
	enableAuth(t, srv)
	for _, path := range []string{
		"/api/v1/telemetry",
		"/api/v1/capabilities",
		"/api/v1/discover",
		"/api/v1/power",
		"/api/v1/events",
		"/api/v1/safety",
		"/api/v1/auto-drain/status",
		"/api/v1/healthz",
		"/api/v1/auth/status",
		"/api/v1/actions/status",
		"/api/v1/actions/preflight",
		"/api/v1/config",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status %d body %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestWrongContentTypeRejected(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(`{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestTrailingJSONRejected(t *testing.T) {
	srv := newConfigServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"enabled":true,"warning_threshold":20,"critical_threshold":10}}{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestCrossOriginStateChangeRejected(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	req.Host = "127.0.0.1:8585"
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatal("must not use wildcard CORS")
	}
}

func TestSameOriginStateChangeAllowed(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	req.Host = "127.0.0.1:8585"
	req.Header.Set("Origin", "http://127.0.0.1:8585")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestViteOriginAllowed(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	req.Host = "127.0.0.1:8585"
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestNoOriginWorksForCLI(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestConfigAndAuthStatusNeverReturnToken(t *testing.T) {
	srv := newConfigServer(t, nil)
	token := enableAuth(t, srv)
	for _, path := range []string{"/api/v1/config", "/api/v1/auth/status"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		body := rec.Body.String()
		if strings.Contains(body, token) || strings.Contains(body, "token_hash") || strings.Contains(body, srv.currentConfig().Auth.TokenHash) {
			t.Fatalf("%s leaked secret: %s", path, body)
		}
		if !strings.Contains(body, `"auth_enabled":true`) && !strings.Contains(body, `"auth_enabled": true`) {
			t.Fatalf("%s missing auth_enabled: %s", path, body)
		}
	}
}

func TestAuthDisableRemoteRejectedWhenUnauthenticated(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPost, "/api/v1/auth/disable", `{}`)
	req.RemoteAddr = "192.0.2.10:4444"
	req.Host = "example.ts.net"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAuthDisableLocalConsoleWhenDisabled(t *testing.T) {
	srv := newConfigServer(t, nil)
	req := jsonRequest(http.MethodPost, "/api/v1/auth/disable", `{}`)
	req.RemoteAddr = "127.0.0.1:9"
	req.Host = "127.0.0.1:8585"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAuthDisableWithToken(t *testing.T) {
	srv := newConfigServer(t, nil)
	token := enableAuth(t, srv)
	req := jsonRequest(http.MethodPost, "/api/v1/auth/disable", `{}`)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if srv.currentConfig().Auth.Enabled {
		t.Fatal("expected disabled")
	}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("after disable PUT %d %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRotateHTTPDoesNotReturnToken(t *testing.T) {
	srv := newConfigServer(t, nil)
	token := enableAuth(t, srv)
	req := jsonRequest(http.MethodPost, "/api/v1/auth/rotate", `{}`)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), token) || strings.Contains(rec.Body.String(), "lg_") {
		t.Fatalf("rotate leaked token: %s", rec.Body.String())
	}
	if !srv.currentConfig().Auth.VerifyToken(token) {
		t.Fatal("HTTP rotate must not mint/invalidate; CLI does that")
	}
}

func TestTokenPlaintextNeverLogged(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(config.NewRedactingHandler(slog.NewTextHandler(&buf, nil)))
	srv := newConfigServer(t, log)
	token := enableAuth(t, srv)
	req := jsonRequest(http.MethodPut, "/api/v1/config", `{"notifications":{"provider":"ntfy","enabled":true,"webhook_url":"https://ntfy.example.invalid/secret-topic"}}`)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	out := buf.String()
	if strings.Contains(out, token) {
		t.Fatalf("plaintext token logged:\n%s", out)
	}
	if strings.Contains(out, "secret-topic") || strings.Contains(out, "ntfy.example.invalid") {
		t.Fatalf("webhook leaked in logs:\n%s", out)
	}
}

func TestAuditEventsContainNoSecrets(t *testing.T) {
	srv := newConfigServer(t, nil)
	store, err := storage.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv.AttachPower(nil, store)
	token := enableAuth(t, srv)

	bad := jsonRequest(http.MethodPut, "/api/v1/config", `{"notifications":{"provider":"ntfy","webhook_url":"https://hooks.example.invalid/whsec"}}`)
	bad.Header.Set("Authorization", "Bearer "+token+"x")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, bad)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}

	ok := jsonRequest(http.MethodPut, "/api/v1/config", `{"notifications":{"provider":"ntfy","enabled":true,"webhook_url":"https://hooks.example.invalid/whsec"}}`)
	ok.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, ok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}

	rows, err := store.ListAudit(t.Context(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected audit rows")
	}
	raw, _ := json.Marshal(rows)
	s := string(raw)
	for _, leak := range []string{token, "whsec", "hooks.example.invalid", "Authorization", "Bearer "} {
		if strings.Contains(s, leak) {
			t.Fatalf("audit leaked %q: %s", leak, s)
		}
	}
}

func TestSafetyAndConfigDoNotExecuteHostCommands(t *testing.T) {
	srv := newConfigServer(t, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPut, "/api/v1/config", `{"docker":{"stop_enabled":true},"shutdown":{"enabled":true,"warning_threshold":20,"critical_threshold":10}}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, jsonRequest(http.MethodPost, "/api/v1/safety/test", `{"scenario":"critical"}`))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	body := rec.Body.String()
	for _, cmd := range []string{"docker stop", "systemctl poweroff", "systemctl reboot", "/sbin/shutdown", "reboot -h"} {
		if strings.Contains(body, cmd) {
			t.Fatalf("response mentioned %q: %s", cmd, body)
		}
	}
	if strings.Contains(body, `"commands_executed":true`) {
		t.Fatal("commands_executed")
	}
	snap := srv.Safety().Snapshot()
	if snap.CommandsExecuted {
		t.Fatal("executor ran host commands")
	}
}

func TestQueryTokenRejected(t *testing.T) {
	srv := newConfigServer(t, nil)
	token := enableAuth(t, srv)
	req := jsonRequest(http.MethodPut, "/api/v1/config?token="+token, `{"shutdown":{"warning_threshold":21,"critical_threshold":9}}`)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tokens in URLs must be rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func enableAuth(t *testing.T, srv *Server) string {
	t.Helper()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	token, err := srv.cfg.GenerateToken(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.cfg.Save(srv.cfg.ConfigPath); err != nil {
		t.Fatal(err)
	}
	return token
}

func assertUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control %q", rec.Header().Get("Cache-Control"))
	}
	if !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}
