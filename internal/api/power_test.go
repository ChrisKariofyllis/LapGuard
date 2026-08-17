package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lapguard/internal/battery"
	"lapguard/internal/config"
	"lapguard/internal/power"
	"lapguard/internal/storage"
)

func TestGetPowerFromSysfsFixture(t *testing.T) {
	srv := New(battery.NewMockProvider(), config.Config{
		Listen:    "127.0.0.1:8585",
		SysfsRoot: sysfsFixture(t),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/power", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body powerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Source != power.SourceAC {
		t.Fatalf("source %s, fixture AC is online", body.Source)
	}
	if len(body.Adapters) == 0 || body.Adapters[0].Name == "" {
		t.Fatalf("adapters %+v", body.Adapters)
	}
	if body.Watcher.Running {
		t.Fatal("watcher should not run unless attached")
	}
}

func TestGetPowerUnknownWithoutMains(t *testing.T) {
	srv := New(battery.NewMockProvider(), config.Config{
		Listen:    "127.0.0.1:8585",
		SysfsRoot: t.TempDir(),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/power", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var body powerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Source != power.SourceUnknown {
		t.Fatalf("source %s", body.Source)
	}
	if body.Reason == "" {
		t.Fatal("expected reason when no mains adapter is detected")
	}
}

func TestGetEventsLimitAndType(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	base := time.Date(2026, 8, 17, 18, 0, 0, 0, time.UTC)
	if _, err := store.Insert(ctx, storage.Event{Type: power.EventACDisconnected, Timestamp: base, Source: power.DetectionSource}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Insert(ctx, storage.Event{Type: power.EventACConnected, Timestamp: base.Add(time.Minute), Source: power.DetectionSource}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Insert(ctx, storage.Event{Type: power.EventACUnknown, Timestamp: base.Add(2 * time.Minute), Source: power.DetectionSource}); err != nil {
		t.Fatal(err)
	}

	srv := New(battery.NewMockProvider(), config.Config{Listen: "127.0.0.1:8585"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	srv.AttachPower(nil, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events?limit=2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var body eventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Available || len(body.Events) != 2 {
		t.Fatalf("%+v", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/events?type=AC_DISCONNECTED", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 1 || body.Events[0].Type != power.EventACDisconnected {
		t.Fatalf("%+v", body.Events)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/events?type=not-a-type", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestWatcherBaselineDoesNotWriteEvent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "AC"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AC", "type"), []byte("Mains\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AC", "online"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, 8, 17, 19, 0, 0, 0, time.UTC)
	w := power.NewWatcher(power.Options{
		SysfsRoot: root,
		Interval:  time.Second,
		Debounce:  10 * time.Second,
		Now:       func() time.Time { return now },
		OnEvent: func(tr power.Transition) {
			_, _ = store.Insert(context.Background(), storage.Event{
				Type:       tr.Type,
				Timestamp:  tr.At,
				Source:     tr.Source,
				DurationMs: tr.DurationMs,
			})
		},
	})
	if tr := w.Tick(); tr != nil {
		t.Fatalf("baseline %+v", tr)
	}
	list, err := store.List(context.Background(), storage.ListOptions{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("baseline must not persist events: %+v", list)
	}
}
