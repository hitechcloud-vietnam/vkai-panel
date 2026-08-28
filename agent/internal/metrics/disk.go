package metrics

// Filesystem capacity, per mount.
//
// The previous implementation reported one number for the whole machine: the
// size and used bytes of "/", obtained by running df in a subprocess. On the
// shape of host this panel actually manages that is close to useless. Sites
// live on a data volume, MySQL lives on another, /boot fills up on its own
// schedule and takes the next kernel upgrade down with it, and a report that
// only ever describes / cannot show any of it.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// FSStat is what a statfs call returns, in the units the caller wants.
type FSStat struct {
	TotalBytes int64
	FreeBytes  int64
	// AvailableBytes excludes the blocks reserved for root, which is the figure
	// df prints and the one that predicts when a customer's upload fails.
	AvailableBytes int64
	InodesTotal    int64
	InodesFree     int64
}

// Disks is every mounted filesystem worth reporting.
type Disks struct {
	Availability
	Mounts []Mount `json:"mounts,omitempty"`
	// Truncated is set when the host has more mounts than MaxDisks. The panel's
	// status endpoint caps the body it will read, so an unbounded list is a way
	// to have every report rejected.
	Truncated bool `json:"truncated,omitempty"`
}

// Mount is one filesystem.
type Mount struct {
	Availability
	Device     string `json:"device"`
	Mountpoint string `json:"mountpoint"`
	FSType     string `json:"fstype"`
	ReadOnly   bool   `json:"read_only,omitempty"`
	IsRoot     bool   `json:"is_root,omitempty"`

	TotalBytes     *int64   `json:"total_bytes,omitempty"`
	UsedBytes      *int64   `json:"used_bytes,omitempty"`
	FreeBytes      *int64   `json:"free_bytes,omitempty"`
	AvailableBytes *int64   `json:"available_bytes,omitempty"`
	UsedPercent    *float64 `json:"used_percent,omitempty"`

	InodesTotal   *int64   `json:"inodes_total,omitempty"`
	InodesUsed    *int64   `json:"inodes_used,omitempty"`
	InodesPercent *float64 `json:"inodes_used_percent,omitempty"`
}

// realFilesystems is the set of filesystem types that hold a customer's data
// and can therefore run out. Everything else on a running host - proc, sysfs,
// cgroup, devtmpfs, the dozens of squashfs mounts a snap installation leaves
// behind - is either backed by memory or has no capacity to speak of, and
// listing them buries the two or three mounts an operator cares about.
var realFilesystems = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true,
	"xfs": true, "btrfs": true, "zfs": true, "f2fs": true,
	"jfs": true, "reiserfs": true, "ufs": true, "hfsplus": true,
	"vfat": true, "exfat": true, "ntfs": true, "ntfs3": true,
	"nfs": true, "nfs4": true, "cifs": true, "glusterfs": true, "ceph": true,
	"lustre": true, "9p": true, "virtiofs": true, "fuse.sshfs": true,
	"overlay": true,
}

func (c *Collector) collectDisks() Disks {
	// /proc/self/mounts is what this process can actually see, which inside a
	// container is the container's view. /etc/mtab is a symlink to it on any
	// current distribution, and reading the file directly avoids depending on
	// that.
	data, err := c.read("self", "mounts")
	if err != nil {
		return Disks{Availability: unavailable("cannot read %s: %v", c.proc("self", "mounts"), err)}
	}

	seenDevice := map[string]bool{}
	seenMountpoint := map[string]bool{}
	var mounts []Mount
	truncated := false

	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		device := unescapeMountField(fields[0])
		mountpoint := unescapeMountField(fields[1])
		fstype := fields[2]
		if !realFilesystems[fstype] {
			continue
		}
		if seenMountpoint[mountpoint] {
			continue
		}
		// A bind mount and its origin are the same filesystem with the same
		// capacity, and reporting both makes a host look like it has twice the
		// disk it has. Only real block devices are deduplicated this way:
		// "overlay" and "none" are shared names, not shared filesystems.
		if strings.HasPrefix(device, "/") {
			if seenDevice[device] {
				continue
			}
			seenDevice[device] = true
		}
		seenMountpoint[mountpoint] = true

		if len(mounts) >= c.MaxDisks {
			truncated = true
			break
		}
		mounts = append(mounts, c.describeMount(device, mountpoint, fstype, fields[3]))
	}

	sort.Slice(mounts, func(a, b int) bool { return mounts[a].Mountpoint < mounts[b].Mountpoint })

	if len(mounts) == 0 {
		return Disks{Availability: unavailable(
			"%s lists no filesystem this agent recognises as holding data", c.proc("self", "mounts"))}
	}
	return Disks{Availability: ok(), Mounts: mounts, Truncated: truncated}
}

