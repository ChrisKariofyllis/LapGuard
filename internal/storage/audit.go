package storage

import (
	"context"
	"database/sql"
	"net"
	"strings"
	"time"
)

const (
	MaxAuditEvents = 1000
	MaxAuditAge    = 90 * 24 * time.Hour

	auditSchema = `
CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp TEXT NOT NULL,
  event_type TEXT NOT NULL,
  success INTEGER NOT NULL,
  remote TEXT NOT NULL,
  method TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  reason TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_events(timestamp DESC);
`
)

const (
	AuditConfigUpdate     = "config_update"
	AuditNotificationTest = "notification_test"
	AuditSafetyTest       = "safety_simulation"
	AuditAuthEnable       = "auth_enable"
	AuditAuthDisable      = "auth_disable"
	AuditAuthRotate       = "auth_rotate"
	AuditUnauthorized     = "unauthorized"
	AuditInvalidOrigin    = "invalid_origin"
	AuditPowerOffAttempt  = "poweroff_attempt"
	AuditPowerOffResult   = "poweroff_result"
	AuditDockerAttempt    = "docker_drain_attempt"
	AuditDockerResult     = "docker_drain_result"
)

// AuditEvent is a redacted security log row. It never stores tokens, headers,
// webhook URLs, chat IDs, passwords, or battery serials.
type AuditEvent struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"event_type"`
	Success   bool      `json:"success"`
	Remote    string    `json:"remote"`
	Method    string    `json:"method"`
	Endpoint  string    `json:"endpoint"`
	Reason    string    `json:"reason"`
}

func (s *Store) ensureAudit(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, auditSchema)
	return err
}

func (s *Store) InsertAudit(ctx context.Context, ev AuditEvent) (AuditEvent, error) {
	if s == nil || s.db == nil {
		return AuditEvent{}, nil
	}
	if err := s.ensureAudit(ctx); err != nil {
		return AuditEvent{}, err
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	ev.Timestamp = ev.Timestamp.UTC()
	ev.Type = strings.TrimSpace(ev.Type)
	ev.Method = strings.TrimSpace(ev.Method)
	ev.Endpoint = sanitizeEndpoint(ev.Endpoint)
	ev.Remote = sanitizeRemote(ev.Remote)
	ev.Reason = sanitizeAuditText(ev.Reason)
	if ev.Type == "" {
		ev.Type = "unknown"
	}
	success := 0
	if ev.Success {
		success = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_events (timestamp, event_type, success, remote, method, endpoint, reason) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.Timestamp.Format(time.RFC3339Nano), ev.Type, success, ev.Remote, ev.Method, ev.Endpoint, ev.Reason,
	)
	if err != nil {
		return AuditEvent{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return AuditEvent{}, err
	}
	ev.ID = id
	if err := s.pruneAudit(ctx); err != nil {
		return ev, err
	}
	return ev, nil
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEvent, error) {
	if s == nil || s.db == nil {
		return []AuditEvent{}, nil
	}
	if err := s.ensureAudit(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, timestamp, event_type, success, remote, method, endpoint, reason FROM audit_events ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var ev AuditEvent
		var ts string
		var success int
		if err := rows.Scan(&ev.ID, &ts, &ev.Type, &success, &ev.Remote, &ev.Method, &ev.Endpoint, &ev.Reason); err != nil {
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
		ev.Success = success != 0
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) pruneAudit(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-MaxAuditAge).Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM audit_events WHERE timestamp < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit_events WHERE id NOT IN (SELECT id FROM (SELECT id FROM audit_events ORDER BY id DESC LIMIT ?))`, MaxAuditEvents)
	return err
}

func sanitizeRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(raw)
	if err == nil {
		raw = host
	}
	ip := net.ParseIP(raw)
	if ip != nil {
		return ip.String()
	}
	return "unknown"
}

func sanitizeEndpoint(path string) string {
	path = strings.TrimSpace(path)
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if !strings.HasPrefix(path, "/") {
		return "/"
	}
	return path
}

func sanitizeAuditText(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, needle := range []string{"authorization", "bearer ", "webhook", "chat_id", "password", "token_hash", "serial"} {
		if strings.Contains(lower, needle) {
			return "redacted"
		}
	}
	if strings.Contains(s, "://") || strings.Contains(s, "lg_") {
		return "redacted"
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Ensure audit schema exists alongside the power-event schema.
func initAudit(db *sql.DB) error {
	_, err := db.Exec(auditSchema)
	return err
}
