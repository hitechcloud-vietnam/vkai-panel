package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Server management",
	Long:  `Commands for viewing server status, system information, and managing server resources.`,
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server status",
	Run:   runServerStatus,
}

var serverInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show system information",
	Run:   runServerInfo,
}

var serverServicesCmd = &cobra.Command{
	Use:   "services",
	Short: "Show service status",
	Run:   runServerServices,
}

var serverPortsCmd = &cobra.Command{
	Use:   "ports",
	Short: "Show listening ports",
	Run:   runServerPorts,
}

func init() {
	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverInfoCmd)
	serverCmd.AddCommand(serverServicesCmd)
	serverCmd.AddCommand(serverPortsCmd)
}

func runServerStatus(cmd *cobra.Command, args []string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Println("=== VKAI Panel Server Status ===")
	fmt.Println()

	// System info
	fmt.Fprintf(w, "Hostname:\t%s\n", getHostname())
	fmt.Fprintf(w, "OS:\t%s %s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(w, "Go Version:\t%s\n", runtime.Version())
	fmt.Fprintf(w, "CPUs:\t%d\n", runtime.NumCPU())
	fmt.Fprintf(w, "Uptime:\t%s\n", getUptime())
	fmt.Fprintf(w, "Time:\t%s\n", time.Now().Format("2006-01-02 15:04:05"))
	w.Flush()

	fmt.Println()
	fmt.Println("=== Services ===")

	services := []struct {
		name string
		port string
	}{
		{"vkai-api", "30110"},
		{"vkai-ui", "3000"},
		{"postgresql", "5432"},
		{"redis", "6379"},
		{"nginx", "80"},
	}

	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS\tPORT")
	fmt.Fprintln(w, "-------\t------\t----")

	for _, svc := range services {
		status := getServiceStatus(svc.name)
		fmt.Fprintf(w, "%s\t%s\t%s\n", svc.name, status, svc.port)
	}
	w.Flush()

	fmt.Println()
	fmt.Println("=== Disk Usage ===")
	runCommand("df", "-h", "/")
}

func runServerInfo(cmd *cobra.Command, args []string) {
	fmt.Println("=== System Information ===")
	fmt.Println()

	// CPU info
	if cpuInfo, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		lines := strings.Split(string(cpuInfo), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "model name") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					fmt.Printf("CPU: %s\n", strings.TrimSpace(parts[1]))
					break
				}
			}
		}
	}

	// Memory info
	if memInfo, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(memInfo), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal") || strings.HasPrefix(line, "MemFree") || strings.HasPrefix(line, "MemAvailable") {
				fmt.Println(line)
			}
		}
	}

	fmt.Println()
	fmt.Printf("Hostname: %s\n", getHostname())
	fmt.Printf("Kernel: %s\n", getKernelVersion())
	fmt.Printf("Uptime: %s\n", getUptime())
}

func runServerServices(cmd *cobra.Command, args []string) {
	services := []string{
		"vkai-api",
		"vkai-ui",
		"postgresql",
		"redis-server",
		"nginx",
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS\tPID\tMEMORY")
	fmt.Fprintln(w, "-------\t------\t---\t------")

	for _, svc := range services {
		status := getServiceStatus(svc)
		pid := getServicePID(svc)
		mem := getServiceMemory(svc)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", svc, status, pid, mem)
	}
	w.Flush()
}

func runServerPorts(cmd *cobra.Command, args []string) {
	fmt.Println("=== Listening Ports ===")
	runCommand("ss", "-tlnp")
}

// Helper functions

func getHostname() string {
	hostname, _ := os.Hostname()
	return hostname
}

func getUptime() string {
	if uptimeBytes, err := os.ReadFile("/proc/uptime"); err == nil {
		parts := strings.Fields(string(uptimeBytes))
		if len(parts) > 0 {
			var seconds float64
			fmt.Sscanf(parts[0], "%f", &seconds)
			duration := time.Duration(seconds * float64(time.Second))
			days := int(duration.Hours()) / 24
			hours := int(duration.Hours()) % 24
			minutes := int(duration.Minutes()) % 60
			return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
		}
	}
	return "unknown"
}

func getKernelVersion() string {
	if version, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(version))
		if len(parts) > 2 {
			return parts[2]
		}
	}
	return "unknown"
}

func getServiceStatus(service string) string {
	cmd := exec.Command("systemctl", "is-active", service)
	output, err := cmd.Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(output))
}

func getServicePID(service string) string {
	cmd := exec.Command("systemctl", "show", service, "--property=MainPID")
	output, err := cmd.Output()
	if err != nil {
		return "-"
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "=")
	if len(parts) > 1 && parts[1] != "0" {
		return parts[1]
	}
	return "-"
}

func getServiceMemory(service string) string {
	cmd := exec.Command("systemctl", "show", service, "--property=MemoryCurrent")
	output, err := cmd.Output()
	if err != nil {
		return "-"
	}
	parts := strings.Split(strings.TrimSpace(string(output)), "=")
	if len(parts) > 1 && parts[1] != "[not set]" {
		var bytes int64
		fmt.Sscanf(parts[1], "%d", &bytes)
		if bytes > 0 {
			return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
		}
	}
	return "-"
}

func runCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
