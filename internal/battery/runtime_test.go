package battery

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestEstimateRuntimeFiveHours(t *testing.T) {
	energy := 50.0
	power := 10.0
	est := EstimateRuntime(true, "Discharging", &energy, &power)
	if !est.Available || est.Seconds == nil || *est.Seconds != 18000 {
		t.Fatalf("%+v", est)
	}
	if est.Hours == nil || *est.Hours != 5 {
		t.Fatalf("hours %+v", est.Hours)
	}
	if est.Reason != RuntimeReasonDischarging {
		t.Fatalf("reason %q", est.Reason)
	}
}

func TestEstimateRuntimeNegativeDischargePower(t *testing.T) {
	energy := 50.0
	power := -10.0
	est := EstimateRuntime(true, "Discharging", &energy, &power)
	if !est.Available || est.Seconds == nil || *est.Seconds != 18000 {
		t.Fatalf("negative power should use abs: %+v", est)
	}
}

func TestEstimateRuntimeChargingUnavailable(t *testing.T) {
	energy := 50.0
	power := 10.0
	est := EstimateRuntime(true, "Charging", &energy, &power)
	assertRuntimeUnavailable(t, est, RuntimeReasonOnAC)
}

func TestEstimateRuntimeNotChargingZeroPower(t *testing.T) {
	energy := 32.0
	power := 0.0
	est := EstimateRuntime(true, "Not charging", &energy, &power)
	assertRuntimeUnavailable(t, est, RuntimeReasonOnAC)
}

func TestEstimateRuntimeFullUnavailable(t *testing.T) {
	energy := 42.0
	power := 0.0
	est := EstimateRuntime(true, "Full", &energy, &power)
	assertRuntimeUnavailable(t, est, RuntimeReasonOnAC)
}

func TestEstimateRuntimeZeroDischargePower(t *testing.T) {
	energy := 32.0
	power := 0.0
	est := EstimateRuntime(true, "Discharging", &energy, &power)
	assertRuntimeUnavailable(t, est, RuntimeReasonZeroPower)
}

func TestEstimateRuntimeMissingEnergy(t *testing.T) {
	power := -10.0
	est := EstimateRuntime(true, "Discharging", nil, &power)
	assertRuntimeUnavailable(t, est, RuntimeReasonNoEnergy)
}

func TestEstimateRuntimeMissingPower(t *testing.T) {
	energy := 32.0
	est := EstimateRuntime(true, "Discharging", &energy, nil)
	assertRuntimeUnavailable(t, est, RuntimeReasonNoPower)
}

func TestEstimateRuntimeNoInfinityOrNaNInJSON(t *testing.T) {
	cases := []RuntimeEstimate{
		EstimateRuntime(true, "Discharging", ptr(50.0), ptr(10.0)),
		EstimateRuntime(true, "Discharging", ptr(50.0), ptr(0.0)),
		EstimateRuntime(true, "Charging", ptr(50.0), ptr(10.0)),
		EstimateRuntime(true, "Discharging", ptr(math.Inf(1)), ptr(10.0)),
		EstimateRuntime(true, "Discharging", ptr(50.0), ptr(math.NaN())),
		EstimateRuntime(true, "Discharging", ptr(50.0), ptr(math.Inf(-1))),
		EstimateRuntime(false, "Discharging", ptr(50.0), ptr(10.0)),
	}
	for i, est := range cases {
		b := Battery{
			Present:                   i != len(cases)-1,
			EstimatedRuntimeSeconds:   est.Seconds,
			EstimatedRuntimeHours:     est.Hours,
			EstimatedRuntimeAvailable: est.Available,
		}
		if est.Reason != "" {
			r := est.Reason
			b.EstimatedRuntimeReason = &r
		}
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("case %d marshal: %v", i, err)
		}
		s := string(raw)
		if strings.Contains(s, "Infinity") || strings.Contains(s, "NaN") || strings.Contains(s, "+Inf") || strings.Contains(s, "-Inf") {
			t.Fatalf("case %d leaked non-finite JSON: %s", i, s)
		}
		if est.Available {
			continue
		}
		if !strings.Contains(s, `"estimated_runtime_seconds":null`) || !strings.Contains(s, `"estimated_runtime_hours":null`) {
			t.Fatalf("case %d should null runtime fields: %s", i, s)
		}
		if strings.Contains(s, `"estimated_runtime_available":true`) {
			t.Fatalf("case %d should not be available: %s", i, s)
		}
	}
}

func TestSysfsFixtureRuntime(t *testing.T) {
	p := NewSysfsProvider(sysfsFixture(t), "BAT0")
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b := snap.Battery
	if !b.EstimatedRuntimeAvailable || b.EstimatedRuntimeSeconds == nil {
		t.Fatalf("fixture is discharging: %+v", b)
	}
	want := int(math.Round((32.0 / 14.082) * 3600))
	if *b.EstimatedRuntimeSeconds != want {
		t.Fatalf("seconds %d, want %d", *b.EstimatedRuntimeSeconds, want)
	}
}

func TestFormatEstimatedRuntime(t *testing.T) {
	if got := FormatEstimatedRuntime(42*60, true); got != "42 min" {
		t.Fatalf("got %q", got)
	}
	if got := FormatEstimatedRuntime(5*3600+12*60, true); got != "5h 12m" {
		t.Fatalf("got %q", got)
	}
	if got := FormatEstimatedRuntime(86400+3*3600, true); got != "1d 3h" {
		t.Fatalf("got %q", got)
	}
	if got := FormatEstimatedRuntime(18000, false); got != "—" {
		t.Fatalf("unavailable got %q", got)
	}
	if got := FormatEstimatedRuntime(0, true); got != "—" {
		t.Fatalf("zero got %q", got)
	}
}

func assertRuntimeUnavailable(t *testing.T, est RuntimeEstimate, reason string) {
	t.Helper()
	if est.Available {
		t.Fatalf("expected unavailable, got %+v", est)
	}
	if est.Seconds != nil || est.Hours != nil {
		t.Fatalf("seconds/hours must be nil: %+v", est)
	}
	if est.Reason != reason {
		t.Fatalf("reason %q, want %q", est.Reason, reason)
	}
}
