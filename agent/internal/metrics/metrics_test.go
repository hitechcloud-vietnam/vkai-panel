package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The whole collector is driven from synthetic /proc content. Nothing here
// reads the machine the tests run on, so the assertions are about arithmetic
// and about what happens when a file is missing - which is the case that used
// to produce a dashboard reading 0%.

// fakeProc serves scripted file contents. A path can be given several
// successive contents, which is how two readings of /proc/stat a measured
// interval apart are simulated without any time passing.
type fakeProc struct {
	files map[string][]string
	reads map[string]int
	clock time.Time
}

func newFakeProc() *fakeProc {
	return &fakeProc{
		files: map[string][]string{},
		reads: map[string]int{},
		clock: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC),
	}
}

func (p *fakeProc) set(path string, contents ...string) {
	p.files[path] = contents
}

func (p *fakeProc) readFile(path string) ([]byte, error) {
	contents, found := p.files[path]
	if !found {
		return nil, fmt.Errorf("open %s: no such file or directory", path)
	}
	index := p.reads[path]
	p.reads[path]++
	if index >= len(contents) {
		index = len(contents) - 1
	}
	// Every read advances the clock by one second, so the collector measures a
	// known interval without the test waiting for one.
	p.clock = p.clock.Add(time.Second)
	return []byte(contents[index]), nil
}

// collector wires a fake /proc into a collector with a fake statfs.
func (p *fakeProc) collector() *Collector {
	c := &Collector{
		ProcRoot:    "/proc",
		ReadFile:    p.readFile,
		CPUInterval: time.Millisecond,
		Now:         func() time.Time { return p.clock },
		Statfs: func(string) (FSStat, error) {
			return FSStat{}, fmt.Errorf("no statfs in this test")
		},
	}
	c.applyDefaults()
	return c
}

// statLine builds a /proc/stat body from jiffy totals.
func statLine(aggregate [8]uint64, cores ...[8]uint64) string {
	var b strings.Builder
	write := func(name string, values [8]uint64) {
		b.WriteString(name)
		for _, v := range values {
			fmt.Fprintf(&b, " %d", v)
		}
		b.WriteString("\n")
	}
	write("cpu ", aggregate)
	for idx, core := range cores {
		write(fmt.Sprintf("cpu%d", idx), core)
	}
	b.WriteString("intr 12345\nctxt 999\nbtime 1700000000\n")
	return b.String()
}

// ============================================================
// CPU
// ============================================================

// The old implementation divided the one minute load average by the core count
// and reported the result as a CPU percentage. This is the test that the
// percentage is now measured: two readings of /proc/stat with a known amount of
// busy and idle time between them must produce the arithmetic answer.
func TestCPUPercentIsMeasuredBetweenTwoReadingsOfProcStat(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/stat",
		statLine([8]uint64{100, 0, 0, 900, 0, 0, 0, 0},
			[8]uint64{50, 0, 0, 450, 0, 0, 0, 0},
			[8]uint64{50, 0, 0, 450, 0, 0, 0, 0}),
		statLine([8]uint64{600, 0, 0, 1400, 0, 0, 0, 0},
			[8]uint64{550, 0, 0, 450, 0, 0, 0, 0},
			[8]uint64{50, 0, 0, 950, 0, 0, 0, 0}),
	)
	cpu := proc.collector().collectCPU(context.Background())

	if !cpu.Available {
		t.Fatalf("CPU is unavailable: %s", cpu.Reason)
	}
	// 500 busy jiffies out of 1000 elapsed.
	if *cpu.UsagePercent != 50 {
		t.Fatalf("usage is %.2f%%, want 50%%", *cpu.UsagePercent)
	}
	if *cpu.IdlePercent != 50 {
		t.Fatalf("idle is %.2f%%, want 50%%", *cpu.IdlePercent)
	}
	if *cpu.IntervalSeconds != 1 {
		t.Fatalf("the interval is %.3fs, want 1s", *cpu.IntervalSeconds)
	}
	if cpu.Cores != 2 {
		t.Fatalf("cores is %d, want 2", cpu.Cores)
	}
	// One core did all of the work; the aggregate cannot show that and the
	// per-core breakdown is the reason it is collected.
	if len(cpu.PerCore) != 2 || cpu.PerCore[0].UsagePercent != 100 || cpu.PerCore[1].UsagePercent != 0 {
		t.Fatalf("per-core usage is %+v, want core 0 at 100%% and core 1 at 0%%", cpu.PerCore)
	}
}

