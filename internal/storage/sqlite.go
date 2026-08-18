package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	MaxEvents = 1000
	MaxAge    = 90 * 24 * time.Hour

	schema = `
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type TEXT NOT NULL,
  timestamp TEXT NOT NULL,
  source TEXT NOT NULL,
  duration_ms INTEGER
);
CREATE INDEX IF NOT EXISTS idx_events_type_ts ON events(event_type, timestamp DESC);
`
)

// Event is one persisted power-loss / AC transition. It never includes secrets
// or battery serial numbers.
type Event struct {
	ID         int64     `json:"id"`
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
	Source     string    `json:"source"`
	DurationMs *int64    `json:"duration_ms,omitempty"`
}

type Store struct {
	db   *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("events database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := initAudit(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return &Store{db: db, path: path}, nil
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Insert(ctx context.Context, ev Event) (Event, error) {
	if s == nil || s.db == nil {
		return Event{}, fmt.Errorf("event log is not available")
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	ev.Timestamp = ev.Timestamp.UTC()
	var dur any
	if ev.DurationMs != nil {
		dur = *ev.DurationMs
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO events (event_type, timestamp, source, duration_ms) VALUES (?, ?, ?, ?)`,
		ev.Type, ev.Timestamp.Format(time.RFC3339Nano), ev.Source, dur)
	if err != nil {
		return Event{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Event{}, err
	}
	ev.ID = id
	if err := s.prune(ctx); err != nil {
		return ev, err
	}
	return ev, nil
}

type ListOptions struct {
	Limit int
	Type  string
}

func (s *Store) List(ctx context.Context, opts ListOptions) ([]Event, error) {
	if s == nil || s.db == nil {
		return []Event{}, nil
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}
	q := `SELECT id, event_type, timestamp, source, duration_ms FROM events`
	args := []any{}
	if opts.Type != "" {
		q += ` WHERE event_type = ?`
		args = append(args, opts.Type)
	}
	q += ` ORDER BY timestamp DESC, id DESC LIMIT ?`
	args = append(args, opts.Limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var ev Event
		var ts string
		var dur sql.NullInt64
		if err := rows.Scan(&ev.ID, &ev.Type, &ts, &ev.Source, &dur); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, ts)
			if err != nil {
				return nil, err
			}
		}
		ev.Timestamp = parsed.UTC()
		if dur.Valid {
			v := dur.Int64
			ev.DurationMs = &v
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) prune(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-MaxAge).Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE timestamp < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id NOT IN (SELECT id FROM (SELECT id FROM events ORDER BY id DESC LIMIT ?))`, MaxEvents)
	return err
}
