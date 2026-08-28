package repository

// Storage for the per-site PHP pool settings, the WordPress runtime identity
// and the staging environments added by migrations/pending/php_wordpress.sql.
//
// Every statement here was executed against a real PostgreSQL 16 with the whole
// migration sequence applied, and every one of them was PREPAREd first so that
// PostgreSQL, not a reviewer, confirmed the columns exist. The jobs table in
// this repository was queried in a shape that never existed on any install, and
// CREATE TABLE IF NOT EXISTS hid it; a PREPARE would have caught that on the
// first run.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

// PHPRuntimeRepository stores the per-pool settings that reach the pool file.
type PHPRuntimeRepository struct {
	db *sqlx.DB
}

// NewPHPRuntimeRepository builds the repository.
func NewPHPRuntimeRepository(db *sqlx.DB) *PHPRuntimeRepository {
	return &PHPRuntimeRepository{db: db}
}

// ErrNoSettings is returned when a pool has no settings row yet.
var ErrNoSettings = errors.New("this pool has no PHP settings recorded")

const poolSettingsColumns = `pool_id, tenant_id, memory_limit, max_execution_time,
	upload_max_filesize, extensions, post_max_size, max_input_time, max_file_uploads,
	timezone, display_errors, disabled_functions, open_basedir,
	applied_php_version, pool_file, socket_path, last_applied_at, last_error,
	created_at, updated_at`

