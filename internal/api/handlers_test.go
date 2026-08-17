package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"lapguard/internal/battery"
	"lapguard/internal/config"
)

func TestTelemetryFromSysfsFixture(t *testing.T) {
	p := battery.NewSysfsProvider(sysfsFixture(t), "BAT0")
	srv := New(p, config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	var snap battery.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Provider != "sysfs" || !snap.Battery.Present {
		t.Fatalf("snapshot %+v", snap)
	}
	if snap.Battery.CapacityPercent == nil || *snap.Battery.CapacityPercent != 76 {
		t.Fatalf("capacity %+v", snap.Battery.CapacityPercent)
	}
	if snap.Battery.PowerW == nil || *snap.Battery.PowerW != -14.082 {
		t.Fatalf("power_w %+v", snap.Battery.PowerW)
	}
	if snap.Battery.HealthPercent == nil || *snap.Battery.HealthPercent != 84.2 {
		t.Fatalf("health %+v", snap.Battery.HealthPercent)
	}
}

func TestCapabilitiesFromSysfsFixture(t *testing.T) {
	p := battery.NewSysfsProvider(sysfsFixture(t), "BAT0")
	srv := New(p, config.Config{Listen: "127.0.0.1:8585", SysfsRoot: sysfsFixture(t)}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}

	var body capabilitiesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.App != "lapguard" || body.Provider != "sysfs" {
		t.Fatalf("body %+v", body)
	}
	if !body.BatteryPresent || body.BatteryName != "BAT0" {
		t.Fatalf("battery %+v", body)
	}
	if body.Features.Shutdown || body.Features.Docker || body.Features.Authentication {
		t.Fatalf("milestone 1 features must be disabled: %+v", body.Features)
	}
	if len(body.AvailableFields) == 0 {
		t.Fatal("expected available fields")
	}
}

func TestTelemetryWithoutBatteryIsOK(t *testing.T) {
	p := battery.NewSysfsProvider(t.TempDir(), "BAT0")
	srv := New(p, config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var snap battery.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Battery.Present {
		t.Fatal("expected present=false")
	}
}

func TestMockTelemetry(t *testing.T) {
	srv := New(battery.NewMockProvider(), config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestUnknownAPIIs404(t *testing.T) {
	srv := New(battery.NewMockProvider(), config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/shutdown", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

func sysfsFixture(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "testdata", "sysfs"))
}
