package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// testClock is a manual clock so window and lock expiry can be tested without
// a test suite that takes fifteen minutes to run.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// flakyStore turns store failures on and off, so the fail-closed behaviour can
// be tested rather than assumed.
type flakyStore struct {
	inner   Store
	mu      sync.Mutex
	failing bool
}

func newFlakyStore(inner Store) *flakyStore { return &flakyStore{inner: inner} }

func (f *flakyStore) setFailing(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = v
}

func (f *flakyStore) down() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failing
}

func (f *flakyStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if f.down() {
		return 0, wrapStoreErr(errors.New("connection refused"))
	}
	return f.inner.Incr(ctx, key, ttl)
}

func (f *flakyStore) Get(ctx context.Context, key string) (int64, bool, error) {
	if f.down() {
		return 0, false, wrapStoreErr(errors.New("connection refused"))
	}
	return f.inner.Get(ctx, key)
}

func (f *flakyStore) Set(ctx context.Context, key string, value int64, ttl time.Duration) error {
	if f.down() {
		return wrapStoreErr(errors.New("connection refused"))
	}
	return f.inner.Set(ctx, key, value, ttl)
}

func (f *flakyStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	if f.down() {
		return 0, wrapStoreErr(errors.New("connection refused"))
	}
	return f.inner.TTL(ctx, key)
}

func (f *flakyStore) Delete(ctx context.Context, keys ...string) error {
	if f.down() {
		return wrapStoreErr(errors.New("connection refused"))
	}
	return f.inner.Delete(ctx, keys...)
}

func newTestGuard(t *testing.T) (*Guard, *MemoryStore, *testClock) {
	t.Helper()
	clock := newTestClock()
	store := NewMemoryStore()
	store.SetClock(clock.Now)
	return New(store, DefaultPolicy()), store, clock
}

func mustCheck(t *testing.T, g *Guard, scope string, s Subject) Decision {
	t.Helper()
	d, err := g.Check(context.Background(), scope, s)
	if err != nil {
		t.Fatalf("Check returned an error: %v", err)
	}
	return d
}

func mustFail(t *testing.T, g *Guard, scope string, s Subject) Decision {
	t.Helper()
	d, err := g.RecordFailure(context.Background(), scope, s)
	if err != nil {
		t.Fatalf("RecordFailure returned an error: %v", err)
	}
	return d
}

// --------------------------------------------------------------------------
// Progressive delay
// --------------------------------------------------------------------------

func TestProgressiveDelayGrowsAndCaps(t *testing.T) {
	g, _, _ := newTestGuard(t)
	subject := NewSubject("203.0.113.10", "operator")

	// The delay the NEXT attempt will be charged, after n recorded failures.
	want := []time.Duration{
		0,                      // 1
		0,                      // 2
		0,                      // 3 - the free typo budget
		500 * time.Millisecond, // 4
		1 * time.Second,        // 5
		2 * time.Second,        // 6
		4 * time.Second,        // 7 - at the cap
	}

	for i, expected := range want {
		mustFail(t, g, "login", subject)
		got := mustCheck(t, g, "login", subject)
		if !got.Allow {
			t.Fatalf("after %d failures the attempt should still be allowed", i+1)
		}
		if got.Delay != expected {
			t.Fatalf("after %d failures: delay = %v, want %v", i+1, got.Delay, expected)
		}
	}
}

func TestDelayNeverExceedsTheCap(t *testing.T) {
	p := DefaultPolicy()
	for failures := 0; failures < 200; failures++ {
		if d := p.delayFor(failures); d > p.MaxDelay {
			t.Fatalf("delayFor(%d) = %v, above the cap %v", failures, d, p.MaxDelay)
		}
	}
}

// --------------------------------------------------------------------------
// Lock and unlock
// --------------------------------------------------------------------------

