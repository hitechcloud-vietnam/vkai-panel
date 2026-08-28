// Package metrics collects what a managed node actually reports about itself.
//
// # Why this package exists
//
// The agent used to report resource usage from internal/ops/system.go, and that
// report had two faults that a dashboard cannot recover from.
//
// The first is that it did not measure CPU. It divided the one minute load
// average by the core count and called the result a percentage. Load average
// counts runnable and uninterruptible tasks; a box blocked on a slow disk reads
// as busy CPU, and a box running one hot thread on sixteen cores reads as 6%.
// A real percentage is the difference between two readings of /proc/stat over a
// known interval, which is what this package takes.
//
// The second is that every failure was reported as zero. An unreadable
// /proc/meminfo produced "0 bytes of RAM", a df that did not run produced "0
// bytes of disk", and a dashboard drawing those is not showing a gap - it is
// showing a lie that looks like an idle machine. Every metric group here
// carries its own Availability, and a group that could not be collected says so
// and omits its numbers entirely rather than emitting zeros.
//
// # Reading the host
//
// Everything comes from /proc and from statfs. Nothing shells out: the previous
// implementation ran `uname -r` and `df` as subprocesses, which is two process
// spawns per report on every managed server, and it meant a report could fail
// because a PATH was wrong. The one thing /proc cannot answer is filesystem
// capacity, and that is a direct statfs syscall from the standard library.
//
// Both the file reads and the statfs call are injectable, so the tests drive
// the whole collector from synthetic /proc data and never touch the host.
package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Availability is embedded in every metric group. A group that could not be
// collected sets Available false and says why, and its numeric fields - which
// are all pointers or slices - are omitted from the JSON rather than sent as
// zero.
type Availability struct {
	Available bool `json:"available"`
	// Reason is why the group is unavailable. It is empty when it is.
	Reason string `json:"reason,omitempty"`
}

func ok() Availability { return Availability{Available: true} }

func unavailable(format string, args ...any) Availability {
	return Availability{Available: false, Reason: fmt.Sprintf(format, args...)}
}

// f is shorthand for taking the address of a float64 literal. Numeric metric
// fields are pointers so that "not collected" and "collected, and it is zero"
// are different things on the wire: a genuinely idle CPU reports 0, and a CPU
// that could not be read reports nothing at all next to available:false.
func f(v float64) *float64 { return &v }

// i is the same for integers.
func i(v int64) *int64 { return &v }

// Sample is one complete reading of the host.
type Sample struct {
	CollectedAt time.Time `json:"collected_at"`
	Hostname    string    `json:"hostname,omitempty"`
	// Sequence counts samples since the agent started. A gap in it across two
	// samples the panel received means samples were dropped from the buffer.
	Sequence uint64 `json:"sequence"`

	CPU     CPU     `json:"cpu"`
	Memory  Memory  `json:"memory"`
	Load    Load    `json:"load"`
	Uptime  Uptime  `json:"uptime"`
	Disks   Disks   `json:"disks"`
	Network Network `json:"network"`
}

// Unavailable names the metric groups that could not be collected. It is what
// an operator reads first when a dashboard has a gap in it, and what the panel
// should show instead of a zero.
func (s Sample) Unavailable() []string {
	var out []string
	for _, g := range []struct {
		name string
		a    Availability
	}{
		{"cpu", s.CPU.Availability},
		{"memory", s.Memory.Availability},
		{"load", s.Load.Availability},
		{"uptime", s.Uptime.Availability},
		{"disks", s.Disks.Availability},
		{"network", s.Network.Availability},
	} {
		if !g.a.Available {
			out = append(out, g.name)
		}
	}
	return out
}

// Complete reports whether every group was collected.
func (s Sample) Complete() bool { return len(s.Unavailable()) == 0 }

