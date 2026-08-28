package metrics

// Taking one complete sample.

import (
	"context"
	"os"
)

// Sample reads the host once.
//
// It never returns an error. A metric group that could not be collected is
// marked unavailable with the reason attached, and the groups that were
// collected are still reported: a host whose /proc/net/dev is unreadable should
// not lose its disk report as well. Sample.Unavailable lists what is missing,
// and that list is what a dashboard draws a gap from.
//
// The only thing that can block is the CPU group, and only on the first call
// after startup, where there is no previous reading of /proc/stat to difference
// against and a pair has to be taken CPUInterval apart. Cancelling ctx cuts
// that wait short and marks CPU unavailable rather than returning a fabricated
// percentage.
func (c *Collector) Sample(ctx context.Context) Sample {
	c.applyDefaults()

	c.mu.Lock()
	c.sequence++
	sequence := c.sequence
	c.mu.Unlock()

	hostname, _ := os.Hostname()
	s := Sample{
		CollectedAt: c.Now().UTC(),
		Hostname:    hostname,
		Sequence:    sequence,
		CPU:         c.collectCPU(ctx),
		Memory:      c.collectMemory(),
		Uptime:      c.collectUptime(),
		Disks:       c.collectDisks(),
		Network:     c.collectNetwork(),
	}
	s.Load = c.collectLoad(s.CPU.Cores)
	return s
}
