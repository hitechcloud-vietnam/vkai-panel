// Package reporter sends the node's samples to the panel on a cadence, and
// holds on to them when the panel is not there.
//
// # Cadence and jitter
//
// The agent used to report on a bare ticker. Every managed node was installed
// by the same script, started within the same minute of each other, and
// therefore reported on the same second of every interval for the rest of its
// life. Fifty nodes are fifty simultaneous signed requests, each one a
// signature verification and a database write on the panel, followed by
// twenty-nine seconds of nothing. The cost of that is paid entirely by the
// panel, and it grows with the size of the fleet while the useful work does not.
//
// Each interval here is perturbed by a random fraction of itself, so nodes that
// started together drift apart within a few intervals and stay apart. The mean
// cadence is unchanged, which is what matters for a "last seen" timeout.
//
// # The buffer, and its bound
//
// A sample that could not be delivered used to be logged and discarded. A panel
// that was down for an hour left an hour-shaped hole in the history of every
// node it managed, and the hole was indistinguishable from an hour in which
// nothing happened.
//
// Samples are now queued instead. The queue is bounded and drops its oldest
// entry when it is full, because the alternative - a queue that grows until the
// outage ends - turns a panel outage into an out-of-memory kill on every
// managed server, which is a considerably worse failure than a gap in a graph.
// The bound is a sample count, DefaultBufferSize by default: at the default
// thirty second cadence that is six hours of history, and roughly two megabytes
// of retained JSON at the size a sample actually marshals to. What was dropped
// is counted and reported, so a gap in the record is visible as a gap rather
// than passing for quiet.
//
// # Draining
//
// The backlog is delivered oldest first, a bounded number per interval, so a
// panel coming back up after a long outage is not met by every node in the
// fleet replaying six hours at once - which would be the same stampede the
// jitter exists to prevent, concentrated into the minute the panel is at its
// most fragile.
package reporter

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"
)

// Defaults.
const (
	// DefaultInterval is how often a sample is taken and sent.
	DefaultInterval = 30 * time.Second

	// DefaultJitter is the fraction of the interval each tick is perturbed by.
	// At 30 seconds and 0.15 the cadence varies between 25.5 and 34.5 seconds.
	DefaultJitter = 0.15

	// DefaultBufferSize is how many undelivered samples are retained. At the
	// default cadence this is six hours of outage.
	DefaultBufferSize = 720

	// DefaultMaxPerFlush is how many samples are delivered in one interval. A
	// full buffer therefore clears in twelve intervals - six minutes at the
	// default cadence - rather than in one burst.
	DefaultMaxPerFlush = 60
)

// Stats describes the queue. It is read by the code that builds a sample, so a
// report can carry the fact that earlier reports were lost.
type Stats struct {
	// Buffered is how many samples are waiting to be delivered.
	Buffered int `json:"buffered_samples"`
	// Capacity is the bound. It is reported so the panel does not have to guess
	// how much history an agent can hold across an outage.
	Capacity int `json:"buffer_capacity"`
	// Dropped is how many samples have been discarded because the buffer was
	// full, since the agent started. A non-zero value is a gap in this node's
	// history that no retry will fill.
	Dropped int64 `json:"dropped_samples"`
	// DeliveryFailures counts consecutive failed deliveries, and is zero while
	// the panel is reachable.
	DeliveryFailures int64 `json:"delivery_failures,omitempty"`
	// LastDeliveredAt is when a sample last reached the panel.
	LastDeliveredAt time.Time `json:"last_delivered_at,omitempty"`
}

// Options configures a Reporter.
type Options struct {
	Interval    time.Duration
	Jitter      float64
	BufferSize  int
	MaxPerFlush int

	// Collect produces one sample. It is given the queue's state as it stands
	// before this sample joins it, so the sample can report what was lost.
	Collect func(ctx context.Context, stats Stats) any

	// Send delivers one sample. Returning an error keeps the sample queued.
	Send func(ctx context.Context, payload any) error

	Logger *log.Logger

	// Float returns a number in [0,1). It exists so the tests can make the
	// cadence deterministic; production uses math/rand.
	Float func() float64
	Now   func() time.Time
}

// Reporter runs the sampling and delivery loop.
type Reporter struct {
	opts Options

	mu               sync.Mutex
	queue            []any
	dropped          int64
	deliveryFailures int64
	lastDelivered    time.Time
}

