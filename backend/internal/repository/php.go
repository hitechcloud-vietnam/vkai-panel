package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
)

type PHPRepository struct {
	db *sqlx.DB
}

func NewPHPRepository(db *sqlx.DB) *PHPRepository {
	return &PHPRepository{db: db}
}

// CreatePHPVersion creates a new PHP version
func (r *PHPRepository) CreatePHPVersion(ctx context.Context, php *models.PHPVersion) error {
	query := `
		INSERT INTO php_versions (id, version, path, fpm_path, fpm_config, ini_path, extensions, is_active, is_default, server_id, tenant_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	extensionsJSON, err := json.Marshal(php.Extensions)
	if err != nil {
		return fmt.Errorf("failed to marshal extensions: %w", err)
	}

	now := time.Now()
	php.CreatedAt = now
	php.UpdatedAt = now

	_, err = r.db.ExecContext(ctx, query,
		php.ID, php.Version, php.Path, php.FPMPath, php.FPMConfig, php.IniPath,
		string(extensionsJSON), php.IsActive, php.IsDefault, php.ServerID, php.TenantID,
		php.CreatedAt, php.UpdatedAt,
	)

	return err
}

// GetPHPVersion gets a PHP version by ID
func (r *PHPRepository) GetPHPVersion(ctx context.Context, id, tenantID string) (*models.PHPVersion, error) {
	query := `
		SELECT id, version, path, fpm_path, fpm_config, ini_path, extensions, is_active, is_default, server_id, tenant_id, created_at, updated_at
		FROM php_versions
		WHERE id = $1 AND tenant_id = $2
	`

	var php models.PHPVersion
	var extensionsJSON string

	err := r.db.QueryRowContext(ctx, query, id, tenantID).Scan(
		&php.ID, &php.Version, &php.Path, &php.FPMPath, &php.FPMConfig, &php.IniPath,
		&extensionsJSON, &php.IsActive, &php.IsDefault, &php.ServerID, &php.TenantID,
		&php.CreatedAt, &php.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(extensionsJSON), &php.Extensions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extensions: %w", err)
	}

	return &php, nil
}

// ListPHPVersions lists all PHP versions for a tenant
func (r *PHPRepository) ListPHPVersions(ctx context.Context, tenantID, serverID string) ([]*models.PHPVersion, error) {
	query := `
		SELECT id, version, path, fpm_path, fpm_config, ini_path, extensions, is_active, is_default, server_id, tenant_id, created_at, updated_at
		FROM php_versions
		WHERE tenant_id = $1
	`

	args := []interface{}{tenantID}
	if serverID != "" {
		query += " AND server_id = $2"
		args = append(args, serverID)
	}

	query += " ORDER BY version DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var phpVersions []*models.PHPVersion
	for rows.Next() {
		var php models.PHPVersion
		var extensionsJSON string

		err := rows.Scan(
			&php.ID, &php.Version, &php.Path, &php.FPMPath, &php.FPMConfig, &php.IniPath,
			&extensionsJSON, &php.IsActive, &php.IsDefault, &php.ServerID, &php.TenantID,
			&php.CreatedAt, &php.UpdatedAt,
		)

		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(extensionsJSON), &php.Extensions); err != nil {
			return nil, fmt.Errorf("failed to unmarshal extensions: %w", err)
		}

		phpVersions = append(phpVersions, &php)
	}

	return phpVersions, nil
}

// UpdatePHPVersion updates a PHP version
func (r *PHPRepository) UpdatePHPVersion(ctx context.Context, php *models.PHPVersion) error {
	query := `
		UPDATE php_versions
		SET is_active = $1, is_default = $2, updated_at = $3
		WHERE id = $4 AND tenant_id = $5
	`

	php.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		php.IsActive, php.IsDefault, php.UpdatedAt, php.ID, php.TenantID,
	)

	return err
}

// DeletePHPVersion deletes a PHP version
func (r *PHPRepository) DeletePHPVersion(ctx context.Context, id, tenantID string) error {
	query := `DELETE FROM php_versions WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

// CreatePHPPool creates a new PHP-FPM pool
func (r *PHPRepository) CreatePHPPool(ctx context.Context, pool *models.PHPPool) error {
	query := `
		INSERT INTO php_pools (id, name, php_version_id, "user", "group", listen, listen_owner, listen_group, listen_mode, pm, pm_max_children, pm_start_servers, pm_min_spare_servers, pm_max_spare_servers, pm_max_requests, pm_process_idle_timeout, status_path, access_log, error_log, php_admin_flag, php_value, php_admin_value, env, is_active, website_id, server_id, tenant_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
	`

	phpAdminFlagJSON, _ := json.Marshal(pool.PhpAdminFlag)
	phpValueJSON, _ := json.Marshal(pool.PhpValue)
	phpAdminValueJSON, _ := json.Marshal(pool.PhpAdminValue)
	envJSON, _ := json.Marshal(pool.Env)

	now := time.Now()
	pool.CreatedAt = now
	pool.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		pool.ID, pool.Name, pool.PHPVersionID, pool.User, pool.Group, pool.Listen,
		pool.ListenOwner, pool.ListenGroup, pool.ListenMode, pool.PM, pool.PMMaxChildren,
		pool.PMStartServers, pool.PMMinSpareServers, pool.PMMaxSpareServers, pool.PMMaxRequests,
		pool.PMProcessIdleTimeout, pool.StatusPath, pool.AccessLog, pool.ErrorLog,
		string(phpAdminFlagJSON), string(phpValueJSON), string(phpAdminValueJSON), string(envJSON),
		pool.IsActive, pool.WebsiteID, pool.ServerID, pool.TenantID,
		pool.CreatedAt, pool.UpdatedAt,
	)

	return err
}

