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

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup management",
	Long:  `Commands for creating, listing, and restoring backups.`,
}

var backupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a backup",
	Run:   runBackupCreate,
}

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List backups",
	Run:   runBackupList,
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore from backup",
	Run:   runBackupRestore,
}

var backupDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a backup",
	Run:   runBackupDelete,
}

var backupCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean old backups",
	Run:   runBackupCleanup,
}

var (
	backupFile     string
	backupType     string
	backupKeepDays int
)

func init() {
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	backupCmd.AddCommand(backupDeleteCmd)
	backupCmd.AddCommand(backupCleanupCmd)

	backupCreateCmd.Flags().StringVarP(&backupType, "type", "t", "full", "Backup type (full/database/config)")
	backupCreateCmd.Flags().StringVarP(&backupFile, "name", "n", "", "Backup name (optional)")

	backupRestoreCmd.Flags().StringVarP(&backupFile, "file", "f", "", "Backup file path (required)")
	backupRestoreCmd.MarkFlagRequired("file")

	backupDeleteCmd.Flags().StringVarP(&backupFile, "file", "f", "", "Backup file path (required)")
	backupDeleteCmd.MarkFlagRequired("file")

	backupCleanupCmd.Flags().IntVarP(&backupKeepDays, "days", "d", 30, "Days to keep backups")
}

