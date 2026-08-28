package notify

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Delivery is one outbox row joined to the channel it is bound for. The
// channel's config travels with it so the dispatcher never has to read the
// channels table per attempt.
type Delivery struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	ChannelID     uuid.UUID
	ChannelName   string
	ChannelType   string
	ChannelConfig Config
	ChannelActive bool

	DedupKey string
	Kind     EventKind
	Subject  string
	Body     string
	Alert    Alert

	Attempts    int
	MaxAttempts int
}

// Store is the persistence the dispatcher needs. It is an interface so the
// retry, backoff and dead-letter behaviour can be driven in a test without a
// database, and so the same behaviour can be driven against a real one.
type Store interface {
	// ReleaseStale returns rows stuck in 'sending' to 'pending'. A worker that
	// was killed mid-send leaves them behind, and without this they are the
	// silent drop this package exists to prevent.
	ReleaseStale(ctx context.Context, stuckSince time.Time) (int, error)

	// ClaimDue atomically marks up to limit due rows as 'sending' and returns
	// them. Concurrent dispatchers must not receive the same row.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]Delivery, error)

	// MarkSent records a successful delivery.
	MarkSent(ctx context.Context, id uuid.UUID, at time.Time) error

	// Reschedule returns a row to 'pending' with a later due time.
	Reschedule(ctx context.Context, id uuid.UUID, attempts int, nextAttemptAt time.Time, lastError string) error

	// DeadLetter gives up on a row, permanently and visibly.
	DeadLetter(ctx context.Context, id uuid.UUID, at time.Time, lastError string) error
}

// DeadLetterHook is called when a delivery is abandoned. The panel uses it to
// write an in-panel notification, so that a channel which has stopped working
// is discovered by looking at the panel rather than by grepping logs after the
// next outage.
type DeadLetterHook func(ctx context.Context, d Delivery, cause string)

// Dispatcher options. Every duration is a field rather than a constant so the
// tests can run a full retry-to-dead-letter cycle in milliseconds instead of
// half an hour.
type DispatcherOptions struct {
	// Interval is how often the outbox is polled. Alerts are low volume; a
	// couple of seconds of latency costs nothing and a tight loop costs a
	// connection.
	Interval time.Duration
	// BatchSize is how many rows one pass claims.
	BatchSize int
	// Concurrency is how many sends run at once within a pass.
	Concurrency int
	// BaseBackoff is the delay after the first failed attempt. It doubles from
	// there.
	BaseBackoff time.Duration
	// MaxBackoff caps the delay. Without a cap, attempt 12 is a day away and
	// the alert is worthless by the time it lands.
	MaxBackoff time.Duration
	// SendTimeout bounds one attempt.
	SendTimeout time.Duration
	// LeaseTimeout is how long a row may sit in 'sending' before it is assumed
	// abandoned. It must be comfortably larger than SendTimeout.
	LeaseTimeout time.Duration
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
	// OnDeadLetter is called for every abandoned delivery.
	OnDeadLetter DeadLetterHook
}

// Dispatcher defaults, chosen for a panel that sends tens of alerts a day.
const (
	DefaultInterval     = 5 * time.Second
	DefaultBatchSize    = 20
	DefaultConcurrency  = 4
	DefaultBaseBackoff  = 30 * time.Second
	DefaultMaxBackoff   = 15 * time.Minute
	DefaultSendTimeout  = 30 * time.Second
	DefaultLeaseTimeout = 5 * time.Minute
	// DefaultMaxAttempts is the attempt budget for one delivery. With the
	// backoff above, five attempts span roughly half an hour: long enough to
	// ride out a mail server restart, short enough that a genuinely broken
	// channel is dead-lettered while somebody is still awake.
	DefaultMaxAttempts = 5
)

// withDefaults fills in the zero values.
func (o DispatcherOptions) withDefaults() DispatcherOptions {
	if o.Interval <= 0 {
		o.Interval = DefaultInterval
	}
	if o.BatchSize <= 0 {
		o.BatchSize = DefaultBatchSize
	}
	if o.Concurrency <= 0 {
		o.Concurrency = DefaultConcurrency
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = DefaultBaseBackoff
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = DefaultMaxBackoff
	}
	if o.SendTimeout <= 0 {
		o.SendTimeout = DefaultSendTimeout
	}
	if o.LeaseTimeout <= 0 {
		o.LeaseTimeout = DefaultLeaseTimeout
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// Dispatcher drains the outbox. It is the only thing in the panel that
// actually sends, and it runs off the request path.
type Dispatcher struct {
	store    Store
	registry *Registry
	logger   *zap.Logger
	opts     DispatcherOptions
}

// NewDispatcher builds a dispatcher.
func NewDispatcher(store Store, registry *Registry, logger *zap.Logger, opts DispatcherOptions) *Dispatcher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Dispatcher{
		store:    store,
		registry: registry,
		logger:   logger,
		opts:     opts.withDefaults(),
	}
}

// Backoff returns how long to wait before attempt+1, given that `attempt`
// attempts have now failed. It doubles from base and is capped.
//
// There is no jitter. The panel sends to a handful of channels, not to a
// thundering herd, and a deterministic delay is one an operator can predict
// while watching an incident: "the retry is thirty seconds from now" is a
// useful sentence.
func Backoff(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if base <= 0 {
		base = DefaultBaseBackoff
	}
	if max <= 0 {
		max = DefaultMaxBackoff
	}
	// Past 30 doublings the shift overflows; the cap has long since applied.
	if attempt > 30 {
		return max
	}
	delay := base << uint(attempt-1)
	if delay > max || delay <= 0 {
		return max
	}
	return delay
}

// Run polls the outbox until the context is cancelled. It is the goroutine
// started from main.
func (d *Dispatcher) Run(ctx context.Context) {
	d.logger.Info("Notification dispatcher started",
		zap.Duration("interval", d.opts.Interval),
		zap.Int("batch_size", d.opts.BatchSize),
		zap.Int("concurrency", d.opts.Concurrency),
		zap.Strings("channel_types", d.registry.Types()),
	)

	ticker := time.NewTicker(d.opts.Interval)
	defer ticker.Stop()

	for {
		if _, err := d.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			d.logger.Error("Notification dispatcher pass failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			d.logger.Info("Notification dispatcher stopped")
			return
		case <-ticker.C:
		}
	}
}

// RunOnce performs a single pass and returns how many deliveries it attempted.
// It is exported because it is what the tests drive, and because it makes the
// loop above trivial enough to read.
func (d *Dispatcher) RunOnce(ctx context.Context) (int, error) {
	now := d.opts.Now()

	if released, err := d.store.ReleaseStale(ctx, now.Add(-d.opts.LeaseTimeout)); err != nil {
		d.logger.Warn("Could not release abandoned notification deliveries", zap.Error(err))
	} else if released > 0 {
		d.logger.Warn("Released notification deliveries abandoned by a previous worker",
			zap.Int("count", released))
	}

	deliveries, err := d.store.ClaimDue(ctx, now, d.opts.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("claim due notification deliveries: %w", err)
	}
	if len(deliveries) == 0 {
		return 0, nil
	}

	slots := make(chan struct{}, d.opts.Concurrency)
	var wg sync.WaitGroup
	for i := range deliveries {
		delivery := deliveries[i]
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			d.attempt(ctx, delivery)
		}()
	}
	wg.Wait()

	return len(deliveries), nil
}

