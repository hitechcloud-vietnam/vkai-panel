package reporter

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"
)

// panel stands in for the panel's status endpoint. It can be switched off, and
// it records what it received in the order it received it.
type panel struct {
	reachable bool
	received  []int
}

func (p *panel) send(_ context.Context, payload any) error {
	if !p.reachable {
		return errors.New("dial tcp 10.0.0.1:443: connect: connection refused")
	}
	p.received = append(p.received, payload.(int))
	return nil
}

// newReporter builds a reporter whose samples are simply increasing integers,
// so the order the panel receives them in can be asserted exactly.
func newReporter(t *testing.T, target *panel, bufferSize, maxPerFlush int) (*Reporter, *int) {
	t.Helper()
	next := 0
	seen := &next
	return New(Options{
		Interval:    time.Second,
		BufferSize:  bufferSize,
		MaxPerFlush: maxPerFlush,
		Logger:      log.New(io.Discard, "", 0),
		Float:       func() float64 { return 0.5 },
		Collect: func(context.Context, Stats) any {
			*seen++
			return *seen
		},
		Send: target.send,
	}), seen
}

// The failure this replaces: a sample that could not be delivered was logged
// and thrown away, so a panel outage left a hole in every node's history that
// was indistinguishable from a quiet hour.
func TestSamplesTakenDuringAnOutageAreDeliveredWhenThePanelReturns(t *testing.T) {
	target := &panel{reachable: false}
	rep, _ := newReporter(t, target, 100, 100)
	ctx := context.Background()

	for tick := 0; tick < 5; tick++ {
		rep.Tick(ctx)
	}
	if len(target.received) != 0 {
		t.Fatalf("the panel received %v while it was unreachable", target.received)
	}
	if stats := rep.Stats(); stats.Buffered != 5 || stats.Dropped != 0 {
		t.Fatalf("after five failed ticks the queue holds %d samples with %d dropped, want 5 and 0",
			stats.Buffered, stats.Dropped)
	}

	target.reachable = true
	rep.Tick(ctx)

	// Five buffered samples plus the one taken on this tick, oldest first.
	want := []int{1, 2, 3, 4, 5, 6}
	if len(target.received) != len(want) {
		t.Fatalf("the panel received %v, want %v", target.received, want)
	}
	for idx := range want {
		if target.received[idx] != want[idx] {
			t.Fatalf("the backlog was delivered as %v, want %v in chronological order", target.received, want)
		}
	}
	if stats := rep.Stats(); stats.Buffered != 0 || stats.DeliveryFailures != 0 {
		t.Fatalf("the queue was not drained: %+v", stats)
	}
}

// The bound is the whole point. An unbounded queue turns a panel outage into an
// out-of-memory kill on every managed server, which is a worse failure than the
// gap it was avoiding.
func TestTheBufferIsBoundedAndDropsTheOldestSampleUnderALongOutage(t *testing.T) {
	const bound = 10
	target := &panel{reachable: false}
	rep, _ := newReporter(t, target, bound, 100)
	ctx := context.Background()

	// A thousand intervals with the panel unreachable. At the default cadence
	// this is more than eight hours.
	const ticks = 1000
	for tick := 0; tick < ticks; tick++ {
		rep.Tick(ctx)
	}

	stats := rep.Stats()
	if stats.Buffered != bound {
		t.Fatalf("the queue holds %d samples after %d failed ticks, want it capped at %d",
			stats.Buffered, ticks, bound)
	}
	if stats.Capacity != bound {
		t.Fatalf("the queue reports a capacity of %d, want %d", stats.Capacity, bound)
	}
	if stats.Dropped != ticks-bound {
		t.Fatalf("%d samples were dropped, want %d - every sample that did not fit must be counted",
			stats.Dropped, ticks-bound)
	}
	if stats.DeliveryFailures != int64(ticks) {
		t.Fatalf("%d consecutive delivery failures were counted, want %d", stats.DeliveryFailures, ticks)
	}

	// What survives must be the newest samples: a dashboard that has been dark
	// for eight hours needs the current state first.
	// One more tick, this time with the panel back. It takes sample ticks+1,
	// which itself displaces the oldest survivor, so what arrives is the last
	// `bound` samples of the ticks+1 that were ever taken.
	target.reachable = true
	rep.Tick(ctx)

	taken := ticks + 1
	wantOldest := taken - bound + 1
	if len(target.received) != bound {
		t.Fatalf("the panel received %d samples, want %d - the queue never holds more than its bound",
			len(target.received), bound)
	}
	if target.received[0] != wantOldest {
		t.Fatalf("the oldest surviving sample is %d, want %d - the queue must discard from the front",
			target.received[0], wantOldest)
	}
	if target.received[len(target.received)-1] != taken {
		t.Fatalf("the newest sample delivered is %d, want %d", target.received[len(target.received)-1], taken)
	}
}

