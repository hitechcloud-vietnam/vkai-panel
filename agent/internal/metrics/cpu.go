package metrics

// CPU utilisation, measured the only way it can be measured: as the difference
// between two readings of /proc/stat.
//
// /proc/stat's counters are monotonic jiffy totals since boot. A single reading
// says how the machine has been used since it was switched on, which for a host
// that has been up for a month is a number that barely moves. The percentage an
// operator wants is (busy jiffies elapsed) / (all jiffies elapsed) between two
// readings a known distance apart.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CPU is processor utilisation over the interval between the two readings that
// produced it. Every percentage is a pointer: a group that could not be read
// omits them rather than reporting a machine that is 0% busy.
type CPU struct {
	Availability
	// Cores is the number of processors /proc/stat reported lines for.
	Cores int `json:"cores,omitempty"`
	// IntervalSeconds is the distance between the two readings. A percentage
	// without it cannot be judged: 3% over 200ms and 3% over 10 minutes are
	// very different claims.
	IntervalSeconds *float64 `json:"interval_seconds,omitempty"`

	UsagePercent  *float64 `json:"usage_percent,omitempty"`
	UserPercent   *float64 `json:"user_percent,omitempty"`
	SystemPercent *float64 `json:"system_percent,omitempty"`
	IOWaitPercent *float64 `json:"iowait_percent,omitempty"`
	StealPercent  *float64 `json:"steal_percent,omitempty"`
	IdlePercent   *float64 `json:"idle_percent,omitempty"`

	// PerCore is the same usage figure for each processor. It is what
	// distinguishes a saturated single-threaded process from an idle machine,
	// which the aggregate percentage on a large host cannot show.
	PerCore []CorePercent `json:"per_core,omitempty"`
	// PerCoreNote explains an absent per-core breakdown, so a reader does not
	// take the absence for a machine with no cores.
	PerCoreNote string `json:"per_core_note,omitempty"`
}

// CorePercent is one processor's share of the interval.
type CorePercent struct {
	Core         int     `json:"core"`
	UsagePercent float64 `json:"usage_percent"`
}

// cpuTimes is one line of /proc/stat, in jiffies.
type cpuTimes struct {
	user, nice, system, idle, iowait, irq, softirq, steal uint64
}

func (t cpuTimes) total() uint64 {
	return t.user + t.nice + t.system + t.idle + t.iowait + t.irq + t.softirq + t.steal
}

// busy is everything that is not idle. iowait is excluded from busy because a
// core waiting on a disk is not executing anything; it is reported separately
// so an operator can see that the machine is blocked rather than loaded.
func (t cpuTimes) busy() uint64 {
	return t.user + t.nice + t.system + t.irq + t.softirq + t.steal
}

// cpuSnapshot is one complete reading of /proc/stat.
type cpuSnapshot struct {
	at    time.Time
	total cpuTimes
	cores []cpuTimes
}

// readCPU parses /proc/stat.
func (c *Collector) readCPU() (*cpuSnapshot, error) {
	data, err := c.read("stat")
	if err != nil {
		return nil, err
	}
	snap := &cpuSnapshot{at: c.Now()}
	seenAggregate := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		times, parseErr := parseCPUTimes(fields[1:])
		if parseErr != nil {
			continue
		}
		if fields[0] == "cpu" {
			snap.total = times
			seenAggregate = true
			continue
		}
		// cpu0, cpu1, ... The index is not trusted to be contiguous; the
		// position in the slice is the reported core number instead, and the
		// two readings are lined up by position.
		snap.cores = append(snap.cores, times)
	}
	if !seenAggregate {
		return nil, errNoAggregateCPULine
	}
	return snap, nil
}

// parseCPUTimes reads the jiffy columns. A kernel that reports fewer columns
// than the newest one - guest and guest_nice are relatively recent - is fine:
// missing trailing columns are left at zero, which is what they mean.
func parseCPUTimes(fields []string) (cpuTimes, error) {
	values := make([]uint64, 8)
	for idx := 0; idx < len(values) && idx < len(fields); idx++ {
		v, err := strconv.ParseUint(fields[idx], 10, 64)
		if err != nil {
			return cpuTimes{}, err
		}
		values[idx] = v
	}
	return cpuTimes{
		user: values[0], nice: values[1], system: values[2], idle: values[3],
		iowait: values[4], irq: values[5], softirq: values[6], steal: values[7],
	}, nil
}

