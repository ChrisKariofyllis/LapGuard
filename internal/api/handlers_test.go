package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"lapguard/internal/battery"
	"lapguard/internal/config"
	"lapguard/internal/discovery"
)

func TestTelemetryFromSysfsFixture(t *testing.T) {
	p := battery.NewSysfsProvider(sysfsFixture(t), "BAT0")
	srv := New(p, config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

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
	if !snap.Battery.EstimatedRuntimeAvailable || snap.Battery.EstimatedRuntimeSeconds == nil {
		t.Fatal("fixture is discharging and should include estimated runtime")
	}
	if !contains(snap.AvailableFields, "energy_full") {
		t.Fatalf("available_fields %v", snap.AvailableFields)
	}
}

func TestCapabilitiesFromSysfsFixture(t *testing.T) {
	p := battery.NewSysfsProvider(sysfsFixture(t), "BAT0")
	report := discovery.CapabilityReport{
		Timestamp: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
		Hostname:  "fixture",
		OS:        "Ubuntu Test",
		Kernel:    "6.17.0-test",
		Battery: discovery.BatteryIdentity{
			Path:         sysfsFixture(t) + "/BAT0",
			Name:         "BAT0",
			Present:      true,
			Manufacturer: "LGC",
			Model:        "FixturePack",
			Serial:       "TEST-BAT-001",
			Technology:   "Li-ion",
		},
		AvailableFields: []string{"status", "capacity", "energy_full"},
		Features: discovery.Features{
			ChargeThresholds:      discovery.MethodNone,
			CycleCount:            true,
			PowerNow:              false,
			RawPowerNowSupported:  false,
			DerivedPowerSupported: true,
			CurrentVoltage:        true,
			Temperature:           true,
		},
		AvailableTools: discovery.Tools{TLP: true, TLPVersion: "1.8.0"},
		KernelModules:  []string{"fujitsu_laptop"},
		Thresholds: discovery.ThresholdPlan{
			Method:          discovery.MethodNone,
			DetectionMethod: "sysfs+tlp+thinkpad_acpi",
			WhyNot:          "fujitsu_laptop is loaded but did not register charge control; TLP is installed but cannot control this hardware.",
			Recommendation:  "Keep the pack between ~20–80% with a smart plug.",
		},
	}
	srv := New(p, config.Config{Listen: "127.0.0.1:8585", SysfsRoot: sysfsFixture(t)}, slog.New(slog.NewTextHandler(io.Discard, nil)), discovery.Static{Report: report})

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
	if body.ThresholdMethod != "none" {
		t.Fatalf("threshold_method %q", body.ThresholdMethod)
	}
	if len(body.AvailableFields) == 0 {
		t.Fatal("expected available fields")
	}
	if len(body.Features) == 0 {
		t.Fatal("expected feature statuses")
	}
	foundCharge := false
	for _, f := range body.Features {
		if f.Key != "charge_thresholds" {
			continue
		}
		foundCharge = true
		if f.Enabled {
			t.Fatal("charge thresholds should be disabled on the fixture")
		}
		if f.DetectionMethod == "" || f.WhyNot == "" || f.Recommendation == "" {
			t.Fatalf("charge feature incomplete: %+v", f)
		}
	}
	if !foundCharge {
		t.Fatal("missing charge_thresholds feature")
	}
	if body.FeatureFlags.RawPowerNowSupported || body.FeatureFlags.PowerNow {
		t.Fatal("raw_power_now_supported must be false when power_now is absent")
	}
	if !body.FeatureFlags.DerivedPowerSupported {
		t.Fatal("derived_power_supported must be true when current and voltage exist")
	}
	var raw, derived *discovery.FeatureStatus
	for i := range body.Features {
		switch body.Features[i].Key {
		case "raw_power_now":
			raw = &body.Features[i]
		case "derived_power":
			derived = &body.Features[i]
		}
	}
	if raw == nil || derived == nil {
		t.Fatal("expected raw_power_now and derived_power features")
	}
	if raw.Enabled {
		t.Fatal("raw power_now should not be enabled")
	}
	if !derived.Enabled || derived.WhyNot != "" {
		t.Fatalf("derived power should be enabled without why_not: %+v", derived)
	}
	if !body.Tools.TLP || body.Tools.TLPVersion != "1.8.0" {
		t.Fatalf("tools %+v", body.Tools)
	}
}

func TestDiscoverEndpoint(t *testing.T) {
	p := battery.NewMockProvider()
	report := discovery.CapabilityReport{
		Hostname: "think-test",
		Features: discovery.Features{ChargeThresholds: discovery.MethodTLP, CycleCount: true},
		Thresholds: discovery.ThresholdPlan{
			Method:          discovery.MethodTLP,
			DetectionMethod: "tlp",
			Recommendation:  "Use TLP setcharge",
		},
		KernelModules: []string{"thinkpad_acpi"},
	}
	srv := New(p, config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)), discovery.Static{Report: report})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/discover", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body discovery.CapabilityReport
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Features.ChargeThresholds != "tlp" {
		t.Fatalf("report %+v", body.Features)
	}
	if len(body.KernelModules) != 1 || body.KernelModules[0] != "thinkpad_acpi" {
		t.Fatalf("modules %v", body.KernelModules)
	}
}

func TestTelemetryWithoutBatteryIsOK(t *testing.T) {
	p := battery.NewSysfsProvider(t.TempDir(), "BAT0")
	srv := New(p, config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

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
	if snap.Battery.EstimatedRuntimeAvailable {
		t.Fatal("no battery must not estimate runtime")
	}
	if snap.Battery.EstimatedRuntimeSeconds != nil || snap.Battery.EstimatedRuntimeHours != nil {
		t.Fatalf("runtime must be null: %+v", snap.Battery)
	}
	if snap.Battery.EstimatedRuntimeReason == nil || *snap.Battery.EstimatedRuntimeReason != battery.RuntimeReasonNoBattery {
		t.Fatalf("reason %+v", snap.Battery.EstimatedRuntimeReason)
	}
}

func TestMockTelemetry(t *testing.T) {
	srv := New(battery.NewMockProvider(), config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
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
	if !snap.Battery.EstimatedRuntimeAvailable || snap.Battery.EstimatedRuntimeSeconds == nil {
		t.Fatal("live mock should include estimated runtime")
	}
}

func TestRootJSONWhenFrontendUnavailable(t *testing.T) {
	srv := New(battery.NewMockProvider(), config.Config{Listen: "127.0.0.1:8585", WebDir: "none"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["app"] != "lapguard" {
		t.Fatalf("body %+v", body)
	}
}

func TestUnknownAPIIs404(t *testing.T) {
	srv := New(battery.NewMockProvider(), config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
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

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
