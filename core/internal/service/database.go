package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/quota"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/utils"
)

// allowedCharsets / allowedCollations keep MySQL DDL free of caller-controlled
// text in positions that cannot be quoted.
var allowedCharsets = map[string]bool{
	"utf8mb4": true, "utf8": true, "latin1": true, "ascii": true, "utf8mb3": true,
}

var allowedCollations = map[string]bool{
	"utf8mb4_unicode_ci": true, "utf8mb4_general_ci": true, "utf8mb4_bin": true,
	"utf8mb4_0900_ai_ci": true, "utf8_general_ci": true, "utf8_unicode_ci": true,
	"latin1_swedish_ci": true, "ascii_general_ci": true,
}

// validateDBRequest gates every field that ends up inside an administrative SQL
// statement. Names are restricted to an identifier charset and the password is
// only ever used as a quoted literal, never as raw SQL.
func validateDBRequest(name, username, password, charset, collation string) error {
	if err := utils.ValidateSQLIdentifier(name, "database name"); err != nil {
		return err
	}
	if err := utils.ValidateSQLIdentifier(username, "username"); err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}
	if _, err := utils.QuoteSQLLiteral(password); err != nil {
		return fmt.Errorf("password contains unsupported characters")
	}
	if charset != "" && !allowedCharsets[charset] {
		return fmt.Errorf("charset %q is not allowed", charset)
	}
	if collation != "" && !allowedCollations[collation] {
		return fmt.Errorf("collation %q is not allowed", collation)
	}
	return nil
}

// runSQLStdin feeds a statement to a client over stdin so secrets never appear
// in /proc/<pid>/cmdline or in the output of ps.
func runSQLStdin(ctx context.Context, sql string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(sql)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %s: %w", name, out.String(), err)
	}
	return nil
}

type DatabaseService struct {
	dbRepo     *repository.DatabaseRepository
	serverRepo *repository.ServerRepository
	quota      *quota.Enforcer
}

// NewDatabaseService takes the quota enforcer as a REQUIRED argument, so that
// omitting quota enforcement is a compile error rather than a silent hole. See
// NewWebsiteService for the reasoning.
func NewDatabaseService(
	dbRepo *repository.DatabaseRepository,
	serverRepo *repository.ServerRepository,
	quotaEnforcer *quota.Enforcer,
) *DatabaseService {
	return &DatabaseService{
		dbRepo:     dbRepo,
		serverRepo: serverRepo,
		quota:      quotaEnforcer,
	}
}

func (s *DatabaseService) CreateServer(ctx context.Context, req *models.CreateDBServerRequest, tenantID uuid.UUID) (*models.DatabaseServer, error) {
	server := &models.DatabaseServer{
		TenantID: tenantID,
		ServerID: req.ServerID,
		Type:     req.Type,
		Version:  req.Version,
		Port:     req.Port,
		Status:   "active",
	}

	if err := s.dbRepo.CreateServer(ctx, server); err != nil {
		return nil, fmt.Errorf("failed to create database server: %w", err)
	}

	return server, nil
}

func (s *DatabaseService) GetServerByID(ctx context.Context, tenantID, id uuid.UUID) (*models.DatabaseServer, error) {
	return s.dbRepo.GetServerByID(ctx, tenantID, id)
}

func (s *DatabaseService) ListServersByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.DatabaseServer, error) {
	return s.dbRepo.ListServersByTenant(ctx, tenantID)
}

func (s *DatabaseService) DeleteServer(ctx context.Context, tenantID, id uuid.UUID) error {
	return s.dbRepo.DeleteServer(ctx, tenantID, id)
}