// GetPHPPool gets a PHP-FPM pool by ID
func (r *PHPRepository) GetPHPPool(ctx context.Context, id, tenantID string) (*models.PHPPool, error) {
	query := `
		SELECT id, name, php_version_id, "user", "group", listen, listen_owner, listen_group, listen_mode, pm, pm_max_children, pm_start_servers, pm_min_spare_servers, pm_max_spare_servers, pm_max_requests, pm_process_idle_timeout, status_path, access_log, error_log, php_admin_flag, php_value, php_admin_value, env, is_active, website_id, server_id, tenant_id, created_at, updated_at
		FROM php_pools
		WHERE id = $1 AND tenant_id = $2
	`

	var pool models.PHPPool
	var phpAdminFlagJSON, phpValueJSON, phpAdminValueJSON, envJSON string

	err := r.db.QueryRowContext(ctx, query, id, tenantID).Scan(
		&pool.ID, &pool.Name, &pool.PHPVersionID, &pool.User, &pool.Group, &pool.Listen,
		&pool.ListenOwner, &pool.ListenGroup, &pool.ListenMode, &pool.PM, &pool.PMMaxChildren,
		&pool.PMStartServers, &pool.PMMinSpareServers, &pool.PMMaxSpareServers, &pool.PMMaxRequests,
		&pool.PMProcessIdleTimeout, &pool.StatusPath, &pool.AccessLog, &pool.ErrorLog,
		&phpAdminFlagJSON, &phpValueJSON, &phpAdminValueJSON, &envJSON,
		&pool.IsActive, &pool.WebsiteID, &pool.ServerID, &pool.TenantID,
		&pool.CreatedAt, &pool.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(phpAdminFlagJSON), &pool.PhpAdminFlag)
	json.Unmarshal([]byte(phpValueJSON), &pool.PhpValue)
	json.Unmarshal([]byte(phpAdminValueJSON), &pool.PhpAdminValue)
	json.Unmarshal([]byte(envJSON), &pool.Env)

	return &pool, nil
}

