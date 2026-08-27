package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type FirewallRepository struct {
	db *sqlx.DB
}

func NewFirewallRepository(db *sqlx.DB) *FirewallRepository {
	return &FirewallRepository{db: db}
}

func (r *FirewallRepository) Create(ctx context.Context, rule *models.FirewallRule) error {
	query := `
		INSERT INTO firewall_rules (id, tenant_id, server_id, protocol, port, source, action, direction, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		RETURNING created_at, updated_at`

	rule.ID = uuid.New()
	return r.db.QueryRowContext(ctx, query,
		rule.ID, rule.TenantID, rule.ServerID, rule.Protocol,
		rule.Port, rule.Source, rule.Action, rule.Direction, rule.Status,
	).Scan(&rule.CreatedAt, &rule.UpdatedAt)
}

func (r *FirewallRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.FirewallRule, error) {
	var rule models.FirewallRule
	query := `SELECT * FROM firewall_rules WHERE id = $1 AND tenant_id = $2`
	if err := r.db.GetContext(ctx, &rule, query, id, tenantID); err != nil {
		return nil, fmt.Errorf("firewall rule not found: %w", err)
	}
	return &rule, nil
}

func (r *FirewallRepository) ListByServer(ctx context.Context, serverID uuid.UUID) ([]models.FirewallRule, error) {
	var rules []models.FirewallRule
	query := `SELECT * FROM firewall_rules WHERE server_id = $1 ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &rules, query, serverID); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *FirewallRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.FirewallRule, error) {
	var rules []models.FirewallRule
	query := `SELECT * FROM firewall_rules WHERE tenant_id = $1 ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &rules, query, tenantID); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *FirewallRepository) Update(ctx context.Context, rule *models.FirewallRule) error {
	query := `UPDATE firewall_rules SET protocol = $2, port = $3, source = $4, action = $5, direction = $6, status = $7, updated_at = NOW() WHERE id = $1 AND tenant_id = $8`
	_, err := r.db.ExecContext(ctx, query,
		rule.ID, rule.Protocol, rule.Port, rule.Source, rule.Action, rule.Direction, rule.Status, rule.TenantID,
	)
	return err
}

func (r *FirewallRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	query := `DELETE FROM firewall_rules WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}
