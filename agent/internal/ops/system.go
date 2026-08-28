package ops

// The host reports, as the panel asks for them.
//
// The collection itself lives in internal/metrics. What this file adds is the
// wire shape: a flat set of fields the panel has been decoding since before any
// of this existed, alongside the full sample underneath it.
//
// The flat fields are kept because a panel and an agent are upgraded at
// different times, on different machines, by different people, and an agent
// that stops answering the question the running panel knows how to ask is an
// agent that has taken a server off the dashboard. They differ from the old
// ones in one way that matters: every one of them is a pointer, so a metric
// that could not be collected is absent from the JSON rather than present as
// zero. A panel decoding an absent field gets the zero it would have got
// before; a panel that looks at "unavailable" first can tell the difference
// between a machine at rest and a machine it cannot see.

import (
	"context"

	"github.com/hitechcloud-vietnam/vkai-panel/agent/internal/metrics"
)

// SystemInfo is the static picture of a host.
type SystemInfo struct {
	// The flat fields the panel already decodes.
	Hostname  string   `json:"hostname"`
	OS        string   `json:"os"`
	OSPretty  string   `json:"os_pretty,omitempty"`
	Kernel    string   `json:"kernel,omitempty"`
	Arch      string   `json:"arch"`
	CPUCores  int      `json:"cpu_cores"`
	RAMTotal  *int64   `json:"ram_total,omitempty"`
	DiskTotal *int64   `json:"disk_total,omitempty"`
	Uptime    *int64   `json:"uptime,omitempty"`
	Load1     *float64 `json:"load1,omitempty"`
	Load5     *float64 `json:"load5,omitempty"`
	Load15    *float64 `json:"load15,omitempty"`

	// Host carries everything the flat fields cannot: the distribution's own
	// identifiers, the processor model, whether this is a guest.
	Host metrics.Host `json:"host"`

	// Unavailable names the metric groups that could not be read. It is empty
	// on a healthy host, and it is the field to look at before believing any
	// absent number above.
	Unavailable []string `json:"unavailable,omitempty"`
}

// Metrics is the moving picture.
type Metrics struct {
	// The flat fields the panel already decodes. CPUPercent is now a measured
	// percentage: the difference between two readings of /proc/stat, rather
	// than the one minute load average divided by the core count, which is a
	// different quantity that was being reported under this name.
	CPUPercent *float64 `json:"cpu_percent,omitempty"`
	RAMUsed    *int64   `json:"ram_used,omitempty"`
	RAMTotal   *int64   `json:"ram_total,omitempty"`
	DiskUsed   *int64   `json:"disk_used,omitempty"`
	DiskTotal  *int64   `json:"disk_total,omitempty"`
	NetIn      *int64   `json:"net_in,omitempty"`
	NetOut     *int64   `json:"net_out,omitempty"`

	// Sample is the full reading: per-mount disk, per-core CPU, per-interface
	// network, swap, inodes, and the availability of each group.
	Sample metrics.Sample `json:"sample"`

	// Unavailable names the metric groups that could not be read.
	Unavailable []string `json:"unavailable,omitempty"`
}

// CollectSystemInfo gathers the static facts, using the given collector. A nil
// collector reads the real host.
func CollectSystemInfo(ctx context.Context, collector *metrics.Collector) SystemInfo {
	if collector == nil {
		collector = metrics.NewCollector()
	}
	return SystemInfoFrom(collector.CollectHost(), collector.Sample(ctx))
}

// SystemInfoFrom projects a host description and one sample onto the wire
// shape. It is separate from collection so the periodic report can build both
// the static and the moving picture from a single sample. Taking a second
// sample for the static half would be worse than wasteful: two readings of
// /proc/stat taken microseconds apart have no elapsed CPU time between them,
// and the CPU group of the second one would correctly report that it cannot
// measure anything.
func SystemInfoFrom(host metrics.Host, sample metrics.Sample) SystemInfo {
	info := SystemInfo{
		Hostname:    host.Hostname,
		OS:          host.OS,
		OSPretty:    host.OSPretty,
		Kernel:      host.Kernel,
		Arch:        host.Architecture,
		CPUCores:    host.CPUCores,
		Host:        host,
		Unavailable: sample.Unavailable(),
	}
	info.RAMTotal = sample.Memory.TotalBytes
	info.Uptime = sample.Uptime.Seconds
	info.Load1, info.Load5, info.Load15 = sample.Load.One, sample.Load.Five, sample.Load.Fifteen
	if root, found := sample.Disks.Root(); found {
		info.DiskTotal = root.TotalBytes
	}
	return info
}

// CollectMetrics gathers current usage.
func CollectMetrics(ctx context.Context, collector *metrics.Collector) Metrics {
	if collector == nil {
		collector = metrics.NewCollector()
	}
	return FromSample(collector.Sample(ctx))
}

// FromSample projects a sample onto the wire shape. It is separate from
// collection so the periodic report can send one sample without taking a
// second one for the flat view.
func FromSample(sample metrics.Sample) Metrics {
	m := Metrics{Sample: sample, Unavailable: sample.Unavailable()}
	m.CPUPercent = sample.CPU.UsagePercent
	m.RAMUsed = sample.Memory.UsedBytes
	m.RAMTotal = sample.Memory.TotalBytes
	m.NetIn = sample.Network.BytesIn
	m.NetOut = sample.Network.BytesOut
	// The flat disk figures describe the root filesystem, which is what they
	// have always described. Everything mounted elsewhere - and on a host that
	// keeps sites on a data volume that is everything that matters - is in
	// Sample.Disks.Mounts.
	if root, found := sample.Disks.Root(); found {
		m.DiskTotal = root.TotalBytes
		m.DiskUsed = root.UsedBytes
	}
	return m
}
