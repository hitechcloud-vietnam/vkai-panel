package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// fakeStore is an in-memory outbox. It records every state change so a test
// can assert on the sequence of attempts, not just the final row.
type fakeStore struct {
	mu sync.Mutex

	pending map[uuid.UUID]*Delivery
	due     map[uuid.UUID]time.Time
	sending map[uuid.UUID]time.Time

	sent        []uuid.UUID
	deadLetters []deadLetterEntry
	reschedules []rescheduleEntry

	claimErr error
}

type deadLetterEntry struct {
	ID     uuid.UUID
	Reason string
}

type rescheduleEntry struct {
	ID       uuid.UUID
	Attempts int
	NextAt   time.Time
	Reason   string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		pending: make(map[uuid.UUID]*Delivery),
		due:     make(map[uuid.UUID]time.Time),
		sending: make(map[uuid.UUID]time.Time),
	}
}

// add queues a delivery, due immediately.
func (s *fakeStore) add(d Delivery) uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.MaxAttempts == 0 {
		d.MaxAttempts = DefaultMaxAttempts
	}
	copied := d
	s.pending[d.ID] = &copied
	s.due[d.ID] = time.Time{}
	return d.ID
}

func (s *fakeStore) ReleaseStale(_ context.Context, stuckSince time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	released := 0
	for id, since := range s.sending {
		if since.Before(stuckSince) {
			delete(s.sending, id)
			s.due[id] = time.Time{}
			released++
		}
	}
	return released, nil
}

func (s *fakeStore) ClaimDue(_ context.Context, now time.Time, limit int) ([]Delivery, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Delivery
	for id, dueAt := range s.due {
		if len(out) >= limit {
			break
		}
		if dueAt.After(now) {
			continue
		}
		delete(s.due, id)
		s.sending[id] = now
		out = append(out, *s.pending[id])
	}
	return out, nil
}

func (s *fakeStore) MarkSent(_ context.Context, id uuid.UUID, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sending, id)
	s.sent = append(s.sent, id)
	return nil
}

func (s *fakeStore) Reschedule(_ context.Context, id uuid.UUID, attempts int, nextAt time.Time, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sending, id)
	s.pending[id].Attempts = attempts
	s.due[id] = nextAt
	s.reschedules = append(s.reschedules, rescheduleEntry{ID: id, Attempts: attempts, NextAt: nextAt, Reason: lastError})
	return nil
}

func (s *fakeStore) DeadLetter(_ context.Context, id uuid.UUID, _ time.Time, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sending, id)
	delete(s.due, id)
	s.deadLetters = append(s.deadLetters, deadLetterEntry{ID: id, Reason: lastError})
	return nil
}

// scriptedSender fails a set number of times and then succeeds, or fails
// forever with a chosen error.
type scriptedSender struct {
	mu       sync.Mutex
	calls    int
	failWith []error
	messages []Message
}

func (s *scriptedSender) Type() string { return "scripted" }

func (s *scriptedSender) Send(_ context.Context, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.messages = append(s.messages, msg)
	if len(s.failWith) == 0 {
		return nil
	}
	err := s.failWith[0]
	if len(s.failWith) > 1 {
		s.failWith = s.failWith[1:]
	}
	return err
}

