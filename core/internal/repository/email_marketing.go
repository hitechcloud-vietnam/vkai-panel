package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type EmailMarketingRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewEmailMarketingRepository(db *sqlx.DB, logger *zap.Logger) *EmailMarketingRepository {
	return &EmailMarketingRepository{db: db, logger: logger}
}

func (r *EmailMarketingRepository) CreateCampaign(ctx context.Context, c *models.EmailCampaign) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	return r.db.QueryRowContext(ctx, `
		INSERT INTO email_campaigns (id, tenant_id, name, subject, html_content, plain_text, status,
			scheduled_at, total_recipients, sent_count, open_count, click_count, bounce_count,
			unsubscribe_count, from_name, from_email, reply_to, tags, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		RETURNING id, created_at, updated_at`,
		c.ID, c.TenantID, c.Name, c.Subject, c.HTMLContent, c.PlainText, c.Status,
		c.ScheduledAt, c.TotalRecipients, c.SentCount, c.OpenCount, c.ClickCount, c.BounceCount,
		c.UnsubscribeCount, c.FromName, c.FromEmail, c.ReplyTo, c.Tags, c.CreatedAt, c.UpdatedAt,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *EmailMarketingRepository) ListCampaigns(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.EmailCampaign, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM email_campaigns WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	var campaigns []models.EmailCampaign
	err = r.db.SelectContext(ctx, &campaigns, `
		SELECT id, tenant_id, name, subject, html_content, plain_text, status,
			scheduled_at, sent_at, total_recipients, sent_count, open_count, click_count,
			bounce_count, unsubscribe_count, from_name, from_email, reply_to, tags, created_at, updated_at
		FROM email_campaigns WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	return campaigns, total, err
}

func (r *EmailMarketingRepository) GetCampaign(ctx context.Context, tenantID, id uuid.UUID) (*models.EmailCampaign, error) {
	var c models.EmailCampaign
	err := r.db.GetContext(ctx, &c, `
		SELECT id, tenant_id, name, subject, html_content, plain_text, status,
			scheduled_at, sent_at, total_recipients, sent_count, open_count, click_count,
			bounce_count, unsubscribe_count, from_name, from_email, reply_to, tags, created_at, updated_at
		FROM email_campaigns WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *EmailMarketingRepository) UpdateCampaign(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateCampaignRequest) error {
	query := `UPDATE email_campaigns SET updated_at = $1`
	args := []interface{}{time.Now()}
	argIdx := 2
	if req.Name != nil {
		query += fmt.Sprintf(", name = $%d", argIdx)
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Subject != nil {
		query += fmt.Sprintf(", subject = $%d", argIdx)
		args = append(args, *req.Subject)
		argIdx++
	}
	if req.HTMLContent != nil {
		query += fmt.Sprintf(", html_content = $%d", argIdx)
		args = append(args, *req.HTMLContent)
		argIdx++
	}
	if req.FromName != nil {
		query += fmt.Sprintf(", from_name = $%d", argIdx)
		args = append(args, *req.FromName)
		argIdx++
	}
	if req.FromEmail != nil {
		query += fmt.Sprintf(", from_email = $%d", argIdx)
		args = append(args, *req.FromEmail)
		argIdx++
	}
	query += fmt.Sprintf(" WHERE id = $%d AND tenant_id = $%d AND deleted_at IS NULL", argIdx, argIdx+1)
	args = append(args, id, tenantID)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *EmailMarketingRepository) DeleteCampaign(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE email_campaigns SET deleted_at = $1 WHERE id = $2 AND tenant_id = $3`, time.Now(), id, tenantID)
	return err
}

func (r *EmailMarketingRepository) UpdateCampaignStatus(ctx context.Context, tenantID, id uuid.UUID, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE email_campaigns SET status = $1, updated_at = $2 WHERE id = $3 AND tenant_id = $4`, status, time.Now(), id, tenantID)
	return err
}

func (r *EmailMarketingRepository) CreateContact(ctx context.Context, c *models.EmailContact) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	if c.Status == "" {
		c.Status = "active"
	}
	return r.db.QueryRowContext(ctx, `
		INSERT INTO email_contacts (id, tenant_id, email, first_name, last_name, status, source, tags, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, created_at, updated_at`,
		c.ID, c.TenantID, c.Email, c.FirstName, c.LastName, c.Status, c.Source, c.Tags, c.Metadata, c.CreatedAt, c.UpdatedAt,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *EmailMarketingRepository) ListContacts(ctx context.Context, tenantID uuid.UUID, limit, offset int, search string) ([]models.EmailContact, int, error) {
	where := `WHERE tenant_id = $1 AND deleted_at IS NULL`
	args := []interface{}{tenantID}
	argIdx := 2
	if search != "" {
		where += fmt.Sprintf(` AND (email ILIKE $%d OR first_name ILIKE $%d OR last_name ILIKE $%d)`, argIdx, argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}
	var total int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_contacts `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	var contacts []models.EmailContact
	err = r.db.SelectContext(ctx, &contacts, fmt.Sprintf(`
		SELECT id, tenant_id, email, first_name, last_name, status, source, tags, metadata, confirmed_at, created_at, updated_at
		FROM email_contacts %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1), args...)
	return contacts, total, err
}

