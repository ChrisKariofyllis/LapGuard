package config

import (
	"context"
	"log/slog"
	"strings"
)

const redacted = "[redacted]"

var secretKeyFragments = []string{
	"webhook",
	"token",
	"password",
	"secret",
	"authorization",
	"api_key",
	"apikey",
	"chat_id",
}

// NewRedactingHandler wraps a slog handler so notification secrets, tokens,
// passwords, and webhook URLs never reach the log sink.
func NewRedactingHandler(inner slog.Handler) slog.Handler {
	if inner == nil {
		inner = slog.Default().Handler()
	}
	if _, ok := inner.(*RedactingHandler); ok {
		return inner
	}
	return &RedactingHandler{inner: inner}
}

type RedactingHandler struct {
	inner slog.Handler
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, redactString(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, nr)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactAttr(a)
	}
	return &RedactingHandler{inner: h.inner.WithAttrs(out)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if secretKey(a.Key) {
		return slog.String(a.Key, redacted)
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, redactString(a.Value.String()))
	case slog.KindGroup:
		group := a.Value.Group()
		out := make([]slog.Attr, len(group))
		for i, child := range group {
			out[i] = redactAttr(child)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	case slog.KindAny:
		if s, ok := a.Value.Any().(string); ok {
			return slog.String(a.Key, redactString(s))
		}
	}
	return a
}

func secretKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	k = strings.ReplaceAll(k, "-", "_")
	for _, frag := range secretKeyFragments {
		if k == frag || strings.Contains(k, frag) {
			return true
		}
	}
	return false
}

func redactString(s string) string {
	if strings.Contains(s, "://") {
		return redacted
	}
	return s
}

// SafeNotifications returns log-safe notification fields. Webhook URLs, tokens,
// and chat IDs are omitted.
func SafeNotifications(n NotificationsConfig) slog.Attr {
	return slog.Group("notifications",
		slog.String("provider", n.Provider),
		slog.Bool("enabled", n.Enabled),
		slog.Bool("dry_run", n.DryRun),
		slog.Bool("configured", n.ProviderConfigured()),
	)
}
