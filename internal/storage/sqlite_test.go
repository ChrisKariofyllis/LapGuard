package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreInsertAndList(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	ts := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	dur := int64(15000)
	ev, err := s.Insert(ctx, Event{Type: "AC_DISCONNECTED", Timestamp: ts, Source: "sysfs"})
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID == 0 {
		t.Fatal("expected id")
	}
	_, err = s.Insert(ctx, Event{Type: "AC_CONNECTED", Timestamp: ts.Add(time.Minute), Source: "sysfs", DurationMs: &dur})
	if err != nil {
		t.Fatal(err)
	}

	all, err := s.List(ctx, ListOptions{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("len %d", len(all))
	}
	if all[0].Type != "AC_CONNECTED" {
		t.Fatalf("newest first: %+v", all[0])
	}
	if all[0].DurationMs == nil || *all[0].DurationMs != dur {
		t.Fatalf("duration %+v", all[0].DurationMs)
	}

	only, err := s.List(ctx, ListOptions{Limit: 10, Type: "AC_DISCONNECTED"})
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != 1 || only[0].Type != "AC_DISCONNECTED" {
		t.Fatalf("%+v", only)
	}

	limited, err := s.List(ctx, ListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("limit: %+v", limited)
	}
}

func TestStoreDoesNotKeepSerials(t *testing.T) {
	s := openTestStore(t)
	_, err := s.Insert(context.Background(), Event{
		Type:      "AC_DISCONNECTED",
		Timestamp: time.Now().UTC(),
		Source:    "sysfs",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.Query(`SELECT source FROM events`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			t.Fatal(err)
		}
		if src != "sysfs" {
			t.Fatalf("unexpected source %q", src)
		}
	}
}

func TestStorePrunesToMaxEvents(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < MaxEvents+5; i++ {
		if _, err := s.Insert(ctx, Event{
			Type:      "AC_DISCONNECTED",
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Source:    "sysfs",
		}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.List(ctx, ListOptions{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != MaxEvents {
		t.Fatalf("count %d, want %d (listed %d)", count, MaxEvents, len(all))
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
