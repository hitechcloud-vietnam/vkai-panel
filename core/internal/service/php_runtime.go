package service

// The half of PHP management that touches the host.
//
// What service/php.go did before this file existed: it wrote rows. Every one of
// its methods - CreatePHPVersion, CreatePHPPool, InstallPHPExtension,
// UpdatePHPConfig - inserted or updated a database record and returned it. No
// PHP was ever installed, no pool file was ever written, no FPM was ever
// reloaded, and "InstallPHPExtension" was a row with is_installed hard-coded to
// true. A panel built on it reported PHP 8.3 with redis enabled on a host that
// had neither.
//
// This file is the other half. Everything here ends in a file on disk and a
// reloaded service, or in a rollback and an error that says the site is still
// serving what it was serving.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/phpfpm"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
)

// phpRuntime is the lazily built host-side half of PHPService.
type phpRuntime struct {
	once    sync.Once
	manager *phpfpm.Manager
	repo    *repository.PHPRuntimeRepository
	err     error
}

// ErrPHPRuntimeUnavailable is returned when the host could not be identified.
// It is a distinct error so a handler answers 503 with the reason rather than
// 500 with a stack trace - the panel still works, this one capability does not.
var ErrPHPRuntimeUnavailable = errors.New("PHP host management is unavailable on this machine")

// runtime builds the manager and the runtime repository on first use.
//
// Constructing it lazily rather than in NewPHPService is what keeps
// cmd/api/main.go unchanged: the service already holds the repository, the
// repository holds the connection, and the distribution is read from
// /etc/os-release at the moment somebody first asks for it.
func (s *PHPService) runtime() (*phpfpm.Manager, *repository.PHPRuntimeRepository, error) {
	s.rt.once.Do(func() {
		if s.phpRepo == nil {
			s.rt.err = fmt.Errorf("%w: this service was built without a database", ErrPHPRuntimeUnavailable)
			return
		}
		s.rt.repo = repository.NewPHPRuntimeRepository(s.phpRepo.DB())

		manager, err := phpfpm.New(phpfpm.Options{Logger: s.logger})
		if err != nil {
			s.rt.err = fmt.Errorf("%w: %v", ErrPHPRuntimeUnavailable, err)
			s.logger.Error("PHP host management is unavailable: the operating system could not "+
				"be identified, so no pool file can be written and no PHP can be installed",
				zap.Error(err))
			return
		}
		s.rt.manager = manager
		s.logger.Info("PHP host management ready",
			zap.String("distribution", manager.Distro().Pretty),
			zap.String("family", string(manager.Distro().Family)),
			zap.Bool("multi_version_supported", manager.Distro().SupportsMultiVersion()),
			zap.Strings("installed_versions", manager.InstalledVersions()))
	})
	if s.rt.err != nil {
		return nil, nil, s.rt.err
	}
	return s.rt.manager, s.rt.repo, nil
}

// SetRuntimeForTest replaces the host-side half with one a test controls. It is
// the only way a test can drive the write -> validate -> reload -> roll back
// path, because the real one needs php-fpm and systemd.
func (s *PHPService) SetRuntimeForTest(manager *phpfpm.Manager, repo *repository.PHPRuntimeRepository) {
	s.rt.once.Do(func() {})
	s.rt.manager = manager
	s.rt.repo = repo
	s.rt.err = nil
}

// ---------------------------------------------------------------------------
// The capability report
// ---------------------------------------------------------------------------

// PHPSystemReport is what GET /php/system answers: what this host is, what it
// can do, and what is installed right now.
type PHPSystemReport struct {
	Distribution          string `json:"distribution"`
	Family                string `json:"family"`
	PackageManager        string `json:"package_manager"`
	MultiVersionSupported bool   `json:"multi_version_supported"`
	// RefusalReason is set, and says why, when MultiVersionSupported is false.
	RefusalReason       string                 `json:"refusal_reason,omitempty"`
	Repository          string                 `json:"repository,omitempty"`
	InstalledVersions   []string               `json:"installed_versions"`
	InstallableVersions []string               `json:"installable_versions"`
	SupportMatrix       []phpfpm.FamilySupport `json:"support_matrix"`
}