func runBackupCreate(cmd *cobra.Command, args []string) {
	backupDir := config.BackupRoot()
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		printError("Failed to create backup directory: %v", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	if backupFile == "" {
		backupFile = fmt.Sprintf("backup_%s_%s.tar.gz", backupType, timestamp)
	}

	backupPath := filepath.Join(backupDir, backupFile)

	switch backupType {
	case "full":
		createFullBackup(backupPath)
	case "database":
		createDatabaseBackup(backupPath)
	case "config":
		createConfigBackup(backupPath)
	default:
		printError("Unknown backup type: %s", backupType)
	}
}

func createFullBackup(backupPath string) {
	printInfo("Creating full backup...")

	// Create temp directory for staging
	tempDir := filepath.Join(config.TmpRoot(), "backup-"+time.Now().Format("20060102150405"))
	os.MkdirAll(tempDir, 0o750)
	defer os.RemoveAll(tempDir)

	// Backup database
	printInfo("Backing up database...")
	dbFile := filepath.Join(tempDir, "database.sql")
	cmd := exec.Command("pg_dumpall", "-U", "vkai_panel", "-f", dbFile)
	cmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024")
	if err := cmd.Run(); err != nil {
		printError("Database backup failed: %v", err)
	}

	// Backup config
	printInfo("Backing up configuration...")
	configDir := filepath.Join(tempDir, "config")
	os.MkdirAll(configDir, 0755)
	copyFile(config.ConfigFile(), filepath.Join(configDir, "config.yaml"))
	copyFile(config.EnvFile(), filepath.Join(configDir, ".env"))

	// Backup nginx config
	printInfo("Backing up nginx configuration...")
	nginxDir := filepath.Join(tempDir, "nginx")
	os.MkdirAll(nginxDir, 0755)
	runCommand("cp", "-r", "/etc/nginx/sites-available", nginxDir)

	// Create tar.gz archive
	printInfo("Creating archive...")
	cmd = exec.Command("tar", "-czf", backupPath, "-C", tempDir, ".")
	if err := cmd.Run(); err != nil {
		printError("Failed to create archive: %v", err)
	}

	printSuccess("Full backup created: %s", backupPath)
}

func createDatabaseBackup(backupPath string) {
	printInfo("Creating database backup...")

	cmd := exec.Command("pg_dumpall", "-U", "vkai_panel", "-f", backupPath)
	cmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024")
	if err := cmd.Run(); err != nil {
		printError("Database backup failed: %v", err)
	}

	printSuccess("Database backup created: %s", backupPath)
}

func createConfigBackup(backupPath string) {
	printInfo("Creating config backup...")

	// Create temp directory
	tempDir := filepath.Join(config.TmpRoot(), "config-backup-"+time.Now().Format("20060102150405"))
	os.MkdirAll(tempDir, 0o750)
	defer os.RemoveAll(tempDir)

	// Copy config files
	copyFile(config.ConfigFile(), filepath.Join(tempDir, "config.yaml"))
	copyFile(config.EnvFile(), filepath.Join(tempDir, ".env"))

	// Copy nginx config
	nginxDir := filepath.Join(tempDir, "nginx")
	os.MkdirAll(nginxDir, 0755)
	runCommand("cp", "-r", "/etc/nginx/sites-available", nginxDir)

	// Create tar.gz archive
	cmd := exec.Command("tar", "-czf", backupPath, "-C", tempDir, ".")
	if err := cmd.Run(); err != nil {
		printError("Failed to create archive: %v", err)
	}

	printSuccess("Config backup created: %s", backupPath)
}

func copyFile(src, dst string) {
	cmd := exec.Command("cp", src, dst)
	cmd.Run()
}

func runBackupList(cmd *cobra.Command, args []string) {
	backupDir := config.BackupRoot()
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		printInfo("No backups found")
		return
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		printError("Failed to read backup directory: %v", err)
	}

	fmt.Println("=== Backups ===")
	fmt.Println()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, _ := entry.Info()
		size := info.Size()
		modTime := info.ModTime()

		fmt.Printf("  %s\n", entry.Name())
		fmt.Printf("    Size: %s\n", formatSize(size))
		fmt.Printf("    Date: %s\n", modTime.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func runBackupRestore(cmd *cobra.Command, args []string) {
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		printError("Backup file not found: %s", backupFile)
	}

	printInfo("Restoring from backup: %s", backupFile)

	// Extract to temp directory
	tempDir := filepath.Join(config.TmpRoot(), "restore-"+time.Now().Format("20060102150405"))
	os.MkdirAll(tempDir, 0o750)
	defer os.RemoveAll(tempDir)

	extractCmd := exec.Command("tar", "-xzf", backupFile, "-C", tempDir)
	if err := extractCmd.Run(); err != nil {
		printError("Failed to extract backup: %v", err)
	}

	// Restore database if exists
	dbFile := filepath.Join(tempDir, "database.sql")
	if _, err := os.Stat(dbFile); err == nil {
		printInfo("Restoring database...")
		restoreCmd := exec.Command("psql", "-U", "vkai_panel", "-f", dbFile)
		restoreCmd.Env = append(os.Environ(), "PGPASSWORD=vkai_panel_2024")
		if err := restoreCmd.Run(); err != nil {
			printError("Database restore failed: %v", err)
		}
	}

	// Restore config if exists
	configFile := filepath.Join(tempDir, "config", "config.yaml")
	if _, err := os.Stat(configFile); err == nil {
		printInfo("Restoring configuration...")
		copyFile(configFile, config.ConfigFile())
	}

	printSuccess("Backup restored successfully")
	printInfo("Restart services: systemctl restart vkai-api vkai-ui")
}

func runBackupDelete(cmd *cobra.Command, args []string) {
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		printError("Backup file not found: %s", backupFile)
	}

	if err := os.Remove(backupFile); err != nil {
		printError("Failed to delete backup: %v", err)
	}

	printSuccess("Backup deleted: %s", backupFile)
}

func runBackupCleanup(cmd *cobra.Command, args []string) {
	backupDir := config.BackupRoot()
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		printInfo("No backups directory found")
		return
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		printError("Failed to read backup directory: %v", err)
	}

	cutoff := time.Now().AddDate(0, 0, -backupKeepDays)
	deleted := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, _ := entry.Info()
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(backupDir, entry.Name())
			if err := os.Remove(path); err != nil {
				printError("Failed to delete %s: %v", entry.Name(), err)
			} else {
				deleted++
			}
		}
	}

	printSuccess("Cleaned up %d old backups (older than %d days)", deleted, backupKeepDays)
}