func TestPairLocksAtThresholdAndEscalates(t *testing.T) {
	g, _, clock := newTestGuard(t)
	policy := g.Policy()
	subject := NewSubject("203.0.113.11", "operator")

	for cycle, wantLock := range policy.LockSteps {
		for i := 1; i <= policy.PairLockThreshold; i++ {
			d := mustFail(t, g, "login", subject)
			if i < policy.PairLockThreshold && !d.Allow {
				t.Fatalf("cycle %d: failure %d should not have locked yet", cycle, i)
			}
			if i == policy.PairLockThreshold {
				if d.Allow {
					t.Fatalf("cycle %d: failure %d should have locked the pair", cycle, i)
				}
				if d.Outcome != OutcomeLocked || d.Dimension != DimensionPair {
					t.Fatalf("cycle %d: outcome = %s/%s, want locked/pair", cycle, d.Outcome, d.Dimension)
				}
				if d.RetryAfter != wantLock {
					t.Fatalf("cycle %d: lock duration = %v, want %v", cycle, d.RetryAfter, wantLock)
				}
			}
		}

		blocked := mustCheck(t, g, "login", subject)
		if blocked.Allow {
			t.Fatalf("cycle %d: a locked pair must not be allowed through", cycle)
		}
		if blocked.RetryAfter <= 0 || blocked.RetryAfter > wantLock {
			t.Fatalf("cycle %d: retry-after = %v, want (0, %v]", cycle, blocked.RetryAfter, wantLock)
		}

		// Waiting out the lock returns a fresh allowance, not a fresh attempt.
		clock.Advance(wantLock + time.Second)
		if next := mustCheck(t, g, "login", subject); !next.Allow {
			t.Fatalf("cycle %d: the pair should be usable again once the lock expires", cycle)
		}
	}

	// Past the end of the table the longest step repeats rather than growing
	// without bound.
	for i := 1; i <= policy.PairLockThreshold; i++ {
		if d := mustFail(t, g, "login", subject); i == policy.PairLockThreshold {
			last := policy.LockSteps[len(policy.LockSteps)-1]
			if d.RetryAfter != last {
				t.Fatalf("lock past the table = %v, want the last step %v", d.RetryAfter, last)
			}
		}
	}
}

func TestCorrectPasswordClearsLockAndEscalation(t *testing.T) {
	g, _, clock := newTestGuard(t)
	policy := g.Policy()
	subject := NewSubject("203.0.113.12", "operator")

	for i := 0; i < policy.PairLockThreshold; i++ {
		mustFail(t, g, "login", subject)
	}
	if mustCheck(t, g, "login", subject).Allow {
		t.Fatal("the pair should be locked")
	}

	clock.Advance(policy.LockSteps[0] + time.Second)
	if err := g.RecordSuccess(context.Background(), "login", subject); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	// The pair starts over: no residual failures and no residual delay.
	after := mustCheck(t, g, "login", subject)
	if !after.Allow || after.Delay != 0 || after.Failures != 0 {
		t.Fatalf("after a success: %+v, want a clean allow", after)
	}
	if !after.Recognised {
		t.Fatal("a successful authentication should recognise the address")
	}
	if known, err := g.Recognised(context.Background(), subject); err != nil || !known {
		t.Fatalf("Recognised = %v, err = %v; want true", known, err)
	}
	stranger := NewSubject("198.51.100.99", "operator")
	if known, err := g.Recognised(context.Background(), stranger); err != nil || known {
		t.Fatalf("an address that has never signed in reported as recognised (%v, %v)", known, err)
	}

	// And the escalation memory is gone, so the next lock is the first step
	// again rather than the second.
	for i := 0; i < policy.PairLockThreshold*2; i++ {
		mustFail(t, g, "login", subject)
	}
	// This address is now recognised, so it is delayed rather than refused;
	// use a different address to observe the reset lock step.
	fresh := NewSubject("203.0.113.13", "operator")
	var locked Decision
	for i := 0; i < policy.PairLockThreshold; i++ {
		locked = mustFail(t, g, "login", fresh)
	}
	if locked.RetryAfter != policy.LockSteps[0] {
		t.Fatalf("a new pair locks for %v, want the first step %v", locked.RetryAfter, policy.LockSteps[0])
	}
}

