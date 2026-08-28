package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type MailServerRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewMailServerRepository(db *sqlx.DB, logger *zap.Logger) *MailServerRepository {
	return &MailServerRepository{db: db, logger: logger}
}

// --- Domains ---

func (r *MailServerRepository) CreateDomain(ctx context.Context, tenantID uuid.UUID, req models.CreateDomainRequest) (*models.MailDomain, error) {
	d := &models.MailDomain{
		ID:       uuid.New(),
		TenantID: tenantID,
		Domain:   req.Domain,
		IsActive: true,
	}
	query := `INSERT INTO mail_domains (id, tenant_id, domain, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, query, d.ID, d.TenantID, d.Domain, d.IsActive)
	if err != nil {
		return nil, err
	}
	return r.GetDomain(ctx, d.ID)
}

func (r *MailServerRepository) ListDomains(ctx context.Context, tenantID uuid.UUID) ([]models.MailDomain, error) {
	var domains []models.MailDomain
	query := `SELECT id, tenant_id, domain, is_verified,
		       COALESCE(mx_record, '') AS mx_record,
		       COALESCE(spf_record, '') AS spf_record,
		       dkim_enabled,
		       COALESCE(dmarc_record, '') AS dmarc_record,
		       is_active, created_at, updated_at
		  FROM mail_domains WHERE tenant_id = $1 ORDER BY created_at DESC`
	err := r.db.SelectContext(ctx, &domains, query, tenantID)
	return domains, err
}

func (r *MailServerRepository) GetDomain(ctx context.Context, id uuid.UUID) (*models.MailDomain, error) {
	var d models.MailDomain
	query := `SELECT id, tenant_id, domain, is_verified,
		       COALESCE(mx_record, '') AS mx_record,
		       COALESCE(spf_record, '') AS spf_record,
		       dkim_enabled,
		       COALESCE(dmarc_record, '') AS dmarc_record,
		       is_active, created_at, updated_at
		  FROM mail_domains WHERE id = $1`
	err := r.db.GetContext(ctx, &d, query, id)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *MailServerRepository) DeleteDomain(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM mail_domains WHERE id = $1`, id)
	return err
}

// The mail queries below spell their columns out and COALESCE the nullable
// text ones.
//
// "SELECT *" into models.MailAccount fails at scan time the moment forward_to
// or auto_reply_msg is NULL - "converting NULL to string is unsupported" - and
// those columns are nullable with no default, so every account created through
// the panel hit it: the row was inserted and the call still returned an error.
// The struct fields are plain strings and are read as such all the way out to
// the interface, so the fix belongs here rather than in the model.
// --- Accounts ---

func (r *MailServerRepository) CreateAccount(ctx context.Context, tenantID uuid.UUID, req models.CreateAccountRequest, hashedPwd string) (*models.MailAccount, error) {
	a := &models.MailAccount{
		ID:       uuid.New(),
		TenantID: tenantID,
		DomainID: req.DomainID,
		Email:    req.Email,
		Password: hashedPwd,
		QuotaMB:  req.QuotaMB,
		IsActive: true,
	}
	if a.QuotaMB == 0 {
		a.QuotaMB = 1024
	}
	query := `INSERT INTO mail_accounts (id, tenant_id, domain_id, email, password, quota_mb, used_mb, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $7, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, query, a.ID, a.TenantID, a.DomainID, a.Email, a.Password, a.QuotaMB, a.IsActive)
	if err != nil {
		return nil, err
	}
	return r.GetAccount(ctx, a.ID)
}

func (r *MailServerRepository) ListAccounts(ctx context.Context, tenantID uuid.UUID) ([]models.MailAccount, error) {
	var accounts []models.MailAccount
	query := `SELECT id, tenant_id, domain_id, email, password, quota_mb, used_mb, is_active,
		       COALESCE(forward_to, '') AS forward_to, auto_reply,
		       COALESCE(auto_reply_msg, '') AS auto_reply_msg,
		       last_login_at, created_at, updated_at
		  FROM mail_accounts WHERE tenant_id = $1 ORDER BY created_at DESC`
	err := r.db.SelectContext(ctx, &accounts, query, tenantID)
	return accounts, err
}

func (r *MailServerRepository) GetAccount(ctx context.Context, id uuid.UUID) (*models.MailAccount, error) {
	var a models.MailAccount
	query := `SELECT id, tenant_id, domain_id, email, password, quota_mb, used_mb, is_active,
		       COALESCE(forward_to, '') AS forward_to, auto_reply,
		       COALESCE(auto_reply_msg, '') AS auto_reply_msg,
		       last_login_at, created_at, updated_at
		  FROM mail_accounts WHERE id = $1`
	err := r.db.GetContext(ctx, &a, query, id)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *MailServerRepository) UpdateAccount(ctx context.Context, id uuid.UUID, req models.UpdateAccountRequest) (*models.MailAccount, error) {
	a, err := r.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.QuotaMB > 0 {
		a.QuotaMB = req.QuotaMB
	}
	if req.IsActive != nil {
		a.IsActive = *req.IsActive
	}
	a.ForwardTo = req.ForwardTo
	if req.AutoReply != nil {
		a.AutoReply = *req.AutoReply
	}
	a.AutoReplyMsg = req.AutoReplyMsg

	query := `UPDATE mail_accounts SET quota_mb=$1, is_active=$2, forward_to=$3, auto_reply=$4, auto_reply_msg=$5, updated_at=NOW() WHERE id=$6`
	_, err = r.db.ExecContext(ctx, query, a.QuotaMB, a.IsActive, a.ForwardTo, a.AutoReply, a.AutoReplyMsg, id)
	if err != nil {
		return nil, err
	}
	return r.GetAccount(ctx, id)
}

func (r *MailServerRepository) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM mail_accounts WHERE id = $1`, id)
	return err
}

