package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type NotificationRepository struct {
	db *sqlx.DB
}

func NewNotificationRepository(db *sqlx.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// Notifications
func (r *NotificationRepository) Create(ctx context.Context, notification *models.Notification) error {
	query := `
		INSERT INTO notifications (tenant_id, user_id, type, title, message, details, is_read)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	return r.db.QueryRowContext(ctx, query,
		notification.TenantID, notification.UserID, notification.Type,
		notification.Title, notification.Message, notification.Details, notification.IsRead,
	).Scan(&notification.ID, &notification.CreatedAt)
}

func (r *NotificationRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.Notification, error) {
	var notification models.Notification
	err := r.db.GetContext(ctx, &notification,
		"SELECT * FROM notifications WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &notification, nil
}

func (r *NotificationRepository) List(ctx context.Context, tenantID uuid.UUID, userID *uuid.UUID, isRead *bool, limit, offset int) ([]models.Notification, int, error) {
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if userID != nil {
		where += " AND (user_id = $2 OR user_id IS NULL)"
		args = append(args, *userID)
		argIdx++
	}
	if isRead != nil {
		where += " AND is_read = $" + string(rune('0'+argIdx))
		args = append(args, *isRead)
		argIdx++
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM notifications " + where
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get notifications
	selectQuery := `
		SELECT * FROM notifications ` + where + `
		ORDER BY created_at DESC
		LIMIT $` + string(rune('0'+argIdx)) + ` OFFSET $` + string(rune('0'+argIdx+1))
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notifications []models.Notification
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.TenantID, &n.UserID, &n.Type, &n.Title,
			&n.Message, &n.Details, &n.IsRead, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, n)
	}

	return notifications, total, nil
}

func (r *NotificationRepository) MarkAsRead(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE notifications SET is_read = TRUE, read_at = NOW() WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE notifications SET is_read = TRUE, read_at = NOW() WHERE tenant_id = $1 AND (user_id = $2 OR user_id IS NULL) AND is_read = FALSE",
		tenantID, userID,
	)
	return err
}

func (r *NotificationRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM notifications WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

func (r *NotificationRepository) DeleteOld(ctx context.Context, tenantID uuid.UUID, days int) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM notifications WHERE tenant_id = $1 AND created_at < NOW() - INTERVAL '1 day' * $2",
		tenantID, days,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Templates
func (r *NotificationRepository) CreateTemplate(ctx context.Context, template *models.NotificationTemplate) error {
	query := `
		INSERT INTO notification_templates (tenant_id, name, type, subject, body, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		template.TenantID, template.Name, template.Type,
		template.Subject, template.Body, template.IsActive,
	).Scan(&template.ID, &template.CreatedAt, &template.UpdatedAt)
}

func (r *NotificationRepository) GetTemplateByID(ctx context.Context, tenantID, id uuid.UUID) (*models.NotificationTemplate, error) {
	var template models.NotificationTemplate
	err := r.db.GetContext(ctx, &template,
		"SELECT * FROM notification_templates WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &template, nil
}

func (r *NotificationRepository) ListTemplates(ctx context.Context, tenantID uuid.UUID) ([]models.NotificationTemplate, error) {
	var templates []models.NotificationTemplate
	err := r.db.SelectContext(ctx, &templates,
		"SELECT * FROM notification_templates WHERE tenant_id = $1 ORDER BY name",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	return templates, nil
}

func (r *NotificationRepository) UpdateTemplate(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateNotificationTemplateRequest) (*models.NotificationTemplate, error) {
	template, err := r.GetTemplateByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		template.Name = *req.Name
	}
	if req.Type != nil {
		template.Type = *req.Type
	}
	if req.Subject != nil {
		template.Subject = *req.Subject
	}
	if req.Body != nil {
		template.Body = *req.Body
	}
	if req.IsActive != nil {
		template.IsActive = *req.IsActive
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE notification_templates SET name=$1, type=$2, subject=$3, body=$4, is_active=$5, updated_at=NOW()
		 WHERE tenant_id=$6 AND id=$7`,
		template.Name, template.Type, template.Subject, template.Body, template.IsActive,
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}

	return template, nil
}

func (r *NotificationRepository) DeleteTemplate(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM notification_templates WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

// Channels
func (r *NotificationRepository) CreateChannel(ctx context.Context, channel *models.NotificationChannel) error {
	query := `
		INSERT INTO notification_channels (tenant_id, name, type, config, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		channel.TenantID, channel.Name, channel.Type, channel.Config, channel.IsActive,
	).Scan(&channel.ID, &channel.CreatedAt, &channel.UpdatedAt)
}

func (r *NotificationRepository) GetChannelByID(ctx context.Context, tenantID, id uuid.UUID) (*models.NotificationChannel, error) {
	var channel models.NotificationChannel
	err := r.db.GetContext(ctx, &channel,
		"SELECT * FROM notification_channels WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}
	return &channel, nil
}

func (r *NotificationRepository) ListChannels(ctx context.Context, tenantID uuid.UUID) ([]models.NotificationChannel, error) {
	var channels []models.NotificationChannel
	err := r.db.SelectContext(ctx, &channels,
		"SELECT * FROM notification_channels WHERE tenant_id = $1 ORDER BY name",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	return channels, nil
}

func (r *NotificationRepository) UpdateChannel(ctx context.Context, tenantID, id uuid.UUID, req *models.UpdateNotificationChannelRequest) (*models.NotificationChannel, error) {
	channel, err := r.GetChannelByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		channel.Name = *req.Name
	}
	if req.Type != nil {
		channel.Type = *req.Type
	}
	if req.Config != nil {
		channel.Config = *req.Config
	}
	if req.IsActive != nil {
		channel.IsActive = *req.IsActive
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE notification_channels SET name=$1, type=$2, config=$3, is_active=$4, updated_at=NOW()
		 WHERE tenant_id=$5 AND id=$6`,
		channel.Name, channel.Type, channel.Config, channel.IsActive,
		tenantID, id,
	)
	if err != nil {
		return nil, err
	}

	return channel, nil
}

func (r *NotificationRepository) DeleteChannel(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM notification_channels WHERE tenant_id = $1 AND id = $2",
		tenantID, id,
	)
	return err
}

// Preferences
func (r *NotificationRepository) GetPreferences(ctx context.Context, tenantID, userID uuid.UUID) ([]models.NotificationPreference, error) {
	var preferences []models.NotificationPreference
	err := r.db.SelectContext(ctx, &preferences,
		"SELECT * FROM notification_preferences WHERE tenant_id = $1 AND user_id = $2 ORDER BY type, channel",
		tenantID, userID,
	)
	if err != nil {
		return nil, err
	}
	return preferences, nil
}

func (r *NotificationRepository) SetPreference(ctx context.Context, tenantID, userID uuid.UUID, prefType, channel string, enabled bool) error {
	query := `
		INSERT INTO notification_preferences (tenant_id, user_id, type, channel, enabled)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, user_id, type, channel) DO UPDATE SET enabled = $5, updated_at = NOW()
	`
	_, err := r.db.ExecContext(ctx, query, tenantID, userID, prefType, channel, enabled)
	return err
}