// SystemReport answers "what can this panel really do with PHP on this host".
//
// It is served rather than documented because the difference between the seven
// families where multi-version PHP works and the two where it does not is a
// thing an operator needs at the moment they click "install PHP 8.3", not in a
// README they read once.
func (s *PHPService) SystemReport(ctx context.Context) (*PHPSystemReport, error) {
	manager, _, err := s.runtime()
	if err != nil {
		return nil, err
	}
	distro := manager.Distro()
	report := &PHPSystemReport{
		Distribution:          distro.Pretty,
		Family:                string(distro.Family),
		PackageManager:        distro.PackageManager,
		MultiVersionSupported: distro.SupportsMultiVersion(),
		RefusalReason:         distro.RefusalReason,
		Repository:            distro.Repository,
		InstalledVersions:     manager.InstalledVersions(),
		SupportMatrix:         phpfpm.SupportedFamiliesReport(),
	}
	if distro.SupportsMultiVersion() {
		report.InstallableVersions = phpfpm.SupportedVersions()
	}
	if report.InstalledVersions == nil {
		report.InstalledVersions = []string{}
	}
	if report.InstallableVersions == nil {
		report.InstallableVersions = []string{}
	}
	return report, nil
}

// ---------------------------------------------------------------------------
// Installing a version
// ---------------------------------------------------------------------------

// InstallPHPVersionRequest is the body of POST /php/install.
type InstallPHPVersionRequest struct {
	Version    string   `json:"version" binding:"required"`
	ServerID   string   `json:"server_id" binding:"required"`
	Extensions []string `json:"extensions"`
	SetDefault bool     `json:"set_default"`
}

// InstallVersion installs a PHP version on the host and then records it.
//
// The order is deliberate: the package manager runs first, and the database row
// is written only after the host actually has the binary. The old
// CreatePHPVersion wrote the row and stopped, which is how the panel came to
// list versions that did not exist.
func (s *PHPService) InstallVersion(ctx context.Context, req *InstallPHPVersionRequest, tenantID string) (*models.PHPVersion, error) {
	manager, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	if err := phpfpm.ValidateVersion(req.Version); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(req.ServerID); err != nil {
		return nil, fmt.Errorf("invalid server_id: %w", err)
	}

	if !manager.Distro().SupportsMultiVersion() {
		return nil, &phpfpm.ErrMultiVersionUnsupported{Distro: manager.Distro()}
	}

	layout, err := manager.Layout(req.Version)
	if err != nil {
		return nil, err
	}

	if err := manager.InstallVersion(ctx, req.Version, req.Extensions); err != nil {
		return nil, err
	}

	// Only now, with the binary on disk, does the panel claim the version.
	php := &models.PHPVersion{
		ID:         uuid.New().String(),
		Version:    req.Version,
		Path:       layout.CLIBinary,
		FPMPath:    layout.Binary,
		FPMConfig:  layout.MainConfig,
		IniPath:    layout.ConfDir,
		Extensions: normaliseExtensions(req.Extensions),
		IsActive:   true,
		IsDefault:  req.SetDefault,
		ServerID:   req.ServerID,
		TenantID:   tenantID,
	}
	if err := s.phpRepo.CreatePHPVersion(ctx, php); err != nil {
		return nil, fmt.Errorf("PHP %s was installed on this host but could not be recorded: %w",
			req.Version, err)
	}
	_ = runtimeRepo // the runtime repository is used by the pool paths below

	s.logger.Info("PHP version installed and recorded",
		zap.String("version", php.Version),
		zap.String("fpm_binary", php.FPMPath),
		zap.String("server_id", php.ServerID))
	return php, nil
}

// UninstallVersion removes a PHP version from the host and its record.
func (s *PHPService) UninstallVersion(ctx context.Context, id, tenantID string) error {
	manager, _, err := s.runtime()
	if err != nil {
		return err
	}
	php, err := s.phpRepo.GetPHPVersion(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get PHP version: %w", err)
	}
	if err := manager.RemoveVersion(ctx, php.Version); err != nil {
		return err
	}
	if err := s.phpRepo.DeletePHPVersion(ctx, id, tenantID); err != nil {
		return fmt.Errorf("PHP %s was removed from this host but its record could not be "+
			"deleted: %w", php.Version, err)
	}
	s.logger.Info("PHP version uninstalled", zap.String("version", php.Version))
	return nil
}

