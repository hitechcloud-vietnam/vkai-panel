package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type WAFRepository struct {
	db *sqlx.DB
}

func NewWAFRepository(db *sqlx.DB) *WAFRepository {
	return &WAFRepository{db: db}
}

// WAF Rules

func (r *WAFRepository) ListRules(ctx context.Context, tenantID uuid.UUID) ([]models.WAFRule, error) {
	var rules []models.WAFRule
	err := r.db.SelectContext(ctx, &rules, `
		SELECT id, tenant_id, name, description, rule_type, severity, action, pattern, enabled, created_at, updated_at
		FROM waf_rules 
		WHERE tenant_id = $1 AND deleted_at IS NULL 
		ORDER BY severity DESC, name
	`, tenantID)
	return rules, err
}

func (r *WAFRepository) GetRule(ctx context.Context, id uuid.UUID) (*models.WAFRule, error) {
	var rule models.WAFRule
	err := r.db.GetContext(ctx, &rule, `
		SELECT id, tenant_id, name, description, rule_type, severity, action, pattern, enabled, created_at, updated_at
		FROM waf_rules 
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *WAFRepository) CreateRule(ctx context.Context, rule *models.WAFRule) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO waf_rules (tenant_id, name, description, rule_type, severity, action, pattern, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, rule.TenantID, rule.Name, rule.Description, rule.RuleType, rule.Severity, rule.Action, rule.Pattern, rule.Enabled).
		Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
}

func (r *WAFRepository) UpdateRule(ctx context.Context, rule *models.WAFRule) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE waf_rules 
		SET name = $1, description = $2, rule_type = $3, severity = $4, action = $5, pattern = $6, enabled = $7, updated_at = NOW()
		WHERE id = $8 AND deleted_at IS NULL
	`, rule.Name, rule.Description, rule.RuleType, rule.Severity, rule.Action, rule.Pattern, rule.Enabled, rule.ID)
	return err
}

func (r *WAFRepository) DeleteRule(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE waf_rules SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL
	`, id)
	return err
}

func (r *WAFRepository) ToggleRule(ctx context.Context, id uuid.UUID, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE waf_rules SET enabled = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL
	`, enabled, id)
	return err
}

// WAF Policies

func (r *WAFRepository) ListPolicies(ctx context.Context, tenantID uuid.UUID) ([]models.WAFPolicy, error) {
	var policies []models.WAFPolicy
	err := r.db.SelectContext(ctx, &policies, `
		SELECT id, tenant_id, name, description, mode, paranoia_level, anomaly_threshold, enabled, created_at, updated_at
		FROM waf_policies 
		WHERE tenant_id = $1 AND deleted_at IS NULL 
		ORDER BY name
	`, tenantID)
	return policies, err
}

func (r *WAFRepository) GetPolicy(ctx context.Context, id uuid.UUID) (*models.WAFPolicy, error) {
	var policy models.WAFPolicy
	err := r.db.GetContext(ctx, &policy, `
		SELECT id, tenant_id, name, description, mode, paranoia_level, anomaly_threshold, enabled, created_at, updated_at
		FROM waf_policies 
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *WAFRepository) CreatePolicy(ctx context.Context, policy *models.WAFPolicy) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO waf_policies (tenant_id, name, description, mode, paranoia_level, anomaly_threshold, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`, policy.TenantID, policy.Name, policy.Description, policy.Mode, policy.ParanoiaLevel, policy.AnomalyThreshold, policy.Enabled).
		Scan(&policy.ID, &policy.CreatedAt, &policy.UpdatedAt)
}

func (r *WAFRepository) UpdatePolicy(ctx context.Context, policy *models.WAFPolicy) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE waf_policies 
		SET name = $1, description = $2, mode = $3, paranoia_level = $4, anomaly_threshold = $5, enabled = $6, updated_at = NOW()
		WHERE id = $7 AND deleted_at IS NULL
	`, policy.Name, policy.Description, policy.Mode, policy.ParanoiaLevel, policy.AnomalyThreshold, policy.Enabled, policy.ID)
	return err
}

func (r *WAFRepository) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE waf_policies SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL
	`, id)
	return err
}

// WAF Events

func (r *WAFRepository) ListEvents(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.WAFEvent, error) {
	var events []models.WAFEvent
	err := r.db.SelectContext(ctx, &events, `
		SELECT id, tenant_id, rule_id, website_id, source_ip, method, path, user_agent, 
		       attack_type, severity, action, details, blocked, created_at
		FROM waf_events 
		WHERE tenant_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	return events, err
}

func (r *WAFRepository) CreateEvent(ctx context.Context, event *models.WAFEvent) error {
	return r.db.QueryRowContext(ctx, `
		INSERT INTO waf_events (tenant_id, rule_id, website_id, source_ip, method, path, user_agent, attack_type, severity, action, details, blocked)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at
	`, event.TenantID, event.RuleID, event.WebsiteID, event.SourceIP, event.Method, event.Path, event.UserAgent,
		event.AttackType, event.Severity, event.Action, event.Details, event.Blocked).
		Scan(&event.ID, &event.CreatedAt)
}

func (r *WAFRepository) GetStats(ctx context.Context, tenantID uuid.UUID, since time.Time) (*models.WAFStats, error) {
	stats := &models.WAFStats{}

	// Total requests
	err := r.db.GetContext(ctx, &stats.TotalRequests, `
		SELECT COUNT(*) FROM waf_events WHERE tenant_id = $1 AND created_at >= $2
	`, tenantID, since)
	if err != nil {
		return nil, err
	}

	// Blocked requests
	err = r.db.GetContext(ctx, &stats.BlockedRequests, `
		SELECT COUNT(*) FROM waf_events WHERE tenant_id = $1 AND blocked = true AND created_at >= $2
	`, tenantID, since)
	if err != nil {
		return nil, err
	}

	stats.AllowedRequests = stats.TotalRequests - stats.BlockedRequests

	// Top attack types
	err = r.db.SelectContext(ctx, &stats.TopAttackTypes, `
		SELECT attack_type as type, COUNT(*) as count 
		FROM waf_events 
		WHERE tenant_id = $1 AND created_at >= $2 
		GROUP BY attack_type 
		ORDER BY count DESC 
		LIMIT 10
	`, tenantID, since)
	if err != nil {
		return nil, err
	}

	// Top source IPs
	err = r.db.SelectContext(ctx, &stats.TopSourceIPs, `
		SELECT source_ip as ip, COUNT(*) as count 
		FROM waf_events 
		WHERE tenant_id = $1 AND created_at >= $2 
		GROUP BY source_ip 
		ORDER BY count DESC 
		LIMIT 10
	`, tenantID, since)
	if err != nil {
		return nil, err
	}

	// Recent events
	err = r.db.SelectContext(ctx, &stats.RecentEvents, `
		SELECT id, tenant_id, rule_id, website_id, source_ip, method, path, user_agent, 
		       attack_type, severity, action, details, blocked, created_at
		FROM waf_events 
		WHERE tenant_id = $1 
		ORDER BY created_at DESC 
		LIMIT 20
	`, tenantID)
	if err != nil {
		return nil, err
	}

	return stats, nil
}