// describeMount fills in one mount's capacity. A statfs that fails - a stale
// NFS handle, a filesystem that was unmounted between reading the list and
// asking about it - leaves that mount marked unavailable while every other
// mount in the report is still good.
func (c *Collector) describeMount(device, mountpoint, fstype, options string) Mount {
	m := Mount{
		Device:     device,
		Mountpoint: mountpoint,
		FSType:     fstype,
		IsRoot:     mountpoint == "/",
		ReadOnly:   hasMountOption(options, "ro"),
	}
	stat, err := c.Statfs(mountpoint)
	if err != nil {
		m.Availability = unavailable("statfs(%s) failed: %v", mountpoint, err)
		return m
	}
	if stat.TotalBytes <= 0 {
		m.Availability = unavailable("statfs(%s) reports a filesystem with no capacity", mountpoint)
		return m
	}
	m.Availability = ok()
	used := stat.TotalBytes - stat.FreeBytes
	m.TotalBytes = i(stat.TotalBytes)
	m.UsedBytes = i(used)
	m.FreeBytes = i(stat.FreeBytes)
	m.AvailableBytes = i(stat.AvailableBytes)
	// df's definition: the denominator is what an ordinary user could ever
	// occupy, not the raw size, so a filesystem with 5% reserved for root reads
	// as 100% full at the point where writes actually start failing.
	denominator := used + stat.AvailableBytes
	if denominator > 0 {
		m.UsedPercent = f(clampPercent(float64(used) * 100 / float64(denominator)))
	}
	if stat.InodesTotal > 0 {
		inodesUsed := stat.InodesTotal - stat.InodesFree
		m.InodesTotal = i(stat.InodesTotal)
		m.InodesUsed = i(inodesUsed)
		m.InodesPercent = f(clampPercent(float64(inodesUsed) * 100 / float64(stat.InodesTotal)))
	}
	return m
}

// Root returns the mount for "/", if it was collected.
func (d Disks) Root() (Mount, bool) {
	for _, m := range d.Mounts {
		if m.IsRoot {
			return m, true
		}
	}
	return Mount{}, false
}

func hasMountOption(options, want string) bool {
	for _, opt := range strings.Split(options, ",") {
		if opt == want {
			return true
		}
	}
	return false
}

// unescapeMountField undoes the octal escaping the kernel applies to device and
// mount point names in /proc/self/mounts. A directory with a space in it
// appears there as \040, and a mount point read without undoing that does not
// match the path anything else uses.
func unescapeMountField(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	var b strings.Builder
	for idx := 0; idx < len(field); idx++ {
		if field[idx] == '\\' && idx+3 < len(field) {
			if v, err := strconv.ParseUint(field[idx+1:idx+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				idx += 3
				continue
			}
		}
		b.WriteByte(field[idx])
	}
	return b.String()
}

// StatPath reports the capacity of the filesystem holding a path. It is the
// same statfs the periodic sample uses, exposed for the disk.usage operation.
func StatPath(path string) (FSStat, error) { return statfs(path) }

// MountFor finds the filesystem a path is on, by taking the longest mount point
// that is a prefix of it. This is how df resolves a path, and it is the only
// way to attribute a directory to a filesystem without a syscall that the
// standard library does not expose.
//
// It returns a Mount whose capacity has been filled in, or an error if the
// mount table cannot be read. A path on a filesystem type this agent does not
// treat as real - a tmpfs, an overlay layer - is still resolved: the caller
// asked about a specific path, not for a survey.
func (c *Collector) MountFor(path string) (Mount, error) {
	c.applyDefaults()
	data, err := c.read("self", "mounts")
	if err != nil {
		return Mount{}, fmt.Errorf("cannot read %s: %w", c.proc("self", "mounts"), err)
	}
	best := Mount{}
	bestLength := -1
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mountpoint := unescapeMountField(fields[1])
		if !pathWithin(path, mountpoint) {
			continue
		}
		if len(mountpoint) > bestLength {
			bestLength = len(mountpoint)
			best = c.describeMount(unescapeMountField(fields[0]), mountpoint, fields[2], fields[3])
		}
	}
	if bestLength < 0 {
		return Mount{}, fmt.Errorf("no mount point in %s contains %s", c.proc("self", "mounts"), path)
	}
	return best, nil
}

// pathWithin reports whether path is at or below root, comparing whole path
// elements so that /var does not appear to contain /variable.
func pathWithin(path, root string) bool {
	if root == "/" {
		return strings.HasPrefix(path, "/")
	}
	return path == root || strings.HasPrefix(path, root+"/")
}
