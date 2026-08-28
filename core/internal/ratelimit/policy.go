package ratelimit

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Policy holds the numbers. Every default below is stated with the reason it
// has that value, because a limit nobody can justify is a limit somebody will
// raise to silence a support ticket.
type Policy struct {
	// PairWindow is how long failures against one address-account pair are
	// remembered. Fifteen minutes spans a burst of guessing while still
	// clearing on its own for a user who walked away and came back.
	PairWindow time.Duration

	// PairFreeAttempts is how many failures cost nothing. Three is the human
	// typo budget: a wrong password, the same wrong password with caps lock,
	// and one more before the user stops and looks at the keyboard.
	PairFreeAttempts int

	// BaseDelay is the delay applied to the first failure past the free
	// attempts; it doubles on each subsequent failure up to MaxDelay.
	//
	// The delay is deliberately server-side sleep rather than an error: an
	// error tells the attacker to move on to the next candidate immediately,
	// while a held connection costs them a worker. MaxDelay is capped low
	// enough that the panel cannot be made to hold thousands of sleeping
	// requests as a memory exhaustion trick of its own.
	BaseDelay time.Duration
	MaxDelay  time.Duration

	// PairLockThreshold is the failure count at which the pair locks. Eight is
	// two rounds of "I am sure I know this password" past the free attempts,
	// and it is the point past which no honest session continues.
	PairLockThreshold int

	// LockSteps are the successive lock durations for a pair that keeps
	// failing. Each lock costs the attacker the whole step; escalation means a
	// patient attacker gets slower rather than merely inconvenienced once.
	LockSteps []time.Duration

	// LockMemory is how long the escalation step is remembered, so an attacker
	// cannot reset themselves to the one-minute step by pausing for the length
	// of a lock.
	LockMemory time.Duration

	// AddressLimit / AddressWindow throttle one source address across all
	// accounts - the spray case. Sixty failures in fifteen minutes is roughly
	// eight distinct users each failing all the way to their pair lock at the
	// same instant from behind one NAT gateway, which is far past anything an
	// office produces and well short of anything useful for spraying.
	AddressLimit  int
	AddressWindow time.Duration

	// AccountLimit / AccountWindow throttle one account across all addresses -
	// the distributed case. Only failures from addresses that have never
	// successfully authenticated for that account are counted, which is what
	// keeps this dimension from becoming the denial of service it is meant to
	// prevent: the victim's own laptop is exempt, the botnet is not.
	//
	// Thirty in fifteen minutes is 120 an hour. It takes at least four
	// distinct addresses to reach it, and it costs an attacker with ten
	// thousand candidate passwords more than three months.
	AccountLimit  int
	AccountWindow time.Duration

	// RecognisedTTL is how long an address stays recognised for an account
	// after a successful authentication.
	RecognisedTTL time.Duration

	// FailOpen inverts the behaviour on a store error. It defaults to false
	// and should stay false: an attacker who can make Redis unreachable would
	// otherwise be able to switch the whole defence off.
	FailOpen bool

	// KeyPrefix namespaces every key, so the panel's counters cannot collide
	// with anything else sharing the Redis instance.
	KeyPrefix string
}

// DefaultPolicy returns the shipped numbers.
//
// What they add up to, for an attacker who knows a username:
//
//   - From one address: 8 attempts, then a lock. After four lock cycles the
//     steady rate is 8 attempts per hour. A ten-thousand-entry password list
//     takes about 52 days.
//   - From many addresses: 30 attempts per fifteen minutes in total, because
//     none of those addresses is recognised for the account. The same list
//     takes about 3.5 days, and a password with real entropy is out of reach
//     entirely.
//   - The legitimate user, from an address they have logged in from before,
//     is exempt from the account dimension and can clear their own lock with
//     a correct password. They notice none of this.
//   - An attacker who shares a source address with such a user inherits that
//     exemption, and is bounded instead by the address dimension: 60 failures
//     per fifteen minutes, 240 an hour, and then nothing at all until the
//     window rolls. That is the deliberate trade. The alternative is locking
//     an administrator out of the panel that holds root on every server they
//     manage, which is a worse outcome than slowing down an attacker who is
//     already inside that administrator's network.
func DefaultPolicy() Policy {
	return Policy{
		PairWindow:        15 * time.Minute,
		PairFreeAttempts:  3,
		BaseDelay:         500 * time.Millisecond,
		MaxDelay:          4 * time.Second,
		PairLockThreshold: 8,
		LockSteps: []time.Duration{
			1 * time.Minute,
			5 * time.Minute,
			15 * time.Minute,
			1 * time.Hour,
		},
		LockMemory:    24 * time.Hour,
		AddressLimit:  60,
		AddressWindow: 15 * time.Minute,
		AccountLimit:  30,
		AccountWindow: 15 * time.Minute,
		RecognisedTTL: 30 * 24 * time.Hour,
		FailOpen:      false,
		KeyPrefix:     "vkai:authlimit",
	}
}

