package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/notify"
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
		// Fold the incoming config onto the stored one rather than replacing
		// it. The API hands out configs with secrets replaced by the
		// placeholder, so a UI that reads a channel, changes the SMTP host and
		// writes the whole object back would otherwise store the literal
		// string "[REDACTED]" as the password - a channel that silently stops
		// working and is discovered during the next incident.
		channel.Config = models.JSONMap(notify.MergeConfig(channel.Config, *req.Config))
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

// ============================================================
// DELIVERY OUTBOX
//
// Everything below implements notify.Store plus the reads the API needs. The
// outbox is the queue of record for outbound alerts: a row is written on the
// request path and the dispatcher in internal/notify drains it. See
// migrations/pending/notify.sql for why it lives in Postgres and not in Redis.
// ============================================================

// ErrDeliveryTablesMissing is returned by EnsureDeliverySchema when the
// notify migration has not been applied. It is checked at startup so an
// operator learns about it from one clear log line, rather than from every
// alert failing at 3am.
var ErrDeliveryTablesMissing = errors.New(
	"notification delivery tables are missing: migrations/pending/notify.sql has not been applied")

// EnsureDeliverySchema verifies the two tables this package needs exist.
func (r *NotificationRepository) EnsureDeliverySchema(ctx context.Context) error {
	const query = `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('notification_deliveries', 'notification_alert_state')
	`
	var found int
	if err := r.db.QueryRowContext(ctx, query).Scan(&found); err != nil {
		return fmt.Errorf("check notification delivery schema: %w", err)
	}
	if found < 2 {
		return ErrDeliveryTablesMissing
	}
	return nil
}

// EnqueueDelivery writes one rendered message to the outbox. Nothing is sent
// here: the row is due immediately and the dispatcher takes it from there.
func (r *NotificationRepository) EnqueueDelivery(ctx context.Context, req notify.EnqueueRequest) (uuid.UUID, error) {
	payload, err := json.Marshal(req.Alert)
	if err != nil {
		return uuid.Nil, fmt.Errorf("encode alert payload: %w", err)
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = notify.DefaultMaxAttempts
	}

	const query = `
		INSERT INTO notification_deliveries
			(tenant_id, channel_id, dedup_key, event_kind, subject, body, payload, max_attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`
	var id uuid.UUID
	err = r.db.QueryRowContext(ctx, query,
		req.TenantID, req.ChannelID, req.DedupKey, string(req.Kind),
		req.Subject, req.Body, payload, maxAttempts,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("enqueue notification delivery: %w", err)
	}
	return id, nil
}