// ---------------------------------------------------------------------------
// Per-site pool settings
// ---------------------------------------------------------------------------

// PoolSettingsRequest is the body of PUT /php/pools/:id/settings.
//
// The four fields the task names are first and required-by-default; the rest
// have defaults that match a WordPress site that works.
type PoolSettingsRequest struct {
	MemoryLimit       string   `json:"memory_limit"`
	MaxExecutionTime  int      `json:"max_execution_time"`
	UploadMaxFilesize string   `json:"upload_max_filesize"`
	Extensions        []string `json:"extensions"`

	PostMaxSize       string   `json:"post_max_size"`
	MaxInputTime      int      `json:"max_input_time"`
	MaxFileUploads    int      `json:"max_file_uploads"`
	Timezone          string   `json:"timezone"`
	DisplayErrors     bool     `json:"display_errors"`
	DisabledFunctions []string `json:"disabled_functions"`
	OpenBasedir       []string `json:"open_basedir"`
}

// PoolSettingsResult is what an apply returns: what was asked for, what is now
// in force, and where it was written.
type PoolSettingsResult struct {
	Settings   *models.PHPPoolSettings `json:"settings"`
	Applied    models.AppliedState     `json:"applied"`
	PoolFile   string                  `json:"pool_file"`
	SocketPath string                  `json:"socket_path"`
	Service    string                  `json:"reloaded_service"`
	// RenderedPool is the pool file that was written, so an operator can see
	// exactly what the panel put on disk without shelling into the box.
	RenderedPool string `json:"rendered_pool"`
}

// GetPoolSettings reads a pool's settings, filling in the defaults for a pool
// that has never been configured.
func (s *PHPService) GetPoolSettings(ctx context.Context, poolID, tenantID string) (*models.PHPPoolSettings, error) {
	_, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	settings, err := runtimeRepo.GetPoolSettings(ctx, poolID, tenantID)
	if errors.Is(err, repository.ErrNoSettings) {
		return defaultPoolSettings(poolID, tenantID), nil
	}
	return settings, err
}

// defaultPoolSettings is what a pool gets before anybody configures it. The
// numbers are what a WordPress site with a page builder needs; the panel's own
// WordPress installer relies on them.
func defaultPoolSettings(poolID, tenantID string) *models.PHPPoolSettings {
	return &models.PHPPoolSettings{
		PoolID:            poolID,
		TenantID:          tenantID,
		MemoryLimit:       "256M",
		MaxExecutionTime:  30,
		UploadMaxFilesize: "64M",
		Extensions:        []string{},
		PostMaxSize:       "64M",
		MaxInputTime:      60,
		MaxFileUploads:    20,
		Timezone:          "UTC",
		DisabledFunctions: []string{},
		OpenBasedir:       []string{},
	}
}

