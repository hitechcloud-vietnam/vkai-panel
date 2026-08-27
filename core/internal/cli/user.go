package cli

import (
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
	Long:  `Commands for managing panel users including create, list, update, and delete operations.`,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	Run:   runUserList,
}

var userCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new user",
	Run:   runUserCreate,
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a user",
	Run:   runUserDelete,
}

var userResetPasswordCmd = &cobra.Command{
	Use:   "reset-password",
	Short: "Reset user password",
	Run:   runUserResetPassword,
}

var userShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show user details",
	Run:   runUserShow,
}

var (
	userUsername  string
	userEmail     string
	userPassword  string
	userFirstName string
	userLastName  string
	userID        string
)

func init() {
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userDeleteCmd)
	userCmd.AddCommand(userResetPasswordCmd)
	userCmd.AddCommand(userShowCmd)

	userCreateCmd.Flags().StringVarP(&userUsername, "username", "u", "", "Username (required)")
	userCreateCmd.Flags().StringVarP(&userEmail, "email", "e", "", "Email (required)")
	userCreateCmd.Flags().StringVarP(&userPassword, "password", "p", "", "Password (required)")
	userCreateCmd.Flags().StringVarP(&userFirstName, "first-name", "f", "", "First name")
	userCreateCmd.Flags().StringVarP(&userLastName, "last-name", "l", "", "Last name")
	userCreateCmd.MarkFlagRequired("username")
	userCreateCmd.MarkFlagRequired("email")
	userCreateCmd.MarkFlagRequired("password")

	userDeleteCmd.Flags().StringVarP(&userID, "id", "i", "", "User ID (required)")
	userDeleteCmd.MarkFlagRequired("id")

	userResetPasswordCmd.Flags().StringVarP(&userID, "id", "i", "", "User ID (required)")
	userResetPasswordCmd.Flags().StringVarP(&userPassword, "password", "p", "", "New password (required)")
	userResetPasswordCmd.MarkFlagRequired("id")
	userResetPasswordCmd.MarkFlagRequired("password")

	userShowCmd.Flags().StringVarP(&userID, "id", "i", "", "User ID (required)")
	userShowCmd.MarkFlagRequired("id")
}

func getDB() (*sql.DB, error) {
	dsn := os.Getenv("VKAI_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://vkai_panel:vkai_panel_2024@localhost:5432/vkai_panel?sslmode=disable"
	}
	return sql.Open("pgx", dsn)
}

func runUserList(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, username, email, first_name, last_name, status, created_at 
		FROM users 
		WHERE deleted_at IS NULL 
		ORDER BY created_at DESC
	`)
	if err != nil {
		printError("Failed to query users: %v", err)
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tEMAIL\tNAME\tSTATUS\tCREATED")
	fmt.Fprintln(w, "---\t--------\t-----\t----\t------\t-------")

	count := 0
	for rows.Next() {
		var id, username, email, firstName, lastName, status string
		var createdAt sql.NullTime
		err := rows.Scan(&id, &username, &email, &firstName, &lastName, &status, &createdAt)
		if err != nil {
			printError("Failed to scan row: %v", err)
		}
		name := firstName + " " + lastName
		if name == " " {
			name = "-"
		}
		created := "-"
		if createdAt.Valid {
			created = createdAt.Time.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", id[:8], username, email, name, status, created)
		count++
	}

	w.Flush()
	fmt.Printf("\nTotal: %d users\n", count)
}

func runUserCreate(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(userPassword), bcrypt.DefaultCost)
	if err != nil {
		printError("Failed to hash password: %v", err)
	}

	tenantID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	var userID string
	err = db.QueryRow(`
		INSERT INTO users (tenant_id, username, email, password_hash, first_name, last_name, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'active')
		RETURNING id
	`, tenantID, userUsername, userEmail, string(hash), userFirstName, userLastName).Scan(&userID)

	if err != nil {
		printError("Failed to create user: %v", err)
	}

	printSuccess("User created successfully")
	fmt.Printf("  ID:       %s\n", userID)
	fmt.Printf("  Username: %s\n", userUsername)
	fmt.Printf("  Email:    %s\n", userEmail)
}

func runUserDelete(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	result, err := db.Exec(`
		UPDATE users SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL
	`, userID)
	if err != nil {
		printError("Failed to delete user: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		printError("User not found: %s", userID)
	}

	printSuccess("User deleted: %s", userID)
}

func runUserResetPassword(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(userPassword), bcrypt.DefaultCost)
	if err != nil {
		printError("Failed to hash password: %v", err)
	}

	result, err := db.Exec(`
		UPDATE users SET password_hash = $1, updated_at = NOW() 
		WHERE id = $2 AND deleted_at IS NULL
	`, string(hash), userID)
	if err != nil {
		printError("Failed to reset password: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		printError("User not found: %s", userID)
	}

	printSuccess("Password reset for user: %s", userID)
}

func runUserShow(cmd *cobra.Command, args []string) {
	db, err := getDB()
	if err != nil {
		printError("Failed to connect to database: %v", err)
	}
	defer db.Close()

	var id, username, email, firstName, lastName, status string
	var lastLoginIP sql.NullString
	var createdAt, updatedAt sql.NullTime

	err = db.QueryRow(`
		SELECT id, username, email, first_name, last_name, status, 
		       last_login_ip, created_at, updated_at
		FROM users 
		WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&id, &username, &email, &firstName, &lastName, &status,
		&lastLoginIP, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		printError("User not found: %s", userID)
	}
	if err != nil {
		printError("Failed to query user: %v", err)
	}

	fmt.Printf("User Details:\n")
	fmt.Printf("  ID:        %s\n", id)
	fmt.Printf("  Username:  %s\n", username)
	fmt.Printf("  Email:     %s\n", email)
	fmt.Printf("  Name:      %s %s\n", firstName, lastName)
	fmt.Printf("  Status:    %s\n", status)
	if lastLoginIP.Valid {
		fmt.Printf("  Last IP:   %s\n", lastLoginIP.String)
	}
	if createdAt.Valid {
		fmt.Printf("  Created:   %s\n", createdAt.Time.Format("2006-01-02 15:04:05"))
	}
	if updatedAt.Valid {
		fmt.Printf("  Updated:   %s\n", updatedAt.Time.Format("2006-01-02 15:04:05"))
	}
}
