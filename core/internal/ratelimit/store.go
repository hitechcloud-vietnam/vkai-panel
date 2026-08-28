package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrStoreUnavailable wraps any failure to reach the counter store. The guard
// treats it as a reason to deny, never as a reason to allow.
var ErrStoreUnavailable = errors.New("rate limit store unavailable")

// Store is the small set of operations the guard needs from Redis. It is an
// interface so the guard can be tested against a store that fails on demand -
// fail-closed behaviour is not something to take on trust.
type Store interface {
	// Incr adds one to the counter at key and returns the new value. When the
	// key did not exist it is created with the given time to live; an existing
	// key keeps the expiry it already has, which is what makes the counter a
	// fixed window rather than one an attacker can extend indefinitely.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)

	// Get returns the counter value and whether the key exists at all.
	Get(ctx context.Context, key string) (int64, bool, error)

	// Set writes value at key with the given time to live, replacing any
	// existing value and expiry.
	Set(ctx context.Context, key string, value int64, ttl time.Duration) error

	// TTL returns the remaining life of key, or zero when the key is gone.
	TTL(ctx context.Context, key string) (time.Duration, error)

	// Delete removes keys. Missing keys are not an error.
	Delete(ctx context.Context, keys ...string) error
}

// redisCmds is the subset of go-redis used here. *redis.Client satisfies it,
// and so does any wrapper that embeds one.
type redisCmds interface {
	Incr(ctx context.Context, key string) *redis.IntCmd
	Expire(ctx context.Context, key string, ttl time.Duration) *redis.BoolCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd
	PTTL(ctx context.Context, key string) *redis.DurationCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// RedisStore is the production store. Because the counters are shared, a
// limit of five means five across the whole cluster and not five per instance,
// which is the difference between a limit and a suggestion.
type RedisStore struct {
	client redisCmds
}

// NewRedisStore adapts a go-redis client. redis.Cmdable is accepted so callers
// can hand over *redis.Client or the panel's own thin wrapper around it.
func NewRedisStore(client redis.Cmdable) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, wrapStoreErr(err)
	}
	if n == 1 {
		// Only the request that created the key sets the expiry. Refreshing it
		// on every hit would let a steady stream of attempts keep the window
		// open forever and the counter would never reset for a legitimate user.
		if err := s.client.Expire(ctx, key, ttl).Err(); err != nil {
			return 0, wrapStoreErr(err)
		}
	}
	return n, nil
}

func (s *RedisStore) Get(ctx context.Context, key string) (int64, bool, error) {
	raw, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, wrapStoreErr(err)
	}
	n, convErr := strconv.ParseInt(raw, 10, 64)
	if convErr != nil {
		// A value that is not a number is a corrupt key, not a licence to
		// allow the request: report it as unavailable and let the caller fail
		// closed.
		return 0, false, wrapStoreErr(convErr)
	}
	return n, true, nil
}

func (s *RedisStore) Set(ctx context.Context, key string, value int64, ttl time.Duration) error {
	return wrapStoreErr(s.client.Set(ctx, key, value, ttl).Err())
}

func (s *RedisStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := s.client.PTTL(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, wrapStoreErr(err)
	}
	// go-redis reports -2 for a missing key and -1 for a key without expiry.
	if d < 0 {
		return 0, nil
	}
	return d, nil
}

func (s *RedisStore) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return wrapStoreErr(s.client.Del(ctx, keys...).Err())
}

func wrapStoreErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(ErrStoreUnavailable, err)
}

// MemoryStore is an in-process Store. It exists for tests and for a
// single-process development run. It is never selected automatically: with two
// panel instances its counters diverge, and a limiter whose numbers depend on
// which instance answered is not a limiter.
type MemoryStore struct {
	mu     sync.Mutex
	now    func() time.Time
	values map[string]*memoryEntry
}

type memoryEntry struct {
	value     int64
	expiresAt time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{now: time.Now, values: make(map[string]*memoryEntry)}
}

// SetClock replaces the time source, so a test can move a window forward
// without sleeping through it.
func (s *MemoryStore) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func (s *MemoryStore) liveLocked(key string) (*memoryEntry, bool) {
	e, ok := s.values[key]
	if !ok {
		return nil, false
	}
	if !e.expiresAt.IsZero() && !s.now().Before(e.expiresAt) {
		delete(s.values, key)
		return nil, false
	}
	return e, true
}

func (s *MemoryStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.liveLocked(key)
	if !ok {
		e = &memoryEntry{expiresAt: s.now().Add(ttl)}
		s.values[key] = e
	}
	e.value++
	return e.value, nil
}

func (s *MemoryStore) Get(ctx context.Context, key string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.liveLocked(key)
	if !ok {
		return 0, false, nil
	}
	return e.value, true, nil
}

func (s *MemoryStore) Set(ctx context.Context, key string, value int64, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = &memoryEntry{value: value, expiresAt: s.now().Add(ttl)}
	return nil
}

func (s *MemoryStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.liveLocked(key)
	if !ok {
		return 0, nil
	}
	remaining := e.expiresAt.Sub(s.now())
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}

func (s *MemoryStore) Delete(ctx context.Context, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.values, k)
	}
	return nil
}
