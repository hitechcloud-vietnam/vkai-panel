package notify

import (
	"testing"
	"time"
)

// base is a fixed instant so every case reads as an offset from one clock.
var base = time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)

func at(offset time.Duration) time.Time { return base.Add(offset) }

func ptr(t time.Time) *time.Time { return &t }

// TestDecideFirstFiringAlwaysNotifies covers the case the whole feature exists
// for: nothing is known, the disk is full, someone has to be told.
func TestDecideFirstFiringAlwaysNotifies(t *testing.T) {
	d := Decide(nil, Observation{Kind: KindFiring, At: base, QuietPeriod: time.Hour, Value: 92, Threshold: 90})

	if !d.Notify {
		t.Fatalf("first firing did not notify: %+v", d)
	}
	if d.Reason != ReasonFirstFiring {
		t.Errorf("reason = %q, want %q", d.Reason, ReasonFirstFiring)
	}
	if !d.Persist {
		t.Errorf("first firing was not persisted, so the next check would notify again")
	}
	if d.State.State != StateFiring || d.State.Occurrences != 1 {
		t.Errorf("state = %+v, want firing with one occurrence", d.State)
	}
	if d.State.LastNotifiedAt == nil || !d.State.LastNotifiedAt.Equal(base) {
		t.Errorf("last notified = %v, want the observation time", d.State.LastNotifiedAt)
	}
}

// TestDecideSuppressesWithinQuietPeriod is the six-hours-of-messages case: a
// disk checked every five minutes must produce one message, not seventy-two.
func TestDecideSuppressesWithinQuietPeriod(t *testing.T) {
	state := AlertState{
		State:          StateFiring,
		FirstSeenAt:    base,
		LastSeenAt:     base,
		LastNotifiedAt: ptr(base),
		Occurrences:    1,
		QuietPeriod:    time.Hour,
	}

	// Five minutes later, still firing.
	d := Decide(&state, Observation{Kind: KindFiring, At: at(5 * time.Minute), QuietPeriod: time.Hour, Value: 93, Threshold: 90})

	if d.Notify {
		t.Fatalf("an observation five minutes into a one-hour quiet period notified: %+v", d)
	}
	if d.Reason != ReasonSuppressed {
		t.Errorf("reason = %q, want %q", d.Reason, ReasonSuppressed)
	}
	if !d.Persist {
		t.Errorf("a suppressed observation must still be recorded, or occurrences never rise")
	}
	if d.State.Occurrences != 2 {
		t.Errorf("occurrences = %d, want 2", d.State.Occurrences)
	}
	if !d.SuppressedUntil.Equal(at(time.Hour)) {
		t.Errorf("suppressed until %v, want %v", d.SuppressedUntil, at(time.Hour))
	}

	// The whole six hours, at the panel's five-minute check interval.
	notified := 1
	current := state
	for minute := 5; minute <= 360; minute += 5 {
		decision := Decide(&current, Observation{
			Kind: KindFiring, At: at(time.Duration(minute) * time.Minute),
			QuietPeriod: time.Hour, Value: 93, Threshold: 90,
		})
		if decision.Notify {
			notified++
		}
		current = decision.State
	}
	// One at the start plus one per elapsed hour.
	if notified != 7 {
		t.Errorf("six hours of five-minute checks produced %d messages, want 7 (one per hour)", notified)
	}
	if current.Occurrences != 73 {
		t.Errorf("occurrences = %d, want 73 checks folded into one incident", current.Occurrences)
	}
}

// TestDecideNotifiesAgainAfterQuietPeriod: an outage nobody fixed must not go
// quiet forever.
func TestDecideNotifiesAgainAfterQuietPeriod(t *testing.T) {
	state := AlertState{
		State:          StateFiring,
		FirstSeenAt:    base,
		LastSeenAt:     at(55 * time.Minute),
		LastNotifiedAt: ptr(base),
		Occurrences:    11,
		QuietPeriod:    time.Hour,
	}

	// Exactly on the boundary counts as elapsed.
	d := Decide(&state, Observation{Kind: KindFiring, At: at(time.Hour), QuietPeriod: time.Hour})
	if !d.Notify {
		t.Fatalf("an observation exactly one quiet period later did not notify: %+v", d)
	}
	if d.Reason != ReasonQuietElapsed {
		t.Errorf("reason = %q, want %q", d.Reason, ReasonQuietElapsed)
	}
	if d.State.FirstSeenAt != base {
		t.Errorf("first seen = %v, want the incident start preserved", d.State.FirstSeenAt)
	}
	if d.State.Occurrences != 12 {
		t.Errorf("occurrences = %d, want 12", d.State.Occurrences)
	}
}

// TestQuietPeriodMeasuredFromLastMessage is the rule that stops an alert going
// silent forever: the clock runs from the last message sent, not from the last
// observation seen.
func TestQuietPeriodMeasuredFromLastMessage(t *testing.T) {
	current := AlertState{
		State:          StateFiring,
		FirstSeenAt:    base,
		LastSeenAt:     base,
		LastNotifiedAt: ptr(base),
		Occurrences:    1,
		QuietPeriod:    time.Hour,
	}

	// Observations every 59 minutes. If the quiet period were measured from
	// the last observation, none of these would ever notify.
	notified := 0
	for i := 1; i <= 4; i++ {
		decision := Decide(&current, Observation{
			Kind: KindFiring, At: at(time.Duration(i) * 59 * time.Minute), QuietPeriod: time.Hour,
		})
		if decision.Notify {
			notified++
		}
		current = decision.State
	}
	// If the quiet period ran from the last observation, every gap would be 59
	// minutes and nothing would ever notify again - the alert would go silent
	// while the outage continued. Running it from the last message sent, the
	// observations at 118 and 236 minutes are each more than an hour after the
	// previous message, so two reminders get through.
	if notified == 0 {
		t.Fatalf("an alert observed every 59 minutes for four hours never notified again; " +
			"the quiet period is being measured from the last observation instead of the last message")
	}
	if notified != 2 {
		t.Errorf("notified %d times over four hours of 59-minute checks, want 2", notified)
	}
}

