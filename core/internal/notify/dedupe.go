package notify

import "time"

// DefaultQuietPeriod is how long an incident stays quiet between reminders
// when a caller does not choose. One hour is the compromise the shape of the
// problem forces: short enough that a forgotten outage is raised again the
// same morning, long enough that a disk at 92% checked every five minutes
// produces one message an hour instead of twelve.
const DefaultQuietPeriod = time.Hour

// Alert states as stored in notification_alert_state.state.
const (
	StateFiring   = "firing"
	StateResolved = "resolved"
)

// AlertState is the stored state of one incident, keyed by the caller's dedup
// key.
type AlertState struct {
	State          string
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
	LastNotifiedAt *time.Time
	Occurrences    int
	QuietPeriod    time.Duration
	LastValue      float64
	Threshold      float64
}

// Observation is one report about an alert: it is firing, or it has cleared.
type Observation struct {
	Kind        EventKind
	At          time.Time
	QuietPeriod time.Duration
	Value       float64
	Threshold   float64
}

// Reasons a decision came out the way it did. They are returned to the caller
// and logged, so "why did I not get a message" has an answer that does not
// require reading this file.
const (
	ReasonFirstFiring     = "first_firing"
	ReasonRefiring        = "refiring_after_resolve"
	ReasonQuietElapsed    = "quiet_period_elapsed"
	ReasonNoQuietPeriod   = "no_quiet_period"
	ReasonSuppressed      = "suppressed_within_quiet_period"
	ReasonResolved        = "resolved"
	ReasonNeverFired      = "resolve_for_alert_that_never_fired"
	ReasonAlreadyResolved = "already_resolved"
	ReasonTest            = "test_send"
)

// Decision is the outcome of folding one observation into the stored state.
type Decision struct {
	// Notify says whether a message should be enqueued.
	Notify bool
	// Kind is the event kind to render: firing or resolved.
	Kind EventKind
	// Reason names the rule that decided, for the log and the API response.
	Reason string
	// Persist says whether State should be written back. It is false only for
	// a resolve of an alert that was never seen, where writing a row would
	// create state for an incident that never existed.
	Persist bool
	// State is the state to store when Persist is true.
	State AlertState
	// SuppressedUntil is set when Notify is false because of the quiet period:
	// the moment the next observation would be allowed to notify.
	SuppressedUntil time.Time
}

// Decide folds one observation into the previous state.
//
// This is the whole deduplication policy, as a pure function over its inputs,
// so it can be read in one sitting and tested without a database:
//
//   - A firing observation with no previous state, or whose previous state is
//     resolved, is a new incident and always notifies.
//   - A firing observation on an incident already firing notifies only if the
//     quiet period has elapsed since the last message. The quiet period is
//     measured from the last message sent, not from the last observation, so
//     suppressed checks do not push the next reminder further away - which
//     would let an alert that keeps firing go quiet forever.
//   - A resolve notifies exactly once, and only for an incident that had
//     actually fired. A resolve for something that never fired sends nothing:
//     an operator who was never told about a problem must not be told it is
//     over.
//   - A quiet period of zero or less disables suppression. It exists for
//     callers that have already deduplicated upstream.
func Decide(prev *AlertState, obs Observation) Decision {
	quiet := obs.QuietPeriod
	if quiet < 0 {
		quiet = 0
	}

	switch obs.Kind {
	case KindResolved:
		return decideResolved(prev, obs, quiet)
	default:
		return decideFiring(prev, obs, quiet)
	}
}

// decideFiring handles an observation that the condition is still crossed.
func decideFiring(prev *AlertState, obs Observation, quiet time.Duration) Decision {
	next := AlertState{
		State:       StateFiring,
		FirstSeenAt: obs.At,
		LastSeenAt:  obs.At,
		Occurrences: 1,
		QuietPeriod: quiet,
		LastValue:   obs.Value,
		Threshold:   obs.Threshold,
	}

	// A new incident: nothing stored, or the last thing stored was a resolve.
	if prev == nil || prev.State != StateFiring {
		reason := ReasonFirstFiring
		if prev != nil {
			reason = ReasonRefiring
		}
		at := obs.At
		next.LastNotifiedAt = &at
		return Decision{Notify: true, Kind: KindFiring, Reason: reason, Persist: true, State: next}
	}

	// An incident already firing: continue it.
	next.FirstSeenAt = prev.FirstSeenAt
	next.Occurrences = prev.Occurrences + 1
	next.LastNotifiedAt = prev.LastNotifiedAt

	if quiet == 0 {
		at := obs.At
		next.LastNotifiedAt = &at
		return Decision{Notify: true, Kind: KindFiring, Reason: ReasonNoQuietPeriod, Persist: true, State: next}
	}

	if prev.LastNotifiedAt == nil {
		// Firing but never actually told anyone: the previous attempt was
		// suppressed by a condition that no longer holds, or the state
		// predates delivery. Notify rather than stay silent.
		at := obs.At
		next.LastNotifiedAt = &at
		return Decision{Notify: true, Kind: KindFiring, Reason: ReasonQuietElapsed, Persist: true, State: next}
	}

	nextAllowed := prev.LastNotifiedAt.Add(quiet)
	if !obs.At.Before(nextAllowed) {
		at := obs.At
		next.LastNotifiedAt = &at
		return Decision{Notify: true, Kind: KindFiring, Reason: ReasonQuietElapsed, Persist: true, State: next}
	}

	return Decision{
		Notify:          false,
		Kind:            KindFiring,
		Reason:          ReasonSuppressed,
		Persist:         true,
		State:           next,
		SuppressedUntil: nextAllowed,
	}
}

// decideResolved handles an observation that the condition has cleared.
func decideResolved(prev *AlertState, obs Observation, quiet time.Duration) Decision {
	if prev == nil {
		// Nothing ever fired under this key. Telling somebody a problem they
		// were never told about is over is noise that trains people to ignore
		// the channel.
		return Decision{Notify: false, Kind: KindResolved, Reason: ReasonNeverFired, Persist: false}
	}

	if prev.State != StateFiring {
		next := *prev
		next.LastSeenAt = obs.At
		next.QuietPeriod = quiet
		return Decision{Notify: false, Kind: KindResolved, Reason: ReasonAlreadyResolved, Persist: true, State: next}
	}

	at := obs.At
	next := AlertState{
		State:          StateResolved,
		FirstSeenAt:    prev.FirstSeenAt,
		LastSeenAt:     obs.At,
		LastNotifiedAt: &at,
		Occurrences:    prev.Occurrences,
		QuietPeriod:    quiet,
		LastValue:      obs.Value,
		Threshold:      obs.Threshold,
	}
	return Decision{Notify: true, Kind: KindResolved, Reason: ReasonResolved, Persist: true, State: next}
}
