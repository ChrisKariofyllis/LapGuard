package power

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanDoesNotHardcodeAdapterNames(t *testing.T) {
	root := t.TempDir()
	writeMains(t, root, "ADP1", "1")
	writeMains(t, root, "weird-dock", "0")
	writeFile(t, filepath.Join(root, "BAT1", "type"), "Battery")
	writeFile(t, filepath.Join(root, "BAT1", "online"), "1")

	snap := Scan(root)
	if len(snap.Adapters) != 2 {
		t.Fatalf("adapters %+v", snap.Adapters)
	}
	if snap.Source != SourceAC {
		t.Fatalf("source %s, want AC (at least one online)", snap.Source)
	}
	if !snap.PowerLossDetection {
		t.Fatal("readable mains online should enable power_loss_detection")
	}
}

func TestClassifyMultipleAdapters(t *testing.T) {
	on, off := true, false
	src, _ := Classify([]Adapter{
		{Name: "A", Readable: true, Online: &off},
		{Name: "B", Readable: true, Online: &on},
	})
	if src != SourceAC {
		t.Fatalf("got %s", src)
	}
	src, _ = Classify([]Adapter{
		{Name: "A", Readable: true, Online: &off},
		{Name: "B", Readable: true, Online: &off},
	})
	if src != SourceBattery {
		t.Fatalf("got %s, want BATTERY when all mains are offline", src)
	}
}

func TestClassifyMissingOnlineIsUnknown(t *testing.T) {
	src, reason := Classify(nil)
	if src != SourceUnknown || reason == "" {
		t.Fatalf("%s %q", src, reason)
	}
	off := false
	src, _ = Classify([]Adapter{
		{Name: "A", Readable: true, Online: &off},
		{Name: "B", Readable: false},
	})
	if src != SourceUnknown {
		t.Fatalf("got %s, want UNKNOWN when an online file is unreadable and none are online", src)
	}
}

func TestScanMalformedOnlineIsUnknown(t *testing.T) {
	root := t.TempDir()
	writeMains(t, root, "AC", "maybe")
	snap := Scan(root)
	if snap.Source != SourceUnknown {
		t.Fatalf("source %s", snap.Source)
	}
	if snap.PowerLossDetection {
		t.Fatal("malformed online is not a readable mains attribute")
	}
}

func TestScanMissingOnlineFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AC", "type"), "Mains")
	snap := Scan(root)
	if snap.Source != SourceUnknown || len(snap.Adapters) != 1 {
		t.Fatalf("%+v", snap)
	}
}

func TestWatcherBaselineEmitsNothing(t *testing.T) {
	fx := newFixture(t, "1")
	w := newTestWatcher(fx)
	if tr := w.Tick(); tr != nil {
		t.Fatalf("baseline emitted %+v", tr)
	}
	if tr := w.Tick(); tr != nil {
		t.Fatalf("unchanged AC emitted %+v", tr)
	}
}

func TestWatcherDisconnectAfterDebounce(t *testing.T) {
	fx := newFixture(t, "1")
	w := newTestWatcher(fx)
	w.Tick()
	fx.setOnline("0")
	if tr := w.Tick(); tr != nil {
		t.Fatalf("emitted before debounce: %+v", tr)
	}
	fx.advance(5 * time.Second)
	if tr := w.Tick(); tr != nil {
		t.Fatalf("emitted at 5s: %+v", tr)
	}
	fx.advance(5 * time.Second)
	tr := w.Tick()
	if tr == nil || tr.Type != EventACDisconnected {
		t.Fatalf("got %+v", tr)
	}
}

func TestWatcherReconnectAfterDebounce(t *testing.T) {
	fx := newFixture(t, "0")
	w := newTestWatcher(fx)
	w.Tick()
	fx.setOnline("1")
	if tr := w.Tick(); tr != nil {
		t.Fatalf("emitted before debounce: %+v", tr)
	}
	fx.advance(10 * time.Second)
	tr := w.Tick()
	if tr == nil || tr.Type != EventACConnected {
		t.Fatalf("got %+v", tr)
	}
	if tr.DurationMs == nil || *tr.DurationMs != (10*time.Second).Milliseconds() {
		t.Fatalf("duration %+v", tr.DurationMs)
	}
}

func TestWatcherNoDuplicateEvents(t *testing.T) {
	fx := newFixture(t, "1")
	w := newTestWatcher(fx)
	w.Tick()
	fx.setOnline("0")
	w.Tick()
	fx.advance(10 * time.Second)
	if tr := w.Tick(); tr == nil || tr.Type != EventACDisconnected {
		t.Fatal("expected disconnect")
	}
	fx.advance(30 * time.Second)
	if tr := w.Tick(); tr != nil {
		t.Fatalf("duplicate %+v", tr)
	}
}

func TestWatcherFlappingBeforeDebounceEmitsNothing(t *testing.T) {
	fx := newFixture(t, "1")
	w := newTestWatcher(fx)
	w.Tick()
	fx.setOnline("0")
	w.Tick()
	fx.advance(4 * time.Second)
	if tr := w.Tick(); tr != nil {
		t.Fatalf("early %+v", tr)
	}
	fx.setOnline("1")
	fx.advance(1 * time.Second)
	if tr := w.Tick(); tr != nil {
		t.Fatalf("flap cancel %+v", tr)
	}
	fx.advance(30 * time.Second)
	if tr := w.Tick(); tr != nil {
		t.Fatalf("stable original state %+v", tr)
	}
}

func TestWatcherMultipleAdapters(t *testing.T) {
	root := t.TempDir()
	writeMains(t, root, "AC", "1")
	writeMains(t, root, "ACAD", "0")
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	w := NewWatcher(Options{
		SysfsRoot: root,
		Interval:  time.Second,
		Debounce:  10 * time.Second,
		Now:       func() time.Time { return now },
	})
	if tr := w.Tick(); tr != nil {
		t.Fatal("baseline")
	}
	writeFile(t, filepath.Join(root, "AC", "online"), "0")
	if tr := w.Tick(); tr != nil {
		t.Fatalf("before debounce %+v", tr)
	}
	now = now.Add(10 * time.Second)
	tr := w.Tick()
	if tr == nil || tr.Type != EventACDisconnected {
		t.Fatalf("both offline should be battery: %+v", tr)
	}
}

type fixture struct {
	t    *testing.T
	root string
	now  time.Time
}

func newFixture(t *testing.T, online string) *fixture {
	t.Helper()
	root := t.TempDir()
	writeMains(t, root, "AC", online)
	return &fixture{
		t:    t,
		root: root,
		now:  time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
	}
}

func (f *fixture) setOnline(v string) {
	f.t.Helper()
	writeFile(f.t, filepath.Join(f.root, "AC", "online"), v)
}

func (f *fixture) advance(d time.Duration) { f.now = f.now.Add(d) }

func newTestWatcher(f *fixture) *Watcher {
	return NewWatcher(Options{
		SysfsRoot: f.root,
		Interval:  time.Second,
		Debounce:  10 * time.Second,
		Now:       func() time.Time { return f.now },
	})
}

func writeMains(t *testing.T, root, name, online string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "type"), "Mains")
	writeFile(t, filepath.Join(dir, "online"), online)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
