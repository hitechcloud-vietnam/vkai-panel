package service

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/google/uuid"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

type DatabaseService struct {
	dbRepo     *repository.DatabaseRepository
	serverRepo *repository.ServerRepository
}

func NewDatabaseService(dbRepo *repository.DatabaseRepository, serverRepo *repository.ServerRepository) *DatabaseService {
	return &DatabaseService{
		dbRepo:     dbRepo,
		serverRepo: serverRepo,
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

func (s *DatabaseService) GetServerByID(ctx context.Context, id uuid.UUID) (*models.DatabaseServer, error) {
	return s.dbRepo.GetServerByID(ctx, id)
}

func (s *DatabaseService) ListServersByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.DatabaseServer, error) {
	return s.dbRepo.ListServersByTenant(ctx, tenantID)
}

func (s *DatabaseService) DeleteServer(ctx context.Context, id uuid.UUID) error {
	return s.dbRepo.DeleteServer(ctx, id)
}

// CreateDatabase creates a new database on the specified database server
func (s *DatabaseService) CreateDatabase(ctx context.Context, req *models.CreateDBEntryRequest, tenantID uuid.UUID) (*models.DatabaseEntry, error) {
	dbServer, err := s.dbRepo.GetServerByID(ctx, req.DatabaseServerID)
	if err != nil {
		return nil, fmt.Errorf("database server not found: %w", err)
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

	entry := &models.DatabaseEntry{
		TenantID:        tenantID,
		DatabaseServerID: req.DatabaseServerID,
		Name:            req.Name,
		Username:         req.Username,
		Password:        req.Password,
		Charset:         req.Charset,
		Collation:       req.Collation,
		Status:          "active",
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
	// Create database
	cmd := exec.CommandContext(ctx, "mysql", "-u", "root", "-e",
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET %s COLLATE %s;",
			req.Name, req.Charset, req.Collation))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create MySQL database: %s: %w", string(output), err)
	}

	// Create user and grant privileges
	grantSQL := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'localhost' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'localhost'; FLUSH PRIVILEGES;",
		req.Username, req.Password, req.Name, req.Username)
	cmd = exec.CommandContext(ctx, "mysql", "-u", "root", "-e", grantSQL)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create MySQL user: %s: %w", string(output), err)
	}

	return nil
}

func (s *DatabaseService) createPostgresDatabase(ctx context.Context, server *models.DatabaseServer, req *models.CreateDBEntryRequest) error {
	// Create database
	cmd := exec.CommandContext(ctx, "sudo", "-u", "postgres", "createdb", req.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL database: %s: %w", string(output), err)
	}

	// Create user
	cmd = exec.CommandContext(ctx, "sudo", "-u", "postgres", "psql", "-c",
		fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s';", req.Username, req.Password))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL user: %s: %w", string(output), err)
	}

	// Grant privileges
	cmd = exec.CommandContext(ctx, "sudo", "-u", "postgres", "psql", "-c",
		fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s;", req.Name, req.Username))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to grant privileges: %s: %w", string(output), err)
	}

	return nil
}

func (s *DatabaseService) ListEntriesByServer(ctx context.Context, serverID uuid.UUID) ([]models.DatabaseEntry, error) {
	return s.dbRepo.ListEntriesByServer(ctx, serverID)
}

func (s *DatabaseService) ListEntriesByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.DatabaseEntry, error) {
	return s.dbRepo.ListEntriesByTenant(ctx, tenantID)
}

func (s *DatabaseService) DeleteEntry(ctx context.Context, id uuid.UUID) error {
	entry, err := s.dbRepo.GetEntryByID(ctx, id)
	if err != nil {
		return err
	}

	dbServer, err := s.dbRepo.GetServerByID(ctx, entry.DatabaseServerID)
	if err != nil {
		return err
	}

	// Drop the actual database
	switch dbServer.Type {
	case "mysql":
		cmd := exec.Command("mysql", "-u", "root", "-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", entry.Name))
		cmd.Run()
		cmd = exec.Command("mysql", "-u", "root", "-e", fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost';", entry.Username))
		cmd.Run()
	case "postgresql":
		cmd := exec.Command("sudo", "-u", "postgres", "dropdb", entry.Name)
		cmd.Run()
		cmd = exec.Command("sudo", "-u", "postgres", "psql", "-c", fmt.Sprintf("DROP USER IF EXISTS %s;", entry.Username))
		cmd.Run()
	}

	return s.dbRepo.DeleteEntry(ctx, id)
}

func (s *DatabaseService) ChangePassword(ctx context.Context, id uuid.UUID, newPassword string) error {
	entry, err := s.dbRepo.GetEntryByID(ctx, id)
	if err != nil {
		return err
	}

	dbServer, err := s.dbRepo.GetServerByID(ctx, entry.DatabaseServerID)
	if err != nil {
		return err
	}

	switch dbServer.Type {
	case "mysql":
		cmd := exec.Command("mysql", "-u", "root", "-e",
			fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';", entry.Username, newPassword))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to change MySQL password: %s: %w", string(output), err)
		}
	case "postgresql":
		cmd := exec.Command("sudo", "-u", "postgres", "psql", "-c",
			fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s';", entry.Username, newPassword))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to change PostgreSQL password: %w", err)
			_ = output
		}
	}

	return s.dbRepo.UpdateEntryPassword(ctx, id, newPassword)
}