// A percentage that is plausible is not the same as a percentage that is
// correct, and this is the plausibility half: whatever the counters say, the
// answer is a percentage of one machine.
func TestCPUPercentStaysWithinZeroAndOneHundred(t *testing.T) {
	for _, shape := range []struct {
		name    string
		first   [8]uint64
		second  [8]uint64
		wantMin float64
		wantMax float64
	}{
		{"fully idle", [8]uint64{0, 0, 0, 1000, 0, 0, 0, 0}, [8]uint64{0, 0, 0, 2000, 0, 0, 0, 0}, 0, 0},
		{"fully busy", [8]uint64{0, 0, 0, 1000, 0, 0, 0, 0}, [8]uint64{1000, 0, 0, 1000, 0, 0, 0, 0}, 100, 100},
		{"a quarter busy", [8]uint64{0, 0, 0, 1000, 0, 0, 0, 0}, [8]uint64{250, 0, 0, 1750, 0, 0, 0, 0}, 25, 25},
		{"blocked on disk", [8]uint64{0, 0, 0, 1000, 0, 0, 0, 0}, [8]uint64{0, 0, 0, 1000, 1000, 0, 0, 0}, 0, 0},
	} {
		t.Run(shape.name, func(t *testing.T) {
			proc := newFakeProc()
			proc.set("/proc/stat", statLine(shape.first), statLine(shape.second))
			cpu := proc.collector().collectCPU(context.Background())
			if !cpu.Available {
				t.Fatalf("CPU is unavailable: %s", cpu.Reason)
			}
			if *cpu.UsagePercent < shape.wantMin || *cpu.UsagePercent > shape.wantMax {
				t.Fatalf("usage is %.2f%%, want between %.0f and %.0f", *cpu.UsagePercent, shape.wantMin, shape.wantMax)
			}
		})
	}
}

// Time spent waiting on a disk is not time spent computing, and reporting it as
// CPU usage is how a storage problem gets diagnosed as a CPU problem.
func TestIOWaitIsReportedSeparatelyFromUsage(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/stat",
		statLine([8]uint64{0, 0, 0, 1000, 0, 0, 0, 0}),
		statLine([8]uint64{0, 0, 0, 1000, 1000, 0, 0, 0}),
	)
	cpu := proc.collector().collectCPU(context.Background())
	if *cpu.UsagePercent != 0 {
		t.Fatalf("usage is %.2f%%, want 0%% - the interval was spent in iowait", *cpu.UsagePercent)
	}
	if *cpu.IOWaitPercent != 100 {
		t.Fatalf("iowait is %.2f%%, want 100%%", *cpu.IOWaitPercent)
	}
}