// normalize fills in anything a caller left at zero and rejects nonsense, so a
// half-built Policy cannot silently disable a dimension.
func (p Policy) normalize() Policy {
	d := DefaultPolicy()
	if p.PairWindow <= 0 {
		p.PairWindow = d.PairWindow
	}
	if p.PairFreeAttempts < 0 {
		p.PairFreeAttempts = d.PairFreeAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = d.BaseDelay
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = d.MaxDelay
	}
	if p.MaxDelay < p.BaseDelay {
		p.MaxDelay = p.BaseDelay
	}
	if p.PairLockThreshold <= 0 {
		p.PairLockThreshold = d.PairLockThreshold
	}
	if p.PairLockThreshold <= p.PairFreeAttempts {
		// A lock at or below the free attempts would skip the progressive
		// delay entirely and turn the pair dimension back into a hard cutoff.
		p.PairLockThreshold = p.PairFreeAttempts + 1
	}
	if len(p.LockSteps) == 0 {
		p.LockSteps = d.LockSteps
	}
	if p.LockMemory <= 0 {
		p.LockMemory = d.LockMemory
	}
	if p.AddressLimit <= 0 {
		p.AddressLimit = d.AddressLimit
	}
	if p.AddressWindow <= 0 {
		p.AddressWindow = d.AddressWindow
	}
	if p.AccountLimit <= 0 {
		p.AccountLimit = d.AccountLimit
	}
	if p.AccountWindow <= 0 {
		p.AccountWindow = d.AccountWindow
	}
	if p.RecognisedTTL <= 0 {
		p.RecognisedTTL = d.RecognisedTTL
	}
	if strings.TrimSpace(p.KeyPrefix) == "" {
		p.KeyPrefix = d.KeyPrefix
	}
	return p
}

// delayFor returns the pause owed after the given number of failures against
// one pair. It is zero inside the free allowance and doubles after it.
func (p Policy) delayFor(failures int) time.Duration {
	if failures <= p.PairFreeAttempts {
		return 0
	}
	steps := failures - p.PairFreeAttempts - 1
	delay := p.BaseDelay
	for i := 0; i < steps; i++ {
		delay *= 2
		if delay >= p.MaxDelay {
			return p.MaxDelay
		}
	}
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

// lockFor returns the duration of the step-th lock, counting from one. Steps
// past the end of the table repeat the last, longest one.
func (p Policy) lockFor(step int) time.Duration {
	if step < 1 {
		step = 1
	}
	if step > len(p.LockSteps) {
		step = len(p.LockSteps)
	}
	return p.LockSteps[step-1]
}

// PolicyFromEnv returns DefaultPolicy with the handful of operator overrides
// applied. Anything unset or unparseable keeps the shipped value: a typo in a
// unit file must not widen a limit.
//
//	VKAI_AUTH_LIMIT_FAIL_OPEN   allow authentication when Redis is unreachable
//	VKAI_AUTH_LIMIT_PAIR_LOCK   failures before an address-account pair locks
//	VKAI_AUTH_LIMIT_ADDRESS     failures per source address per window
//	VKAI_AUTH_LIMIT_ACCOUNT     failures per account per window from unknown addresses
func PolicyFromEnv() Policy {
	p := DefaultPolicy()

	if truthy(os.Getenv("VKAI_AUTH_LIMIT_FAIL_OPEN")) {
		p.FailOpen = true
	}
	if n, ok := positiveInt(os.Getenv("VKAI_AUTH_LIMIT_PAIR_LOCK")); ok {
		p.PairLockThreshold = n
	}
	if n, ok := positiveInt(os.Getenv("VKAI_AUTH_LIMIT_ADDRESS")); ok {
		p.AddressLimit = n
	}
	if n, ok := positiveInt(os.Getenv("VKAI_AUTH_LIMIT_ACCOUNT")); ok {
		p.AccountLimit = n
	}

	return p.normalize()
}

func truthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func positiveInt(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