// A node whose history has a hole in it must say so, or the panel will draw the
// hole as a period of quiet.
func TestTheSampleIsToldWhatTheBufferHasLost(t *testing.T) {
	target := &panel{reachable: false}
	var lastSeen Stats
	rep := New(Options{
		Interval:   time.Second,
		BufferSize: 3,
		Logger:     log.New(io.Discard, "", 0),
		Collect: func(_ context.Context, stats Stats) any {
			lastSeen = stats
			return 1
		},
		Send: target.send,
	})
	for tick := 0; tick < 10; tick++ {
		rep.Tick(context.Background())
	}
	if lastSeen.Dropped == 0 {
		t.Fatal("the sample was not told that earlier samples had been discarded")
	}
	if lastSeen.Capacity != 3 {
		t.Fatalf("the sample was told the capacity is %d, want 3", lastSeen.Capacity)
	}
	if lastSeen.DeliveryFailures == 0 {
		t.Fatal("the sample was not told that delivery is failing")
	}
}

// A panel coming back after a long outage must not be met by every node in the
// fleet replaying its whole backlog in one burst.
func TestTheBacklogIsDeliveredInBoundedBatches(t *testing.T) {
	const perFlush = 4
	target := &panel{reachable: false}
	rep, _ := newReporter(t, target, 100, perFlush)
	ctx := context.Background()

	for tick := 0; tick < 20; tick++ {
		rep.Tick(ctx)
	}
	target.reachable = true

	rep.Tick(ctx)
	if len(target.received) != perFlush {
		t.Fatalf("the first tick after recovery delivered %d samples, want at most %d",
			len(target.received), perFlush)
	}
	if stats := rep.Stats(); stats.Buffered != 21-perFlush {
		t.Fatalf("%d samples remain queued, want %d", stats.Buffered, 21-perFlush)
	}

	// The rest drains over the following intervals.
	for tick := 0; tick < 10; tick++ {
		rep.Tick(ctx)
	}
	if stats := rep.Stats(); stats.Buffered != 0 {
		t.Fatalf("the backlog did not drain: %d samples still queued", stats.Buffered)
	}
}

// The panel being unreachable is a condition of the connection, not of one
// sample. Trying the whole queue against a dead panel would spend the entire
// interval timing out, once per queued sample.
func TestAFailedDeliveryStopsTheFlushImmediately(t *testing.T) {
	attempts := 0
	rep := New(Options{
		Interval:    time.Second,
		BufferSize:  100,
		MaxPerFlush: 100,
		Logger:      log.New(io.Discard, "", 0),
		Collect:     func(context.Context, Stats) any { return 1 },
		Send: func(context.Context, any) error {
			attempts++
			return errors.New("connection refused")
		},
	})
	for tick := 0; tick < 20; tick++ {
		rep.Tick(context.Background())
	}
	if attempts != 20 {
		t.Fatalf("the reporter made %d delivery attempts over 20 ticks, want one per tick", attempts)
	}
}

// Every node in a fleet is installed by the same script and started within the
// same minute of the others. A bare ticker would keep them all reporting on the
// same second of every interval for the rest of their lives.
func TestTheCadenceIsJitteredAroundTheConfiguredInterval(t *testing.T) {
	values := []float64{0, 0.25, 0.5, 0.75, 0.999}
	index := 0
	rep := New(Options{
		Interval: 30 * time.Second,
		Jitter:   0.2,
		Logger:   log.New(io.Discard, "", 0),
		Collect:  func(context.Context, Stats) any { return 1 },
		Send:     func(context.Context, any) error { return nil },
		Float: func() float64 {
			v := values[index%len(values)]
			index++
			return v
		},
	})

	seen := map[time.Duration]bool{}
	total := time.Duration(0)
	const rounds = 100
	for round := 0; round < rounds; round++ {
		next := rep.nextInterval()
		if next < 24*time.Second || next > 36*time.Second {
			t.Fatalf("an interval of %s is outside 30s +/-20%%", next)
		}
		seen[next] = true
		total += next
	}
	if len(seen) < 3 {
		t.Fatalf("the cadence took only %d distinct values; it is not jittered", len(seen))
	}
	// The mean must stay at the configured interval, or a "last seen" timeout
	// on the panel would have to be widened to accommodate the drift.
	mean := total / rounds
	if mean < 28*time.Second || mean > 32*time.Second {
		t.Fatalf("the mean cadence is %s, want approximately 30s", mean)
	}
}

func TestJitterCanBeTurnedOff(t *testing.T) {
	rep := New(Options{
		Interval: 30 * time.Second,
		Jitter:   0,
		Logger:   log.New(io.Discard, "", 0),
		Collect:  func(context.Context, Stats) any { return 1 },
		Send:     func(context.Context, any) error { return nil },
	})
	if next := rep.nextInterval(); next != 30*time.Second {
		t.Fatalf("with jitter off the interval is %s, want exactly 30s", next)
	}
}

// Run must return promptly when the agent is shutting down, rather than sitting
// in a timer.
func TestRunStopsWhenTheContextIsCancelled(t *testing.T) {
	rep := New(Options{
		Interval: time.Hour,
		Logger:   log.New(io.Discard, "", 0),
		Collect:  func(context.Context, Stats) any { return 1 },
		Send:     func(context.Context, any) error { return nil },
		Float:    func() float64 { return 0 },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { rep.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop when its context was cancelled")
	}
}