// New builds a Reporter. Collect and Send are required; everything else has a
// default.
func New(opts Options) *Reporter {
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Jitter < 0 || opts.Jitter > 0.5 {
		opts.Jitter = DefaultJitter
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = DefaultBufferSize
	}
	if opts.MaxPerFlush <= 0 {
		opts.MaxPerFlush = DefaultMaxPerFlush
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.Float == nil {
		opts.Float = rand.Float64
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Reporter{opts: opts, queue: make([]any, 0, opts.BufferSize)}
}

// Stats reports the queue's state.
func (r *Reporter) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statsLocked()
}

func (r *Reporter) statsLocked() Stats {
	return Stats{
		Buffered:         len(r.queue),
		Capacity:         r.opts.BufferSize,
		Dropped:          r.dropped,
		DeliveryFailures: r.deliveryFailures,
		LastDeliveredAt:  r.lastDelivered,
	}
}

// enqueue adds a sample, discarding the oldest if the buffer is full.
func (r *Reporter) enqueue(sample any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) >= r.opts.BufferSize {
		// Drop from the front. The oldest sample is the least useful one: a
		// dashboard that has been dark for six hours needs the most recent
		// state first, and the history behind it second.
		overflow := len(r.queue) - r.opts.BufferSize + 1
		r.queue = append(r.queue[:0], r.queue[overflow:]...)
		r.dropped += int64(overflow)
	}
	r.queue = append(r.queue, sample)
}

// Tick takes one sample and delivers as much of the queue as the per-interval
// limit allows. It is exported so the tests can drive the loop without waiting
// for real time to pass.
func (r *Reporter) Tick(ctx context.Context) {
	sample := r.opts.Collect(ctx, r.Stats())
	if sample != nil {
		r.enqueue(sample)
	}
	r.flush(ctx)
}

// flush delivers from the front of the queue. It stops at the first failure:
// the panel being unreachable is not a per-sample condition, and continuing
// would spend the whole interval timing out once per queued sample.
func (r *Reporter) flush(ctx context.Context) {
	for sent := 0; sent < r.opts.MaxPerFlush; sent++ {
		if ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		if len(r.queue) == 0 {
			r.mu.Unlock()
			return
		}
		next := r.queue[0]
		r.mu.Unlock()

		if err := r.opts.Send(ctx, next); err != nil {
			r.recordFailure(err)
			return
		}
		r.mu.Lock()
		// The queue is only ever appended to elsewhere, so the sample just
		// delivered is still at the front.
		if len(r.queue) > 0 {
			r.queue = r.queue[1:]
		}
		recovered := r.deliveryFailures > 0
		backlog := len(r.queue)
		r.deliveryFailures = 0
		r.lastDelivered = r.opts.Now().UTC()
		r.mu.Unlock()

		if recovered {
			r.opts.Logger.Printf("the panel is reachable again; %d buffered sample(s) still to deliver", backlog)
		}
	}
}

// recordFailure counts a failed delivery and logs it without filling the
// journal. During a multi-hour outage the same error would otherwise be written
// once every interval, thousands of times, burying whatever else the agent has
// to say.
func (r *Reporter) recordFailure(err error) {
	r.mu.Lock()
	r.deliveryFailures++
	failures := r.deliveryFailures
	stats := r.statsLocked()
	r.mu.Unlock()

	if failures == 1 || failures%10 == 0 {
		r.opts.Logger.Printf(
			"cannot deliver a status sample to the panel (%d consecutive failure(s), %d/%d samples buffered, %d dropped): %v",
			failures, stats.Buffered, stats.Capacity, stats.Dropped, err)
	}
	if stats.Dropped > 0 && failures%10 == 0 {
		r.opts.Logger.Printf(
			"WARNING: the sample buffer is full and %d sample(s) have been discarded. "+
				"This node's history in the panel will have a gap for the length of this outage.", stats.Dropped)
	}
}

// Run samples and delivers until ctx is cancelled.
//
// The first sample is taken after a short random delay rather than immediately.
// The delay is a fraction of one interval, so an operator watching an install
// still sees the node appear within seconds, while a fleet that was restarted
// together by a package upgrade does not arrive at the panel in one burst.
func (r *Reporter) Run(ctx context.Context) {
	startup := time.Duration(r.opts.Float() * r.opts.Jitter * float64(r.opts.Interval))
	r.opts.Logger.Printf(
		"reporting every %s (jitter +/-%.0f%%, first report in %s); "+
			"up to %d undelivered samples are buffered, oldest discarded first",
		r.opts.Interval, r.opts.Jitter*100, startup.Round(time.Millisecond), r.opts.BufferSize)

	timer := time.NewTimer(startup)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.Tick(ctx)
			timer.Reset(r.nextInterval())
		}
	}
}

// nextInterval is the interval perturbed by up to Jitter of itself, in either
// direction, so the mean cadence is the configured one.
func (r *Reporter) nextInterval() time.Duration {
	if r.opts.Jitter <= 0 {
		return r.opts.Interval
	}
	offset := (r.opts.Float()*2 - 1) * r.opts.Jitter
	next := time.Duration(float64(r.opts.Interval) * (1 + offset))
	if next < time.Second {
		next = time.Second
	}
	return next
}
