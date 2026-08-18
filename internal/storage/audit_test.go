package storage

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestInsertAuditRedactsSecretsAndKeepsIP(t *testing.T) {
	s := openTestStore(t)
	ev, err := s.InsertAudit(context.Background(), AuditEvent{
		Type:     AuditUnauthorized,
		Success:  false,
		Remote:   "127.0.0.1:54321",
		Method:   "PUT",
		Endpoint: "/api/v1/config?token=lg_shouldnotstore",
		Reason:   "Authorization: Bearer lg_secrettokenvaluehere",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Remote != "127.0.0.1" {
		t.Fatalf("remote %q", ev.Remote)
	}
	if strings.Contains(ev.Endpoint, "?") || strings.Contains(ev.Endpoint, "token") {
		t.Fatalf("endpoint leaked query: %q", ev.Endpoint)
	}
	if strings.Contains(strings.ToLower(ev.Reason), "bearer") || strings.Contains(ev.Reason, "lg_") {
		t.Fatalf("reason leaked secret: %q", ev.Reason)
	}

	rows, err := s.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("len %d", len(rows))
	}
	blob := rows[0].Reason + rows[0].Endpoint + rows[0].Remote
	for _, leak := range []string{"Bearer", "webhook", "chat_id", "password", "serial"} {
		if strings.Contains(strings.ToLower(blob), leak) && leak != "serial" {
			t.Fatalf("audit leaked %q: %+v", leak, rows[0])
		}
	}
}

func TestAuditRetentionBound(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := s.InsertAudit(ctx, AuditEvent{
			Type:      AuditConfigUpdate,
			Success:   true,
			Remote:    "192.0.2.10:1",
			Method:    "PUT",
			Endpoint:  "/api/v1/config",
			Reason:    "ok",
			Timestamp: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListAudit(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("len %d", len(rows))
	}
}