func (r *EmailMarketingRepository) DeleteContact(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE email_contacts SET deleted_at = $1 WHERE id = $2 AND tenant_id = $3`, time.Now(), id, tenantID)
	return err
}

func (r *EmailMarketingRepository) CreateList(ctx context.Context, l *models.EmailList) error {
	l.ID = uuid.New()
	l.CreatedAt = time.Now()
	l.UpdatedAt = time.Now()
	return r.db.QueryRowContext(ctx, `
		INSERT INTO email_lists (id, tenant_id, name, description, contact_count, double_opt_in, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, created_at, updated_at`,
		l.ID, l.TenantID, l.Name, l.Description, l.ContactCount, l.DoubleOptIn, l.CreatedAt, l.UpdatedAt,
	).Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt)
}

func (r *EmailMarketingRepository) ListLists(ctx context.Context, tenantID uuid.UUID) ([]models.EmailList, error) {
	var lists []models.EmailList
	err := r.db.SelectContext(ctx, &lists, `
		SELECT id, tenant_id, name, description, contact_count, double_opt_in, created_at, updated_at
		FROM email_lists WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`, tenantID)
	return lists, err
}

func (r *EmailMarketingRepository) DeleteList(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE email_lists SET deleted_at = $1 WHERE id = $2 AND tenant_id = $3`, time.Now(), id, tenantID)
	return err
}

func (r *EmailMarketingRepository) CreateTemplate(ctx context.Context, t *models.EmailTemplate) error {
	t.ID = uuid.New()
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	return r.db.QueryRowContext(ctx, `
		INSERT INTO email_templates (id, tenant_id, name, subject, html_content, category, thumbnail, is_default, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at`,
		t.ID, t.TenantID, t.Name, t.Subject, t.HTMLContent, t.Category, t.Thumbnail, t.IsDefault, t.CreatedAt, t.UpdatedAt,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (r *EmailMarketingRepository) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]models.EmailTemplate, error) {
	var templates []models.EmailTemplate
	err := r.db.SelectContext(ctx, &templates, `
		SELECT id, tenant_id, name, subject, html_content, category, thumbnail, is_default, created_at, updated_at
		FROM email_templates WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`, tenantID)
	return templates, err
}

func (r *EmailMarketingRepository) DeleteTemplate(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE email_templates SET deleted_at = $1 WHERE id = $2 AND tenant_id = $3`, time.Now(), id, tenantID)
	return err
}

func (r *EmailMarketingRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.EmailStats, error) {
	var s models.EmailStats
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_campaigns WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&s.TotalCampaigns); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_contacts WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&s.TotalContacts); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_lists WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&s.TotalLists); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(sent_count),0), COALESCE(SUM(open_count),0), COALESCE(SUM(click_count),0), COALESCE(SUM(bounce_count),0)
		FROM email_campaigns WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID,
	).Scan(&s.TotalSent, &s.TotalOpened, &s.TotalClicked, &s.TotalBounced); err != nil {
		return nil, err
	}
	if s.TotalSent > 0 {
		s.AvgOpenRate = float64(s.TotalOpened) / float64(s.TotalSent) * 100
		s.AvgClickRate = float64(s.TotalClicked) / float64(s.TotalSent) * 100
		s.AvgBounceRate = float64(s.TotalBounced) / float64(s.TotalSent) * 100
	}
	return &s, nil
}