// ListPHPPools lists all PHP-FPM pools for a tenant
func (r *PHPRepository) ListPHPPools(ctx context.Context, tenantID, serverID, websiteID string) ([]*models.PHPPool, error) {
	query := `
		SELECT id, name, php_version_id, "user", "group", listen, listen_owner, listen_group, listen_mode, pm, pm_max_children, pm_start_servers, pm_min_spare_servers, pm_max_spare_servers, pm_max_requests, pm_process_idle_timeout, status_path, access_log, error_log, php_admin_flag, php_value, php_admin_value, env, is_active, website_id, server_id, tenant_id, created_at, updated_at
		FROM php_pools
		WHERE tenant_id = $1
	`

	args := []interface{}{tenantID}
	argIndex := 2

	if serverID != "" {
		query += fmt.Sprintf(" AND server_id = $%d", argIndex)
		args = append(args, serverID)
		argIndex++
	}

	if websiteID != "" {
		query += fmt.Sprintf(" AND website_id = $%d", argIndex)
		args = append(args, websiteID)
		argIndex++
	}

	query += " ORDER BY name ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []*models.PHPPool
	for rows.Next() {
		var pool models.PHPPool
		var phpAdminFlagJSON, phpValueJSON, phpAdminValueJSON, envJSON string

		err := rows.Scan(
			&pool.ID, &pool.Name, &pool.PHPVersionID, &pool.User, &pool.Group, &pool.Listen,
			&pool.ListenOwner, &pool.ListenGroup, &pool.ListenMode, &pool.PM, &pool.PMMaxChildren,
			&pool.PMStartServers, &pool.PMMinSpareServers, &pool.PMMaxSpareServers, &pool.PMMaxRequests,
			&pool.PMProcessIdleTimeout, &pool.StatusPath, &pool.AccessLog, &pool.ErrorLog,
			&phpAdminFlagJSON, &phpValueJSON, &phpAdminValueJSON, &envJSON,
			&pool.IsActive, &pool.WebsiteID, &pool.ServerID, &pool.TenantID,
			&pool.CreatedAt, &pool.UpdatedAt,
		)

		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(phpAdminFlagJSON), &pool.PhpAdminFlag)
		json.Unmarshal([]byte(phpValueJSON), &pool.PhpValue)
		json.Unmarshal([]byte(phpAdminValueJSON), &pool.PhpAdminValue)
		json.Unmarshal([]byte(envJSON), &pool.Env)

		pools = append(pools, &pool)
	}

	return pools, nil
}

// UpdatePHPPool updates a PHP-FPM pool
func (r *PHPRepository) UpdatePHPPool(ctx context.Context, pool *models.PHPPool) error {
	query := `
		UPDATE php_pools
		SET "user" = $1, "group" = $2, listen = $3, listen_owner = $4, listen_group = $5, listen_mode = $6, pm = $7, pm_max_children = $8, pm_start_servers = $9, pm_min_spare_servers = $10, pm_max_spare_servers = $11, pm_max_requests = $12, pm_process_idle_timeout = $13, status_path = $14, access_log = $15, error_log = $16, php_admin_flag = $17, php_value = $18, php_admin_value = $19, env = $20, is_active = $21, updated_at = $22
		WHERE id = $23 AND tenant_id = $24
	`

	phpAdminFlagJSON, _ := json.Marshal(pool.PhpAdminFlag)
	phpValueJSON, _ := json.Marshal(pool.PhpValue)
	phpAdminValueJSON, _ := json.Marshal(pool.PhpAdminValue)
	envJSON, _ := json.Marshal(pool.Env)

	pool.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		pool.User, pool.Group, pool.Listen, pool.ListenOwner, pool.ListenGroup, pool.ListenMode,
		pool.PM, pool.PMMaxChildren, pool.PMStartServers, pool.PMMinSpareServers, pool.PMMaxSpareServers,
		pool.PMMaxRequests, pool.PMProcessIdleTimeout, pool.StatusPath, pool.AccessLog, pool.ErrorLog,
		string(phpAdminFlagJSON), string(phpValueJSON), string(phpAdminValueJSON), string(envJSON),
		pool.IsActive, pool.UpdatedAt, pool.ID, pool.TenantID,
	)

	return err
}

