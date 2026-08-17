package power

import "time"

const (
	SourceAC      Source = "AC"
	SourceBattery Source = "BATTERY"
	SourceUnknown Source = "UNKNOWN"

	EventACConnected    = "AC_CONNECTED"
	EventACDisconnected = "AC_DISCONNECTED"
	EventACUnknown      = "AC_UNKNOWN"

	DefaultPollInterval = 5 * time.Second
	DefaultDebounce     = 10 * time.Second

	DetectionSource = "sysfs"
)

// Source is the classified mains/battery state.
type Source string

func (s Source) EventType() string {
	switch s {
	case SourceAC:
		return EventACConnected
	case SourceBattery:
		return EventACDisconnected
	default:
		return EventACUnknown
	}
}

func ValidEventType(v string) bool {
	switch v {
	case EventACConnected, EventACDisconnected, EventACUnknown:
		return true
	default:
		return false
	}
}

// Adapter is one type=Mains power_supply entry. Serial numbers are never stored.
type Adapter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Online   *bool  `json:"online"`
	Readable bool   `json:"readable"`
}

// Snapshot is one poll of all mains adapters.
type Snapshot struct {
	Timestamp          time.Time `json:"timestamp"`
	Source             Source    `json:"source"`
	Adapters           []Adapter `json:"adapters"`
	PowerLossDetection bool      `json:"power_loss_detection"`
	Reason             string    `json:"reason,omitempty"`
}

// Classify applies the multi-adapter rules:
//   - AC if at least one readable mains adapter is online
//   - BATTERY if every detected mains adapter is readable and offline
//   - UNKNOWN when there are no mains adapters, or any online file is missing/malformed
//     and no adapter is online
func Classify(adapters []Adapter) (Source, string) {
	if len(adapters) == 0 {
		return SourceUnknown, "no mains adapter detected"
	}
	anyOnline := false
	unreadable := 0
	for _, a := range adapters {
		if !a.Readable || a.Online == nil {
			unreadable++
			continue
		}
		if *a.Online {
			anyOnline = true
		}
	}
	if anyOnline {
		return SourceAC, ""
	}
	if unreadable > 0 {
		return SourceUnknown, "mains online attribute is missing or malformed"
	}
	return SourceBattery, ""
}

func (s Snapshot) HasReadableMains() bool {
	for _, a := range s.Adapters {
		if a.Readable && a.Online != nil {
			return true
		}
	}
	return false
}