// CreateDatabase creates a new database on the specified database server
func (s *DatabaseService) CreateDatabase(ctx context.Context, req *models.CreateDBEntryRequest, tenantID uuid.UUID) (*models.DatabaseEntry, error) {
	// ENFORCEMENT POINT: the hosting package's database count.
	//
	// Before the MySQL or PostgreSQL server is touched. A refusal after CREATE
	// DATABASE would leave a real database on the host with no row describing
	// it, which is worse than the refusal itself.
	if err := s.quota.Check(ctx, tenantID, quota.ResourceDatabases); err != nil {
		return nil, err
	}

	dbServer, err := s.dbRepo.GetServerByID(ctx, tenantID, req.DatabaseServerID)
	if err != nil {
		return nil, fmt.Errorf("database server not found: %w", err)
	}

	if err := validateDBRequest(req.Name, req.Username, req.Password, req.Charset, req.Collation); err != nil {
		return nil, err
	}

	// Create the actual database based on type
	switch dbServer.Type {
	case "mysql":
		if err := s.createMySQLDatabase(ctx, dbServer, req); err != nil {
			return nil, err
		}
	case "postgresql":
		if err := s.createPostgresDatabase(ctx, dbServer, req); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbServer.Type)
	}

	storedPassword, err := s.protectPassword(req.Password)
	if err != nil {
		return nil, err
	}

	entry := &models.DatabaseEntry{
		TenantID:         tenantID,
		DatabaseServerID: req.DatabaseServerID,
		Name:             req.Name,
		Username:         req.Username,
		Password:         storedPassword,
		Charset:          req.Charset,
		Collation:        req.Collation,
		Status:           "active",
	}

	if entry.Charset == "" {
		entry.Charset = "utf8mb4"
	}
	if entry.Collation == "" {
		entry.Collation = "utf8mb4_unicode_ci"
	}

	if err := s.dbRepo.CreateEntry(ctx, entry); err != nil {
		return nil, fmt.Errorf("failed to save database entry: %w", err)
	}

	return entry, nil
}

func (s *DatabaseService) createMySQLDatabase(ctx context.Context, server *models.DatabaseServer, req *models.CreateDBEntryRequest) error {
	if err := validateDBRequest(req.Name, req.Username, req.Password, req.Charset, req.Collation); err != nil {
		return err
	}

	charset := req.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	collation := req.Collation
	if collation == "" {
		collation = "utf8mb4_unicode_ci"
	}

	dbIdent := utils.QuoteMySQLIdentifier(req.Name)
	userLit, err := utils.QuoteSQLLiteral(req.Username)
	if err != nil {
		return err
	}
	passLit, err := utils.QuoteSQLLiteral(req.Password)
	if err != nil {
		return err
	}

	// Create database. Charset and collation come from a fixed allowlist.
	createSQL := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET %s COLLATE %s;",
		dbIdent, charset, collation)
	if err := runSQLStdin(ctx, createSQL, "mysql", "-u", "root"); err != nil {
		return fmt.Errorf("failed to create MySQL database: %w", err)
	}

	// Create user and grant privileges. The password travels on stdin, so it is
	// never visible in the process listing.
	grantSQL := fmt.Sprintf(
		"CREATE USER IF NOT EXISTS %s@'localhost' IDENTIFIED BY %s; GRANT ALL PRIVILEGES ON %s.* TO %s@'localhost'; FLUSH PRIVILEGES;",
		userLit, passLit, dbIdent, userLit)
	if err := runSQLStdin(ctx, grantSQL, "mysql", "-u", "root"); err != nil {
		return fmt.Errorf("failed to create MySQL user: %w", err)
	}

	return nil
}

func (s *DatabaseService) createPostgresDatabase(ctx context.Context, server *models.DatabaseServer, req *models.CreateDBEntryRequest) error {
	if err := validateDBRequest(req.Name, req.Username, req.Password, req.Charset, req.Collation); err != nil {
		return err
	}

	dbIdent := utils.QuoteSQLIdentifier(req.Name)
	userIdent := utils.QuoteSQLIdentifier(req.Username)
	passLit, err := utils.QuoteSQLLiteral(req.Password)
	if err != nil {
		return err
	}

	// Create database. "--" stops createdb from reading the name as an option.
	cmd := exec.CommandContext(ctx, "sudo", "-u", "postgres", "createdb", "--", req.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL database: %s: %w", string(output), err)
	}

	// Create user. The statement goes over stdin, and psql runs with
	// ON_ERROR_STOP plus single-transaction so a partial statement cannot be
	// chained into extra commands.
	createUser := fmt.Sprintf("CREATE USER %s WITH PASSWORD %s;", userIdent, passLit)
	if err := runSQLStdin(ctx, createUser, "sudo", "-u", "postgres", "psql",
		"-v", "ON_ERROR_STOP=1", "--no-psqlrc", "-q"); err != nil {
		return fmt.Errorf("failed to create PostgreSQL user: %w", err)
	}

	// Grant privileges
	grant := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s;", dbIdent, userIdent)
	if err := runSQLStdin(ctx, grant, "sudo", "-u", "postgres", "psql",
		"-v", "ON_ERROR_STOP=1", "--no-psqlrc", "-q"); err != nil {
		return fmt.Errorf("failed to grant privileges: %w", err)
	}

	return nil
}

