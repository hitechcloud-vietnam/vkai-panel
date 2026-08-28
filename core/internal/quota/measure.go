package quota

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// DiskSample is the result of one walk over an account's document roots.
type DiskSample struct {
	UsedBytes int64
	FileCount int64
	Duration  time.Duration

	// Partial is true when the walk stopped on its inode budget or its
	// deadline. UsedBytes is then a LOWER BOUND. The enforcer refuses nothing
	// on a partial sample: telling a customer they are over quota on a
	// measurement that did not finish is worse than being late to notice.
	Partial bool
}

// WalkBudget bounds one account's disk walk.
//
// # THE COST OF MEASURING DISK
//
// There is no cheap way to ask a Linux filesystem "how much space does this
// directory tree use". du walks it, and the walk is O(inodes): one lstat per
// file, plus a getdents per directory. On a WordPress tree of roughly 50,000
// inodes that is a few hundred milliseconds warm - every dentry already in the
// kernel cache - and one to three seconds cold, dominated by seeks. Twenty such
// sites on one account is a minute of continuous IO, and the page cache the
// walk evicts is the customer's own working set: a du over a large tree every
// minute is its own outage, and the sites get slower for as long as it runs.
//
// So the walk is bounded in three ways at once:
//
//  1. It runs on a SCHEDULE, from one background goroutine, one account at a
//     time, with a pause between accounts. It never runs on a request. A page
//     that would trigger a walk to render is a page that can be turned into a
//     denial of service by reloading it.
//
//  2. It stops at MaxFiles inodes or MaxDuration seconds, whichever comes
//     first, and marks the sample partial. The default 2,000,000 inodes is
//     about forty times the size of a large WordPress install; an account that
//     hits it is not a hosting customer any more, and grinding through it every
//     interval would cost more than the limit is worth.
//
//  3. The interval is thirty minutes, not one. Disk fills over hours. The cost
//     of being thirty minutes late to notice is bounded by what the customer
//     can write in thirty minutes, and that is what the grace band is for.
//
// The proper long-term answer is not to walk at all: filesystem project quotas
// (XFS project quotas, or ext4 with a per-site UID) make the kernel keep the
// number for free and enforce it at write time. That needs one UID per site,
// which this panel does not have yet - every site runs as the same user today -
// and it is roadmap item 16, tenant isolation. When it lands, MeasureTrees is
// replaced by reading the quota, and nothing above this function changes.
type WalkBudget struct {
	MaxFiles    int64
	MaxDuration time.Duration
}

// DefaultWalkBudget is the bound applied when none is configured.
func DefaultWalkBudget() WalkBudget {
	return WalkBudget{MaxFiles: 2_000_000, MaxDuration: 60 * time.Second}
}

func (b WalkBudget) withDefaults() WalkBudget {
	d := DefaultWalkBudget()
	if b.MaxFiles <= 0 {
		b.MaxFiles = d.MaxFiles
	}
	if b.MaxDuration <= 0 {
		b.MaxDuration = d.MaxDuration
	}
	return b
}

// hardlinkKey identifies one inode, so a file with several names is counted
// once. Without this a hardlinked backup tree doubles a customer's reported
// usage and pushes them over a quota they never exceeded.
type hardlinkKey struct {
	dev uint64
	ino uint64
}

// MeasureTrees walks every root and returns the total.
//
// It measures ALLOCATED BLOCKS, not apparent size, because that is what du
// reports and what the customer's filesystem actually spends: a 1-byte file
// costs 4KB. Where the platform does not expose block counts the apparent size
// is used instead, which under-reports rather than over-reports - the direction
// that never accuses a customer wrongly.
//
// Symbolic links are never followed. A link into /proc, or a link that points
// at its own ancestor, turns a bounded walk into an unbounded one; the link
// itself is counted, its target is not.
func MeasureTrees(ctx context.Context, roots []string, budget WalkBudget) DiskSample {
	budget = budget.withDefaults()

	started := time.Now()
	deadline := started.Add(budget.MaxDuration)

	var (
		sample DiskSample
		seen   = make(map[hardlinkKey]struct{})
	)

	for _, root := range roots {
		if sample.Partial {
			break
		}
		walkTree(ctx, root, budget, deadline, seen, &sample)
	}

	sample.Duration = time.Since(started)
	return sample
}

func walkTree(ctx context.Context, root string, budget WalkBudget, deadline time.Time, seen map[hardlinkKey]struct{}, sample *DiskSample) {
	if root == "" {
		return
	}
	// A root that is not there is not an error: a site can be created in the
	// database a moment before its directory exists, and a measurement is not
	// the place to complain about it.
	if _, err := os.Lstat(root); err != nil {
		return
	}

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree is skipped, not fatal. The alternative is
			// that one bad permission bit zeroes a customer's whole figure.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		select {
		case <-ctx.Done():
			sample.Partial = true
			return filepath.SkipAll
		default:
		}

		sample.FileCount++
		if sample.FileCount > budget.MaxFiles || time.Now().After(deadline) {
			sample.Partial = true
			return filepath.SkipAll
		}

		// Never descend through a symlink; WalkDir already does not follow
		// them, and the link's own size is counted below.
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Directories occupy blocks too, and du counts them.
			sample.UsedBytes += allocatedBytes(info)
			return nil
		}

		if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
			key := hardlinkKey{dev: uint64(st.Dev), ino: uint64(st.Ino)}
			if _, dup := seen[key]; dup {
				return nil
			}
			seen[key] = struct{}{}
		}

		sample.UsedBytes += allocatedBytes(info)
		return nil
	})
}

// allocatedBytes returns what the file costs the filesystem: st_blocks * 512,
// the same arithmetic du does. Falls back to the apparent size where the
// platform does not provide block counts.
func allocatedBytes(info fs.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Blocks * 512
	}
	if info.Size() < 0 {
		return 0
	}
	return info.Size()
}
