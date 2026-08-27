package cli

import (
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var firewallCmd = &cobra.Command{
	Use:   "firewall",
	Short: "Firewall management",
	Long:  `Commands for managing firewall rules including iptables and UFW.`,
}

var firewallListCmd = &cobra.Command{
	Use:   "list",
	Short: "List firewall rules",
	Run:   runFirewallList,
}

var firewallAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add firewall rule",
	Run:   runFirewallAdd,
}

var firewallDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete firewall rule",
	Run:   runFirewallDelete,
}

var firewallEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable firewall",
	Run:   runFirewallEnable,
}

var firewallDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable firewall",
	Run:   runFirewallDisable,
}

var firewallStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show firewall status",
	Run:   runFirewallStatus,
}

var (
	firewallRuleID   string
	firewallPort     string
	firewallProtocol string
	firewallSource   string
	firewallAction   string
	firewallComment  string
)

func init() {
	firewallCmd.AddCommand(firewallListCmd)
	firewallCmd.AddCommand(firewallAddCmd)
	firewallCmd.AddCommand(firewallDeleteCmd)
	firewallCmd.AddCommand(firewallEnableCmd)
	firewallCmd.AddCommand(firewallDisableCmd)
	firewallCmd.AddCommand(firewallStatusCmd)

	firewallAddCmd.Flags().StringVarP(&firewallPort, "port", "p", "", "Port number (required)")
	firewallAddCmd.Flags().StringVarP(&firewallProtocol, "protocol", "P", "tcp", "Protocol (tcp/udp)")
	firewallAddCmd.Flags().StringVarP(&firewallSource, "source", "s", "", "Source IP (optional)")
	firewallAddCmd.Flags().StringVarP(&firewallAction, "action", "a", "allow", "Action (allow/deny)")
	firewallAddCmd.Flags().StringVarP(&firewallComment, "comment", "c", "", "Comment (optional)")
	firewallAddCmd.MarkFlagRequired("port")

	firewallDeleteCmd.Flags().StringVarP(&firewallRuleID, "id", "i", "", "Rule ID (required)")
	firewallDeleteCmd.MarkFlagRequired("id")
}

func runFirewallList(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, port, protocol, source_ip, action, comment, enabled, created_at 
		FROM firewall_rules 
		WHERE deleted_at IS NULL 
		ORDER BY port
	`)
	if err != nil {
		printError("Failed to query firewall rules: %v", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPORT\tPROTO\tSOURCE\tACTION\tENABLED\tCOMMENT")
	fmt.Fprintln(w, "--\t----\t-----\t------\t------\t-------\t-------")

	count := 0
	for rows.Next() {
		var id, port, protocol, action string
		var source, comment sql.NullString
		var enabled bool
		var createdAt sql.NullTime
		err := rows.Scan(&id, &port, &protocol, &source, &action, &comment, &enabled, &createdAt)
		if err != nil {
			printError("Failed to scan row: %v", err)
		}
		sourceStr := "*"
		if source.Valid {
			sourceStr = source.String
		}
		commentStr := "-"
		if comment.Valid {
			commentStr = comment.String
		}
		enabledStr := "no"
		if enabled {
			enabledStr = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", id[:8], port, protocol, sourceStr, action, enabledStr, commentStr)
		count++
	}

	w.Flush()
	fmt.Printf("\nTotal: %d rules\n", count)
}

func runFirewallAdd(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	tenantID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

	var ruleID string
	err = db.QueryRow(`
		INSERT INTO firewall_rules (tenant_id, port, protocol, source_ip, action, comment, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		RETURNING id
	`, tenantID, firewallPort, firewallProtocol, firewallSource, firewallAction, firewallComment).Scan(&ruleID)

	if err != nil {
		printError("Failed to add firewall rule: %v", err)
	}

	printSuccess("Firewall rule added successfully")
	fmt.Printf("  ID:       %s\n", ruleID)
	fmt.Printf("  Port:     %s\n", firewallPort)
	fmt.Printf("  Protocol: %s\n", firewallProtocol)
	fmt.Printf("  Action:   %s\n", firewallAction)
}

func runFirewallDelete(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	result, err := db.Exec(`
		UPDATE firewall_rules SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL
	`, firewallRuleID)
	if err != nil {
		printError("Failed to delete firewall rule: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		printError("Firewall rule not found: %s", firewallRuleID)
	}

	printSuccess("Firewall rule deleted: %s", firewallRuleID)
}

func runFirewallEnable(cmd *cobra.Command, args []string) {
	printInfo("Enabling firewall...")

	// Enable UFW
	runCommand("ufw", "--force", "enable")
	printSuccess("Firewall enabled")
}

func runFirewallDisable(cmd *cobra.Command, args []string) {
	printInfo("Disabling firewall...")

	// Disable UFW
	runCommand("ufw", "disable")
	printSuccess("Firewall disabled")
}

func runFirewallStatus(cmd *cobra.Command, args []string) {
	printInfo("Firewall status:")
	runCommand("ufw", "status", "verbose")
}
