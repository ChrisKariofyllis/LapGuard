package discovery

import (
	"regexp"
	"strings"
)

var tlpVersionRe = regexp.MustCompile(`(?i)\bTLP\s+([0-9]+(?:\.[0-9]+)*)`)

type toolScan struct {
	Tools     Tools
	TLPStat   string
	TLPCanSet bool
}

func detectTools(opts Options) toolScan {
	r := opts.Runner
	scan := toolScan{}

	if path, err := r.LookPath("tlp"); err == nil && path != "" {
		scan.Tools.TLP = true
		scan.Tools.TLPVersion = detectTLPVersion(r)
		out, can := tlpThresholdSupport(r)
		scan.TLPStat = out
		scan.TLPCanSet = can
		scan.Tools.TLPCanSet = can
	}
	if path, err := r.LookPath("upower"); err == nil && path != "" {
		scan.Tools.UPower = true
	}
	if path, err := r.LookPath("acpi"); err == nil && path != "" {
		scan.Tools.ACPI = true
	}
	if path, err := r.LookPath("tp-smapi-cli"); err == nil && path != "" {
		scan.Tools.TPSMAPI = true
	}
	if !scan.Tools.TPSMAPI {
		if path, err := r.LookPath("tpacpi-bat"); err == nil && path != "" {
			scan.Tools.TPSMAPI = true
		}
	}
	if path, err := r.LookPath("i8kctl"); err == nil && path != "" {
		scan.Tools.I8kutils = true
	} else if path, err := r.LookPath("i8kfan"); err == nil && path != "" {
		scan.Tools.I8kutils = true
	}

	return scan
}

func detectTLPVersion(r Runner) string {
	for _, args := range [][]string{{"--version"}, {"-v"}} {
		out, err := r.CombinedOutput("tlp", args...)
		if err == nil {
			if v := parseTLPVersion(string(out)); v != "" {
				return v
			}
		}
		out, err = r.CombinedOutput("tlp-stat", args...)
		if err == nil {
			if v := parseTLPVersion(string(out)); v != "" {
				return v
			}
		}
	}
	out, err := r.CombinedOutput("tlp-stat", "-s")
	if err == nil {
		return parseTLPVersion(string(out))
	}
	return ""
}

func parseTLPVersion(text string) string {
	m := tlpVersionRe.FindStringSubmatch(text)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func tlpThresholdSupport(r Runner) (output string, supported bool) {
	out, err := r.CombinedOutput("tlp-stat", "-b")
	text := string(out)
	if err != nil && text == "" {
		out2, err2 := r.CombinedOutput("tlp-stat", "-s")
		text = string(out2)
		if err2 != nil && text == "" {
			return "", false
		}
	}
	return text, tlpOutputSupportsThresholds(text)
}

func tlpOutputSupportsThresholds(text string) bool {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "no thresholds") || strings.Contains(lower, "no charge thresholds") {
		return false
	}
	if strings.Contains(lower, "unsupported hardware") && !strings.Contains(lower, "active (start") {
		// TLP lists inactive backends as "unsupported hardware". If any
		// backend is active with start/stop values, thresholds work.
		if !strings.Contains(lower, "= active") {
			return false
		}
	}
	if strings.Contains(lower, "cannot control") || strings.Contains(lower, "not configurable") {
		return false
	}
	if strings.Contains(lower, "active (start") || strings.Contains(lower, "start =") && strings.Contains(lower, "stop =") {
		return true
	}
	if strings.Contains(lower, "charge thresholds") && strings.Contains(lower, "= active") {
		return true
	}
	return false
}
