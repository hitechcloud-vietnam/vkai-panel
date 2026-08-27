package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "vkai",
	Short: "vKAI Panel CLI - Server Management Tool",
	Long: `vKAI Panel CLI is a command-line interface for managing your vKAI Panel server.
It provides commands for user management, server configuration, database operations,
service management, and more.

Examples:
  vkai user list                    List all users
  vkai user create --username admin Create a new user
  vkai server status                Show server status
  vkai service restart nginx        Restart nginx service
  vkai db backup                    Backup all databases
  vkai ssl request example.com      Request SSL certificate
  vkai site list                    List all websites
  vkai firewall list                List firewall rules
  vkai backup create                Create a backup`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	// Add all subcommands
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(dbCmd)
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(sslCmd)
	rootCmd.AddCommand(siteCmd)
	rootCmd.AddCommand(firewallCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("vKAI Panel CLI v0.3.0")
		fmt.Println("Build: 2026-08-28")
	},
}

func printError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+msg+"\n", args...)
	os.Exit(1)
}

func printSuccess(msg string, args ...interface{}) {
	fmt.Printf("✓ "+msg+"\n", args...)
}

func printInfo(msg string, args ...interface{}) {
	fmt.Printf("ℹ "+msg+"\n", args...)
}