// GetPoolSettings reads one pool's settings.
func (r *PHPRuntimeRepository) GetPoolSettings(ctx context.Context, poolID, tenantID string) (*models.PHPPoolSettings, error) {
	query := `SELECT ` + poolSettingsColumns + `
		FROM php_pool_settings WHERE pool_id = $1 AND tenant_id = $2`

	settings := &models.PHPPoolSettings{}
	err := r.db.QueryRowContext(ctx, query, poolID, tenantID).Scan(
		&settings.PoolID, &settings.TenantID, &settings.MemoryLimit, &settings.MaxExecutionTime,
		&settings.UploadMaxFilesize, pq.Array(&settings.Extensions), &settings.PostMaxSize,
		&settings.MaxInputTime, &settings.MaxFileUploads, &settings.Timezone,
		&settings.DisplayErrors, pq.Array(&settings.DisabledFunctions),
		pq.Array(&settings.OpenBasedir), &settings.AppliedPHPVersion, &settings.PoolFile,
		&settings.SocketPath, &settings.LastAppliedAt, &settings.LastError,
		&settings.CreatedAt, &settings.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSettings
	}
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// UpsertPoolSettings writes the settings a caller asked for. It does not touch
// the applied_* columns: those record what actually reached the disk and are
// written only by MarkApplied and MarkFailed, so the gap between "asked for"
// and "in force" stays visible.
func (r *PHPRuntimeRepository) UpsertPoolSettings(ctx context.Context, s *models.PHPPoolSettings) error {
	query := `
		INSERT INTO php_pool_settings (
			pool_id, tenant_id, memory_limit, max_execution_time, upload_max_filesize,
			extensions, post_max_size, max_input_time, max_file_uploads, timezone,
			display_errors, disabled_functions, open_basedir, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
		ON CONFLICT (pool_id) DO UPDATE SET
			memory_limit        = EXCLUDED.memory_limit,
			max_execution_time  = EXCLUDED.max_execution_time,
			upload_max_filesize = EXCLUDED.upload_max_filesize,
			extensions          = EXCLUDED.extensions,
			post_max_size       = EXCLUDED.post_max_size,
			max_input_time      = EXCLUDED.max_input_time,
			max_file_uploads    = EXCLUDED.max_file_uploads,
			timezone            = EXCLUDED.timezone,
			display_errors      = EXCLUDED.display_errors,
			disabled_functions  = EXCLUDED.disabled_functions,
			open_basedir        = EXCLUDED.open_basedir,
			updated_at          = NOW()`

	_, err := r.db.ExecContext(ctx, query,
		s.PoolID, s.TenantID, s.MemoryLimit, s.MaxExecutionTime, s.UploadMaxFilesize,
		pq.Array(s.Extensions), s.PostMaxSize, s.MaxInputTime, s.MaxFileUploads,
		s.Timezone, s.DisplayErrors, pq.Array(s.DisabledFunctions), pq.Array(s.OpenBasedir),
	)
	return err
}

// MarkApplied records that a pool file was written and FPM reloaded.
func (r *PHPRuntimeRepository) MarkApplied(ctx context.Context, poolID, tenantID, version, poolFile, socketPath string) error {
	query := `
		UPDATE php_pool_settings
		SET applied_php_version = $1, pool_file = $2, socket_path = $3,
		    last_applied_at = NOW(), last_error = NULL, updated_at = NOW()
		WHERE pool_id = $4 AND tenant_id = $5`
	_, err := r.db.ExecContext(ctx, query, version, poolFile, socketPath, poolID, tenantID)
	return err
}

// MarkFailed records why a pool change did not take, leaving applied_* alone:
// after a rollback the host is still running the previous version, and that is
// what applied_php_version must keep saying.
func (r *PHPRuntimeRepository) MarkFailed(ctx context.Context, poolID, tenantID, reason string) error {
	query := `UPDATE php_pool_settings SET last_error = $1, updated_at = NOW()
		WHERE pool_id = $2 AND tenant_id = $3`
	_, err := r.db.ExecContext(ctx, query, reason, poolID, tenantID)
	return err
}

// PoolForWebsite finds the pool bound to one website, which is how a site's
// chosen PHP version is answered.
func (r *PHPRuntimeRepository) PoolForWebsite(ctx context.Context, websiteID, tenantID string) (*models.PHPPool, error) {
	query := `SELECT id FROM php_pools WHERE website_id = $1 AND tenant_id = $2 AND is_active = TRUE
		ORDER BY created_at LIMIT 1`
	var id string
	err := r.db.QueryRowContext(ctx, query, websiteID, tenantID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no PHP-FPM pool is bound to website %s", websiteID)
	}
	if err != nil {
		return nil, err
	}
	return NewPHPRepository(r.db).GetPHPPool(ctx, id, tenantID)
}

// SetPoolVersion moves a pool onto a different PHP version.
func (r *PHPRuntimeRepository) SetPoolVersion(ctx context.Context, poolID, tenantID, phpVersionID string) error {
	query := `UPDATE php_pools SET php_version_id = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3`
	result, err := r.db.ExecContext(ctx, query, phpVersionID, poolID, tenantID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("pool %s not found for this tenant", poolID)
	}
	return nil
}

// SetWebsitePHPVersion keeps the websites table's php_version column in step,
// because that is the column the web server adapters read when they write a
// vhost.
func (r *PHPRuntimeRepository) SetWebsitePHPVersion(ctx context.Context, websiteID, tenantID, version string) error {
	query := `UPDATE websites SET php_version = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3`
	_, err := r.db.ExecContext(ctx, query, version, websiteID, tenantID)
	return err
}

// FindVersionByNumber resolves a version string ("8.3") to its php_versions row
// on one server.
func (r *PHPRuntimeRepository) FindVersionByNumber(ctx context.Context, tenantID, serverID, version string) (*models.PHPVersion, error) {
	query := `SELECT id FROM php_versions
		WHERE tenant_id = $1 AND server_id = $2 AND version = $3`
	var id string
	err := r.db.QueryRowContext(ctx, query, tenantID, serverID, version).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("PHP %s is not installed on this server", version)
	}
	if err != nil {
		return nil, err
	}
	return NewPHPRepository(r.db).GetPHPVersion(ctx, id, tenantID)
}

// SetVersionExtensions records the extensions enabled on a PHP version.
func (r *PHPRuntimeRepository) SetVersionExtensions(ctx context.Context, versionID, tenantID string, extensions []string) error {
	query := `UPDATE php_versions SET extensions = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3`
	_, err := r.db.ExecContext(ctx, query, pq.Array(extensions), versionID, tenantID)
	return err
}

// ---------------------------------------------------------------------------
// WordPress runtime identity and staging
// ---------------------------------------------------------------------------

