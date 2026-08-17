package discovery

import "testing"

func TestTLPOutputSupportsThresholds(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`+++ Charge Thresholds
NATACPI    = active (start = 75, stop = 80)
`, true},
		{`+++ Charge Thresholds
NATACPI    = inactive (unsupported hardware)
tpacpi-bat = inactive (kernel module 'acpi_call' not installed)
`, false},
		{`No charge thresholds available`, false},
		{`Charge thresholds not configurable for this hardware`, false},
		{"", false},
	}
	for _, tc := range cases {
		if got := tlpOutputSupportsThresholds(tc.in); got != tc.want {
			t.Fatalf("got %v want %v for %q", got, tc.want, tc.in)
		}
	}
}

func TestParseTLPVersion(t *testing.T) {
	if got := parseTLPVersion("--- TLP 1.8.0 --------------------------------------------"); got != "1.8.0" {
		t.Fatalf("got %q", got)
	}
	if got := parseTLPVersion("TLP 1.6.1"); got != "1.6.1" {
		t.Fatalf("got %q", got)
	}
}
