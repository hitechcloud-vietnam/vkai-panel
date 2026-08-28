package ratelimit

import (
	"context"
	"time"
)

// Outcome names what the guard decided. These strings are stable: they are
// written into the authentication log that the shipped fail2ban filter reads,
// so renaming one silently changes what an operator's jail matches.
type Outcome string

const (
	// OutcomeAllowed - the attempt may proceed immediately.
	OutcomeAllowed Outcome = "allowed"
	// OutcomeDelayed - the attempt may proceed after Decision.Delay.
	OutcomeDelayed Outcome = "delayed"
	// OutcomeLocked - this address-account pair is locked out.
	OutcomeLocked Outcome = "locked"
	// OutcomeThrottled - an address or account dimension is over budget.
	OutcomeThrottled Outcome = "throttled"
	// OutcomeUnavailable - the counter store could not be reached and the
	// guard failed closed.
	OutcomeUnavailable Outcome = "unavailable"
)

// Dimension names which layer produced a decision. It is recorded for the
// operator, never returned to the caller: which counter tripped is a detail an
// attacker would use to work out which one to route around.
type Dimension string

const (
	DimensionNone    Dimension = ""
	DimensionPair    Dimension = "pair"
	DimensionAddress Dimension = "address"
	DimensionAccount Dimension = "account"
	DimensionStore   Dimension = "store"
)

// Decision is the guard's answer about one attempt.
type Decision struct {
	// Allow reports whether the attempt may reach the credential check.
	Allow bool
	// Delay is how long the caller must pause before running the credential
	// check. It is only meaningful when Allow is true.
	Delay time.Duration
	// RetryAfter is how long the caller should tell the client to wait. It is
	// only meaningful when Allow is false.
	RetryAfter time.Duration
	// Outcome and Dimension describe why.
	Outcome   Outcome
	Dimension Dimension
	// Failures is the pair failure count behind the decision.
	Failures int
	// Recognised reports whether this address has authenticated successfully
	// for this account before.
	Recognised bool
}

// Guard applies a Policy over a Store.
type Guard struct {
	store  Store
	policy Policy
}

// New builds a Guard. A nil store is accepted and makes every decision a
// fail-closed denial, which is the correct behaviour for a panel that was
// started without Redis rather than one that quietly runs unprotected.
func New(store Store, policy Policy) *Guard {
	return &Guard{store: store, policy: policy.normalize()}
}

// Policy returns the numbers in force.
func (g *Guard) Policy() Policy { return g.policy }

// unavailable is the fail-closed answer. FailOpen inverts it, and exists only
// so an operator can trade the protection away deliberately and in writing.
func (g *Guard) unavailable(err error) (Decision, error) {
	if g.policy.FailOpen {
		return Decision{Allow: true, Outcome: OutcomeAllowed, Dimension: DimensionStore}, err
	}
	return Decision{
		Allow:      false,
		RetryAfter: 30 * time.Second,
		Outcome:    OutcomeUnavailable,
		Dimension:  DimensionStore,
	}, err
}

