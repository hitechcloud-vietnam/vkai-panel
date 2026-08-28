package localnode

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Column widths from migrations/001_initial_schema.sql. A fact longer than its
// column is truncated here rather than rejected by PostgreSQL, because losing
// the tail of a kernel string is better than failing registration over it.
const (
	maxOSLength     = 100
	maxKernelLength = 100
	maxHostLength   = 255
	maxAddrLength   = 45
)

// diskPath is the filesystem whose size is reported as the node's disk_total.
// It is the root filesystem, which is what the agent reports for a remote node
// (agent/internal/ops/system.go), so the same column means the same thing
// whichever kind of node filled it in.
const diskPath = "/"

// Facts is the measured picture of a host.
//
// Everything the panel might not be able to read is a pointer, and a nil
// pointer reaches the database as NULL. That distinction is the point of the
// type: cpu_cores = 0 or os = 'linux' would be the panel asserting something it
// did not measure, and an operator looking at a node with 0 cores cannot tell
// whether the machine is odd or the panel is guessing.
type Facts struct {
	// Hostname and IPAddress are NOT NULL in the schema, so they are the two
	// facts whose absence fails the collection rather than becoming a null.
	Hostname  string
	IPAddress string

	IPv6Address string // empty means no global IPv6 address, stored as NULL

	OS        *string
	Kernel    *string
	CPUCores  *int
	RAMTotal  *int64 // bytes
	DiskTotal *int64 // bytes
}

// CollectFacts measures this machine.
//
// It reads /proc, /etc/os-release, the interface table and statfs directly. It
// deliberately never reads the panel's own configuration: an operator who typed
// the wrong RAM figure into a config file would otherwise have the panel repeat
// it back as a measurement.
func CollectFacts() (Facts, error) {
	var facts Facts

	hostname, err := os.Hostname()
	if err != nil {
		return facts, fmt.Errorf("localnode: cannot read this machine's hostname: %w", err)
	}
	facts.Hostname = truncate(strings.TrimSpace(hostname), maxHostLength)
	if facts.Hostname == "" {
		return facts, fmt.Errorf("localnode: this machine reports an empty hostname")
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return facts, fmt.Errorf("localnode: cannot read this machine's addresses: %w", err)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP != nil {
			ips = append(ips, ipNet.IP)
		}
	}
	facts.IPAddress = truncate(PickIPv4(ips), maxAddrLength)
	if facts.IPAddress == "" {
		return facts, fmt.Errorf("localnode: this machine has no usable IPv4 address")
	}
	facts.IPv6Address = truncate(PickIPv6(ips), maxAddrLength)

	if name := readOSName(); name != "" {
		value := truncate(name, maxOSLength)
		facts.OS = &value
	}
	if kernel := readKernel(); kernel != "" {
		value := truncate(kernel, maxKernelLength)
		facts.Kernel = &value
	}
	// runtime.NumCPU cannot fail, but it reports the CPUs this process is
	// allowed to use, which under a cgroup or a taskset is the honest answer to
	// "how much of this machine the panel can drive".
	if cores := runtime.NumCPU(); cores > 0 {
		facts.CPUCores = &cores
	}
	if total, ok := readMemTotal(); ok {
		facts.RAMTotal = &total
	}
	if total, ok := totalDiskBytes(diskPath); ok {
		facts.DiskTotal = &total
	}
	return facts, nil
}

// PickIPv4 chooses the address that best identifies this host to the outside.
//
// A publicly routable address wins, because that is the one a customer's DNS
// will point at. A private address is the fallback, for a node behind NAT.
// Loopback is last and is still a true statement about the machine: a panel on
// a host with no other address is reachable at 127.0.0.1 and nowhere else.
func PickIPv4(ips []net.IP) string {
	var private, loopback string
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		switch {
		case v4.IsLoopback():
			if loopback == "" {
				loopback = v4.String()
			}
		case v4.IsLinkLocalUnicast(), v4.IsUnspecified(), v4.IsMulticast():
			continue
		case v4.IsPrivate():
			if private == "" {
				private = v4.String()
			}
		case v4.IsGlobalUnicast():
			return v4.String()
		}
	}
	if private != "" {
		return private
	}
	return loopback
}

// PickIPv6 returns the first globally routable IPv6 address, or nothing.
//
// Link-local, unique-local and loopback addresses are all skipped rather than
// used as a fallback: unlike the IPv4 case there is no column that has to be
// filled, so an absent global address is recorded as absent.
func PickIPv6(ips []net.IP) string {
	for _, ip := range ips {
		if ip.To4() != nil || !ip.IsGlobalUnicast() {
			continue
		}
		// fc00::/7, the unique local range. It is routable inside one site and
		// meaningless outside it, which is not what this column is for.
		if len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc {
			continue
		}
		return ip.String()
	}
	return ""
}

func readOSName() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	return ParseOSRelease(data)
}

// ParseOSRelease pulls the human-readable distribution name out of
// /etc/os-release. PRETTY_NAME is preferred; NAME with VERSION is the fallback
// for a file that omits it. A file with neither yields nothing, and the column
// stays null.
func ParseOSRelease(data []byte) string {
	values := make(map[string]string, 8)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if pretty := values["PRETTY_NAME"]; pretty != "" {
		return pretty
	}
	name := values["NAME"]
	if name == "" {
		return ""
	}
	if version := values["VERSION"]; version != "" {
		return name + " " + version
	}
	return name
}

func readKernel() string {
	// /proc/sys/kernel/osrelease is what `uname -r` prints, without starting a
	// process to ask.
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readMemTotal() (int64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	return ParseMemTotal(data)
}

// ParseMemTotal reads MemTotal out of /proc/meminfo and returns it in bytes.
// The second result is false when the field is absent or unparsable, which is
// what keeps ram_total null instead of zero.
func ParseMemTotal(data []byte) (int64, bool) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemTotal:" {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || value <= 0 {
			return 0, false
		}
		// /proc/meminfo reports kibibytes; the column is bytes.
		return value * 1024, true
	}
	return 0, false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
