package discovery

import "testing"

func TestPowerFeatureSemanticsDerivedOnly(t *testing.T) {
	// Fujitsu A3510: no power_now, current_now × voltage_now available.
	report := CapabilityReport{
		Features: Features{
			PowerNow:              false,
			RawPowerNowSupported:  false,
			DerivedPowerSupported: true,
			CurrentVoltage:        true,
		},
	}
	feats := featureStatuses(report)
	raw := mustFeature(t, feats, "raw_power_now")
	if raw.Enabled {
		t.Fatal("raw_power_now_supported must be false when power_now is absent")
	}
	if raw.WhyNot == "" {
		t.Fatal("raw power_now should explain why_not")
	}
	derived := mustFeature(t, feats, "derived_power")
	if !derived.Enabled {
		t.Fatal("derived power must be enabled when current_now and voltage_now exist")
	}
	if derived.WhyNot != "" {
		t.Fatalf("derived power should not be marked unsupported: %+v", derived)
	}
	if derived.DetectionMethod != "derived:current_now*voltage_now" {
		t.Fatalf("detection_method %q", derived.DetectionMethod)
	}
}

func TestPowerFeatureSemanticsRawOnly(t *testing.T) {
	report := CapabilityReport{
		Features: Features{
			PowerNow:              true,
			RawPowerNowSupported:  true,
			DerivedPowerSupported: false,
			CurrentVoltage:        false,
		},
	}
	feats := featureStatuses(report)
	raw := mustFeature(t, feats, "raw_power_now")
	if !raw.Enabled {
		t.Fatal("raw power_now should be enabled")
	}
	derived := mustFeature(t, feats, "derived_power")
	if derived.Enabled {
		t.Fatal("derived power should be disabled without current/voltage")
	}
}

func TestPowerFeatureSemanticsNeither(t *testing.T) {
	report := CapabilityReport{}
	feats := featureStatuses(report)
	raw := mustFeature(t, feats, "raw_power_now")
	derived := mustFeature(t, feats, "derived_power")
	if raw.Enabled || derived.Enabled {
		t.Fatal("both power features should be disabled")
	}
	if derived.WhyNot == "" {
		t.Fatal("expected why_not when power cannot be estimated")
	}
}

func mustFeature(t *testing.T, feats []FeatureStatus, key string) FeatureStatus {
	t.Helper()
	for _, f := range feats {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("missing feature %s", key)
	return FeatureStatus{}
}
