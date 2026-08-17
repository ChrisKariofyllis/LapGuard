package power

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scan lists type=Mains supplies under sysfsRoot. Adapter names are not hardcoded.
func Scan(sysfsRoot string) Snapshot {
	now := time.Now().UTC()
	snap := Snapshot{
		Timestamp: now,
		Adapters:  []Adapter{},
		Source:    SourceUnknown,
	}
	if strings.TrimSpace(sysfsRoot) == "" {
		snap.Reason = "no mains adapter detected"
		return snap
	}

	entries, err := os.ReadDir(sysfsRoot)
	if err != nil {
		snap.Reason = "no mains adapter detected"
		return snap
	}

	for _, entry := range entries {
		name := entry.Name()
		dir := filepath.Join(sysfsRoot, name)
		typ := readTrimmed(filepath.Join(dir, "type"))
		if !strings.EqualFold(typ, "Mains") {
			continue
		}
		adapter := Adapter{
			Name: name,
			Type: "Mains",
		}
		onlinePath := filepath.Join(dir, "online")
		raw, err := os.ReadFile(onlinePath)
		if err != nil {
			snap.Adapters = append(snap.Adapters, adapter)
			continue
		}
		switch strings.TrimSpace(string(raw)) {
		case "1":
			on := true
			adapter.Online = &on
			adapter.Readable = true
		case "0":
			on := false
			adapter.Online = &on
			adapter.Readable = true
		}
		snap.Adapters = append(snap.Adapters, adapter)
	}

	snap.Source, snap.Reason = Classify(snap.Adapters)
	snap.PowerLossDetection = snap.HasReadableMains()
	return snap
}

func readTrimmed(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