// collectCPU produces the CPU group, taking a second reading if the previous
// one is missing or too old to difference against.
//
// The wait is deliberately the only blocking thing in a collection, it happens
// only on the first sample after startup, and it honours the context so a
// shutdown does not sit in it.
func (c *Collector) collectCPU(ctx context.Context) CPU {
	current, err := c.readCPU()
	if err != nil {
		return CPU{Availability: unavailable("cannot read %s: %v", c.proc("stat"), err)}
	}

	c.mu.Lock()
	previous := c.prevCPU
	c.mu.Unlock()

	if previous == nil || current.at.Sub(previous.at) > c.MaxDeltaAge {
		// No usable base. Take one, wait, and read again, so that even the very
		// first report carries a real measurement rather than a placeholder.
		select {
		case <-ctx.Done():
			c.storeCPU(current)
			return CPU{Availability: unavailable("collection was cancelled before a second /proc/stat reading could be taken")}
		case <-time.After(c.CPUInterval):
		}
		previous = current
		current, err = c.readCPU()
		if err != nil {
			return CPU{Availability: unavailable("cannot read %s: %v", c.proc("stat"), err)}
		}
	}
	c.storeCPU(current)
	out := cpuBetween(previous, current)
	if out.Cores > c.MaxCores {
		out.PerCore = nil
		out.PerCoreNote = fmt.Sprintf(
			"this host has %d processors, more than the %d this agent reports individually; only the aggregate is sent",
			out.Cores, c.MaxCores)
	}
	return out
}

func (c *Collector) storeCPU(snap *cpuSnapshot) {
	c.mu.Lock()
	c.prevCPU = snap
	c.mu.Unlock()
}

// cpuBetween turns two readings into percentages.
func cpuBetween(previous, current *cpuSnapshot) CPU {
	interval := current.at.Sub(previous.at).Seconds()
	deltaTotal := current.total.total() - previous.total.total()

	// The counters are monotonic, so a total that did not advance means the two
	// readings landed inside the same jiffy, and a total that went backwards
	// means the machine was reset or the file was not what we think it is.
	// Neither can produce a percentage, and inventing one - typically zero - is
	// exactly the failure this package exists to stop.
	if current.total.total() < previous.total.total() {
		return CPU{Availability: unavailable(
			"the CPU counters in /proc/stat went backwards between two readings; no percentage can be derived from them")}
	}
	if deltaTotal == 0 {
		return CPU{Availability: unavailable(
			"two readings of /proc/stat %.3fs apart show no elapsed CPU time to measure against", interval)}
	}

	scale := 100 / float64(deltaTotal)
	out := CPU{
		Availability:    ok(),
		Cores:           len(current.cores),
		IntervalSeconds: f(interval),
		UsagePercent:    f(clampPercent(float64(current.total.busy()-previous.total.busy()) * scale)),
		UserPercent:     f(clampPercent(float64((current.total.user+current.total.nice)-(previous.total.user+previous.total.nice)) * scale)),
		SystemPercent:   f(clampPercent(float64(current.total.system-previous.total.system) * scale)),
		IOWaitPercent:   f(clampPercent(float64(current.total.iowait-previous.total.iowait) * scale)),
		StealPercent:    f(clampPercent(float64(current.total.steal-previous.total.steal) * scale)),
		IdlePercent:     f(clampPercent(float64(current.total.idle-previous.total.idle) * scale)),
	}

	// Per-core figures are reported only when the two readings agree on how
	// many cores there are. They disagree when a CPU was hotplugged between
	// them, and lining up mismatched slices by position would attribute one
	// core's work to another.
	if len(previous.cores) == len(current.cores) {
		for idx := range current.cores {
			coreDelta := current.cores[idx].total() - previous.cores[idx].total()
			if coreDelta == 0 || current.cores[idx].total() < previous.cores[idx].total() {
				continue
			}
			out.PerCore = append(out.PerCore, CorePercent{
				Core:         idx,
				UsagePercent: clampPercent(float64(current.cores[idx].busy()-previous.cores[idx].busy()) * 100 / float64(coreDelta)),
			})
		}
	}
	return out
}

// clampPercent keeps rounding and counter quirks inside 0..100 and rounds to
// two decimals, which is more precision than any dashboard shows.
func clampPercent(v float64) float64 {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return float64(int64(v*100+0.5)) / 100
}