// The second and subsequent samples must not stop to take a pair of readings:
// the previous sample is the base, and the rate is measured over the whole
// reporting interval.
func TestASecondSampleDifferencesAgainstTheFirstWithoutWaiting(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/stat",
		statLine([8]uint64{0, 0, 0, 1000, 0, 0, 0, 0}),
		statLine([8]uint64{100, 0, 0, 1900, 0, 0, 0, 0}),
		statLine([8]uint64{600, 0, 0, 2400, 0, 0, 0, 0}),
	)
	collector := proc.collector()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := collector.collectCPU(ctx)
	if !first.Available {
		t.Fatalf("the first CPU reading is unavailable: %s", first.Reason)
	}

	// From here on, waiting for a fresh pair would hang the test - which is the
	// assertion: with a usable previous reading, nothing waits.
	collector.CPUInterval = time.Hour

	// Nothing may block here: the previous reading is the base.
	done := make(chan CPU, 1)
	go func() { done <- collector.collectCPU(ctx) }()
	select {
	case second := <-done:
		if !second.Available {
			t.Fatalf("the second CPU reading is unavailable: %s", second.Reason)
		}
		if *second.UsagePercent != 50 {
			t.Fatalf("the second reading is %.2f%%, want 50%%", *second.UsagePercent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the second sample waited for a fresh pair of readings instead of differencing against the first")
	}
}

// ============================================================
// UNAVAILABLE IS NOT ZERO
// ============================================================

// The failure this package was written for. A dashboard drawing 0% CPU because
// /proc/stat could not be read is worse than one drawing a gap, because nobody
// investigates a quiet machine.
func TestAnUnreadableMetricIsUnavailableAndNotZero(t *testing.T) {
	proc := newFakeProc() // nothing is set: every file is missing
	sample := proc.collector().Sample(context.Background())

	if sample.Complete() {
		t.Fatal("a sample with no readable /proc at all reports itself as complete")
	}
	for _, group := range []string{"cpu", "memory", "load", "uptime", "disks", "network"} {
		found := false
		for _, name := range sample.Unavailable() {
			if name == group {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s is missing from the unavailable list %v", group, sample.Unavailable())
		}
	}
	if sample.CPU.UsagePercent != nil {
		t.Fatalf("an uncollectable CPU reported %.2f%% instead of nothing", *sample.CPU.UsagePercent)
	}
	if sample.Memory.TotalBytes != nil || sample.Memory.UsedBytes != nil {
		t.Fatal("an uncollectable memory group reported byte counts")
	}
	if sample.Network.BytesIn != nil {
		t.Fatal("an uncollectable network group reported a byte count")
	}
	if sample.CPU.Reason == "" || sample.Memory.Reason == "" {
		t.Fatal("an unavailable group does not say why")
	}
}

// The same thing, checked on the wire rather than in the struct, because what
// the panel sees is the JSON.
func TestAnUnavailableGroupSendsNoNumbersAtAll(t *testing.T) {
	proc := newFakeProc()
	sample := proc.collector().Sample(context.Background())
	encoded, err := json.Marshal(sample.CPU)
	if err != nil {
		t.Fatalf("cannot encode the CPU group: %v", err)
	}
	body := string(encoded)
	if !strings.Contains(body, `"available":false`) {
		t.Fatalf("the encoded CPU group does not say it is unavailable: %s", body)
	}
	for _, forbidden := range []string{"usage_percent", "idle_percent", "iowait_percent"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("the encoded CPU group carries %s even though it was not collected: %s", forbidden, body)
		}
	}
}

// One unreadable file must not cost the operator every other metric.
func TestOneMissingFileDoesNotLoseTheRestOfTheSample(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/meminfo", "MemTotal: 1024 kB\nMemAvailable: 512 kB\n")
	// /proc/stat, /proc/loadavg, /proc/uptime, mounts and net/dev are absent.
	sample := proc.collector().Sample(context.Background())

	if !sample.Memory.Available {
		t.Fatalf("memory is unavailable even though /proc/meminfo was readable: %s", sample.Memory.Reason)
	}
	if *sample.Memory.TotalBytes != 1024*1024 {
		t.Fatalf("memory total is %d, want %d", *sample.Memory.TotalBytes, 1024*1024)
	}
	if sample.CPU.Available {
		t.Fatal("CPU claims to be available with no /proc/stat")
	}
}

// ============================================================
// MEMORY
// ============================================================

func TestMemoryUsesMemAvailableRatherThanMemFree(t *testing.T) {
	proc := newFakeProc()
	// A healthy Linux box: almost nothing free, most of it reclaimable cache.
	proc.set("/proc/meminfo", strings.Join([]string{
		"MemTotal:       16000000 kB",
		"MemFree:          200000 kB",
		"MemAvailable:   12000000 kB",
		"Buffers:          300000 kB",
		"Cached:         11000000 kB",
		"SwapTotal:       2000000 kB",
		"SwapFree:        1500000 kB",
		"HugePages_Total:       0",
	}, "\n"))
	mem := proc.collector().collectMemory()

	if !mem.Available {
		t.Fatalf("memory is unavailable: %s", mem.Reason)
	}
	if *mem.UsedBytes != 4000000*1024 {
		t.Fatalf("used is %d bytes, want %d - it must be total minus MemAvailable, not total minus MemFree",
			*mem.UsedBytes, 4000000*1024)
	}
	if *mem.UsedPercent != 25 {
		t.Fatalf("used is %.2f%%, want 25%%", *mem.UsedPercent)
	}
	if *mem.SwapUsedBytes != 500000*1024 {
		t.Fatalf("swap used is %d bytes, want %d", *mem.SwapUsedBytes, 500000*1024)
	}
	// HugePages_Total is a count, not a size, and must not be multiplied by 1024
	// or read as bytes; it is simply not one of the fields collected.
	if mem.TotalBytes == nil || *mem.TotalBytes != 16000000*1024 {
		t.Fatalf("total is %v, want %d bytes", mem.TotalBytes, 16000000*1024)
	}
}

// A kernel older than 3.14 has no MemAvailable. Substituting MemFree would
// report every such host as nearly full; the totals are still sent and the used
// figure is withheld with an explanation.
func TestMemoryWithoutMemAvailableWithholdsTheUsedFigure(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/meminfo", "MemTotal: 1000000 kB\nMemFree: 10000 kB\nCached: 800000 kB\n")
	mem := proc.collector().collectMemory()

	if mem.UsedBytes != nil {
		t.Fatalf("used was reported as %d bytes on a kernel with no MemAvailable", *mem.UsedBytes)
	}
	if mem.TotalBytes == nil {
		t.Fatal("the total was withheld along with the used figure")
	}
	if !strings.Contains(mem.Reason, "MemAvailable") {
		t.Fatalf("the reason does not mention MemAvailable: %q", mem.Reason)
	}
}

// ============================================================
// LOAD AND UPTIME
// ============================================================

func TestLoadAverageIsReportedAlongsideCPUAndNotInsteadOfIt(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/loadavg", "2.50 1.25 0.75 3/841 12345\n")
	load := proc.collector().collectLoad(4)

	if !load.Available {
		t.Fatalf("load is unavailable: %s", load.Reason)
	}
	if *load.One != 2.5 || *load.Five != 1.25 || *load.Fifteen != 0.75 {
		t.Fatalf("load is %.2f/%.2f/%.2f, want 2.50/1.25/0.75", *load.One, *load.Five, *load.Fifteen)
	}
	if *load.PerCore != 0.63 {
		t.Fatalf("load per core is %.2f, want 0.63 on four cores", *load.PerCore)
	}
	if *load.RunningEntities != 3 || *load.TotalEntities != 841 {
		t.Fatalf("scheduling entities are %d/%d, want 3/841", *load.RunningEntities, *load.TotalEntities)
	}
}

func TestUptime(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/uptime", "864000.42 1700000.00\n")
	up := proc.collector().collectUptime()

	if !up.Available {
		t.Fatalf("uptime is unavailable: %s", up.Reason)
	}
	if *up.Seconds != 864000 {
		t.Fatalf("uptime is %d seconds, want 864000", *up.Seconds)
	}
	if *up.Days != 10 {
		t.Fatalf("uptime is %.2f days, want 10", *up.Days)
	}
}

func TestAnUnparsableFileIsUnavailableRatherThanZero(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/uptime", "this is not a number\n")
	proc.set("/proc/loadavg", "nonsense\n")
	collector := proc.collector()

	if up := collector.collectUptime(); up.Available || up.Seconds != nil {
		t.Fatalf("an unparsable /proc/uptime produced %+v", up)
	}
	if load := collector.collectLoad(1); load.Available || load.One != nil {
		t.Fatalf("an unparsable /proc/loadavg produced %+v", load)
	}
}

// ============================================================
// DISK, PER MOUNT
// ============================================================

const mountTable = `sysfs /sys sysfs rw,nosuid 0 0
proc /proc proc rw,nosuid 0 0
udev /dev devtmpfs rw,nosuid 0 0
tmpfs /run tmpfs rw,nosuid 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sdb1 /var/www ext4 rw,relatime 0 0
/dev/sdb1 /var/www-bind ext4 rw,relatime 0 0
/dev/sdc1 /mnt/backup\040volume xfs ro,relatime 0 0
cgroup2 /sys/fs/cgroup cgroup2 rw 0 0
`

func mountCollector(t *testing.T, stats map[string]FSStat) *Collector {
	t.Helper()
	proc := newFakeProc()
	proc.set("/proc/self/mounts", mountTable)
	c := proc.collector()
	c.Statfs = func(path string) (FSStat, error) {
		stat, found := stats[path]
		if !found {
			return FSStat{}, fmt.Errorf("statfs %s: no such device", path)
		}
		return stat, nil
	}
	return c
}

func TestEveryRealFilesystemIsReportedAndThePseudoOnesAreNot(t *testing.T) {
	disks := mountCollector(t, map[string]FSStat{
		"/":                  {TotalBytes: 100, FreeBytes: 40, AvailableBytes: 30, InodesTotal: 100, InodesFree: 90},
		"/var/www":           {TotalBytes: 200, FreeBytes: 100, AvailableBytes: 100},
		"/mnt/backup volume": {TotalBytes: 300, FreeBytes: 150, AvailableBytes: 150},
	}).collectDisks()

	if !disks.Available {
		t.Fatalf("disks are unavailable: %s", disks.Reason)
	}
	got := map[string]bool{}
	for _, mount := range disks.Mounts {
		got[mount.Mountpoint] = true
	}
	for _, want := range []string{"/", "/var/www", "/mnt/backup volume"} {
		if !got[want] {
			t.Fatalf("%s is missing from %v", want, got)
		}
	}
	for _, unwanted := range []string{"/proc", "/sys", "/run", "/dev", "/sys/fs/cgroup"} {
		if got[unwanted] {
			t.Fatalf("the pseudo filesystem %s was reported as a disk", unwanted)
		}
	}
	// The bind mount shares /dev/sdb1 with /var/www; counting both would show
	// this host as having twice the disk it has.
	if got["/var/www-bind"] {
		t.Fatal("a bind mount of an already reported device was counted a second time")
	}
}

// The kernel escapes a space in a mount point as \040, and a mount point read
// without undoing that matches nothing else on the system.
func TestAMountPointWithASpaceIsUnescaped(t *testing.T) {
	disks := mountCollector(t, map[string]FSStat{
		"/":                  {TotalBytes: 100, FreeBytes: 40, AvailableBytes: 30},
		"/mnt/backup volume": {TotalBytes: 300, FreeBytes: 150, AvailableBytes: 150},
	}).collectDisks()

	for _, mount := range disks.Mounts {
		if mount.Mountpoint == "/mnt/backup volume" {
			if !mount.Available {
				t.Fatalf("the unescaped mount point was not usable: %s", mount.Reason)
			}
			if !mount.ReadOnly {
				t.Fatal("a mount listed ro was not reported as read only")
			}
			return
		}
	}
	t.Fatalf("the escaped mount point was not unescaped: %+v", disks.Mounts)
}

func TestUsedPercentFollowsDfAndCountsTheRootReserve(t *testing.T) {
	disks := mountCollector(t, map[string]FSStat{
		// 100 total, 40 free, but only 30 of that is available to a non-root
		// user: 10 bytes are the root reserve. df calls this 66.67% full, and
		// the point where a customer's upload starts failing is 100%, not 90%.
		"/": {TotalBytes: 100, FreeBytes: 40, AvailableBytes: 30},
	}).collectDisks()

	root, found := disks.Root()
	if !found {
		t.Fatal("the root filesystem is missing from the report")
	}
	if *root.UsedBytes != 60 {
		t.Fatalf("used is %d bytes, want 60", *root.UsedBytes)
	}
	if *root.UsedPercent != 66.67 {
		t.Fatalf("used is %.2f%%, want 66.67%%", *root.UsedPercent)
	}
}

// A stale NFS handle on one mount must not take the local disks with it.
func TestOneUnstattableMountDoesNotLoseTheOthers(t *testing.T) {
	disks := mountCollector(t, map[string]FSStat{
		"/": {TotalBytes: 100, FreeBytes: 40, AvailableBytes: 30},
		// /var/www and the backup volume have no entry, so statfs fails there.
	}).collectDisks()

	if !disks.Available {
		t.Fatalf("the whole disk group failed because one mount did: %s", disks.Reason)
	}
	for _, mount := range disks.Mounts {
		switch mount.Mountpoint {
		case "/":
			if !mount.Available || mount.TotalBytes == nil {
				t.Fatalf("the root filesystem was lost: %+v", mount)
			}
		case "/var/www":
			if mount.Available {
				t.Fatal("a mount whose statfs failed reported itself as available")
			}
			if mount.TotalBytes != nil || mount.UsedBytes != nil {
				t.Fatalf("a mount whose statfs failed reported byte counts: %+v", mount)
			}
			if mount.Reason == "" {
				t.Fatal("a mount whose statfs failed does not say why")
			}
		}
	}
}

func TestMountForFindsTheLongestMatchingMountPoint(t *testing.T) {
	collector := mountCollector(t, map[string]FSStat{
		"/":        {TotalBytes: 100, FreeBytes: 40, AvailableBytes: 30},
		"/var/www": {TotalBytes: 200, FreeBytes: 100, AvailableBytes: 100},
	})
	mount, err := collector.MountFor("/var/www/example.com/public")
	if err != nil {
		t.Fatalf("MountFor failed: %v", err)
	}
	if mount.Mountpoint != "/var/www" {
		t.Fatalf("the path was attributed to %s, want /var/www", mount.Mountpoint)
	}
	// /variable must not be attributed to /var: the comparison is by whole path
	// element, not by string prefix.
	if mount, err = collector.MountFor("/variable/thing"); err != nil {
		t.Fatalf("MountFor failed: %v", err)
	}
	if mount.Mountpoint != "/" {
		t.Fatalf("/variable/thing was attributed to %s, want /", mount.Mountpoint)
	}
}

// ============================================================
// NETWORK
// ============================================================

func netDev(rxEth, txEth uint64) string {
	return "Inter-|   Receive                                                |  Transmit\n" +
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n" +
		"    lo: 999999 100 0 0 0 0 0 0 999999 100 0 0 0 0 0 0\n" +
		fmt.Sprintf("  eth0: %d 10 1 2 0 0 0 0 %d 20 3 4 0 0 0 0\n", rxEth, txEth)
}

func TestNetworkCountersExcludeLoopbackAndRatesNeedTwoSamples(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/net/dev", netDev(1000, 2000), netDev(6000, 7000))
	collector := proc.collector()

	first := collector.collectNetwork()
	if !first.Available {
		t.Fatalf("network is unavailable: %s", first.Reason)
	}
	if *first.BytesIn != 1000 || *first.BytesOut != 2000 {
		t.Fatalf("totals are %d/%d, want 1000/2000 - loopback must not be counted", *first.BytesIn, *first.BytesOut)
	}
	if first.BytesInPerSecond != nil {
		t.Fatalf("the first sample reported a rate of %.2f B/s with nothing to measure against", *first.BytesInPerSecond)
	}
	if len(first.Interfaces) != 1 || first.Interfaces[0].RateNote == "" {
		t.Fatalf("the first sample does not explain its missing per-interface rate: %+v", first.Interfaces)
	}

	second := collector.collectNetwork()
	// The fake clock advances one second per file read.
	if second.BytesInPerSecond == nil || *second.BytesInPerSecond != 5000 {
		t.Fatalf("the inbound rate is %v, want 5000 B/s", second.BytesInPerSecond)
	}
	if *second.BytesOutPerSecond != 5000 {
		t.Fatalf("the outbound rate is %.2f B/s, want 5000", *second.BytesOutPerSecond)
	}
	if second.Interfaces[0].DropsIn != 2 || second.Interfaces[0].ErrorsOut != 3 {
		t.Fatalf("per-interface errors and drops are wrong: %+v", second.Interfaces[0])
	}
}

// An interface that is recreated, or a 32 bit counter that wraps, makes the
// difference between two readings meaningless. Reporting it would draw a spike
// of traffic that never happened.
func TestACounterThatWentBackwardsProducesNoRate(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/net/dev", netDev(9000, 9000), netDev(10, 10))
	collector := proc.collector()

	collector.collectNetwork()
	second := collector.collectNetwork()

	if second.BytesInPerSecond != nil {
		t.Fatalf("a reset counter produced a rate of %.2f B/s", *second.BytesInPerSecond)
	}
	if second.Interfaces[0].BytesInPerSecond != nil {
		t.Fatalf("a reset interface counter produced a rate: %+v", second.Interfaces[0])
	}
	if !strings.Contains(second.Interfaces[0].RateNote, "reset") {
		t.Fatalf("the missing rate is not explained: %q", second.Interfaces[0].RateNote)
	}
}

// ============================================================
// THE HOST DESCRIPTION
// ============================================================

func TestHostFactsComeFromProcAndNotFromASubprocess(t *testing.T) {
	proc := newFakeProc()
	proc.set("/proc/sys/kernel/osrelease", "6.8.0-79-generic\n")
	proc.set("/etc/os-release", "PRETTY_NAME=\"Ubuntu 24.04.3 LTS\"\nID=ubuntu\nVERSION_ID=\"24.04\"\n")
	proc.set("/proc/cpuinfo", strings.Join([]string{
		"processor\t: 0",
		"model name\t: Intel(R) Xeon(R) Gold 6133 CPU @ 2.50GHz",
		"flags\t\t: fpu vme hypervisor lm",
		"",
		"processor\t: 1",
		"model name\t: Intel(R) Xeon(R) Gold 6133 CPU @ 2.50GHz",
		"flags\t\t: fpu vme hypervisor lm",
	}, "\n"))

	host := proc.collector().CollectHost()
	if host.Kernel != "6.8.0-79-generic" {
		t.Fatalf("kernel is %q, want 6.8.0-79-generic", host.Kernel)
	}
	if host.OSPretty != "Ubuntu 24.04.3 LTS" || host.OSID != "ubuntu" || host.OSVersionID != "24.04" {
		t.Fatalf("the distribution was read as %q/%q/%q", host.OSPretty, host.OSID, host.OSVersionID)
	}
	if host.CPUCores != 2 {
		t.Fatalf("cores is %d, want 2", host.CPUCores)
	}
	if host.Virtualisation == "" {
		t.Fatal("the hypervisor flag was present but the host is not reported as a guest")
	}
}

// ============================================================
// THE REPORT STAYS INSIDE THE PANEL'S BODY LIMIT
// ============================================================

// The panel reads at most 64KB of an agent's status body. A host with a large
// number of cores, mounts and interfaces must not push a sample past that and
// have every one of its reports rejected.
func TestASampleFromALargeHostStaysWellUnderThePanelsBodyLimit(t *testing.T) {
	// The panel reads 64KB. The sample must fit inside a fraction of that,
	// because it travels inside a status payload that also carries the host
	// description and the buffer's state, and because an agent whose reports
	// are exactly at the limit is one distribution's worth of extra mount
	// points away from having every report rejected.
	const sampleBudget = 32 << 10

	proc := newFakeProc()
	before := make([][8]uint64, 256)
	after := make([][8]uint64, 256)
	for idx := range before {
		before[idx] = [8]uint64{100, 0, 0, 900, 0, 0, 0, 0}
		after[idx] = [8]uint64{600, 0, 0, 1400, 0, 0, 0, 0}
	}
	proc.set("/proc/stat", statLine([8]uint64{100, 0, 0, 900, 0, 0, 0, 0}, before...),
		statLine([8]uint64{600, 0, 0, 1400, 0, 0, 0, 0}, after...))
	proc.set("/proc/meminfo", "MemTotal: 16000000 kB\nMemAvailable: 8000000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n")
	proc.set("/proc/loadavg", "1.00 1.00 1.00 1/100 1\n")
	proc.set("/proc/uptime", "1000.00 500.00\n")

	var mounts, interfaces strings.Builder
	for idx := 0; idx < 200; idx++ {
		fmt.Fprintf(&mounts, "/dev/sd%03d /mnt/volume-%03d ext4 rw,relatime 0 0\n", idx, idx)
		fmt.Fprintf(&interfaces, " veth%03d: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0\n", idx)
	}
	proc.set("/proc/self/mounts", mounts.String())
	proc.set("/proc/net/dev", "Inter-|\n face |\n"+interfaces.String())

	collector := proc.collector()
	collector.Statfs = func(string) (FSStat, error) {
		return FSStat{TotalBytes: 1 << 40, FreeBytes: 1 << 39, AvailableBytes: 1 << 39, InodesTotal: 1000, InodesFree: 900}, nil
	}

	sample := collector.Sample(context.Background())
	encoded, err := json.Marshal(sample)
	if err != nil {
		t.Fatalf("cannot encode the sample: %v", err)
	}
	if len(encoded) >= sampleBudget {
		t.Fatalf("a sample from a large host encodes to %d bytes, past the %d byte budget it has "+
			"inside the panel's 64KB status body", len(encoded), sampleBudget)
	}
	if len(sample.Disks.Mounts) > DefaultMaxDisks || !sample.Disks.Truncated {
		t.Fatalf("the mount list was not bounded: %d mounts, truncated=%v",
			len(sample.Disks.Mounts), sample.Disks.Truncated)
	}
	if len(sample.Network.Interfaces) > DefaultMaxInterfaces || !sample.Network.Truncated {
		t.Fatalf("the interface list was not bounded: %d interfaces", len(sample.Network.Interfaces))
	}
	if len(sample.CPU.PerCore) != 0 || sample.CPU.PerCoreNote == "" {
		t.Fatalf("256 cores were reported individually instead of being summarised: %d entries, note %q",
			len(sample.CPU.PerCore), sample.CPU.PerCoreNote)
	}
	// The aggregate must survive the summarising.
	if sample.CPU.UsagePercent == nil {
		t.Fatal("the aggregate CPU percentage was dropped along with the per-core detail")
	}
}