// ApplyPoolSettings is the operation the task's requirement 3 describes: it
// rewrites the pool configuration, validates it, reloads FPM, and rolls back if
// the reload fails.
//
// The database is written in two steps around the host change, and the reason
// is the rollback. The requested settings are stored first, so an operator can
// see what was asked for even when the apply failed; the applied_* columns are
// written only after the reload succeeded, so they always describe the host and
// never the intent. After a rollback the two disagree, and that disagreement is
// the signal an operator needs.
func (s *PHPService) ApplyPoolSettings(ctx context.Context, poolID, tenantID string, req *PoolSettingsRequest) (*PoolSettingsResult, error) {
	manager, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}

	pool, err := s.phpRepo.GetPHPPool(ctx, poolID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get PHP-FPM pool: %w", err)
	}
	version, err := s.phpRepo.GetPHPVersion(ctx, pool.PHPVersionID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the pool's PHP version: %w", err)
	}

	settings := mergePoolSettings(defaultPoolSettings(poolID, tenantID), req)

	// Step 1: record the intent. A failure here means nothing has been touched
	// on the host yet, which is the right place to stop.
	if err := runtimeRepo.UpsertPoolSettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("could not record the PHP settings: %w", err)
	}

	spec := poolSpecFrom(pool, version.Version, settings)
	rendered, err := spec.Render()
	if err != nil {
		_ = runtimeRepo.MarkFailed(ctx, poolID, tenantID, err.Error())
		return nil, err
	}

	// Step 2: the host. ApplyPool rolls back internally on any failure.
	applied, err := manager.ApplyPool(ctx, spec, "")
	if err != nil {
		_ = runtimeRepo.MarkFailed(ctx, poolID, tenantID, err.Error())
		return nil, err
	}

	// Step 3: record what is now in force.
	if err := runtimeRepo.MarkApplied(ctx, poolID, tenantID, version.Version,
		applied.PoolPath, applied.SocketPath); err != nil {
		return nil, fmt.Errorf("PHP %s settings are in force for pool %s but could not be "+
			"recorded: %w", version.Version, pool.Name, err)
	}

	// The extension set is enforced at the version level, because PHP loads
	// extensions in the FPM master process. This is where that happens; the
	// pool file records the request. A failure here is not a rollback of the
	// pool - the pool is correct and serving - so it is reported and the
	// settings stand.
	if len(settings.Extensions) > 0 {
		if err := manager.EnsureExtensions(ctx, version.Version, settings.Extensions); err != nil {
			s.logger.Warn("the pool settings were applied but the extension set could not be "+
				"installed for this PHP version",
				zap.String("pool", pool.Name),
				zap.String("version", version.Version),
				zap.Strings("extensions", settings.Extensions),
				zap.Error(err))
			_ = runtimeRepo.MarkFailed(ctx, poolID, tenantID,
				"pool settings applied; extensions not installed: "+err.Error())
		} else {
			_ = runtimeRepo.SetVersionExtensions(ctx, version.ID, tenantID,
				mergeStrings(version.Extensions, settings.Extensions))
		}
	}

	stored, err := runtimeRepo.GetPoolSettings(ctx, poolID, tenantID)
	if err != nil {
		stored = settings
	}

	layout, _ := manager.Layout(version.Version)
	return &PoolSettingsResult{
		Settings:     stored,
		Applied:      stored.Applied(),
		PoolFile:     applied.PoolPath,
		SocketPath:   applied.SocketPath,
		Service:      layout.Service,
		RenderedPool: string(rendered),
	}, nil
}

// ---------------------------------------------------------------------------
// Switching a site's PHP version
// ---------------------------------------------------------------------------

// SwitchVersionRequest is the body of PUT /php/sites/:website_id/version.
type SwitchVersionRequest struct {
	Version  string `json:"version" binding:"required"`
	ServerID string `json:"server_id" binding:"required"`
}

// SiteVersionResult is what a switch returns.
type SiteVersionResult struct {
	WebsiteID       string              `json:"website_id"`
	PoolName        string              `json:"pool_name"`
	PreviousVersion string              `json:"previous_version"`
	Version         string              `json:"version"`
	PoolFile        string              `json:"pool_file"`
	SocketPath      string              `json:"socket_path"`
	Service         string              `json:"reloaded_service"`
	Applied         models.AppliedState `json:"applied"`
	RenderedPool    string              `json:"rendered_pool"`
}

