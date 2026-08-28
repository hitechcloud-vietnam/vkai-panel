package ops

// Host facts and resource usage.
//
// These read /proc directly wherever /proc has the answer, rather than shelling
// out to cat and parsing its output. Fewer processes, no argument ever reaching
// a shell, and a failure to read one file does not lose the whole report.

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// SystemInfo is the static picture of a host.
type SystemInfo struct {
	Hostname  string  `json:"hostname"`
	OS        string  `json:"os"`
	OSPretty  string  `json:"os_pretty,omitempty"`
	Kernel    string  `json:"kernel"`
	Arch      string  `json:"arch"`
	CPUCores  int     `json:"cpu_cores"`
	RAMTotal  int64   `json:"ram_total"`
	DiskTotal int64   `json:"disk_total"`
	Uptime    int64   `json:"uptime"`
	Load1     float64 `json:"load1"`
	Load5     float64 `json:"load5"`
	Load15    float64 `json:"load15"`
}

// Metrics is the moving picture.
type Metrics struct {
	CPUPercent float64 `json:"cpu_percent"`
	RAMUsed    int64   `json:"ram_used"`
	RAMTotal   int64   `json:"ram_total"`
	DiskUsed   int64   `json:"disk_used"`
	DiskTotal  int64   `json:"disk_total"`
	NetIn      int64   `json:"net_in"`
	NetOut     int64   `json:"net_out"`
}

// CollectSystemInfo gathers the static facts. Anything unreadable is left at
// its zero value rather than failing the call: a partial report is more useful
// to an operator than no report.
func CollectSystemInfo(ctx context.Context, run CommandRunner) SystemInfo {
	if run == nil {
		run = defaultRunner
	}
	info := SystemInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}
	info.Hostname, _ = os.Hostname()
	info.OSPretty = osPrettyName()

	if out, err := run(ctx, "uname", "-r"); err == nil {
		info.Kernel = strings.TrimSpace(string(out))
	}
	memTotal, _ := readMemInfo()
	info.RAMTotal = memTotal
	info.DiskTotal, _ = readDisk(ctx, run)
	info.Uptime = readUptime()
	info.Load1, info.Load5, info.Load15 = readLoadAverage()
	return info
}

// CollectMetrics gathers current usage.
func CollectMetrics(ctx context.Context, run CommandRunner) Metrics {
	if run == nil {
		run = defaultRunner
	}
	var m Metrics
	total, available := readMemInfo()
	m.RAMTotal = total
	if total > 0 && available > 0 && available <= total {
		m.RAMUsed = total - available
	}
	m.DiskTotal, m.DiskUsed = readDisk(ctx, run)
	m.NetIn, m.NetOut = readNetwork()
	m.CPUPercent = cpuPercentFromLoad()
	return m
}

func osPrettyName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return ""
}

// readMemInfo returns total and available memory in bytes.
func readMemInfo() (total, available int64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, convErr := strconv.ParseInt(fields[1], 10, 64)
		if convErr != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemAvailable:":
			available = value * 1024
		}
	}
	return total, available
}

// readDisk returns the total and used bytes of the root filesystem. df is used
// rather than statfs because the agent must build without cgo and without
// golang.org/x/sys, which is where the syscall wrapper lives.
func readDisk(ctx context.Context, run CommandRunner) (total, used int64) {
	out, err := run(ctx, "df", "-B1", "--output=size,used", "/")
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 2 {
		return 0, 0
	}
	total, _ = strconv.ParseInt(fields[0], 10, 64)
	used, _ = strconv.ParseInt(fields[1], 10, 64)
	return total, used
}

func readUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(seconds)
}

func readLoadAverage() (one, five, fifteen float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	one, _ = strconv.ParseFloat(fields[0], 64)
	five, _ = strconv.ParseFloat(fields[1], 64)
	fifteen, _ = strconv.ParseFloat(fields[2], 64)
	return one, five, fifteen
}

// readNetwork sums the bytes in and out across every interface except loopback.
func readNetwork() (in, out int64) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if strings.TrimSpace(name) == "lo" {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		received, _ := strconv.ParseInt(fields[0], 10, 64)
		transmitted, _ := strconv.ParseInt(fields[8], 10, 64)
		in += received
		out += transmitted
	}
	return in, out
}

// cpuPercentFromLoad turns the one minute load average into a percentage of the
// machine's cores. It is an approximation and is labelled as one: a real
// percentage needs two samples of /proc/stat separated in time, which is the
// panel's job to ask for twice, not the agent's job to block on.
func cpuPercentFromLoad() float64 {
	one, _, _ := readLoadAverage()
	cores := float64(runtime.NumCPU())
	if cores <= 0 {
		return 0
	}
	percent := one / cores * 100
	if percent > 100 {
		return 100
	}
	return percent
}
