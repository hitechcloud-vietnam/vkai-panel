package metrics

// Network counters, and the rates derived from them.
//
// /proc/net/dev holds monotonic byte and packet totals since boot. Both are
// reported: the totals because they are what a bandwidth quota is billed
// against, and the rates because a total that only ever grows tells an operator
// nothing about what the machine is doing now.

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Network is the host's traffic.
type Network struct {
	Availability
	// BytesIn and BytesOut are cumulative since boot, summed across every
	// interface except loopback.
	BytesIn  *int64 `json:"bytes_in,omitempty"`
	BytesOut *int64 `json:"bytes_out,omitempty"`

	// The rates are present only from the second sample onwards; the first has
	// nothing to difference against, and reporting 0 B/s for it would draw a
	// silent machine on every agent restart.
	BytesInPerSecond  *float64 `json:"bytes_in_per_second,omitempty"`
	BytesOutPerSecond *float64 `json:"bytes_out_per_second,omitempty"`
	IntervalSeconds   *float64 `json:"interval_seconds,omitempty"`

	Interfaces []Interface `json:"interfaces,omitempty"`
	Truncated  bool        `json:"truncated,omitempty"`
}

// Interface is one network device.
type Interface struct {
	Name       string `json:"name"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
	PacketsIn  int64  `json:"packets_in"`
	PacketsOut int64  `json:"packets_out"`
	ErrorsIn   int64  `json:"errors_in"`
	ErrorsOut  int64  `json:"errors_out"`
	DropsIn    int64  `json:"drops_in"`
	DropsOut   int64  `json:"drops_out"`

	BytesInPerSecond  *float64 `json:"bytes_in_per_second,omitempty"`
	BytesOutPerSecond *float64 `json:"bytes_out_per_second,omitempty"`
	// RateNote explains a missing rate on an interface whose counters could not
	// be differenced - a fresh interface, or one whose counters were reset.
	RateNote string `json:"rate_note,omitempty"`
}

type netCounters struct {
	bytesIn, bytesOut     int64
	packetsIn, packetsOut int64
	errorsIn, errorsOut   int64
	dropsIn, dropsOut     int64
}

type netSnapshot struct {
	at         time.Time
	interfaces map[string]netCounters
	totalIn    int64
	totalOut   int64
}

func (c *Collector) readNetwork() (*netSnapshot, error) {
	data, err := c.read("net", "dev")
	if err != nil {
		return nil, err
	}
	snap := &netSnapshot{at: c.Now(), interfaces: map[string]netCounters{}}
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || name == "lo" {
			continue
		}
		fields := strings.Fields(rest)
		// Eight receive columns then eight transmit columns.
		if len(fields) < 16 {
			continue
		}
		numbers := make([]int64, 16)
		bad := false
		for idx := 0; idx < 16; idx++ {
			v, convErr := strconv.ParseInt(fields[idx], 10, 64)
			if convErr != nil {
				bad = true
				break
			}
			numbers[idx] = v
		}
		if bad {
			continue
		}
		counters := netCounters{
			bytesIn: numbers[0], packetsIn: numbers[1], errorsIn: numbers[2], dropsIn: numbers[3],
			bytesOut: numbers[8], packetsOut: numbers[9], errorsOut: numbers[10], dropsOut: numbers[11],
		}
		snap.interfaces[name] = counters
		snap.totalIn += counters.bytesIn
		snap.totalOut += counters.bytesOut
	}
	if len(snap.interfaces) == 0 {
		return nil, errNoInterfaces
	}
	return snap, nil
}

func (c *Collector) collectNetwork() Network {
	current, err := c.readNetwork()
	if err != nil {
		return Network{Availability: unavailable("cannot read %s: %v", c.proc("net", "dev"), err)}
	}

	c.mu.Lock()
	previous := c.prevNet
	c.prevNet = current
	c.mu.Unlock()

	out := Network{
		Availability: ok(),
		BytesIn:      i(current.totalIn),
		BytesOut:     i(current.totalOut),
	}

	interval := 0.0
	usablePrevious := previous != nil &&
		current.at.After(previous.at) &&
		current.at.Sub(previous.at) <= c.MaxDeltaAge
	if usablePrevious {
		interval = current.at.Sub(previous.at).Seconds()
		out.IntervalSeconds = f(interval)
		if current.totalIn >= previous.totalIn && current.totalOut >= previous.totalOut {
			out.BytesInPerSecond = f(rate(current.totalIn-previous.totalIn, interval))
			out.BytesOutPerSecond = f(rate(current.totalOut-previous.totalOut, interval))
		}
	}

	names := make([]string, 0, len(current.interfaces))
	for name := range current.interfaces {
		names = append(names, name)
	}
	// Busiest first, so the cap below drops the veth pairs a container runtime
	// leaves lying around rather than the interface carrying the traffic.
	sort.Slice(names, func(a, b int) bool {
		x, y := current.interfaces[names[a]], current.interfaces[names[b]]
		xt, yt := x.bytesIn+x.bytesOut, y.bytesIn+y.bytesOut
		if xt != yt {
			return xt > yt
		}
		return names[a] < names[b]
	})
	if len(names) > c.MaxInterfaces {
		names = names[:c.MaxInterfaces]
		out.Truncated = true
	}

	for _, name := range names {
		counters := current.interfaces[name]
		iface := Interface{
			Name:      name,
			BytesIn:   counters.bytesIn,
			BytesOut:  counters.bytesOut,
			PacketsIn: counters.packetsIn, PacketsOut: counters.packetsOut,
			ErrorsIn: counters.errorsIn, ErrorsOut: counters.errorsOut,
			DropsIn: counters.dropsIn, DropsOut: counters.dropsOut,
		}
		switch {
		case !usablePrevious:
			iface.RateNote = "no previous sample to measure a rate against"
		default:
			before, existed := previous.interfaces[name]
			switch {
			case !existed:
				iface.RateNote = "this interface appeared since the previous sample"
			case counters.bytesIn < before.bytesIn || counters.bytesOut < before.bytesOut:
				// A counter that went backwards means the interface was
				// recreated or a 32 bit counter wrapped. The difference would be
				// a huge negative or a huge positive spike; neither is traffic
				// that happened.
				iface.RateNote = "the interface counters were reset since the previous sample"
			default:
				iface.BytesInPerSecond = f(rate(counters.bytesIn-before.bytesIn, interval))
				iface.BytesOutPerSecond = f(rate(counters.bytesOut-before.bytesOut, interval))
			}
		}
		out.Interfaces = append(out.Interfaces, iface)
	}
	return out
}

func rate(delta int64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(int64(float64(delta)/seconds*100+0.5)) / 100
}
