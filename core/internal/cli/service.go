package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Service management",
	Long:  `Commands for managing system services (start, stop, restart, status).`,
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a service",
	Run:   runServiceStart,
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a service",
	Run:   runServiceStop,
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart a service",
	Run:   runServiceRestart,
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show service status",
	Run:   runServiceStatusCmd,
}

var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all services",
	Run:   runServiceList,
}

var serviceLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show service logs",
	Run:   runServiceLogs,
}

var serviceName string

func init() {
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceRestartCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceListCmd)
	serviceCmd.AddCommand(serviceLogsCmd)

	serviceStartCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name (required)")
	serviceStartCmd.MarkFlagRequired("name")

	serviceStopCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name (required)")
	serviceStopCmd.MarkFlagRequired("name")

	serviceRestartCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name (required)")
	serviceRestartCmd.MarkFlagRequired("name")

	serviceStatusCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name (required)")
	serviceStatusCmd.MarkFlagRequired("name")

	serviceLogsCmd.Flags().StringVarP(&serviceName, "name", "n", "", "Service name (required)")
	serviceLogsCmd.MarkFlagRequired("name")
}

func runServiceStart(cmd *cobra.Command, args []string) {
	printInfo("Starting service: %s", serviceName)

	cmdExec := exec.Command("systemctl", "start", serviceName)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		printError("Failed to start service: %v", err)
	}

	printSuccess("Service started: %s", serviceName)
}

func runServiceStop(cmd *cobra.Command, args []string) {
	printInfo("Stopping service: %s", serviceName)

	cmdExec := exec.Command("systemctl", "stop", serviceName)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		printError("Failed to stop service: %v", err)
	}

	printSuccess("Service stopped: %s", serviceName)
}

func runServiceRestart(cmd *cobra.Command, args []string) {
	printInfo("Restarting service: %s", serviceName)

	cmdExec := exec.Command("systemctl", "restart", serviceName)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		printError("Failed to restart service: %v", err)
	}

	printSuccess("Service restarted: %s", serviceName)
}

func runServiceStatusCmd(cmd *cobra.Command, args []string) {
	cmdExec := exec.Command("systemctl", "status", serviceName, "--no-pager")
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr
	cmdExec.Run()
}

func runServiceList(cmd *cobra.Command, args []string) {
	services := []string{
		"vkai-api",
		"vkai-ui",
		"postgresql",
		"redis-server",
		"nginx",
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tSTATUS\tENABLED")
	fmt.Fprintln(w, "-------\t------\t-------")

	for _, svc := range services {
		status := getServiceStatus(svc)
		enabled := getServiceEnabled(svc)
		fmt.Fprintf(w, "%s\t%s\t%s\n", svc, status, enabled)
	}
	w.Flush()
}

func getServiceEnabled(service string) string {
	cmd := exec.Command("systemctl", "is-enabled", service)
	output, err := cmd.Output()
	if err != nil {
		return "disabled"
	}
	return strings.TrimSpace(string(output))
}

func runServiceLogs(cmd *cobra.Command, args []string) {
	printInfo("Showing logs for: %s", serviceName)

	cmdExec := exec.Command("journalctl", "-u", serviceName, "--no-pager", "-n", "50")
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr
	cmdExec.Run()
}