func TestRecognisedAddressIsDelayedByItsLockRatherThanRefused(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()
	subject := NewSubject("203.0.113.14", "operator")

	// The user has signed in from here before.
	if err := g.RecordSuccess(context.Background(), "login", subject); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	for i := 0; i < policy.PairLockThreshold; i++ {
		mustFail(t, g, "login", subject)
	}

	got := mustCheck(t, g, "login", subject)
	if !got.Allow {
		t.Fatal("a recognised address must not be shut out of its own account: " +
			"that is the free denial of service a hard cutoff hands an attacker")
	}
	if got.Delay != policy.MaxDelay {
		t.Fatalf("delay = %v, want the maximum %v", got.Delay, policy.MaxDelay)
	}

	// An unrecognised address at the same point is refused.
	stranger := NewSubject("198.51.100.7", "operator")
	for i := 0; i < policy.PairLockThreshold; i++ {
		mustFail(t, g, "login", stranger)
	}
	if mustCheck(t, g, "login", stranger).Allow {
		t.Fatal("an unrecognised pair should be locked at the threshold")
	}

	// The correct password from the recognised address clears its lock.
	if err := g.RecordSuccess(context.Background(), "login", subject); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	cleared := mustCheck(t, g, "login", subject)
	if !cleared.Allow || cleared.Delay != 0 {
		t.Fatalf("after the correct password: %+v, want a clean allow", cleared)
	}
}

func TestLockIsScopedToOneAddressAccountPair(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()

	victim := NewSubject("198.51.100.20", "operator")
	for i := 0; i < policy.PairLockThreshold; i++ {
		mustFail(t, g, "login", victim)
	}
	if mustCheck(t, g, "login", victim).Allow {
		t.Fatal("the attacking pair should be locked")
	}

	// The same account from a different address is untouched. This is the
	// whole point of locking the pair rather than the account.
	elsewhere := NewSubject("203.0.113.99", "operator")
	if d := mustCheck(t, g, "login", elsewhere); !d.Allow {
		t.Fatalf("the account must remain usable from another address, got %+v", d)
	}

	// And a different scope from the same address is untouched, so a fumbled
	// two-factor code cannot lock the password form.
	if d := mustCheck(t, g, "two_factor", victim); !d.Allow {
		t.Fatalf("another scope must have its own budget, got %+v", d)
	}
}

// --------------------------------------------------------------------------
// Address dimension
// --------------------------------------------------------------------------

func TestAddressDimensionStopsSprayingManyAccounts(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()
	const address = "198.51.100.30"

	// One failure against each of many accounts: no pair ever approaches its
	// lock threshold, so only the address dimension can see this.
	for i := 0; i < policy.AddressLimit; i++ {
		mustFail(t, g, "login", NewSubject(address, fmt.Sprintf("victim-%d", i)))
	}

	next := mustCheck(t, g, "login", NewSubject(address, "victim-fresh"))
	if next.Allow {
		t.Fatal("the address dimension should have stopped the spray")
	}
	if next.Outcome != OutcomeThrottled || next.Dimension != DimensionAddress {
		t.Fatalf("outcome = %s/%s, want throttled/address", next.Outcome, next.Dimension)
	}

	// Another address is unaffected.
	if d := mustCheck(t, g, "login", NewSubject("198.51.100.31", "victim-fresh")); !d.Allow {
		t.Fatalf("an unrelated address should be unaffected, got %+v", d)
	}
}

func TestAddressDimensionAlsoBoundsARecognisedAddress(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()
	const address = "198.51.100.32"

	// The address is recognised for one account - an attacker sharing a NAT
	// gateway with a legitimate user inherits that.
	if err := g.RecordSuccess(context.Background(), "login", NewSubject(address, "resident")); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	for i := 0; i < policy.AddressLimit; i++ {
		mustFail(t, g, "login", NewSubject(address, "resident"))
	}

	got := mustCheck(t, g, "login", NewSubject(address, "resident"))
	if got.Allow {
		t.Fatal("recognition must not exempt an address from the address dimension: " +
			"it is the only ceiling left on an attacker who shares it")
	}
	if got.Dimension != DimensionAddress {
		t.Fatalf("dimension = %s, want address", got.Dimension)
	}
}

func TestAddressDimensionWindowExpires(t *testing.T) {
	g, _, clock := newTestGuard(t)
	policy := g.Policy()
	const address = "198.51.100.33"

	for i := 0; i < policy.AddressLimit; i++ {
		mustFail(t, g, "login", NewSubject(address, fmt.Sprintf("victim-%d", i)))
	}
	if mustCheck(t, g, "login", NewSubject(address, "x")).Allow {
		t.Fatal("expected the address to be throttled")
	}

	clock.Advance(policy.AddressWindow + time.Second)
	if !mustCheck(t, g, "login", NewSubject(address, "x")).Allow {
		t.Fatal("the address window should have rolled")
	}
}