func TestDecideZeroQuietPeriodDisablesSuppression(t *testing.T) {
	state := AlertState{
		State: StateFiring, FirstSeenAt: base, LastSeenAt: base,
		LastNotifiedAt: ptr(base), Occurrences: 1,
	}
	d := Decide(&state, Observation{Kind: KindFiring, At: at(time.Second), QuietPeriod: 0})
	if !d.Notify {
		t.Fatalf("a zero quiet period suppressed an observation: %+v", d)
	}
	if d.Reason != ReasonNoQuietPeriod {
		t.Errorf("reason = %q, want %q", d.Reason, ReasonNoQuietPeriod)
	}
}

// TestDecideResolvesOnce: exactly one resolution message, and only for
// something that actually fired.
func TestDecideResolvesOnce(t *testing.T) {
	firing := AlertState{
		State: StateFiring, FirstSeenAt: base, LastSeenAt: at(time.Hour),
		LastNotifiedAt: ptr(base), Occurrences: 12, QuietPeriod: time.Hour,
	}

	first := Decide(&firing, Observation{Kind: KindResolved, At: at(90 * time.Minute), QuietPeriod: time.Hour, Value: 40, Threshold: 90})
	if !first.Notify {
		t.Fatalf("a firing alert that cleared sent no resolution: %+v", first)
	}
	if first.Kind != KindResolved || first.Reason != ReasonResolved {
		t.Errorf("kind/reason = %s/%s, want resolved/%s", first.Kind, first.Reason, ReasonResolved)
	}
	if first.State.State != StateResolved {
		t.Errorf("state = %q, want %q", first.State.State, StateResolved)
	}
	if first.State.FirstSeenAt != base {
		t.Errorf("first seen = %v, want the incident start preserved so duration can be shown", first.State.FirstSeenAt)
	}

	// A second resolve for the same key sends nothing.
	second := Decide(&first.State, Observation{Kind: KindResolved, At: at(95 * time.Minute), QuietPeriod: time.Hour})
	if second.Notify {
		t.Errorf("a second resolve sent another message: %+v", second)
	}
	if second.Reason != ReasonAlreadyResolved {
		t.Errorf("reason = %q, want %q", second.Reason, ReasonAlreadyResolved)
	}
}

// TestDecideResolveForAlertThatNeverFired: telling somebody a problem they were
// never told about is over trains them to ignore the channel.
func TestDecideResolveForAlertThatNeverFired(t *testing.T) {
	d := Decide(nil, Observation{Kind: KindResolved, At: base, QuietPeriod: time.Hour})
	if d.Notify {
		t.Fatalf("a resolve for an alert that never fired sent a message: %+v", d)
	}
	if d.Reason != ReasonNeverFired {
		t.Errorf("reason = %q, want %q", d.Reason, ReasonNeverFired)
	}
	if d.Persist {
		t.Errorf("a resolve for an unknown key created state for an incident that never existed")
	}
}

// TestDecideRefiringAfterResolveIsANewIncident: the disk fills up again and
// the operator has to hear about it, quiet period or not.
func TestDecideRefiringAfterResolveIsANewIncident(t *testing.T) {
	resolved := AlertState{
		State: StateResolved, FirstSeenAt: base, LastSeenAt: at(time.Hour),
		LastNotifiedAt: ptr(at(time.Hour)), Occurrences: 12, QuietPeriod: time.Hour,
	}

	// One minute after resolving - well inside the quiet period.
	d := Decide(&resolved, Observation{Kind: KindFiring, At: at(61 * time.Minute), QuietPeriod: time.Hour, Value: 95, Threshold: 90})
	if !d.Notify {
		t.Fatalf("an alert that fired again inside the quiet period was suppressed: %+v", d)
	}
	if d.Reason != ReasonRefiring {
		t.Errorf("reason = %q, want %q", d.Reason, ReasonRefiring)
	}
	if d.State.Occurrences != 1 {
		t.Errorf("occurrences = %d, want the count restarted for a new incident", d.State.Occurrences)
	}
	if !d.State.FirstSeenAt.Equal(at(61 * time.Minute)) {
		t.Errorf("first seen = %v, want the new incident's start", d.State.FirstSeenAt)
	}
}

// TestDecideFiringWithNoRecordedNotificationNotifies covers state that says
// "firing" but has never actually told anyone - which is a silent alert, the
// worst outcome available.
func TestDecideFiringWithNoRecordedNotificationNotifies(t *testing.T) {
	state := AlertState{State: StateFiring, FirstSeenAt: base, LastSeenAt: base, Occurrences: 3, QuietPeriod: time.Hour}
	d := Decide(&state, Observation{Kind: KindFiring, At: at(time.Minute), QuietPeriod: time.Hour})
	if !d.Notify {
		t.Fatalf("a firing alert that had never notified anyone stayed silent: %+v", d)
	}
}
