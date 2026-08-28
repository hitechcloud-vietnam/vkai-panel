package twofactor

import (
	"context"
	"sync"
	"time"
)

// Limiter is the narrow slice of a rate limiter that two-factor verification
// needs: one question, one answer.
//
// Six digits is a million combinations. Without a limit, an attacker holding a
// valid session or a stolen password walks that space in minutes, and the
// second factor becomes decoration. The panel-wide limiter (Redis backed,
// effective across instances) is being built separately; anything with an
// Allow(ctx, key) (bool, error) method drops straight in here, so swapping it
// is a one-line change at the call site in cmd/api/main.go.
type Limiter interface {
	// Allow reports whether one more attempt is permitted for key. An error
	// must be treated as "do not allow" by the caller: a limiter that cannot
	// answer is not a licence to brute force.
	Allow(ctx context.Context, key string) (bool, error)
}

// MemoryLimiter is the default limiter: a fixed window per key, in process.
// It is sufficient for a single-instance panel and is the floor, not the
// ceiling - it does not survive a restart and does not span instances.
type MemoryLimiter struct {
	mu      sync.Mutex
	windows map[string]*limiterWindow
	limit   int
	window  time.Duration
	now     func() time.Time
}

type limiterWindow struct {
	count   int
	resetAt time.Time
}

// DefaultVerifyLimit and DefaultVerifyWindow are the verification budget: ten
// attempts per five minutes per user and source address. A human with a phone
// needs two or three.
const (
	DefaultVerifyLimit  = 10
	DefaultVerifyWindow = 5 * time.Minute
)

// NewMemoryLimiter returns an in-process limiter.
func NewMemoryLimiter(limit int, window time.Duration) *MemoryLimiter {
	if limit <= 0 {
		limit = DefaultVerifyLimit
	}
	if window <= 0 {
		window = DefaultVerifyWindow
	}
	return &MemoryLimiter{
		windows: make(map[string]*limiterWindow),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

// Allow implements Limiter.
func (l *MemoryLimiter) Allow(_ context.Context, key string) (bool, error) {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Drop expired windows so a stream of distinct keys cannot grow the map
	// without bound.
	if len(l.windows) > 4096 {
		for k, w := range l.windows {
			if now.After(w.resetAt) {
				delete(l.windows, k)
			}
		}
	}

	w, ok := l.windows[key]
	if !ok || now.After(w.resetAt) {
		l.windows[key] = &limiterWindow{count: 1, resetAt: now.Add(l.window)}
		return true, nil
	}

	w.count++
	return w.count <= l.limit, nil
}