func (s *scriptedSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// testRegistry returns a registry whose only sender is the one given.
func testRegistry(sender Sender) *Registry {
	registry := NewRegistry(Deps{})
	registry.Register("scripted", func(Config, Deps) (Sender, error) { return sender, nil })
	return registry
}

// fixedClock advances only when a test tells it to.
type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixedClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// scriptedDelivery is a delivery bound to the scripted channel type.
func scriptedDelivery() Delivery {
	return Delivery{
		TenantID:      uuid.New(),
		ChannelID:     uuid.New(),
		ChannelName:   "ops on-call",
		ChannelType:   "scripted",
		ChannelConfig: Config{},
		ChannelActive: true,
		DedupKey:      "server:web-01:disk:/var",
		Kind:          KindFiring,
		Subject:       "[CRITICAL] web-01: disk /var is 92.5%",
		Body:          "disk /var on web-01 is 92.5%",
		Alert:         diskAlert(),
		MaxAttempts:   DefaultMaxAttempts,
	}
}

func TestBackoffDoublesAndIsCapped(t *testing.T) {
	base := 30 * time.Second
	max := 15 * time.Minute

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 30 * time.Second},
		{2, time.Minute},
		{3, 2 * time.Minute},
		{4, 4 * time.Minute},
		{5, 8 * time.Minute},
		{6, 15 * time.Minute},  // capped
		{20, 15 * time.Minute}, // still capped
		{40, 15 * time.Minute}, // past the point the shift would overflow
		{0, 30 * time.Second},  // defensive
	}
	for _, tc := range cases {
		if got := Backoff(tc.attempt, base, max); got != tc.want {
			t.Errorf("Backoff(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}

	// A retry that is retried forever becomes a second outage: the cap is what
	// stops the delay growing without bound.
	if Backoff(100, base, max) > max {
		t.Errorf("Backoff exceeded the cap")
	}
}

// TestDispatcherRetriesThenSucceeds: a mail server restarting must not cost an
// alert.
func TestDispatcherRetriesThenSucceeds(t *testing.T) {
	store := newFakeStore()
	id := store.add(scriptedDelivery())

	sender := &scriptedSender{failWith: []error{
		errors.New("dial tcp 10.0.0.5:587: connect: connection refused"),
		errors.New("dial tcp 10.0.0.5:587: connect: connection refused"),
		nil,
	}}
	clock := &fixedClock{now: time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)}

	dispatcher := NewDispatcher(store, testRegistry(sender), zap.NewNop(), DispatcherOptions{
		BaseBackoff: 30 * time.Second,
		MaxBackoff:  15 * time.Minute,
		Now:         clock.Now,
	})
	ctx := context.Background()

	// Pass 1: fails, rescheduled 30s out.
	if _, err := dispatcher.RunOnce(ctx); err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	if len(store.reschedules) != 1 {
		t.Fatalf("after one failure there are %d reschedules, want 1", len(store.reschedules))
	}
	if got := store.reschedules[0].NextAt.Sub(clock.Now()); got != 30*time.Second {
		t.Errorf("first retry scheduled %v out, want 30s", got)
	}

	// Nothing is due yet, so a pass right now must do nothing rather than
	// hammer the failing server.
	if attempted, _ := dispatcher.RunOnce(ctx); attempted != 0 {
		t.Errorf("a delivery was attempted before its backoff elapsed")
	}

	// Pass 2: 30s later, fails again, rescheduled 60s out.
	clock.advance(30 * time.Second)
	if _, err := dispatcher.RunOnce(ctx); err != nil {
		t.Fatalf("pass 2: %v", err)
	}
	if len(store.reschedules) != 2 {
		t.Fatalf("reschedules = %d, want 2", len(store.reschedules))
	}
	if got := store.reschedules[1].NextAt.Sub(clock.Now()); got != time.Minute {
		t.Errorf("second retry scheduled %v out, want 1m (the backoff must double)", got)
	}

	// Pass 3: succeeds.
	clock.advance(time.Minute)
	if _, err := dispatcher.RunOnce(ctx); err != nil {
		t.Fatalf("pass 3: %v", err)
	}
	if len(store.sent) != 1 || store.sent[0] != id {
		t.Fatalf("the delivery was not marked sent: %v", store.sent)
	}
	if len(store.deadLetters) != 0 {
		t.Errorf("a delivery that eventually succeeded was dead-lettered: %v", store.deadLetters)
	}
	if sender.callCount() != 3 {
		t.Errorf("the sender was called %d times, want 3", sender.callCount())
	}
}

// TestDispatcherDeadLettersAfterTheAttemptBudget: an alert retried forever is
// a second outage, so the attempts are bounded - and the row that is left
// behind is how nobody finds out silently.
func TestDispatcherDeadLettersAfterTheAttemptBudget(t *testing.T) {
	store := newFakeStore()
	delivery := scriptedDelivery()
	delivery.MaxAttempts = 3
	id := store.add(delivery)

	sender := &scriptedSender{failWith: []error{errors.New("connection refused")}}
	clock := &fixedClock{now: time.Now()}

	var hooked []string
	dispatcher := NewDispatcher(store, testRegistry(sender), zap.NewNop(), DispatcherOptions{
		BaseBackoff: time.Second,
		MaxBackoff:  time.Minute,
		Now:         clock.Now,
		OnDeadLetter: func(_ context.Context, d Delivery, cause string) {
			hooked = append(hooked, cause)
		},
	})
	ctx := context.Background()

	for pass := 0; pass < 10; pass++ {
		if _, err := dispatcher.RunOnce(ctx); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		clock.advance(2 * time.Minute)
	}

	if sender.callCount() != 3 {
		t.Errorf("the sender was called %d times, want exactly the 3-attempt budget", sender.callCount())
	}
	if len(store.deadLetters) != 1 {
		t.Fatalf("dead letters = %d, want 1", len(store.deadLetters))
	}
	if store.deadLetters[0].ID != id {
		t.Errorf("the wrong delivery was dead-lettered")
	}
	if !strings.Contains(store.deadLetters[0].Reason, "gave up after 3 attempts") {
		t.Errorf("the dead letter does not say why: %q", store.deadLetters[0].Reason)
	}
	if !strings.Contains(store.deadLetters[0].Reason, "connection refused") {
		t.Errorf("the dead letter does not carry the last failure: %q", store.deadLetters[0].Reason)
	}

	// The hook is what puts the failure somewhere a human looks.
	if len(hooked) != 1 {
		t.Fatalf("the dead-letter hook fired %d times, want 1", len(hooked))
	}

	// And it stops: no further attempts once the row is dead.
	before := sender.callCount()
	clock.advance(time.Hour)
	_, _ = dispatcher.RunOnce(ctx)
	if sender.callCount() != before {
		t.Errorf("a dead-lettered delivery was attempted again")
	}
}

// TestDispatcherDeadLettersAPermanentFailureImmediately: retrying a rejected
// password five times only delays the message that tells the operator about it.
func TestDispatcherDeadLettersAPermanentFailureImmediately(t *testing.T) {
	store := newFakeStore()
	store.add(scriptedDelivery())

	sender := &scriptedSender{failWith: []error{
		Permanent(errors.New("SMTP AUTH failed: 535 5.7.8 Authentication credentials invalid")),
	}}
	dispatcher := NewDispatcher(store, testRegistry(sender), zap.NewNop(), DispatcherOptions{
		BaseBackoff: time.Second,
	})

	if _, err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if sender.callCount() != 1 {
		t.Errorf("a permanent failure was attempted %d times, want 1", sender.callCount())
	}
	if len(store.reschedules) != 0 {
		t.Errorf("a permanent failure was rescheduled: %v", store.reschedules)
	}
	if len(store.deadLetters) != 1 {
		t.Fatalf("dead letters = %d, want 1", len(store.deadLetters))
	}
	if !strings.Contains(store.deadLetters[0].Reason, "permanent failure") {
		t.Errorf("the dead letter does not say it was permanent: %q", store.deadLetters[0].Reason)
	}
	if !strings.Contains(store.deadLetters[0].Reason, "535") {
		t.Errorf("the dead letter dropped the server's own reply: %q", store.deadLetters[0].Reason)
	}
}

// TestDispatcherDeadLettersADisabledChannel: an alert queued for a channel
// that has since been turned off must not silently disappear.
func TestDispatcherDeadLettersADisabledChannel(t *testing.T) {
	store := newFakeStore()
	delivery := scriptedDelivery()
	delivery.ChannelActive = false
	store.add(delivery)

	sender := &scriptedSender{}
	dispatcher := NewDispatcher(store, testRegistry(sender), zap.NewNop(), DispatcherOptions{})

	if _, err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if sender.callCount() != 0 {
		t.Errorf("a disabled channel was sent to")
	}
	if len(store.deadLetters) != 1 {
		t.Fatalf("a delivery for a disabled channel vanished instead of dead-lettering: %v", store.deadLetters)
	}
	if !strings.Contains(store.deadLetters[0].Reason, "disabled") {
		t.Errorf("the dead letter does not name the cause: %q", store.deadLetters[0].Reason)
	}
}

// TestDispatcherReleasesAbandonedDeliveries: a worker killed mid-send leaves a
// row marked 'sending'. Without the reaper that row is a silently dropped
// alert.
func TestDispatcherReleasesAbandonedDeliveries(t *testing.T) {
	store := newFakeStore()
	id := store.add(scriptedDelivery())

	// Simulate a worker that claimed the row and died.
	store.mu.Lock()
	delete(store.due, id)
	store.sending[id] = time.Now().Add(-time.Hour)
	store.mu.Unlock()

	sender := &scriptedSender{}
	dispatcher := NewDispatcher(store, testRegistry(sender), zap.NewNop(), DispatcherOptions{
		LeaseTimeout: 5 * time.Minute,
	})

	if _, err := dispatcher.RunOnce(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if sender.callCount() != 1 {
		t.Fatalf("the abandoned delivery was not picked back up (sender called %d times)", sender.callCount())
	}
	if len(store.sent) != 1 {
		t.Errorf("the abandoned delivery was not delivered: %v", store.sent)
	}
}

// TestDispatcherNeverLogsOrStoresASecret reads the dispatcher's own log
// output. This is the test the brief asks for by name: verified by reading the
// output, not by intending it.
func TestDispatcherNeverLogsOrStoresASecret(t *testing.T) {
	const (
		smtpPassword = "smtp-password-do-not-log"
		botToken     = "8123456789:AAHbottokendonotlog"
		webhookPath  = "SECRETWEBHOOKPATHVALUE"
	)

	store := newFakeStore()
	delivery := scriptedDelivery()
	delivery.MaxAttempts = 2
	delivery.ChannelConfig = Config{
		"host":      "smtp.example.vn",
		"password":  smtpPassword,
		"bot_token": botToken,
		"url":       "https://hooks.example.vn/services/" + webhookPath,
	}
	store.add(delivery)

	// A sender that leaks every secret it has into its error - the worst case,
	// and the reason the dispatcher scrubs from the config rather than
	// trusting the sender.
	leaky := &scriptedSender{failWith: []error{
		fmt.Errorf("auth with %s failed against https://hooks.example.vn/services/%s using token %s",
			smtpPassword, webhookPath, botToken),
	}}

	core, logs := observer.New(zap.DebugLevel)
	clock := &fixedClock{now: time.Now()}
	dispatcher := NewDispatcher(store, testRegistry(leaky), zap.New(core), DispatcherOptions{
		BaseBackoff: time.Millisecond,
		Now:         clock.Now,
	})
	ctx := context.Background()

	// Run to exhaustion so both the retry log and the dead-letter log happen.
	for pass := 0; pass < 4; pass++ {
		_, _ = dispatcher.RunOnce(ctx)
		clock.advance(time.Second)
	}

	if len(store.deadLetters) == 0 {
		t.Fatalf("the delivery never dead-lettered, so the dead-letter log was not exercised")
	}

	secrets := map[string]string{
		"the SMTP password":  smtpPassword,
		"the Telegram token": botToken,
		"the webhook secret": webhookPath,
	}

	// 1. The log output.
	entries := logs.All()
	if len(entries) == 0 {
		t.Fatalf("the dispatcher logged nothing, so this test proves nothing")
	}
	for _, entry := range entries {
		rendered := entry.Message
		for _, field := range entry.Context {
			rendered += " " + fmt.Sprintf("%v=%v", field.Key, field.Interface) +
				" " + field.String + " " + fmt.Sprint(field.Integer)
		}
		for what, secret := range secrets {
			if strings.Contains(rendered, secret) {
				t.Errorf("%s appeared in a log entry: %s", what, rendered)
			}
		}
	}

	// The log still has to be useful, or scrubbing has just made the panel
	// undebuggable.
	var sawChannel, sawDeadLetter bool
	for _, entry := range entries {
		if strings.Contains(entry.Message, "dead-lettered") {
			sawDeadLetter = true
		}
		for _, field := range entry.Context {
			if field.Key == "channel" && field.String == "ops on-call" {
				sawChannel = true
			}
		}
	}
	if !sawChannel {
		t.Errorf("the log does not name the channel, so an operator cannot tell which one broke")
	}
	if !sawDeadLetter {
		t.Errorf("the dead letter was not logged at all")
	}

	// 2. What was written to the outbox: last_error on a retry, and the
	//    dead-letter reason.
	for _, entry := range store.reschedules {
		for what, secret := range secrets {
			if strings.Contains(entry.Reason, secret) {
				t.Errorf("%s was stored in last_error on a retry: %s", what, entry.Reason)
			}
		}
	}
	for _, entry := range store.deadLetters {
		for what, secret := range secrets {
			if strings.Contains(entry.Reason, secret) {
				t.Errorf("%s was stored in the dead-letter reason: %s", what, entry.Reason)
			}
		}
		if !strings.Contains(entry.Reason, Redacted) {
			t.Errorf("the stored reason does not show anything was removed: %s", entry.Reason)
		}
	}
}

// TestDispatcherSurvivesAStoreFailure: a database blip must not kill the loop.
func TestDispatcherSurvivesAStoreFailure(t *testing.T) {
	store := newFakeStore()
	store.claimErr = errors.New("connection reset by peer")

	dispatcher := NewDispatcher(store, testRegistry(&scriptedSender{}), zap.NewNop(), DispatcherOptions{})
	if _, err := dispatcher.RunOnce(context.Background()); err == nil {
		t.Fatalf("a store failure was swallowed")
	}

	// And the loop keeps running rather than returning.
	store.claimErr = nil
	store.add(scriptedDelivery())
	if attempted, err := dispatcher.RunOnce(context.Background()); err != nil || attempted != 1 {
		t.Errorf("the dispatcher did not recover: attempted=%d err=%v", attempted, err)
	}
}

// TestDispatcherRunStopsOnContextCancel keeps the goroutine from outliving the
// process it belongs to.
func TestDispatcherRunStopsOnContextCancel(t *testing.T) {
	store := newFakeStore()
	dispatcher := NewDispatcher(store, testRegistry(&scriptedSender{}), zap.NewNop(), DispatcherOptions{
		Interval: time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		dispatcher.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return after its context was cancelled")
	}
}