// Collector reads the host. The zero value is not usable; call NewCollector.
//
// A Collector is stateful on purpose. CPU percentage and network throughput are
// both rates, and a rate needs the previous reading. Holding the previous
// reading here means the periodic report computes its rate over the whole
// reporting interval - the most accurate window available - instead of over an
// artificial sub-second sleep taken on every call.
type Collector struct {
	// ProcRoot is where /proc is mounted. Tests point it at a directory of
	// synthetic files.
	ProcRoot string

	// ReadFile reads one file. Defaults to os.ReadFile.
	ReadFile func(name string) ([]byte, error)

	// Statfs reports filesystem capacity for a path. Defaults to the platform
	// implementation, which is unavailable on anything but Linux.
	Statfs func(path string) (FSStat, error)

	// CPUInterval is how long to wait between the two /proc/stat readings when
	// there is no usable previous reading to difference against - the first
	// call after startup, and any call made after a long idle gap. Subsequent
	// calls difference against the previous sample and do not wait at all.
	CPUInterval time.Duration

	// MaxDeltaAge is how stale a previous reading may be and still be used as
	// the base of a rate. Beyond it the reading is discarded and a fresh pair is
	// taken, because a percentage averaged over an hour is not a current
	// percentage.
	MaxDeltaAge time.Duration

	// MaxCores is the point beyond which per-core figures are dropped from the
	// report. On a 256 thread machine they are half the payload and nobody
	// reads them individually; the aggregate is still reported, and the report
	// says why the detail is missing.
	MaxCores int

	// MaxDisks and MaxInterfaces bound the report. The panel's status endpoint
	// reads at most 64KB of body, and a host with hundreds of bind mounts or
	// virtual interfaces would otherwise push a sample past it and have every
	// report rejected.
	MaxDisks      int
	MaxInterfaces int

	Now func() time.Time

	mu       sync.Mutex
	sequence uint64
	prevCPU  *cpuSnapshot
	prevNet  *netSnapshot
}

// Default collector tuning.
const (
	DefaultCPUInterval   = 300 * time.Millisecond
	DefaultMaxDeltaAge   = 10 * time.Minute
	DefaultMaxDisks      = 64
	DefaultMaxInterfaces = 32
	DefaultMaxCores      = 128
)

// NewCollector returns a collector reading the real host.
func NewCollector() *Collector {
	c := &Collector{}
	c.applyDefaults()
	return c
}

func (c *Collector) applyDefaults() {
	if c.ProcRoot == "" {
		c.ProcRoot = "/proc"
	}
	if c.ReadFile == nil {
		c.ReadFile = os.ReadFile
	}
	if c.Statfs == nil {
		c.Statfs = statfs
	}
	if c.CPUInterval <= 0 {
		c.CPUInterval = DefaultCPUInterval
	}
	if c.MaxDeltaAge <= 0 {
		c.MaxDeltaAge = DefaultMaxDeltaAge
	}
	if c.MaxDisks <= 0 {
		c.MaxDisks = DefaultMaxDisks
	}
	if c.MaxInterfaces <= 0 {
		c.MaxInterfaces = DefaultMaxInterfaces
	}
	if c.MaxCores <= 0 {
		c.MaxCores = DefaultMaxCores
	}
	if c.Now == nil {
		c.Now = time.Now
	}
}

// proc joins a path under the configured /proc.
func (c *Collector) proc(parts ...string) string {
	return filepath.Join(append([]string{c.ProcRoot}, parts...)...)
}

func (c *Collector) read(parts ...string) ([]byte, error) {
	return c.ReadFile(c.proc(parts...))
}

// errNoAggregateCPULine is returned when /proc/stat has no "cpu " line, which
// means the file is not /proc/stat.
var errNoAggregateCPULine = fmt.Errorf("no aggregate \"cpu\" line")

// errNoInterfaces is returned when /proc/net/dev names no interface other than
// loopback, which means the file is not /proc/net/dev.
var errNoInterfaces = fmt.Errorf("no network interface other than loopback")
