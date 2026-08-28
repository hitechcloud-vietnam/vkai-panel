// Package ratelimit implements the credential-guessing defence for the panel.
//
// The threat it answers is not "too many requests". It is an attacker with a
// password list and either one address or ten thousand of them. One counter is
// always the wrong counter for that, so the guard keeps three:
//
//   - per account, so a single victim cannot be ground down from many
//     addresses at once;
//   - per source address, so a single address cannot spray many accounts;
//   - per address-and-account pair, which is the ordinary case and the only
//     dimension that is allowed to lock.
//
// The pair dimension does not cut off at N. A hard cutoff on a known username
// is a free denial of service: an attacker who knows an administrator's login
// name can keep that administrator out of their own panel forever, at the cost
// of a handful of requests. Instead the pair dimension applies a delay that
// starts at nothing, grows sharply, and only then locks - and the lock is
// scoped to one address and one account, so it can never take the account away
// from everybody. An address that has authenticated successfully for that
// account before is "recognised": a lock downgrades to a delay for it, and a
// correct password clears the lock outright.
//
// Counters live in Redis so every panel instance sees the same numbers, and
// every key carries an expiry so nothing accumulates. On a Redis error the
// guard fails CLOSED for authentication: an outage must not quietly turn the
// protection off, because "the cache is down" is a state an attacker can often
// arrange. See Policy.FailOpen for the operator escape hatch.
package ratelimit