// ReleaseStale implements notify.Store. A row left in 'sending' by a worker
// that died is returned to the queue rather than left to rot.
func (r *NotificationRepository) ReleaseStale(ctx context.Context, stuckSince time.Time) (int, error) {
	const query = `
		UPDATE notification_deliveries
		SET status = 'pending', updated_at = NOW()
		WHERE status = 'sending' AND updated_at < $1
	`
	result, err := r.db.ExecContext(ctx, query, stuckSince)
	if err != nil {
		return 0, fmt.Errorf("release stale notification deliveries: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(affected), nil
}

// ClaimDue implements notify.Store.
//
// FOR UPDATE SKIP LOCKED is what makes more than one panel process safe: two
// dispatchers polling the same table take disjoint sets of rows rather than
// both sending the same alert. The channel is joined in so the dispatcher gets
// the type and config in the same round trip.
func (r *NotificationRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]notify.Delivery, error) {
	if limit <= 0 {
		limit = 20
	}
	const query = `
		WITH claimed AS (
			SELECT id
			FROM notification_deliveries
			WHERE status = 'pending' AND next_attempt_at <= $1
			ORDER BY next_attempt_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		), locked AS (
			UPDATE notification_deliveries d
			SET status = 'sending', updated_at = $1
			FROM claimed c
			WHERE d.id = c.id
			RETURNING d.id, d.tenant_id, d.channel_id, d.dedup_key, d.event_kind,
			          d.subject, d.body, d.payload, d.attempts, d.max_attempts
		)
		SELECT l.id, l.tenant_id, l.channel_id, l.dedup_key, l.event_kind,
		       l.subject, l.body, l.payload, l.attempts, l.max_attempts,
		       ch.name, ch.type, ch.config, ch.is_active
		FROM locked l
		JOIN notification_channels ch ON ch.id = l.channel_id
	`
	rows, err := r.db.QueryContext(ctx, query, now, limit)
	if err != nil {
		return nil, fmt.Errorf("claim due notification deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []notify.Delivery
	for rows.Next() {
		var (
			d          notify.Delivery
			kind       string
			payload    []byte
			rawConfig  []byte
			channelCfg = make(map[string]interface{})
		)
		if err := rows.Scan(
			&d.ID, &d.TenantID, &d.ChannelID, &d.DedupKey, &kind,
			&d.Subject, &d.Body, &payload, &d.Attempts, &d.MaxAttempts,
			&d.ChannelName, &d.ChannelType, &rawConfig, &d.ChannelActive,
		); err != nil {
			return nil, fmt.Errorf("scan claimed notification delivery: %w", err)
		}
		d.Kind = notify.EventKind(kind)
		if len(payload) > 0 {
			// A payload that will not decode must not stop the pass: the
			// subject and body are already rendered, and the alert struct is
			// only needed by the senders that post JSON.
			_ = json.Unmarshal(payload, &d.Alert)
		}
		if len(rawConfig) > 0 {
			_ = json.Unmarshal(rawConfig, &channelCfg)
		}
		d.ChannelConfig = notify.Config(channelCfg)
		deliveries = append(deliveries, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read claimed notification deliveries: %w", err)
	}
	return deliveries, nil
}

// MarkSent implements notify.Store.
func (r *NotificationRepository) MarkSent(ctx context.Context, id uuid.UUID, at time.Time) error {
	const query = `
		UPDATE notification_deliveries
		SET status = 'sent', sent_at = $2, attempts = attempts + 1, last_error = '', updated_at = $2
		WHERE id = $1
	`
	if _, err := r.db.ExecContext(ctx, query, id, at); err != nil {
		return fmt.Errorf("mark notification delivery sent: %w", err)
	}
	return nil
}

// Reschedule implements notify.Store.
func (r *NotificationRepository) Reschedule(ctx context.Context, id uuid.UUID, attempts int, nextAttemptAt time.Time, lastError string) error {
	const query = `
		UPDATE notification_deliveries
		SET status = 'pending', attempts = $2, next_attempt_at = $3, last_error = $4, updated_at = NOW()
		WHERE id = $1
	`
	if _, err := r.db.ExecContext(ctx, query, id, attempts, nextAttemptAt, truncateError(lastError)); err != nil {
		return fmt.Errorf("reschedule notification delivery: %w", err)
	}
	return nil
}

// DeadLetter implements notify.Store. The row is kept forever: a dead letter
// is the record that an alert did not reach anyone, and deleting it would put
// the panel back to dropping alerts silently.
func (r *NotificationRepository) DeadLetter(ctx context.Context, id uuid.UUID, at time.Time, lastError string) error {
	const query = `
		UPDATE notification_deliveries
		SET status = 'dead_letter', dead_lettered_at = $2, attempts = attempts + 1,
		    last_error = $3, updated_at = $2
		WHERE id = $1
	`
	if _, err := r.db.ExecContext(ctx, query, id, at, truncateError(lastError)); err != nil {
		return fmt.Errorf("dead-letter notification delivery: %w", err)
	}
	return nil
}

// maxStoredError bounds what one failed delivery can write. An endpoint that
// answers with a megabyte of HTML should not be able to bloat the table.
const maxStoredError = 4000

// truncateError bounds an error message stored in the outbox.
func truncateError(text string) string {
	runes := []rune(text)
	if len(runes) <= maxStoredError {
		return text
	}
	return string(runes[:maxStoredError]) + " [truncated]"
}

// ListDeliveries returns outbox rows for a tenant, newest first.
func (r *NotificationRepository) ListDeliveries(ctx context.Context, tenantID uuid.UUID, filter notify.DeliveryFilter) ([]notify.DeliveryRecord, int, error) {
	where := " WHERE d.tenant_id = $1"
	args := []interface{}{tenantID}

	if filter.Status != "" {
		args = append(args, filter.Status)
		where += fmt.Sprintf(" AND d.status = $%d", len(args))
	}
	if filter.ChannelID != nil {
		args = append(args, *filter.ChannelID)
		where += fmt.Sprintf(" AND d.channel_id = $%d", len(args))
	}
	if filter.DedupKey != "" {
		args = append(args, filter.DedupKey)
		where += fmt.Sprintf(" AND d.dedup_key = $%d", len(args))
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM notification_deliveries d" + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count notification deliveries: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)

	query := `
		SELECT d.id, d.tenant_id, d.channel_id, ch.name AS channel_name, ch.type AS channel_type,
		       d.dedup_key, d.event_kind, d.subject, d.body,
		       d.status, d.attempts, d.max_attempts, d.next_attempt_at, d.last_error,
		       d.sent_at, d.dead_lettered_at, d.created_at, d.updated_at
		FROM notification_deliveries d
		JOIN notification_channels ch ON ch.id = d.channel_id` + where +
		fmt.Sprintf(" ORDER BY d.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	var records []notify.DeliveryRecord
	if err := r.db.SelectContext(ctx, &records, query, args...); err != nil {
		return nil, 0, fmt.Errorf("list notification deliveries: %w", err)
	}
	return records, total, nil
}

// GetDelivery returns one outbox row.
func (r *NotificationRepository) GetDelivery(ctx context.Context, tenantID, id uuid.UUID) (*notify.DeliveryRecord, error) {
	const query = `
		SELECT d.id, d.tenant_id, d.channel_id, ch.name AS channel_name, ch.type AS channel_type,
		       d.dedup_key, d.event_kind, d.subject, d.body,
		       d.status, d.attempts, d.max_attempts, d.next_attempt_at, d.last_error,
		       d.sent_at, d.dead_lettered_at, d.created_at, d.updated_at
		FROM notification_deliveries d
		JOIN notification_channels ch ON ch.id = d.channel_id
		WHERE d.tenant_id = $1 AND d.id = $2
	`
	var record notify.DeliveryRecord
	if err := r.db.GetContext(ctx, &record, query, tenantID, id); err != nil {
		return nil, err
	}
	return &record, nil
}

// RetryDelivery puts a dead letter back in the queue with a fresh attempt
// budget. It is what an operator presses after fixing the channel, and it is
// why a dead letter is a row rather than a log line.
func (r *NotificationRepository) RetryDelivery(ctx context.Context, tenantID, id uuid.UUID) error {
	const query = `
		UPDATE notification_deliveries
		SET status = 'pending', attempts = 0, next_attempt_at = NOW(),
		    dead_lettered_at = NULL, last_error = '', updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2 AND status = 'dead_letter'
	`
	result, err := r.db.ExecContext(ctx, query, tenantID, id)
	if err != nil {
		return fmt.Errorf("retry notification delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ============================================================
// ALERT DEDUPLICATION STATE
// ============================================================

// ObserveAlert folds one observation into the stored state for a dedup key and
// returns what should happen.
//
// The read, the decision and the write are one transaction with the row locked
// for update. Without that lock, two checks arriving in the same second - one
// per monitored server, or one per panel process - both read "not notified
// recently" and both send, which is exactly the duplicate storm the quiet
// period exists to stop.
//
// The decision itself is notify.Decide, a pure function. This method is only
// the locking and the SQL around it.
func (r *NotificationRepository) ObserveAlert(ctx context.Context, tenantID uuid.UUID, dedupKey string, obs notify.Observation) (notify.Decision, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return notify.Decision{}, fmt.Errorf("begin alert state transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const selectQuery = `
		SELECT state, first_seen_at, last_seen_at, last_notified_at, occurrences, quiet_period_seconds
		FROM notification_alert_state
		WHERE tenant_id = $1 AND dedup_key = $2
		FOR UPDATE
	`
	var (
		prev        *notify.AlertState
		state       string
		firstSeen   time.Time
		lastSeen    time.Time
		lastNotify  *time.Time
		occurrences int
		quietSecs   int
	)
	err = tx.QueryRowContext(ctx, selectQuery, tenantID, dedupKey).
		Scan(&state, &firstSeen, &lastSeen, &lastNotify, &occurrences, &quietSecs)
	switch {
	case err == nil:
		prev = &notify.AlertState{
			State:          state,
			FirstSeenAt:    firstSeen,
			LastSeenAt:     lastSeen,
			LastNotifiedAt: lastNotify,
			Occurrences:    occurrences,
			QuietPeriod:    time.Duration(quietSecs) * time.Second,
		}
	case errors.Is(err, sql.ErrNoRows):
		prev = nil
	default:
		return notify.Decision{}, fmt.Errorf("read alert state: %w", err)
	}

	decision := notify.Decide(prev, obs)

	if decision.Persist {
		const upsertQuery = `
			INSERT INTO notification_alert_state
				(tenant_id, dedup_key, state, first_seen_at, last_seen_at, last_notified_at,
				 occurrences, quiet_period_seconds, last_value, threshold, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
			ON CONFLICT (tenant_id, dedup_key) DO UPDATE SET
				state                = EXCLUDED.state,
				first_seen_at        = EXCLUDED.first_seen_at,
				last_seen_at         = EXCLUDED.last_seen_at,
				last_notified_at     = EXCLUDED.last_notified_at,
				occurrences          = EXCLUDED.occurrences,
				quiet_period_seconds = EXCLUDED.quiet_period_seconds,
				last_value           = EXCLUDED.last_value,
				threshold            = EXCLUDED.threshold,
				updated_at           = NOW()
		`
		s := decision.State
		if _, err := tx.ExecContext(ctx, upsertQuery,
			tenantID, dedupKey, s.State, s.FirstSeenAt, s.LastSeenAt, s.LastNotifiedAt,
			s.Occurrences, int(s.QuietPeriod/time.Second), s.LastValue, s.Threshold,
		); err != nil {
			return notify.Decision{}, fmt.Errorf("write alert state: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return notify.Decision{}, fmt.Errorf("commit alert state: %w", err)
	}
	return decision, nil
}

// GetAlertState returns the deduplication state for one key, or nil when the
// key has never been seen.
func (r *NotificationRepository) GetAlertState(ctx context.Context, tenantID uuid.UUID, dedupKey string) (*notify.AlertStateRecord, error) {
	const query = `
		SELECT tenant_id, dedup_key, state, first_seen_at, last_seen_at, last_notified_at,
		       occurrences, quiet_period_seconds, last_value, threshold, updated_at
		FROM notification_alert_state
		WHERE tenant_id = $1 AND dedup_key = $2
	`
	var record notify.AlertStateRecord
	err := r.db.GetContext(ctx, &record, query, tenantID, dedupKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read alert state: %w", err)
	}
	return &record, nil
}

// ListAlertStates returns the deduplication state for a tenant, firing first
// and most recent first within that: the order an operator wants when asking
// "what is wrong right now".
func (r *NotificationRepository) ListAlertStates(ctx context.Context, tenantID uuid.UUID, limit int) ([]notify.AlertStateRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	const query = `
		SELECT tenant_id, dedup_key, state, first_seen_at, last_seen_at, last_notified_at,
		       occurrences, quiet_period_seconds, last_value, threshold, updated_at
		FROM notification_alert_state
		WHERE tenant_id = $1
		ORDER BY (state = 'firing') DESC, last_seen_at DESC
		LIMIT $2
	`
	var records []notify.AlertStateRecord
	if err := r.db.SelectContext(ctx, &records, query, tenantID, limit); err != nil {
		return nil, fmt.Errorf("list alert states: %w", err)
	}
	return records, nil
}

// ListActiveChannels returns the channels a message should fan out to.
// Inactive channels are excluded here rather than at send time, so a disabled
// channel produces no outbox rows at all.
func (r *NotificationRepository) ListActiveChannels(ctx context.Context, tenantID uuid.UUID) ([]models.NotificationChannel, error) {
	var channels []models.NotificationChannel
	err := r.db.SelectContext(ctx, &channels,
		"SELECT * FROM notification_channels WHERE tenant_id = $1 AND is_active = TRUE ORDER BY name",
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list active notification channels: %w", err)
	}
	return channels, nil
}
