package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type DatabaseRepository struct {
	db *sqlx.DB
}

func NewDatabaseRepository(db *sqlx.DB) *DatabaseRepository {
	return &DatabaseRepository{db: db}
}

// Database Server operations
func (r *DatabaseRepository) CreateServer(ctx context.Context, s *models.DatabaseServer) error {
	query := `
		INSERT INTO database_servers (id, tenant_id, server_id, type, version, port, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING created_at, updated_at`

	s.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		s.ID, s.TenantID, s.ServerID, s.Type, s.Version, s.Port, s.Status,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
}

func (r *DatabaseRepository) GetServerByID(ctx context.Context, tenantID, id uuid.UUID) (*models.DatabaseServer, error) {
	var s models.DatabaseServer
	query := `SELECT * FROM database_servers WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &s, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("database server not found: %w", err)
	}
	return &s, nil
}

func (r *DatabaseRepository) ListServersByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.DatabaseServer, error) {
	var servers []models.DatabaseServer
	query := `SELECT * FROM database_servers WHERE tenant_id = $1 ORDER BY type, version`
	if err := r.db.SelectContext(ctx, &servers, query, tenantID); err != nil {
		return nil, err
	}
	return servers, nil
}

func (r *DatabaseRepository) UpdateServer(ctx context.Context, s *models.DatabaseServer) error {
	query := `UPDATE database_servers SET version = $2, port = $3, status = $4, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, s.ID, s.Version, s.Port, s.Status)
	return err
}

func (r *DatabaseRepository) DeleteServer(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `DELETE FROM database_servers WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

// Database Entry operations
func (r *DatabaseRepository) CreateEntry(ctx context.Context, e *models.DatabaseEntry) error {
	query := `
		INSERT INTO database_entries (id, tenant_id, database_server_id, name, username, password, charset, collation, size, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING created_at, updated_at`

	e.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		e.ID, e.TenantID, e.DatabaseServerID, e.Name, e.Username,
		e.Password, e.Charset, e.Collation, e.Size, e.Status,
	).Scan(&e.CreatedAt, &e.UpdatedAt)
}

func (r *DatabaseRepository) GetEntryByID(ctx context.Context, tenantID, id uuid.UUID) (*models.DatabaseEntry, error) {
	var e models.DatabaseEntry
	query := `SELECT * FROM database_entries WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &e, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("database entry not found: %w", err)
	}
	return &e, nil
}

func (r *DatabaseRepository) ListEntriesByServer(ctx context.Context, tenantID, serverID uuid.UUID) ([]models.DatabaseEntry, error) {
	var entries []models.DatabaseEntry
	query := `SELECT * FROM database_entries WHERE database_server_id = $1 AND tenant_id = $2 ORDER BY name`
	if err := r.db.SelectContext(ctx, &entries, query, serverID, tenantID); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *DatabaseRepository) ListEntriesByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.DatabaseEntry, error) {
	var entries []models.DatabaseEntry
	query := `SELECT * FROM database_entries WHERE tenant_id = $1 ORDER BY name`
	if err := r.db.SelectContext(ctx, &entries, query, tenantID); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *DatabaseRepository) UpdateEntry(ctx context.Context, e *models.DatabaseEntry) error {
	query := `UPDATE database_entries SET charset = $2, collation = $3, status = $4, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, e.ID, e.Charset, e.Collation, e.Status)
	return err
}

func (r *DatabaseRepository) DeleteEntry(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `DELETE FROM database_entries WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

func (r *DatabaseRepository) UpdateEntryPassword(ctx context.Context, tenantID, id uuid.UUID, password string) error {
	query := `UPDATE database_entries SET password = $2, updated_at = NOW() WHERE id = $1 AND tenant_id = $3`
	_, err := r.db.ExecContext(ctx, query, id, password, tenantID)
	return err
}