// WordPressRuntimeRepository stores who a site's commands run as, and its
// staging environment.
type WordPressRuntimeRepository struct {
	db *sqlx.DB
}

// NewWordPressRuntimeRepository builds the repository.
func NewWordPressRuntimeRepository(db *sqlx.DB) *WordPressRuntimeRepository {
	return &WordPressRuntimeRepository{db: db}
}

// ErrNoRuntime is returned when a site has no runtime identity recorded, which
// means no WP-CLI command may run for it: there is no non-root user to run as.
var ErrNoRuntime = errors.New("this WordPress site has no system user recorded, " +
	"so no WP-CLI command can be run for it: the panel will not fall back to root")

// GetRuntime reads a site's runtime identity.
func (r *WordPressRuntimeRepository) GetRuntime(ctx context.Context, siteID, tenantID uuid.UUID) (*models.WordPressRuntime, error) {
	query := `SELECT site_id, tenant_id, run_as_user, run_as_group, php_version,
			installed_version, last_ran_as, last_command, last_ran_at, created_at, updated_at
		FROM wordpress_site_runtime WHERE site_id = $1 AND tenant_id = $2`

	runtime := &models.WordPressRuntime{}
	err := r.db.QueryRowContext(ctx, query, siteID, tenantID).Scan(
		&runtime.SiteID, &runtime.TenantID, &runtime.RunAsUser, &runtime.RunAsGroup,
		&runtime.PHPVersion, &runtime.InstalledVersion, &runtime.LastRanAs,
		&runtime.LastCommand, &runtime.LastRanAt, &runtime.CreatedAt, &runtime.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoRuntime
	}
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

// UpsertRuntime writes a site's runtime identity. The database itself refuses
// 'root' through a CHECK constraint, so a bug in the service layer becomes a
// failed write rather than a root WP-CLI run.
func (r *WordPressRuntimeRepository) UpsertRuntime(ctx context.Context, runtime *models.WordPressRuntime) error {
	query := `
		INSERT INTO wordpress_site_runtime (site_id, tenant_id, run_as_user, run_as_group,
			php_version, installed_version, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (site_id) DO UPDATE SET
			run_as_user       = EXCLUDED.run_as_user,
			run_as_group      = EXCLUDED.run_as_group,
			php_version       = EXCLUDED.php_version,
			installed_version = COALESCE(EXCLUDED.installed_version, wordpress_site_runtime.installed_version),
			updated_at        = NOW()`
	_, err := r.db.ExecContext(ctx, query,
		runtime.SiteID, runtime.TenantID, runtime.RunAsUser, runtime.RunAsGroup,
		runtime.PHPVersion, runtime.InstalledVersion,
	)
	return err
}

// RecordRun stores what the last WP-CLI command was and which identity it ran
// under, so that "which user did that run as" survives a log rotation.
func (r *WordPressRuntimeRepository) RecordRun(ctx context.Context, siteID, tenantID uuid.UUID, ranAs, command string) error {
	query := `UPDATE wordpress_site_runtime
		SET last_ran_as = $1, last_command = $2, last_ran_at = NOW(), updated_at = NOW()
		WHERE site_id = $3 AND tenant_id = $4`
	_, err := r.db.ExecContext(ctx, query, truncate(ranAs, 128), truncate(command, 255), siteID, tenantID)
	return err
}

// SetInstalledVersion records the core version WP-CLI reported.
func (r *WordPressRuntimeRepository) SetInstalledVersion(ctx context.Context, siteID, tenantID uuid.UUID, version string) error {
	query := `UPDATE wordpress_site_runtime SET installed_version = $1, updated_at = NOW()
		WHERE site_id = $2 AND tenant_id = $3`
	_, err := r.db.ExecContext(ctx, query, version, siteID, tenantID)
	return err
}

const stagingColumns = `id, tenant_id, production_site_id, staging_domain, staging_path,
	staging_url, staging_db_name, staging_db_user, staging_db_password, staging_db_host,
	status, block_indexing, last_clone_at, last_push_at, last_push_database,
	last_push_backup, last_push_db_backup, last_error, created_at, updated_at`

// GetStaging reads the staging environment for one production site.
func (r *WordPressRuntimeRepository) GetStaging(ctx context.Context, productionSiteID, tenantID uuid.UUID) (*models.WordPressStaging, error) {
	query := `SELECT ` + stagingColumns + `
		FROM wordpress_staging WHERE production_site_id = $1 AND tenant_id = $2`
	return r.scanStaging(r.db.QueryRowContext(ctx, query, productionSiteID, tenantID))
}

// CreateStaging records a staging environment.
func (r *WordPressRuntimeRepository) CreateStaging(ctx context.Context, s *models.WordPressStaging) error {
	query := `
		INSERT INTO wordpress_staging (tenant_id, production_site_id, staging_domain,
			staging_path, staging_url, staging_db_name, staging_db_user, staging_db_password,
			staging_db_host, status, block_indexing)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (production_site_id) DO UPDATE SET
			staging_domain      = EXCLUDED.staging_domain,
			staging_path        = EXCLUDED.staging_path,
			staging_url         = EXCLUDED.staging_url,
			staging_db_name     = EXCLUDED.staging_db_name,
			staging_db_user     = EXCLUDED.staging_db_user,
			staging_db_password = EXCLUDED.staging_db_password,
			staging_db_host     = EXCLUDED.staging_db_host,
			status              = EXCLUDED.status,
			block_indexing      = EXCLUDED.block_indexing,
			updated_at          = NOW()
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		s.TenantID, s.ProductionSiteID, s.StagingDomain, s.StagingPath, s.StagingURL,
		s.StagingDBName, s.StagingDBUser, s.StagingDBPassword, s.StagingDBHost,
		s.Status, s.BlockIndexing,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

// RecordClone marks a successful clone.
func (r *WordPressRuntimeRepository) RecordClone(ctx context.Context, stagingID, tenantID uuid.UUID) error {
	query := `UPDATE wordpress_staging
		SET last_clone_at = NOW(), status = 'ready', last_error = NULL, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, stagingID, tenantID)
	return err
}