// --- Aliases ---

func (r *MailServerRepository) CreateAlias(ctx context.Context, tenantID uuid.UUID, req models.CreateAliasRequest) (*models.MailAlias, error) {
	a := &models.MailAlias{
		ID:          uuid.New(),
		TenantID:    tenantID,
		DomainID:    req.DomainID,
		Source:      req.Source,
		Destination: req.Destination,
		IsActive:    true,
	}
	query := `INSERT INTO mail_aliases (id, tenant_id, domain_id, source, destination, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, query, a.ID, a.TenantID, a.DomainID, a.Source, a.Destination, a.IsActive)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *MailServerRepository) ListAliases(ctx context.Context, tenantID uuid.UUID) ([]models.MailAlias, error) {
	var aliases []models.MailAlias
	query := `SELECT * FROM mail_aliases WHERE tenant_id = $1 ORDER BY created_at DESC`
	err := r.db.SelectContext(ctx, &aliases, query, tenantID)
	return aliases, err
}

func (r *MailServerRepository) DeleteAlias(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM mail_aliases WHERE id = $1`, id)
	return err
}

// --- Queue ---

func (r *MailServerRepository) ListQueueItems(ctx context.Context, tenantID uuid.UUID) ([]models.MailQueueItem, error) {
	var items []models.MailQueueItem
	query := `SELECT * FROM mail_queue WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 100`
	err := r.db.SelectContext(ctx, &items, query, tenantID)
	return items, err
}

func (r *MailServerRepository) DeleteQueueItem(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM mail_queue WHERE id = $1`, id)
	return err
}

func (r *MailServerRepository) FlushQueue(ctx context.Context, tenantID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM mail_queue WHERE tenant_id = $1 AND status = 'failed'`, tenantID)
	return err
}

// --- Spam Filter ---

