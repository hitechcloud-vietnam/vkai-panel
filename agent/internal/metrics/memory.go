package metrics

// Memory, load average and uptime, all read from /proc.

import (
	"strconv"
	"strings"
)

// Memory is the host's memory in bytes. Used is derived from MemAvailable
// rather than from MemFree: free memory on a healthy Linux box is close to zero
// because the kernel uses everything spare for page cache, and reporting that
// as "99% used" is how a monitoring dashboard teaches an operator to ignore it.
type Memory struct {
	Availability
	TotalBytes     *int64   `json:"total_bytes,omitempty"`
	AvailableBytes *int64   `json:"available_bytes,omitempty"`
	UsedBytes      *int64   `json:"used_bytes,omitempty"`
	FreeBytes      *int64   `json:"free_bytes,omitempty"`
	BuffersBytes   *int64   `json:"buffers_bytes,omitempty"`
	CachedBytes    *int64   `json:"cached_bytes,omitempty"`
	UsedPercent    *float64 `json:"used_percent,omitempty"`

	SwapTotalBytes *int64   `json:"swap_total_bytes,omitempty"`
	SwapUsedBytes  *int64   `json:"swap_used_bytes,omitempty"`
	SwapPercent    *float64 `json:"swap_used_percent,omitempty"`
}

func (c *Collector) collectMemory() Memory {
	data, err := c.read("meminfo")
	if err != nil {
		return Memory{Availability: unavailable("cannot read %s: %v", c.proc("meminfo"), err)}
	}
	fields := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		key, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			continue
		}
		value, convErr := strconv.ParseInt(parts[0], 10, 64)
		if convErr != nil {
			continue
		}
		// Every quantity in /proc/meminfo carries its unit, and every one that
		// matters here is in kB. A line without a unit is a count, not a size,
		// and is not one of the lines read below.
		if len(parts) > 1 && strings.EqualFold(parts[1], "kB") {
			value *= 1024
		}
		fields[key] = value
	}

	total, hasTotal := fields["MemTotal"]
	if !hasTotal || total <= 0 {
		return Memory{Availability: unavailable("%s has no usable MemTotal line", c.proc("meminfo"))}
	}
	m := Memory{Availability: ok(), TotalBytes: i(total)}

	if free, found := fields["MemFree"]; found {
		m.FreeBytes = i(free)
	}
	if buffers, found := fields["Buffers"]; found {
		m.BuffersBytes = i(buffers)
	}
	if cached, found := fields["Cached"]; found {
		m.CachedBytes = i(cached)
	}

	// MemAvailable is the kernel's own estimate of what a new allocation could
	// get without swapping. Older kernels (pre 3.14) do not have it, and rather
	// than silently substituting MemFree - which would overstate usage on every
	// one of them - the used figure is reported as unavailable and the totals
	// that were read are still reported.
	available, hasAvailable := fields["MemAvailable"]
	switch {
	case hasAvailable && available >= 0 && available <= total:
		m.AvailableBytes = i(available)
		used := total - available
		m.UsedBytes = i(used)
		m.UsedPercent = f(clampPercent(float64(used) * 100 / float64(total)))
	default:
		m.Availability = Availability{
			Available: true,
			Reason:    "MemAvailable is missing from /proc/meminfo, so used memory is not reported; this kernel is older than 3.14",
		}
	}

	if swapTotal, found := fields["SwapTotal"]; found {
		m.SwapTotalBytes = i(swapTotal)
		if swapFree, freeFound := fields["SwapFree"]; freeFound && swapFree <= swapTotal {
			used := swapTotal - swapFree
			m.SwapUsedBytes = i(used)
			if swapTotal > 0 {
				m.SwapPercent = f(clampPercent(float64(used) * 100 / float64(swapTotal)))
			} else {
				m.SwapPercent = f(0)
			}
		}
	}
	return m
}

// Load is the run queue length averaged over one, five and fifteen minutes. It
// is reported alongside CPU utilisation rather than as a substitute for it:
// they answer different questions, and the old code answered the second with
// the first.
type Load struct {
	Availability
	One     *float64 `json:"one,omitempty"`
	Five    *float64 `json:"five,omitempty"`
	Fifteen *float64 `json:"fifteen,omitempty"`
	// PerCore is the one minute figure divided by the core count, which is the
	// number that is comparable across machines of different sizes.
	PerCore         *float64 `json:"one_per_core,omitempty"`
	RunningEntities *int64   `json:"running_entities,omitempty"`
	TotalEntities   *int64   `json:"total_entities,omitempty"`
}

func (c *Collector) collectLoad(cores int) Load {
	data, err := c.read("loadavg")
	if err != nil {
		return Load{Availability: unavailable("cannot read %s: %v", c.proc("loadavg"), err)}
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return Load{Availability: unavailable("%s does not hold three load figures", c.proc("loadavg"))}
	}
	one, err1 := strconv.ParseFloat(fields[0], 64)
	five, err5 := strconv.ParseFloat(fields[1], 64)
	fifteen, err15 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err5 != nil || err15 != nil {
		return Load{Availability: unavailable("%s does not parse as load figures", c.proc("loadavg"))}
	}
	load := Load{Availability: ok(), One: f(one), Five: f(five), Fifteen: f(fifteen)}
	if cores > 0 {
		load.PerCore = f(float64(int64(one/float64(cores)*100+0.5)) / 100)
	}
	// The fourth column is running/total scheduling entities.
	if len(fields) >= 4 {
		running, total, found := strings.Cut(fields[3], "/")
		if found {
			if v, convErr := strconv.ParseInt(running, 10, 64); convErr == nil {
				load.RunningEntities = i(v)
			}
			if v, convErr := strconv.ParseInt(total, 10, 64); convErr == nil {
				load.TotalEntities = i(v)
			}
		}
	}
	return load
}

// Uptime is how long the host has been running.
type Uptime struct {
	Availability
	Seconds     *int64   `json:"seconds,omitempty"`
	IdleSeconds *int64   `json:"idle_seconds,omitempty"`
	Days        *float64 `json:"days,omitempty"`
}

func (c *Collector) collectUptime() Uptime {
	data, err := c.read("uptime")
	if err != nil {
		return Uptime{Availability: unavailable("cannot read %s: %v", c.proc("uptime"), err)}
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return Uptime{Availability: unavailable("%s is empty", c.proc("uptime"))}
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return Uptime{Availability: unavailable("%s does not begin with a number of seconds", c.proc("uptime"))}
	}
	up := Uptime{
		Availability: ok(),
		Seconds:      i(int64(seconds)),
		Days:         f(float64(int64(seconds/86400*100+0.5)) / 100),
	}
	if len(fields) > 1 {
		if idle, convErr := strconv.ParseFloat(fields[1], 64); convErr == nil {
			up.IdleSeconds = i(int64(idle))
		}
	}
	return up
}