// RecordPush stores the database decision that was made and where production
// was backed up before the push. This row is the answer to "who told the panel
// to overwrite the production database, and where is what it replaced".
func (r *WordPressRuntimeRepository) RecordPush(ctx context.Context, stagingID, tenantID uuid.UUID,
	databaseAction, filesBackup, dbBackup string) error {
	query := `UPDATE wordpress_staging
		SET last_push_at = NOW(), last_push_database = $1, last_push_backup = $2,
		    last_push_db_backup = $3, status = 'ready', last_error = NULL, updated_at = NOW()
		WHERE id = $4 AND tenant_id = $5`
	_, err := r.db.ExecContext(ctx, query, databaseAction, filesBackup, dbBackup, stagingID, tenantID)
	return err
}

// RecordStagingError stores why a clone or push failed.
func (r *WordPressRuntimeRepository) RecordStagingError(ctx context.Context, stagingID, tenantID uuid.UUID, reason string) error {
	query := `UPDATE wordpress_staging SET status = 'error', last_error = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3`
	_, err := r.db.ExecContext(ctx, query, reason, stagingID, tenantID)
	return err
}

// DeleteStaging removes a staging record.
func (r *WordPressRuntimeRepository) DeleteStaging(ctx context.Context, stagingID, tenantID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM wordpress_staging WHERE id = $1 AND tenant_id = $2`, stagingID, tenantID)
	return err
}

func (r *WordPressRuntimeRepository) scanStaging(row *sql.Row) (*models.WordPressStaging, error) {
	s := &models.WordPressStaging{}
	err := row.Scan(
		&s.ID, &s.TenantID, &s.ProductionSiteID, &s.StagingDomain, &s.StagingPath,
		&s.StagingURL, &s.StagingDBName, &s.StagingDBUser, &s.StagingDBPassword,
		&s.StagingDBHost, &s.Status, &s.BlockIndexing, &s.LastCloneAt, &s.LastPushAt,
		&s.LastPushDatabase, &s.LastPushBackup, &s.LastPushDBBackup, &s.LastError,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoStaging
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ErrNoStaging is returned when a site has no staging environment.
var ErrNoStaging = errors.New("this site has no staging environment")

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
