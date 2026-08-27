package cli

import (
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Website management",
	Long:  `Commands for managing websites including list, create, delete, and configuration.`,
}

var siteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all websites",
	Run:   runSiteList,
}

var siteShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show website details",
	Run:   runSiteShow,
}

var siteCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new website",
	Run:   runSiteCreate,
}

var siteDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a website",
	Run:   runSiteDelete,
}

var siteEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable a website",
	Run:   runSiteEnable,
}

var siteDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable a website",
	Run:   runSiteDisable,
}

var (
	siteDomain   string
	siteRoot     string
	sitePHP      string
	siteID       string
)

func init() {
	siteCmd.AddCommand(siteListCmd)
	siteCmd.AddCommand(siteShowCmd)
	siteCmd.AddCommand(siteCreateCmd)
	siteCmd.AddCommand(siteDeleteCmd)
	siteCmd.AddCommand(siteEnableCmd)
	siteCmd.AddCommand(siteDisableCmd)

	siteCreateCmd.Flags().StringVarP(&siteDomain, "domain", "d", "", "Domain name (required)")
	siteCreateCmd.Flags().StringVarP(&siteRoot, "root", "r", "", "Document root (optional)")
	siteCreateCmd.Flags().StringVarP(&sitePHP, "php", "p", "", "PHP version (optional)")
	siteCreateCmd.MarkFlagRequired("domain")

	siteShowCmd.Flags().StringVarP(&siteID, "id", "i", "", "Website ID (required)")
	siteShowCmd.MarkFlagRequired("id")

	siteDeleteCmd.Flags().StringVarP(&siteID, "id", "i", "", "Website ID (required)")
	siteDeleteCmd.MarkFlagRequired("id")

	siteEnableCmd.Flags().StringVarP(&siteID, "id", "i", "", "Website ID (required)")
	siteEnableCmd.MarkFlagRequired("id")

	siteDisableCmd.Flags().StringVarP(&siteID, "id", "i", "", "Website ID (required)")
	siteDisableCmd.MarkFlagRequired("id")
}

func runSiteList(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, domain, document_root, status, created_at 
		FROM websites 
		WHERE deleted_at IS NULL 
		ORDER BY domain
	`)
	if err != nil {
		printError("Failed to query websites: %v", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tDOMAIN\tROOT\tSTATUS\tCREATED")
	fmt.Fprintln(w, "--\t------\t----\t------\t-------")

	count := 0
	for rows.Next() {
		var id, domain, status string
		var root sql.NullString
		var createdAt sql.NullTime
		err := rows.Scan(&id, &domain, &root, &status, &createdAt)
		if err != nil {
			printError("Failed to scan row: %v", err)
		}
		rootStr := "-"
		if root.Valid {
			rootStr = root.String
		}
		created := "-"
		if createdAt.Valid {
			created = createdAt.Time.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id[:8], domain, rootStr, status, created)
		count++
	}

	w.Flush()
	fmt.Printf("\nTotal: %d websites\n", count)
}

func runSiteShow(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	var id, domain, status string
	var root, phpVersion sql.NullString
	var createdAt, updatedAt sql.NullTime

	err = db.QueryRow(`
		SELECT id, domain, document_root, php_version, status, created_at, updated_at
		FROM websites 
		WHERE id = $1 AND deleted_at IS NULL
	`, siteID).Scan(&id, &domain, &root, &phpVersion, &status, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		printError("Website not found: %s", siteID)
	}
	if err != nil {
		printError("Failed to query website: %v", err)
	}

	fmt.Printf("Website Details:\n")
	fmt.Printf("  ID:        %s\n", id)
	fmt.Printf("  Domain:    %s\n", domain)
	if root.Valid {
		fmt.Printf("  Root:      %s\n", root.String)
	}
	if phpVersion.Valid {
		fmt.Printf("  PHP:       %s\n", phpVersion.String)
	}
	fmt.Printf("  Status:    %s\n", status)
	if createdAt.Valid {
		fmt.Printf("  Created:   %s\n", createdAt.Time.Format("2006-01-02 15:04:05"))
	}
	if updatedAt.Valid {
		fmt.Printf("  Updated:   %s\n", updatedAt.Time.Format("2006-01-02 15:04:05"))
	}
}

func runSiteCreate(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	tenantID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	
	if siteRoot == "" {
		siteRoot = "/var/www/" + siteDomain
	}

	var websiteID string
	err = db.QueryRow(`
		INSERT INTO websites (tenant_id, domain, document_root, php_version, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id
	`, tenantID, siteDomain, siteRoot, sitePHP).Scan(&websiteID)

	if err != nil {
		printError("Failed to create website: %v", err)
	}

	printSuccess("Website created successfully")
	fmt.Printf("  ID:     %s\n", websiteID)
	fmt.Printf("  Domain: %s\n", siteDomain)
	fmt.Printf("  Root:   %s\n", siteRoot)
}

func runSiteDelete(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	result, err := db.Exec(`
		UPDATE websites SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL
	`, siteID)
	if err != nil {
		printError("Failed to delete website: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		printError("Website not found: %s", siteID)
	}

	printSuccess("Website deleted: %s", siteID)
}

func runSiteEnable(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	result, err := db.Exec(`
		UPDATE websites SET status = 'active', updated_at = NOW() 
		WHERE id = $1 AND deleted_at IS NULL
	`, siteID)
	if err != nil {
		printError("Failed to enable website: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		printError("Website not found: %s", siteID)
	}

	printSuccess("Website enabled: %s", siteID)
}

func runSiteDisable(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	result, err := db.Exec(`
		UPDATE websites SET status = 'suspended', updated_at = NOW() 
		WHERE id = $1 AND deleted_at IS NULL
	`, siteID)
	if err != nil {
		printError("Failed to disable website: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		printError("Website not found: %s", siteID)
	}

	printSuccess("Website disabled: %s", siteID)
}