// attempt makes one delivery attempt and records the outcome.
func (d *Dispatcher) attempt(ctx context.Context, delivery Delivery) {
	// The scrubber is built from the channel config rather than from the
	// sender, so it catches a secret no matter which sender produced the
	// error, including a sender added later that forgets to scrub its own.
	scrub := ScrubberForConfig(delivery.ChannelConfig)

	// Fields safe to log. The config is never among them.
	fields := []zap.Field{
		zap.String("delivery_id", delivery.ID.String()),
		zap.String("channel", delivery.ChannelName),
		zap.String("channel_type", delivery.ChannelType),
		zap.String("event", string(delivery.Kind)),
		zap.String("dedup_key", delivery.DedupKey),
		zap.Int("attempt", delivery.Attempts+1),
		zap.Int("max_attempts", delivery.MaxAttempts),
	}

	if !delivery.ChannelActive {
		// The channel was disabled after this alert was queued. Dropping the
		// row would be silent; dead-lettering it is not.
		d.abandon(ctx, delivery, scrub, fields,
			"the notification channel was disabled before this message could be sent")
		return
	}

	sender, err := d.registry.Build(delivery.ChannelType, delivery.ChannelConfig)
	if err != nil {
		d.abandon(ctx, delivery, scrub, fields, scrub.Scrub(err.Error()))
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, d.opts.SendTimeout)
	defer cancel()

	err = sender.Send(sendCtx, Message{
		Kind:     delivery.Kind,
		Severity: delivery.Alert.Severity,
		Subject:  delivery.Subject,
		Body:     delivery.Body,
		Link:     delivery.Alert.Link,
		Alert:    delivery.Alert,
	})
	if err == nil {
		if markErr := d.store.MarkSent(ctx, delivery.ID, d.opts.Now()); markErr != nil {
			// The message went out; only the bookkeeping failed. The lease
			// reaper will hand the row back and it will be sent twice. A
			// duplicate alert is a nuisance; a lost one is an outage.
			d.logger.Error("Notification sent but could not be marked sent; it may be delivered again",
				append(fields, zap.Error(markErr))...)
			return
		}
		d.logger.Info("Notification delivered", fields...)
		return
	}

	reason := scrub.Scrub(err.Error())
	attempts := delivery.Attempts + 1

	if IsPermanent(err) {
		d.abandon(ctx, delivery, scrub, fields, "permanent failure: "+reason)
		return
	}
	if attempts >= delivery.MaxAttempts {
		d.abandon(ctx, delivery, scrub, fields,
			fmt.Sprintf("gave up after %d attempts: %s", attempts, reason))
		return
	}

	delay := Backoff(attempts, d.opts.BaseBackoff, d.opts.MaxBackoff)
	nextAt := d.opts.Now().Add(delay)
	if rescheduleErr := d.store.Reschedule(ctx, delivery.ID, attempts, nextAt, reason); rescheduleErr != nil {
		d.logger.Error("Could not reschedule a failed notification",
			append(fields, zap.Error(rescheduleErr))...)
		return
	}
	d.logger.Warn("Notification delivery failed, will retry",
		append(fields,
			zap.String("reason", reason),
			zap.Duration("retry_in", delay),
			zap.Time("next_attempt_at", nextAt),
		)...)
}

// abandon dead-letters a delivery: loudly, permanently, and where an operator
// will find it.
func (d *Dispatcher) abandon(ctx context.Context, delivery Delivery, scrub *Scrubber, fields []zap.Field, cause string) {
	cause = scrub.Scrub(cause)
	if err := d.store.DeadLetter(ctx, delivery.ID, d.opts.Now(), cause); err != nil {
		d.logger.Error("Could not dead-letter a notification",
			append(fields, zap.Error(err), zap.String("reason", cause))...)
		return
	}

	d.logger.Error("Notification dead-lettered: this alert did NOT reach anyone",
		append(fields,
			zap.String("reason", cause),
			zap.String("subject", delivery.Subject),
		)...)

	if d.opts.OnDeadLetter != nil {
		d.opts.OnDeadLetter(ctx, delivery, cause)
	}
}