// DeletePHPPool deletes a PHP-FPM pool
func (r *PHPRepository) DeletePHPPool(ctx context.Context, id, tenantID string) error {
	query := `DELETE FROM php_pools WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

// CreatePHPExtension creates a new PHP extension record
func (r *PHPRepository) CreatePHPExtension(ctx context.Context, ext *models.PHPExtension) error {
	query := `
		INSERT INTO php_extensions (id, name, version, description, is_installed, is_enabled, php_version_id, server_id, tenant_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	now := time.Now()
	ext.CreatedAt = now
	ext.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		ext.ID, ext.Name, ext.Version, ext.Description, ext.IsInstalled, ext.IsEnabled,
		ext.PHPVersionID, ext.ServerID, ext.TenantID, ext.CreatedAt, ext.UpdatedAt,
	)

	return err
}

// GetPHPExtension gets a PHP extension by ID
func (r *PHPRepository) GetPHPExtension(ctx context.Context, id, tenantID string) (*models.PHPExtension, error) {
	query := `
		SELECT id, name, version, description, is_installed, is_enabled, php_version_id, server_id, tenant_id, created_at, updated_at
		FROM php_extensions
		WHERE id = $1 AND tenant_id = $2
	`

	var ext models.PHPExtension

	err := r.db.QueryRowContext(ctx, query, id, tenantID).Scan(
		&ext.ID, &ext.Name, &ext.Version, &ext.Description, &ext.IsInstalled, &ext.IsEnabled,
		&ext.PHPVersionID, &ext.ServerID, &ext.TenantID, &ext.CreatedAt, &ext.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &ext, nil
}

// ListPHPExtensions lists all PHP extensions for a PHP version
func (r *PHPRepository) ListPHPExtensions(ctx context.Context, phpVersionID, tenantID string) ([]*models.PHPExtension, error) {
	query := `
		SELECT id, name, version, description, is_installed, is_enabled, php_version_id, server_id, tenant_id, created_at, updated_at
		FROM php_extensions
		WHERE php_version_id = $1 AND tenant_id = $2
		ORDER BY name ASC
	`

	rows, err := r.db.QueryContext(ctx, query, phpVersionID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var extensions []*models.PHPExtension
	for rows.Next() {
		var ext models.PHPExtension

		err := rows.Scan(
			&ext.ID, &ext.Name, &ext.Version, &ext.Description, &ext.IsInstalled, &ext.IsEnabled,
			&ext.PHPVersionID, &ext.ServerID, &ext.TenantID, &ext.CreatedAt, &ext.UpdatedAt,
		)

		if err != nil {
			return nil, err
		}

		extensions = append(extensions, &ext)
	}

	return extensions, nil
}

// UpdatePHPExtension updates a PHP extension
func (r *PHPRepository) UpdatePHPExtension(ctx context.Context, ext *models.PHPExtension) error {
	query := `
		UPDATE php_extensions
		SET is_installed = $1, is_enabled = $2, updated_at = $3
		WHERE id = $4 AND tenant_id = $5
	`

	ext.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		ext.IsInstalled, ext.IsEnabled, ext.UpdatedAt, ext.ID, ext.TenantID,
	)

	return err
}

// DeletePHPExtension deletes a PHP extension
func (r *PHPRepository) DeletePHPExtension(ctx context.Context, id, tenantID string) error {
	query := `DELETE FROM php_extensions WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}

// CreatePHPConfig creates a new PHP configuration
func (r *PHPRepository) CreatePHPConfig(ctx context.Context, config *models.PHPConfig) error {
	query := `
		INSERT INTO php_configs (id, php_version_id, memory_limit, max_execution_time, max_input_time, post_max_size, upload_max_filesize, max_file_uploads, error_reporting, display_errors, log_errors, error_log, date_format, timezone, opcache_enabled, opcache_memory, opcache_max_files, opcache_revalidate_freq, custom_settings, server_id, tenant_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
	`

	customSettingsJSON, _ := json.Marshal(config.CustomSettings)

	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		config.ID, config.PHPVersionID, config.MemoryLimit, config.MaxExecutionTime, config.MaxInputTime,
		config.PostMaxSize, config.UploadMaxFilesize, config.MaxFileUploads, config.ErrorReporting,
		config.DisplayErrors, config.LogErrors, config.ErrorLog, config.DateFormat, config.Timezone,
		config.OPcacheEnabled, config.OPcacheMemory, config.OPcacheMaxFiles, config.OPcacheRevalidateFreq,
		string(customSettingsJSON), config.ServerID, config.TenantID, config.CreatedAt, config.UpdatedAt,
	)

	return err
}

// GetPHPConfig gets PHP configuration for a PHP version
func (r *PHPRepository) GetPHPConfig(ctx context.Context, phpVersionID, tenantID string) (*models.PHPConfig, error) {
	query := `
		SELECT id, php_version_id, memory_limit, max_execution_time, max_input_time, post_max_size, upload_max_filesize, max_file_uploads, error_reporting, display_errors, log_errors, error_log, date_format, timezone, opcache_enabled, opcache_memory, opcache_max_files, opcache_revalidate_freq, custom_settings, server_id, tenant_id, created_at, updated_at
		FROM php_configs
		WHERE php_version_id = $1 AND tenant_id = $2
	`

	var config models.PHPConfig
	var customSettingsJSON string

	err := r.db.QueryRowContext(ctx, query, phpVersionID, tenantID).Scan(
		&config.ID, &config.PHPVersionID, &config.MemoryLimit, &config.MaxExecutionTime, &config.MaxInputTime,
		&config.PostMaxSize, &config.UploadMaxFilesize, &config.MaxFileUploads, &config.ErrorReporting,
		&config.DisplayErrors, &config.LogErrors, &config.ErrorLog, &config.DateFormat, &config.Timezone,
		&config.OPcacheEnabled, &config.OPcacheMemory, &config.OPcacheMaxFiles, &config.OPcacheRevalidateFreq,
		&customSettingsJSON, &config.ServerID, &config.TenantID, &config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(customSettingsJSON), &config.CustomSettings)

	return &config, nil
}

// UpdatePHPConfig updates PHP configuration
func (r *PHPRepository) UpdatePHPConfig(ctx context.Context, config *models.PHPConfig) error {
	query := `
		UPDATE php_configs
		SET memory_limit = $1, max_execution_time = $2, max_input_time = $3, post_max_size = $4, upload_max_filesize = $5, max_file_uploads = $6, error_reporting = $7, display_errors = $8, log_errors = $9, error_log = $10, date_format = $11, timezone = $12, opcache_enabled = $13, opcache_memory = $14, opcache_max_files = $15, opcache_revalidate_freq = $16, custom_settings = $17, updated_at = $18
		WHERE id = $19 AND tenant_id = $20
	`

	customSettingsJSON, _ := json.Marshal(config.CustomSettings)

	config.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		config.MemoryLimit, config.MaxExecutionTime, config.MaxInputTime, config.PostMaxSize,
		config.UploadMaxFilesize, config.MaxFileUploads, config.ErrorReporting, config.DisplayErrors,
		config.LogErrors, config.ErrorLog, config.DateFormat, config.Timezone, config.OPcacheEnabled,
		config.OPcacheMemory, config.OPcacheMaxFiles, config.OPcacheRevalidateFreq,
		string(customSettingsJSON), config.UpdatedAt, config.ID, config.TenantID,
	)

	return err
}

// DeletePHPConfig deletes PHP configuration
func (r *PHPRepository) DeletePHPConfig(ctx context.Context, id, tenantID string) error {
	query := `DELETE FROM php_configs WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.ExecContext(ctx, query, id, tenantID)
	return err
}