func (r *MailServerRepository) GetSpamFilter(ctx context.Context, tenantID uuid.UUID) (*models.MailSpamFilter, error) {
	var sf models.MailSpamFilter
	query := `SELECT * FROM mail_spam_filters WHERE tenant_id = $1`
	err := r.db.GetContext(ctx, &sf, query, tenantID)
	if err == sql.ErrNoRows {
		// Create default
		sf = models.MailSpamFilter{
			ID:            uuid.New(),
			TenantID:      tenantID,
			Enabled:       true,
			SpamThreshold: 5.0,
			RejectScore:   15.0,
			Greylisting:   false,
		}
		insertQ := `INSERT INTO mail_spam_filters (id, tenant_id, enabled, spam_threshold, reject_score, greylisting, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
		_, err = r.db.ExecContext(ctx, insertQ, sf.ID, sf.TenantID, sf.Enabled, sf.SpamThreshold, sf.RejectScore, sf.Greylisting)
		if err != nil {
			return nil, err
		}
		return &sf, nil
	}
	if err != nil {
		return nil, err
	}
	return &sf, nil
}

func (r *MailServerRepository) UpdateSpamFilter(ctx context.Context, tenantID uuid.UUID, req models.UpdateSpamFilterRequest) (*models.MailSpamFilter, error) {
	sf, err := r.GetSpamFilter(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		sf.Enabled = *req.Enabled
	}
	if req.SpamThreshold != nil {
		sf.SpamThreshold = *req.SpamThreshold
	}
	if req.RejectScore != nil {
		sf.RejectScore = *req.RejectScore
	}
	if req.Greylisting != nil {
		sf.Greylisting = *req.Greylisting
	}
	if req.Blacklist != nil {
		sf.Blacklist = req.Blacklist
	}
	if req.Whitelist != nil {
		sf.Whitelist = req.Whitelist
	}

	query := `UPDATE mail_spam_filters SET enabled=$1, spam_threshold=$2, reject_score=$3, greylisting=$4, updated_at=NOW() WHERE id=$5`
	_, err = r.db.ExecContext(ctx, query, sf.Enabled, sf.SpamThreshold, sf.RejectScore, sf.Greylisting, sf.ID)
	if err != nil {
		return nil, err
	}
	return sf, nil
}

// --- Server Config ---

func (r *MailServerRepository) GetServerConfig(ctx context.Context, tenantID uuid.UUID) (*models.MailServerConfig, error) {
	var cfg models.MailServerConfig
	query := `SELECT * FROM mail_server_configs WHERE tenant_id = $1`
	err := r.db.GetContext(ctx, &cfg, query, tenantID)
	if err == sql.ErrNoRows {
		cfg = models.MailServerConfig{
			ID:             uuid.New(),
			TenantID:       tenantID,
			Hostname:       "mail.example.com",
			SMTPPort:       25,
			SMTPSPort:      587,
			IMAPPort:       143,
			IMAPSPort:      993,
			MaxMessageSize: 25,
			TLSEnabled:     true,
		}
		insertQ := `INSERT INTO mail_server_configs (id, tenant_id, hostname, smtp_port, smtps_port, imap_port, imaps_port, max_message_size, tls_enabled, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`
		_, err = r.db.ExecContext(ctx, insertQ, cfg.ID, cfg.TenantID, cfg.Hostname, cfg.SMTPPort, cfg.SMTPSPort, cfg.IMAPPort, cfg.IMAPSPort, cfg.MaxMessageSize, cfg.TLSEnabled)
		if err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *MailServerRepository) UpdateServerConfig(ctx context.Context, tenantID uuid.UUID, req models.UpdateServerConfigRequest) (*models.MailServerConfig, error) {
	cfg, err := r.GetServerConfig(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if req.Hostname != "" {
		cfg.Hostname = req.Hostname
	}
	if req.SMTPPort != nil {
		cfg.SMTPPort = *req.SMTPPort
	}
	if req.SMTPSPort != nil {
		cfg.SMTPSPort = *req.SMTPSPort
	}
	if req.IMAPPort != nil {
		cfg.IMAPPort = *req.IMAPPort
	}
	if req.IMAPSPort != nil {
		cfg.IMAPSPort = *req.IMAPSPort
	}
	if req.MaxMessageSize != nil {
		cfg.MaxMessageSize = *req.MaxMessageSize
	}
	if req.TLSEnabled != nil {
		cfg.TLSEnabled = *req.TLSEnabled
	}
	if req.CertPath != "" {
		cfg.CertPath = req.CertPath
	}
	if req.KeyPath != "" {
		cfg.KeyPath = req.KeyPath
	}

	query := `UPDATE mail_server_configs SET hostname=$1, smtp_port=$2, smtps_port=$3, imap_port=$4, imaps_port=$5, max_message_size=$6, tls_enabled=$7, cert_path=$8, key_path=$9, updated_at=NOW() WHERE id=$10`
	_, err = r.db.ExecContext(ctx, query, cfg.Hostname, cfg.SMTPPort, cfg.SMTPSPort, cfg.IMAPPort, cfg.IMAPSPort, cfg.MaxMessageSize, cfg.TLSEnabled, cfg.CertPath, cfg.KeyPath, cfg.ID)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// --- Stats ---

func (r *MailServerRepository) GetStats(ctx context.Context, tenantID uuid.UUID) (*models.MailStats, error) {
	stats := &models.MailStats{}

	r.db.GetContext(ctx, &stats.TotalDomains, `SELECT COUNT(*) FROM mail_domains WHERE tenant_id = $1`, tenantID)
	r.db.GetContext(ctx, &stats.TotalAccounts, `SELECT COUNT(*) FROM mail_accounts WHERE tenant_id = $1`, tenantID)
	r.db.GetContext(ctx, &stats.TotalAliases, `SELECT COUNT(*) FROM mail_aliases WHERE tenant_id = $1`, tenantID)
	r.db.GetContext(ctx, &stats.QueueSize, `SELECT COUNT(*) FROM mail_queue WHERE tenant_id = $1 AND status = 'queued'`, tenantID)

	today := time.Now().Truncate(24 * time.Hour)
	r.db.GetContext(ctx, &stats.SentToday, `SELECT COUNT(*) FROM mail_queue WHERE tenant_id = $1 AND status = 'sent' AND sent_at >= $2`, tenantID, today)
	r.db.GetContext(ctx, &stats.FailedToday, `SELECT COUNT(*) FROM mail_queue WHERE tenant_id = $1 AND status = 'failed' AND created_at >= $2`, tenantID, today)
	r.db.GetContext(ctx, &stats.StorageUsedMB, `SELECT COALESCE(SUM(used_mb), 0) FROM mail_accounts WHERE tenant_id = $1`, tenantID)

	return stats, nil
}