// --------------------------------------------------------------------------
// Account dimension
// --------------------------------------------------------------------------

func TestAccountDimensionStopsADistributedAttack(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()

	// One failure per address, so neither the pair nor the address dimension
	// can see it. Only counting per account catches this shape.
	for i := 0; i < policy.AccountLimit; i++ {
		mustFail(t, g, "login", NewSubject(fmt.Sprintf("192.0.2.%d", i+1), "operator"))
	}

	next := mustCheck(t, g, "login", NewSubject("192.0.2.200", "operator"))
	if next.Allow {
		t.Fatal("the account dimension should have stopped the distributed attack")
	}
	if next.Outcome != OutcomeThrottled || next.Dimension != DimensionAccount {
		t.Fatalf("outcome = %s/%s, want throttled/account", next.Outcome, next.Dimension)
	}

	// A different account from the same fresh address is unaffected.
	if d := mustCheck(t, g, "login", NewSubject("192.0.2.200", "someone-else")); !d.Allow {
		t.Fatalf("an unrelated account should be unaffected, got %+v", d)
	}
}

func TestAccountDimensionExemptsARecognisedAddress(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()

	owner := NewSubject("203.0.113.50", "operator")
	if err := g.RecordSuccess(context.Background(), "login", owner); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	// A botnet grinds the account past its limit.
	for i := 0; i < policy.AccountLimit*2; i++ {
		mustFail(t, g, "login", NewSubject(fmt.Sprintf("192.0.2.%d", i+1), "operator"))
	}

	// The owner's own machine still gets in. Without this exemption the
	// account dimension would be a remote button for locking any named user
	// out of the panel, which is worse than the attack it prevents.
	if d := mustCheck(t, g, "login", owner); !d.Allow {
		t.Fatalf("a recognised address must stay exempt from the account dimension, got %+v", d)
	}
}

func TestRecognisedFailuresDoNotFeedTheAccountDimension(t *testing.T) {
	g, store, _ := newTestGuard(t)
	g2 := New(store, DefaultPolicy())
	policy := g.Policy()

	owner := NewSubject("203.0.113.51", "operator")
	if err := g.RecordSuccess(context.Background(), "login", owner); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	for i := 0; i < policy.AccountLimit; i++ {
		mustFail(t, g, "login", owner)
	}

	count, _, err := store.Get(context.Background(), g2.accountKey(owner))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if count != 0 {
		t.Fatalf("account counter = %d, want 0: a recognised user fumbling their own "+
			"password must not help an attacker throttle their account", count)
	}
}

// --------------------------------------------------------------------------
// Fail closed
// --------------------------------------------------------------------------

func TestFailsClosedWhenTheStoreIsUnreachable(t *testing.T) {
	clock := newTestClock()
	memory := NewMemoryStore()
	memory.SetClock(clock.Now)
	store := newFlakyStore(memory)
	g := New(store, DefaultPolicy())
	subject := NewSubject("203.0.113.60", "operator")

	if d := mustCheck(t, g, "login", subject); !d.Allow {
		t.Fatal("a healthy store should allow a first attempt")
	}

	store.setFailing(true)

	decision, err := g.Check(context.Background(), "login", subject)
	if err == nil {
		t.Fatal("a store outage should be reported to the caller")
	}
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("error = %v, want it to wrap ErrStoreUnavailable", err)
	}
	if decision.Allow {
		t.Fatal("the limiter must fail CLOSED: an attacker who can take Redis down " +
			"must not thereby switch brute force protection off")
	}
	if decision.Outcome != OutcomeUnavailable || decision.Dimension != DimensionStore {
		t.Fatalf("outcome = %s/%s, want unavailable/store", decision.Outcome, decision.Dimension)
	}
	if decision.RetryAfter <= 0 {
		t.Fatal("a refusal should tell the client when to come back")
	}

	// Recording a failure fails closed too, rather than silently losing the
	// count.
	if d, err := g.RecordFailure(context.Background(), "login", subject); err == nil || d.Allow {
		t.Fatalf("RecordFailure during an outage: %+v, err=%v; want a closed failure", d, err)
	}

	store.setFailing(false)
	if d := mustCheck(t, g, "login", subject); !d.Allow {
		t.Fatal("the limiter should recover when the store does")
	}
}