// SetSiteVersion moves one website onto a different PHP version.
//
// This is the feature customers migrate for, and it is four things that must
// all happen or none of them:
//
//	the pool file moves from /etc/php/<old>/fpm/pool.d to /etc/php/<new>/...
//	both FPM services reload
//	php_pools.php_version_id points at the new version
//	websites.php_version says the new version, because that is the column the
//	web server adapters read when they rewrite the vhost's fastcgi_pass
//
// The host change goes first and rolls itself back on failure, so a database
// that could not be updated leaves a site running the OLD version with the OLD
// record - consistent, if not what was asked for - rather than a site whose
// vhost points at a socket that no longer exists.
func (s *PHPService) SetSiteVersion(ctx context.Context, websiteID, tenantID string, req *SwitchVersionRequest) (*SiteVersionResult, error) {
	manager, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	if err := phpfpm.ValidateVersion(req.Version); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(websiteID); err != nil {
		return nil, fmt.Errorf("invalid website id: %w", err)
	}

	pool, err := runtimeRepo.PoolForWebsite(ctx, websiteID, tenantID)
	if err != nil {
		return nil, err
	}
	current, err := s.phpRepo.GetPHPVersion(ctx, pool.PHPVersionID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the pool's current PHP version: %w", err)
	}
	target, err := runtimeRepo.FindVersionByNumber(ctx, tenantID, req.ServerID, req.Version)
	if err != nil {
		return nil, err
	}
	if current.Version == target.Version {
		return nil, fmt.Errorf("this site already runs PHP %s", target.Version)
	}

	settings, err := s.GetPoolSettings(ctx, pool.ID, tenantID)
	if err != nil {
		return nil, err
	}

	spec := poolSpecFrom(pool, target.Version, settings)
	rendered, err := spec.Render()
	if err != nil {
		return nil, err
	}

	// The host, with the old version passed in so a failure restores the pool
	// file where it was and reloads the service that was serving it.
	applied, err := manager.ApplyPool(ctx, spec, current.Version)
	if err != nil {
		_ = runtimeRepo.MarkFailed(ctx, pool.ID, tenantID, err.Error())
		return nil, fmt.Errorf("switching %s from PHP %s to PHP %s: %w",
			pool.Name, current.Version, target.Version, err)
	}

	if err := runtimeRepo.SetPoolVersion(ctx, pool.ID, tenantID, target.ID); err != nil {
		return nil, fmt.Errorf("the pool now runs PHP %s but the panel could not record it: %w",
			target.Version, err)
	}
	if err := runtimeRepo.SetWebsitePHPVersion(ctx, websiteID, tenantID, target.Version); err != nil {
		return nil, fmt.Errorf("the pool now runs PHP %s but the website record still says %s, "+
			"so the next vhost rewrite would point at the old socket: %w",
			target.Version, current.Version, err)
	}
	if err := runtimeRepo.MarkApplied(ctx, pool.ID, tenantID, target.Version,
		applied.PoolPath, applied.SocketPath); err != nil {
		s.logger.Warn("the version switch succeeded but could not be recorded",
			zap.String("pool", pool.Name), zap.Error(err))
	}

	layout, _ := manager.Layout(target.Version)
	s.logger.Info("site PHP version switched",
		zap.String("website_id", websiteID),
		zap.String("pool", pool.Name),
		zap.String("from", current.Version),
		zap.String("to", target.Version),
		zap.String("pool_file", applied.PoolPath),
		zap.String("socket", applied.SocketPath))

	stored, err := runtimeRepo.GetPoolSettings(ctx, pool.ID, tenantID)
	if err != nil {
		stored = settings
	}

	return &SiteVersionResult{
		WebsiteID:       websiteID,
		PoolName:        pool.Name,
		PreviousVersion: current.Version,
		Version:         target.Version,
		PoolFile:        applied.PoolPath,
		SocketPath:      applied.SocketPath,
		Service:         layout.Service,
		Applied:         stored.Applied(),
		RenderedPool:    string(rendered),
	}, nil
}

// GetSiteVersion reports which PHP version a website's pool runs.
func (s *PHPService) GetSiteVersion(ctx context.Context, websiteID, tenantID string) (*SiteVersionResult, error) {
	manager, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	pool, err := runtimeRepo.PoolForWebsite(ctx, websiteID, tenantID)
	if err != nil {
		return nil, err
	}
	version, err := s.phpRepo.GetPHPVersion(ctx, pool.PHPVersionID, tenantID)
	if err != nil {
		return nil, err
	}
	settings, err := s.GetPoolSettings(ctx, pool.ID, tenantID)
	if err != nil {
		return nil, err
	}
	layout, err := manager.Layout(version.Version)
	if err != nil {
		return nil, err
	}
	return &SiteVersionResult{
		WebsiteID:  websiteID,
		PoolName:   pool.Name,
		Version:    version.Version,
		PoolFile:   layout.PoolPath(pool.Name),
		SocketPath: layout.SocketPath(pool.Name),
		Service:    layout.Service,
		Applied:    settings.Applied(),
	}, nil
}

// ---------------------------------------------------------------------------
// Translation between the database rows and the pool file
// ---------------------------------------------------------------------------

