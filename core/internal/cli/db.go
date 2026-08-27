package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management",
	Long:  `Commands for database backup, restore, and maintenance operations.`,
}

var dbBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup databases",
	Run:   runDBBackup,
}

var dbRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore database from backup",
	Run:   runDBRestore,
}

var dbListCmd = &cobra.Command{
	Use:   "list",
	Short: "List databases",
	Run:   runDBList,
}

var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show database status",
	Run:   runDBStatus,
}

var dbOptimizeCmd = &cobra.Command{
	Use:   "optimize",
	Short: "Optimize databases",
	Run:   runDBOptimize,
}

var (
	dbBackupFile string
	dbName       string
)

func init() {
	dbCmd.AddCommand(dbBackupCmd)
	dbCmd.AddCommand(dbRestoreCmd)
	dbCmd.AddCommand(dbListCmd)
	dbCmd.AddCommand(dbStatusCmd)
	dbCmd.AddCommand(dbOptimizeCmd)

	dbRestoreCmd.Flags().StringVarP(&dbBackupFile, "file", "f", "", "Backup file path (required)")
	dbRestoreCmd.MarkFlagRequired("file")

	dbBackupCmd.Flags().StringVarP(&dbName, "name", "n", "", "Database name (optional, backups all if empty)")
}

func runDBBackup(cmd *cobra.Command, args []string) {
	backupDir := config.DatabaseBackupDir()
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		printError("Failed to create backup directory: %v", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	if dbName != "" {
		// Backup specific database
		backupFile := filepath.Join(backupDir, fmt.Sprintf("%s_%s.sql.gz", dbName, timestamp))
		backupDatabase(dbName, backupFile)
	} else {
		// Backup all databases
		backupFile := filepath.Join(backupDir, fmt.Sprintf("all_databases_%s.sql.gz", timestamp))
		backupAllDatabases(backupFile)
	}
}

func backupDatabase(name, filepath string) {
	printInfo("Backing up database: %s", name)

	cmd := exec.Command("pg_dump", "-U", "vkai_panel", "-d", name, "-F", "c", "-f", filepath)
	cmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		printError("Backup failed: %v", err)
	}

	printSuccess("Database backed up to: %s", filepath)
}

func backupAllDatabases(filepath string) {
	printInfo("Backing up all databases...")

	cmd := exec.Command("pg_dumpall", "-U", "vkai_panel", "-f", filepath)
	cmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		printError("Backup failed: %v", err)
	}

	printSuccess("All databases backed up to: %s", filepath)
}

func runDBRestore(cmd *cobra.Command, args []string) {
	if _, err := os.Stat(dbBackupFile); os.IsNotExist(err) {
		printError("Backup file not found: %s", dbBackupFile)
	}

	printInfo("Restoring database from: %s", dbBackupFile)

	restoreCmd := exec.Command("pg_restore", "-U", "vkai_panel", "-d", "vkai_panel", "--clean", "--if-exists", dbBackupFile)
	restoreCmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024")
	restoreCmd.Stdout = os.Stdout
	restoreCmd.Stderr = os.Stderr

	if err := restoreCmd.Run(); err != nil {
		printError("Restore failed: %v", err)
	}

	printSuccess("Database restored successfully")
	printInfo("Restart the API service: systemctl restart vkai-api")
}

func runDBList(cmd *cobra.Command, args []string) {
	printInfo("Listing databases...")

	psqlCmd := exec.Command("psql", "-U", "vkai_panel", "-d", "postgres", "-c",
		"SELECT datname, pg_size_pretty(pg_database_size(datname)) as size FROM pg_database WHERE datistemplate = false ORDER BY datname;")
	psqlCmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024", "TERM=dumb", "PAGER=cat")
	psqlCmd.Stdout = os.Stdout
	psqlCmd.Stderr = os.Stderr
	psqlCmd.Run()
}

func runDBStatus(cmd *cobra.Command, args []string) {
	printInfo("Database status:")

	// PostgreSQL status
	statusCmd := exec.Command("systemctl", "is-active", "postgresql")
	output, err := statusCmd.Output()
	if err != nil {
		fmt.Printf("PostgreSQL: inactive\n")
	} else {
		fmt.Printf("PostgreSQL: %s\n", string(output))
	}

	// Connection test
	connCmd := exec.Command("psql", "-U", "vkai_panel", "-d", "vkai_panel", "-c", "SELECT 1;")
	connCmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024", "TERM=dumb", "PAGER=cat")
	if err := connCmd.Run(); err != nil {
		fmt.Printf("Connection: failed\n")
	} else {
		fmt.Printf("Connection: ok\n")
	}

	// Database size
	sizeCmd := exec.Command("psql", "-U", "vkai_panel", "-d", "vkai_panel", "-c",
		"SELECT pg_size_pretty(pg_database_size('vkai_panel')) as size;")
	sizeCmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024", "TERM=dumb", "PAGER=cat")
	sizeCmd.Stdout = os.Stdout
	sizeCmd.Stderr = os.Stderr
	sizeCmd.Run()
}

func runDBOptimize(cmd *cobra.Command, args []string) {
	printInfo("Optimizing databases...")

	vacuumCmd := exec.Command("psql", "-U", "vkai_panel", "-d", "vkai_panel", "-c", "VACUUM ANALYZE;")
	vacuumCmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024", "TERM=dumb", "PAGER=cat")
	vacuumCmd.Stdout = os.Stdout
	vacuumCmd.Stderr = os.Stderr

	if err := vacuumCmd.Run(); err != nil {
		printError("Optimization failed: %v", err)
	}

	printSuccess("Database optimized")
}