// Check is called before the credential is verified. It never increments a
// counter: an attempt that is refused here must not also deepen the hole it
// was refused from, or a client retrying a 429 would extend its own lockout
// without ever reaching the password check.
//
// The dimensions are consulted in order of how little they trust the caller:
//
//  1. the source address, which applies to everybody including a recognised
//     one, because a recognised address that has started producing failures at
//     scale is an address somebody else is now using;
//  2. the address-account lock, which stops an unrecognised pair outright and
//     only slows a recognised one;
//  3. the account, which a recognised address is exempt from.
func (g *Guard) Check(ctx context.Context, scope string, subject Subject) (Decision, error) {
	if g.store == nil {
		return g.unavailable(ErrStoreUnavailable)
	}

	recognised, err := g.isRecognised(ctx, subject)
	if err != nil {
		return g.unavailable(err)
	}

	pairFailures, _, err := g.store.Get(ctx, g.pairKey(scope, subject))
	if err != nil {
		return g.unavailable(err)
	}

	// One address spraying accounts. This is the only ceiling a recognised
	// address is still subject to, and it is what bounds an attacker who
	// happens to share a source address with a legitimate user.
	addressFailures, _, err := g.store.Get(ctx, g.addressKey(subject))
	if err != nil {
		return g.unavailable(err)
	}
	if int(addressFailures) >= g.policy.AddressLimit {
		ttl, ttlErr := g.store.TTL(ctx, g.addressKey(subject))
		if ttlErr != nil {
			return g.unavailable(ttlErr)
		}
		return Decision{
			Allow:      false,
			RetryAfter: ttl,
			Outcome:    OutcomeThrottled,
			Dimension:  DimensionAddress,
			Failures:   int(pairFailures),
			Recognised: recognised,
		}, nil
	}

	// The pair lock: the most specific dimension and the only one that locks.
	lockTTL, err := g.store.TTL(ctx, g.lockKey(scope, subject))
	if err != nil {
		return g.unavailable(err)
	}
	if lockTTL > 0 {
		if recognised {
			// A recognised address is slowed by its own lock, not shut out by
			// it: it may still present a password, and a correct one clears
			// the lock. Otherwise an attacker who knows an administrator's
			// username could keep that administrator out of the panel
			// indefinitely for the price of eight requests - which is the
			// denial of service a hard cutoff hands out for free.
			return Decision{
				Allow:      true,
				Delay:      g.policy.MaxDelay,
				Outcome:    OutcomeDelayed,
				Dimension:  DimensionPair,
				Failures:   int(pairFailures),
				Recognised: true,
			}, nil
		}
		return Decision{
			Allow:      false,
			RetryAfter: lockTTL,
			Outcome:    OutcomeLocked,
			Dimension:  DimensionPair,
			Failures:   int(pairFailures),
		}, nil
	}

	// Many addresses grinding one account. A recognised address is exempt:
	// this dimension exists to stop a botnet, and it must not become the
	// botnet's way of locking the real owner out of their own account.
	if !recognised {
		accountFailures, _, acctErr := g.store.Get(ctx, g.accountKey(subject))
		if acctErr != nil {
			return g.unavailable(acctErr)
		}
		if int(accountFailures) >= g.policy.AccountLimit {
			ttl, ttlErr := g.store.TTL(ctx, g.accountKey(subject))
			if ttlErr != nil {
				return g.unavailable(ttlErr)
			}
			return Decision{
				Allow:      false,
				RetryAfter: ttl,
				Outcome:    OutcomeThrottled,
				Dimension:  DimensionAccount,
				Failures:   int(pairFailures),
			}, nil
		}
	}

	delay := g.policy.delayFor(int(pairFailures))
	outcome := OutcomeAllowed
	dimension := DimensionNone
	if delay > 0 {
		outcome = OutcomeDelayed
		dimension = DimensionPair
	}
	return Decision{
		Allow:      true,
		Delay:      delay,
		Outcome:    outcome,
		Dimension:  dimension,
		Failures:   int(pairFailures),
		Recognised: recognised,
	}, nil
}