// poolSpecFrom builds the typed pool specification from the pool row and its
// settings. This is the single place where a database value becomes something
// that will be written into /etc, so every field crosses it exactly once.
func poolSpecFrom(pool *models.PHPPool, version string, settings *models.PHPPoolSettings) *phpfpm.PoolSpec {
	spec := &phpfpm.PoolSpec{
		Name:                 pool.Name,
		Version:              version,
		User:                 pool.User,
		Group:                pool.Group,
		ListenOwner:          pool.ListenOwner,
		ListenGroup:          pool.ListenGroup,
		ListenMode:           pool.ListenMode,
		PM:                   pool.PM,
		PMMaxChildren:        pool.PMMaxChildren,
		PMStartServers:       pool.PMStartServers,
		PMMinSpareServers:    pool.PMMinSpareServers,
		PMMaxSpareServers:    pool.PMMaxSpareServers,
		PMMaxRequests:        pool.PMMaxRequests,
		PMProcessIdleTimeout: pool.PMProcessIdleTimeout,
		AccessLog:            pool.AccessLog,
		ErrorLog:             pool.ErrorLog,
		Env:                  pool.Env,

		MemoryLimit:       settings.MemoryLimit,
		MaxExecutionTime:  settings.MaxExecutionTime,
		MaxInputTime:      settings.MaxInputTime,
		UploadMaxFilesize: settings.UploadMaxFilesize,
		PostMaxSize:       settings.PostMaxSize,
		MaxFileUploads:    settings.MaxFileUploads,
		Timezone:          settings.Timezone,
		DisplayErrors:     settings.DisplayErrors,
		Extensions:        settings.Extensions,
		DisabledFunctions: settings.DisabledFunctions,
		OpenBasedir:       settings.OpenBasedir,
	}
	if spec.PM == "" {
		spec.PM = "dynamic"
	}
	if spec.PMMaxChildren == 0 {
		spec.PMMaxChildren = 10
	}
	if spec.PM == "dynamic" {
		if spec.PMStartServers == 0 {
			spec.PMStartServers = 2
		}
		if spec.PMMinSpareServers == 0 {
			spec.PMMinSpareServers = 1
		}
		if spec.PMMaxSpareServers == 0 {
			spec.PMMaxSpareServers = 3
		}
	}
	if spec.ListenMode == "" {
		spec.ListenMode = "0660"
	}
	return spec
}

// mergePoolSettings applies a request onto the defaults. An omitted field keeps
// the default rather than becoming zero: a PUT that leaves out memory_limit
// must not silently set it to the empty string, which renders no directive at
// all and gives the site php.ini's 128M.
func mergePoolSettings(base *models.PHPPoolSettings, req *PoolSettingsRequest) *models.PHPPoolSettings {
	out := *base
	if req == nil {
		return &out
	}
	if req.MemoryLimit != "" {
		out.MemoryLimit = req.MemoryLimit
	}
	if req.MaxExecutionTime != 0 {
		out.MaxExecutionTime = req.MaxExecutionTime
	}
	if req.UploadMaxFilesize != "" {
		out.UploadMaxFilesize = req.UploadMaxFilesize
	}
	if req.Extensions != nil {
		out.Extensions = normaliseExtensions(req.Extensions)
	}
	if req.PostMaxSize != "" {
		out.PostMaxSize = req.PostMaxSize
	}
	if req.MaxInputTime != 0 {
		out.MaxInputTime = req.MaxInputTime
	}
	if req.MaxFileUploads != 0 {
		out.MaxFileUploads = req.MaxFileUploads
	}
	if req.Timezone != "" {
		out.Timezone = req.Timezone
	}
	out.DisplayErrors = req.DisplayErrors
	if req.DisabledFunctions != nil {
		out.DisabledFunctions = req.DisabledFunctions
	}
	if req.OpenBasedir != nil {
		out.OpenBasedir = req.OpenBasedir
	}
	return &out
}

// normaliseExtensions lowercases, trims and de-duplicates an extension list so
// that "Redis", "redis " and "redis" are one entry and one package.
func normaliseExtensions(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// mergeStrings unions two lists, preserving order.
func mergeStrings(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, value := range list {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