func TestFailsClosedOnEveryStoreOperation(t *testing.T) {
	// Each operation the guard performs must be a closed door on its own. A
	// single unchecked error is all it takes for the whole defence to become
	// advisory.
	operations := []string{"recognised", "pair", "address", "lock"}
	for _, op := range operations {
		t.Run(op, func(t *testing.T) {
			memory := NewMemoryStore()
			store := &selectiveFailStore{inner: memory, failOn: op}
			g := New(store, DefaultPolicy())
			subject := NewSubject("203.0.113.61", "operator")

			d, err := g.Check(context.Background(), "login", subject)
			if err == nil {
				t.Fatalf("failing %q should surface an error", op)
			}
			if d.Allow {
				t.Fatalf("failing %q should not allow the attempt", op)
			}
		})
	}
}

// selectiveFailStore fails one class of key, so each read the guard depends on
// can be broken in isolation.
type selectiveFailStore struct {
	inner  Store
	failOn string
}

func (s *selectiveFailStore) match(key string) bool {
	switch s.failOn {
	case "recognised":
		return contains(key, ":known:")
	case "pair":
		return contains(key, ":pair:")
	case "address":
		return contains(key, ":addr:")
	case "lock":
		return contains(key, ":lock:")
	}
	return false
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func (s *selectiveFailStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if s.match(key) {
		return 0, wrapStoreErr(errors.New("io timeout"))
	}
	return s.inner.Incr(ctx, key, ttl)
}

func (s *selectiveFailStore) Get(ctx context.Context, key string) (int64, bool, error) {
	if s.match(key) {
		return 0, false, wrapStoreErr(errors.New("io timeout"))
	}
	return s.inner.Get(ctx, key)
}

func (s *selectiveFailStore) Set(ctx context.Context, key string, value int64, ttl time.Duration) error {
	if s.match(key) {
		return wrapStoreErr(errors.New("io timeout"))
	}
	return s.inner.Set(ctx, key, value, ttl)
}

func (s *selectiveFailStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	if s.match(key) {
		return 0, wrapStoreErr(errors.New("io timeout"))
	}
	return s.inner.TTL(ctx, key)
}

func (s *selectiveFailStore) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		if s.match(k) {
			return wrapStoreErr(errors.New("io timeout"))
		}
	}
	return s.inner.Delete(ctx, keys...)
}

func TestNilStoreFailsClosed(t *testing.T) {
	g := New(nil, DefaultPolicy())
	d, err := g.Check(context.Background(), "login", NewSubject("203.0.113.62", "operator"))
	if err == nil || d.Allow {
		t.Fatalf("a guard with no store must refuse everything, got %+v err=%v", d, err)
	}
}

func TestFailOpenIsOptInOnly(t *testing.T) {
	policy := DefaultPolicy()
	if policy.FailOpen {
		t.Fatal("the shipped policy must fail closed")
	}

	policy.FailOpen = true
	store := newFlakyStore(NewMemoryStore())
	store.setFailing(true)
	g := New(store, policy)

	d, err := g.Check(context.Background(), "login", NewSubject("203.0.113.63", "operator"))
	if !d.Allow {
		t.Fatal("an operator who opts into failing open should get what they asked for")
	}
	if err == nil {
		t.Fatal("failing open still has to report the outage")
	}
}

// --------------------------------------------------------------------------
// Normalisation
// --------------------------------------------------------------------------

func TestAccountNormalisationCannotBeEvadedByCase(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()

	spellings := []string{"Operator", "operator", " OPERATOR ", "OpErAtOr"}
	for i := 0; i < policy.PairLockThreshold; i++ {
		mustFail(t, g, "login", NewSubject("198.51.100.40", spellings[i%len(spellings)]))
	}

	if mustCheck(t, g, "login", NewSubject("198.51.100.40", "operator")).Allow {
		t.Fatal("changing the case of a username must not buy a fresh budget")
	}
}