// RecordFailure books one failed credential check against all three
// dimensions and returns the standing that the next attempt will meet.
func (g *Guard) RecordFailure(ctx context.Context, scope string, subject Subject) (Decision, error) {
	if g.store == nil {
		return g.unavailable(ErrStoreUnavailable)
	}

	recognised, err := g.isRecognised(ctx, subject)
	if err != nil {
		return g.unavailable(err)
	}

	pairFailures, err := g.store.Incr(ctx, g.pairKey(scope, subject), g.policy.PairWindow)
	if err != nil {
		return g.unavailable(err)
	}

	if _, err := g.store.Incr(ctx, g.addressKey(subject), g.policy.AddressWindow); err != nil {
		return g.unavailable(err)
	}

	// A recognised address does not feed the account dimension. If it did, a
	// user fumbling their own password on their own laptop would help an
	// attacker push that account over the distributed threshold.
	if !recognised {
		if _, err := g.store.Incr(ctx, g.accountKey(subject), g.policy.AccountWindow); err != nil {
			return g.unavailable(err)
		}
	}

	if int(pairFailures) < g.policy.PairLockThreshold {
		delay := g.policy.delayFor(int(pairFailures))
		outcome := OutcomeAllowed
		dimension := DimensionNone
		if delay > 0 {
			outcome = OutcomeDelayed
			dimension = DimensionPair
		}
		return Decision{
			Allow:      true,
			Delay:      delay,
			Outcome:    outcome,
			Dimension:  dimension,
			Failures:   int(pairFailures),
			Recognised: recognised,
		}, nil
	}

	// Past the lock threshold. The lock is recorded for a recognised pair too
	// - it is what stops the counter climbing forever - but Check turns it
	// into a delay rather than a refusal for that pair.
	lockTTL, err := g.store.TTL(ctx, g.lockKey(scope, subject))
	if err != nil {
		return g.unavailable(err)
	}
	if lockTTL > 0 {
		return Decision{
			Allow:      recognised,
			Delay:      recognisedDelay(recognised, g.policy.MaxDelay),
			RetryAfter: lockTTL,
			Outcome:    lockedOutcome(recognised),
			Dimension:  DimensionPair,
			Failures:   int(pairFailures),
			Recognised: recognised,
		}, nil
	}

	step, err := g.store.Incr(ctx, g.lockStepKey(scope, subject), g.policy.LockMemory)
	if err != nil {
		return g.unavailable(err)
	}
	duration := g.policy.lockFor(int(step))
	if err := g.store.Set(ctx, g.lockKey(scope, subject), int64(step), duration); err != nil {
		return g.unavailable(err)
	}
	// The lock consumes the failures that earned it, so the cycle after it
	// starts from zero: a fixed number of attempts per lock, with the lock
	// itself getting longer each time.
	if err := g.store.Delete(ctx, g.pairKey(scope, subject)); err != nil {
		return g.unavailable(err)
	}

	return Decision{
		Allow:      recognised,
		Delay:      recognisedDelay(recognised, g.policy.MaxDelay),
		RetryAfter: duration,
		Outcome:    lockedOutcome(recognised),
		Dimension:  DimensionPair,
		Failures:   int(pairFailures),
		Recognised: recognised,
	}, nil
}

func recognisedDelay(recognised bool, maxDelay time.Duration) time.Duration {
	if recognised {
		return maxDelay
	}
	return 0
}

func lockedOutcome(recognised bool) Outcome {
	if recognised {
		return OutcomeDelayed
	}
	return OutcomeLocked
}

// RecordSuccess clears everything this pair accumulated and recognises the
// address for the account. A store error here is reported but is not a reason
// to fail an authentication that has already succeeded.
func (g *Guard) RecordSuccess(ctx context.Context, scope string, subject Subject) error {
	if g.store == nil {
		return ErrStoreUnavailable
	}
	if err := g.store.Delete(ctx,
		g.pairKey(scope, subject),
		g.lockKey(scope, subject),
		g.lockStepKey(scope, subject),
	); err != nil {
		return err
	}
	return g.store.Set(ctx, g.recognisedKey(subject), 1, g.policy.RecognisedTTL)
}

// Recognised reports whether the address has authenticated for the account
// before.
func (g *Guard) Recognised(ctx context.Context, subject Subject) (bool, error) {
	if g.store == nil {
		return false, ErrStoreUnavailable
	}
	return g.isRecognised(ctx, subject)
}

func (g *Guard) isRecognised(ctx context.Context, subject Subject) (bool, error) {
	if subject.Account == "" {
		return false, nil
	}
	_, ok, err := g.store.Get(ctx, g.recognisedKey(subject))
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (g *Guard) pairKey(scope string, s Subject) string {
	return g.policy.KeyPrefix + ":pair:" + scope + ":" + s.Address + ":" + accountKey(s.Account)
}

func (g *Guard) lockKey(scope string, s Subject) string {
	return g.policy.KeyPrefix + ":lock:" + scope + ":" + s.Address + ":" + accountKey(s.Account)
}

func (g *Guard) lockStepKey(scope string, s Subject) string {
	return g.policy.KeyPrefix + ":step:" + scope + ":" + s.Address + ":" + accountKey(s.Account)
}

// The address and account dimensions are deliberately shared across scopes: an
// attacker must not get a fresh budget by moving from the login form to the
// password reset form. The pair dimension is per scope so that, for example,
// fumbling a two-factor code does not lock the password form.
func (g *Guard) addressKey(s Subject) string {
	return g.policy.KeyPrefix + ":addr:" + s.Address
}

func (g *Guard) accountKey(s Subject) string {
	return g.policy.KeyPrefix + ":acct:" + accountKey(s.Account)
}

func (g *Guard) recognisedKey(s Subject) string {
	return g.policy.KeyPrefix + ":known:" + accountKey(s.Account) + ":" + s.Address
}
