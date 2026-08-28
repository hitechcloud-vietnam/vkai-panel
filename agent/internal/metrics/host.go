package metrics

// Static facts about the host.
//
// The kernel version used to come from running `uname -r`. It is in
// /proc/sys/kernel/osrelease, where reading it cannot fail because PATH is
// wrong, cannot be slowed down by a loaded machine forking, and cannot be
// affected by anything on the host but the kernel itself.

import (
	"os"
	"runtime"
	"strings"
)

// Host is what does not change between samples.
type Host struct {
	Hostname     string `json:"hostname,omitempty"`
	OS           string `json:"os"`
	OSPretty     string `json:"os_pretty,omitempty"`
	OSID         string `json:"os_id,omitempty"`
	OSVersionID  string `json:"os_version_id,omitempty"`
	Kernel       string `json:"kernel,omitempty"`
	Architecture string `json:"architecture"`
	CPUModel     string `json:"cpu_model,omitempty"`
	CPUCores     int    `json:"cpu_cores"`
	// Virtualisation names the hypervisor when one is detectable, because
	// "steal time is 30%" only makes sense once an operator knows the box is a
	// guest.
	Virtualisation string `json:"virtualisation,omitempty"`
}

// CollectHost gathers the static picture. Individual facts that cannot be read
// are left empty; unlike a metric, an absent hostname is not something a
// dashboard can misread as a measurement.
func (c *Collector) CollectHost() Host {
	c.applyDefaults()
	h := Host{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		CPUCores:     runtime.NumCPU(),
	}
	h.Hostname, _ = os.Hostname()

	if data, err := c.read("sys", "kernel", "osrelease"); err == nil {
		h.Kernel = strings.TrimSpace(string(data))
	}
	h.OSPretty, h.OSID, h.OSVersionID = c.osRelease()
	h.CPUModel, h.Virtualisation = c.cpuModel()

	// /proc/cpuinfo is the authority on how many processors this host has when
	// the agent is confined to a subset of them; runtime.NumCPU reports the
	// process's affinity mask, which is the more useful number of the two and
	// is kept when cpuinfo cannot be read.
	if count := c.processorCount(); count > 0 {
		h.CPUCores = count
	}
	return h
}

// osRelease reads the distribution's own description of itself. The file is
// outside /proc, and its location is fixed by the standard that defines it.
func (c *Collector) osRelease() (pretty, id, versionID string) {
	data, err := c.ReadFile("/etc/os-release")
	if err != nil {
		return "", "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"'`)
		switch key {
		case "PRETTY_NAME":
			pretty = value
		case "ID":
			id = value
		case "VERSION_ID":
			versionID = value
		}
	}
	return pretty, id, versionID
}

// cpuModel reads the processor name and, on x86, the hypervisor vendor.
func (c *Collector) cpuModel() (model, virtualisation string) {
	data, err := c.read("cpuinfo")
	if err != nil {
		return "", ""
	}
	flags := ""
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "model name", "Model", "cpu model":
			if model == "" {
				model = value
			}
		case "Hypervisor vendor":
			virtualisation = value
		case "flags", "Features":
			if flags == "" {
				flags = value
			}
		}
	}
	if virtualisation == "" && flags != "" {
		for _, flag := range strings.Fields(flags) {
			if flag == "hypervisor" {
				virtualisation = "guest"
				break
			}
		}
	}
	return model, virtualisation
}

func (c *Collector) processorCount() int {
	data, err := c.read("cpuinfo")
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if key, _, found := strings.Cut(line, ":"); found && strings.TrimSpace(key) == "processor" {
			count++
		}
	}
	return count
}