func TestIPv6IsCountedByItsAllocationNotItsAddress(t *testing.T) {
	g, _, _ := newTestGuard(t)
	policy := g.Policy()

	// An attacker routinely holds a whole /64. Counting single addresses in
	// that space is the same as not counting at all.
	for i := 0; i < policy.PairLockThreshold; i++ {
		address := fmt.Sprintf("2001:db8:1:1::%x", i+1)
		mustFail(t, g, "login", NewSubject(address, "operator"))
	}

	if mustCheck(t, g, "login", NewSubject("2001:db8:1:1::ffff", "operator")).Allow {
		t.Fatal("moving inside the same /64 must not reset the counter")
	}

	// A different /64 is a different source.
	if !mustCheck(t, g, "login", NewSubject("2001:db8:1:2::1", "operator")).Allow {
		t.Fatal("a different /64 should have its own budget")
	}
}

func TestNormalizeAddress(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5":            "203.0.113.5",
		" 203.0.113.5 ":          "203.0.113.5",
		"203.0.113.5:44321":      "203.0.113.5",
		"[2001:db8:1:1::5]":      "2001:db8:1:1::/64",
		"2001:db8:1:1::5":        "2001:db8:1:1::/64",
		"[2001:db8:1:1::5]:8443": "2001:db8:1:1::/64",
		"":                       "unknown",
		"not-an-address":         "not-an-address",
	}
	for input, want := range cases {
		if got := NormalizeAddress(input); got != want {
			t.Errorf("NormalizeAddress(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAccountKeyDoesNotLeakTheAccountName(t *testing.T) {
	// Redis keys end up in SLOWLOG, in MONITOR output and in every backup of
	// the instance. Usernames and email addresses do not belong there.
	key := accountKey("operator@example.com")
	if contains(key, "operator") || contains(key, "example.com") {
		t.Fatalf("account key %q contains the account name", key)
	}
	if key != accountKey("operator@example.com") {
		t.Fatal("the key must be stable across processes, so it cannot be salted per instance")
	}
}

// --------------------------------------------------------------------------
// Policy
// --------------------------------------------------------------------------

func TestPolicyFromEnvIgnoresNonsense(t *testing.T) {
	t.Setenv("VKAI_AUTH_LIMIT_PAIR_LOCK", "not-a-number")
	t.Setenv("VKAI_AUTH_LIMIT_ADDRESS", "-5")
	t.Setenv("VKAI_AUTH_LIMIT_ACCOUNT", "0")
	t.Setenv("VKAI_AUTH_LIMIT_FAIL_OPEN", "maybe")

	p := PolicyFromEnv()
	d := DefaultPolicy()
	if p.PairLockThreshold != d.PairLockThreshold ||
		p.AddressLimit != d.AddressLimit ||
		p.AccountLimit != d.AccountLimit {
		t.Fatalf("a typo in a unit file must not widen a limit: %+v", p)
	}
	if p.FailOpen {
		t.Fatal("only an explicit true switches failing open on")
	}
}

func TestPolicyFromEnvAppliesRealValues(t *testing.T) {
	t.Setenv("VKAI_AUTH_LIMIT_PAIR_LOCK", "4")
	t.Setenv("VKAI_AUTH_LIMIT_ADDRESS", "12")
	t.Setenv("VKAI_AUTH_LIMIT_ACCOUNT", "9")
	t.Setenv("VKAI_AUTH_LIMIT_FAIL_OPEN", "true")

	p := PolicyFromEnv()
	if p.PairLockThreshold != 4 || p.AddressLimit != 12 || p.AccountLimit != 9 || !p.FailOpen {
		t.Fatalf("overrides not applied: %+v", p)
	}
}

func TestNormalizeRefusesALockThresholdInsideTheFreeAllowance(t *testing.T) {
	p := Policy{PairFreeAttempts: 5, PairLockThreshold: 2}.normalize()
	if p.PairLockThreshold <= p.PairFreeAttempts {
		t.Fatalf("lock threshold %d is inside the free allowance %d, which turns the "+
			"progressive delay back into a hard cutoff", p.PairLockThreshold, p.PairFreeAttempts)
	}
}

// --------------------------------------------------------------------------
// Concurrency
// --------------------------------------------------------------------------

func TestConcurrentFailuresAreAllCounted(t *testing.T) {
	g, store, _ := newTestGuard(t)
	subject := NewSubject("198.51.100.50", "operator")

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, _ = g.RecordFailure(context.Background(), "login", subject)
		}()
	}
	wg.Wait()

	count, _, err := store.Get(context.Background(), g.addressKey(subject))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if count != workers {
		t.Fatalf("address counter = %d after %d concurrent failures, want %d", count, workers, workers)
	}
}