func (s *DatabaseService) ListEntriesByServer(ctx context.Context, tenantID, serverID uuid.UUID) ([]models.DatabaseEntry, error) {
	return s.dbRepo.ListEntriesByServer(ctx, tenantID, serverID)
}

func (s *DatabaseService) ListEntriesByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.DatabaseEntry, error) {
	return s.dbRepo.ListEntriesByTenant(ctx, tenantID)
}

func (s *DatabaseService) DeleteEntry(ctx context.Context, tenantID, id uuid.UUID) error {
	entry, err := s.dbRepo.GetEntryByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	dbServer, err := s.dbRepo.GetServerByID(ctx, tenantID, entry.DatabaseServerID)
	if err != nil {
		return err
	}

	// Names stored before this validation existed are re-checked here so a
	// legacy row cannot smuggle SQL into the DROP statements.
	if err := utils.ValidateSQLIdentifier(entry.Name, "database name"); err != nil {
		return err
	}
	if err := utils.ValidateSQLIdentifier(entry.Username, "username"); err != nil {
		return err
	}

	// Drop the actual database
	switch dbServer.Type {
	case "mysql":
		userLit, err := utils.QuoteSQLLiteral(entry.Username)
		if err != nil {
			return err
		}
		dropSQL := fmt.Sprintf("DROP DATABASE IF EXISTS %s; DROP USER IF EXISTS %s@'localhost';",
			utils.QuoteMySQLIdentifier(entry.Name), userLit)
		_ = runSQLStdin(ctx, dropSQL, "mysql", "-u", "root")
	case "postgresql":
		cmd := exec.CommandContext(ctx, "sudo", "-u", "postgres", "dropdb", "--", entry.Name)
		_ = cmd.Run()
		dropUser := fmt.Sprintf("DROP USER IF EXISTS %s;", utils.QuoteSQLIdentifier(entry.Username))
		_ = runSQLStdin(ctx, dropUser, "sudo", "-u", "postgres", "psql",
			"-v", "ON_ERROR_STOP=1", "--no-psqlrc", "-q")
	}

	return s.dbRepo.DeleteEntry(ctx, tenantID, id)
}

func (s *DatabaseService) ChangePassword(ctx context.Context, tenantID, id uuid.UUID, newPassword string) error {
	entry, err := s.dbRepo.GetEntryByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	dbServer, err := s.dbRepo.GetServerByID(ctx, tenantID, entry.DatabaseServerID)
	if err != nil {
		return err
	}

	if err := utils.ValidateSQLIdentifier(entry.Username, "username"); err != nil {
		return err
	}
	if newPassword == "" {
		return fmt.Errorf("password is required")
	}
	passLit, err := utils.QuoteSQLLiteral(newPassword)
	if err != nil {
		return fmt.Errorf("password contains unsupported characters")
	}

	switch dbServer.Type {
	case "mysql":
		userLit, err := utils.QuoteSQLLiteral(entry.Username)
		if err != nil {
			return err
		}
		alter := fmt.Sprintf("ALTER USER %s@'localhost' IDENTIFIED BY %s;", userLit, passLit)
		if err := runSQLStdin(ctx, alter, "mysql", "-u", "root"); err != nil {
			return fmt.Errorf("failed to change MySQL password: %w", err)
		}
	case "postgresql":
		alter := fmt.Sprintf("ALTER USER %s WITH PASSWORD %s;", utils.QuoteSQLIdentifier(entry.Username), passLit)
		if err := runSQLStdin(ctx, alter, "sudo", "-u", "postgres", "psql",
			"-v", "ON_ERROR_STOP=1", "--no-psqlrc", "-q"); err != nil {
			return fmt.Errorf("failed to change PostgreSQL password: %w", err)
		}
	}

	// The stored copy is encrypted at rest; see storeEntryPassword.
	stored, err := s.protectPassword(newPassword)
	if err != nil {
		return err
	}
	return s.dbRepo.UpdateEntryPassword(ctx, tenantID, id, stored)
}

// protectPassword encrypts a database password before it is written to the
// panel database. When no encryption key is configured the call fails closed
// rather than silently storing plaintext.
func (s *DatabaseService) protectPassword(password string) (string, error) {
	key, err := utils.DatabaseEncryptionKey()
	if err != nil {
		return "", err
	}
	return utils.EncryptSecret(password, key)
}
