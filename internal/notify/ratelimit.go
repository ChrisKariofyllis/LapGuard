package notify

import (
	"sync"
	"time"
)

// Limiter suppresses duplicate event types so AC flapping cannot spam a provider.
type Limiter struct {
	mu       sync.Mutex
	cooldown time.Duration
	now      func() time.Time
	last     map[string]time.Time
}

func NewLimiter(cooldown time.Duration, now func() time.Time) *Limiter {
	if cooldown <= 0 {
		cooldown = DefaultCooldown
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Limiter{
		cooldown: cooldown,
		now:      now,
		last:     map[string]time.Time{},
	}
}

func (l *Limiter) Allow(key string) bool {
	if key == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	if prev, ok := l.last[key]; ok && t.Sub(prev) < l.cooldown {
		return false
	}
	l.last[key] = t
	return true
}
