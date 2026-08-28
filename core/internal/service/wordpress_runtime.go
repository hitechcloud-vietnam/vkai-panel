package service

// The half of the WordPress toolkit that touches the host.
//
// What service/wordpress.go did before this file existed: it wrote rows.
// "Create" inserted a wordpress_sites record and returned it - no download, no
// database, no wp-config.php, no salts, no installer. "InstallPlugin" inserted
// a wordpress_plugins row with status 'active' and is_active true, having
// installed nothing; a panel built on it showed a plugin list that was purely
// a record of what somebody had once typed into a form. There was no staging
// of any kind, and no WP-CLI anywhere in the repository.
//
// This file is the other half. Every operation below ends in a WP-CLI process
// running as the site's own unix user, and every one of them refuses to run at
// all if that user resolves to root.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	panelpaths "github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/repository"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/wpcli"
)

// wpRuntime is the lazily built host-side half of WordPressService.
type wpRuntime struct {
	once    sync.Once
	client  *wpcli.Client
	staging *wpcli.Staging
	repo    *repository.WordPressRuntimeRepository
	err     error
}

// ErrWordPressRuntimeUnavailable is returned when WP-CLI is not usable on this
// host. It is distinct so a handler answers 503 with "install WP-CLI" rather
// than 500 with an exec error.
var ErrWordPressRuntimeUnavailable = errors.New("the WordPress toolkit is unavailable on this host")

func (s *WordPressService) runtime() (*wpcli.Client, *wpcli.Staging, *repository.WordPressRuntimeRepository, error) {
	s.rt.once.Do(func() {
		if s.repo == nil {
			s.rt.err = fmt.Errorf("%w: this service was built without a database",
				ErrWordPressRuntimeUnavailable)
			return
		}
		s.rt.repo = repository.NewWordPressRuntimeRepository(s.repo.DB())
		runner := wpcli.NewRunner("")
		s.rt.client = wpcli.NewClient(runner, s.logger)
		s.rt.staging = wpcli.NewStaging(s.rt.client, s.logger)

		if err := runner.Available(); err != nil {
			// Not fatal: the record-keeping half of this service still works,
			// and the routes that need WP-CLI answer with this reason. What
			// must not happen is the panel reporting a plugin update it never
			// ran, which is what the previous implementation did.
			s.logger.Warn("WP-CLI is not installed: the WordPress toolkit will refuse every "+
				"operation that needs it rather than reporting success", zap.Error(err))
		}
	})
	if s.rt.err != nil {
		return nil, nil, nil, s.rt.err
	}
	return s.rt.client, s.rt.staging, s.rt.repo, nil
}

// SetRuntimeForTest replaces the host-side half with one a test controls.
func (s *WordPressService) SetRuntimeForTest(client *wpcli.Client, staging *wpcli.Staging, repo *repository.WordPressRuntimeRepository) {
	s.rt.once.Do(func() {})
	s.rt.client = client
	s.rt.staging = staging
	s.rt.repo = repo
	s.rt.err = nil
}

// ---------------------------------------------------------------------------
// Resolving a site to a WP-CLI target
// ---------------------------------------------------------------------------

// resolveSite turns a database record into a wpcli.Site: a directory, a URL and
// a resolved non-root identity.
//
// The identity is looked up from the passwd database every time rather than
// cached, so a user that has been removed since the row was written produces a
// clear failure instead of a command that runs as whoever now holds that uid.
func (s *WordPressService) resolveSite(ctx context.Context, tenantID, siteID uuid.UUID) (*models.WordPressSite, *models.WordPressRuntime, wpcli.Site, error) {
	site, err := s.GetByID(ctx, tenantID, siteID)
	if err != nil {
		return nil, nil, wpcli.Site{}, err
	}
	_, _, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, nil, wpcli.Site{}, err
	}
	runtime, err := runtimeRepo.GetRuntime(ctx, siteID, tenantID)
	if err != nil {
		return nil, nil, wpcli.Site{}, err
	}
	identity, err := wpcli.LookupIdentity(runtime.RunAsUser)
	if err != nil {
		return nil, nil, wpcli.Site{}, err
	}
	dir, err := wpcli.Path("site path", site.Path)
	if err != nil {
		return nil, nil, wpcli.Site{}, err
	}
	if _, err := panelpaths.WithinWebRoot(dir); err != nil {
		return nil, nil, wpcli.Site{}, fmt.Errorf("this site's path is outside the panel web "+
			"root, so the panel will not run commands in it: %w", err)
	}
	return site, runtime, wpcli.Site{
		Dir:      dir,
		Identity: identity,
		URL:      siteURLFor(site.Domain),
	}, nil
}

func siteURLFor(domain string) string {
	return "https://" + strings.ToLower(strings.TrimSpace(domain))
}

// record stores what ran and who it ran as, so the answer survives a log
// rotation. A failure to record is never a failure of the operation.
func (s *WordPressService) record(ctx context.Context, tenantID, siteID uuid.UUID, target wpcli.Site, command string) {
	if _, _, runtimeRepo, err := s.runtime(); err == nil {
		_ = runtimeRepo.RecordRun(ctx, siteID, tenantID, target.Identity.String(), command)
	}
}

// ---------------------------------------------------------------------------
// Runtime identity
// ---------------------------------------------------------------------------

// SetRuntimeRequest is the body of PUT /wordpress/:id/runtime.
type SetRuntimeRequest struct {
	RunAsUser  string `json:"run_as_user" binding:"required"`
	RunAsGroup string `json:"run_as_group"`
	PHPVersion string `json:"php_version"`
}

// SetRuntime records the unix identity a site's commands run as.
//
// The user is resolved against the passwd database here, so a name that does
// not exist is refused now rather than at the first plugin update, and a name
// that resolves to uid 0 is refused outright.
func (s *WordPressService) SetRuntime(ctx context.Context, tenantID, siteID uuid.UUID, req *SetRuntimeRequest) (*models.WordPressRuntime, error) {
	if _, err := s.GetByID(ctx, tenantID, siteID); err != nil {
		return nil, err
	}
	_, _, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	identity, err := wpcli.LookupIdentity(req.RunAsUser)
	if err != nil {
		return nil, err
	}
	group := req.RunAsGroup
	if group == "" {
		group = identity.Group
	}
	if _, err := wpcli.UnixName("run as group", group); err != nil {
		return nil, err
	}

	runtime := &models.WordPressRuntime{
		SiteID:     siteID,
		TenantID:   tenantID,
		RunAsUser:  identity.Name,
		RunAsGroup: group,
	}
	if req.PHPVersion != "" {
		runtime.PHPVersion.String, runtime.PHPVersion.Valid = req.PHPVersion, true
	}
	if err := runtimeRepo.UpsertRuntime(ctx, runtime); err != nil {
		return nil, err
	}
	s.logger.Info("WordPress site runtime identity set",
		zap.String("site_id", siteID.String()),
		zap.String("run_as", identity.String()))
	return runtime, nil
}

// GetRuntime reports who a site's commands run as.
func (s *WordPressService) GetRuntime(ctx context.Context, tenantID, siteID uuid.UUID) (*models.WordPressRuntime, error) {
	if _, err := s.GetByID(ctx, tenantID, siteID); err != nil {
		return nil, err
	}
	_, _, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	return runtimeRepo.GetRuntime(ctx, siteID, tenantID)
}

// ---------------------------------------------------------------------------
// Real installation
// ---------------------------------------------------------------------------

// InstallSiteRequest is the body of POST /wordpress/:id/install.
type InstallSiteRequest struct {
	// RunAsUser is the site's unix user. It is required, and it is the whole
	// reason this is not a one-click operation: an installation has to belong
	// to somebody, and that somebody is never root.
	RunAsUser   string `json:"run_as_user" binding:"required"`
	RunAsGroup  string `json:"run_as_group"`
	SiteTitle   string `json:"site_title"`
	CoreVersion string `json:"core_version"`
	Locale      string `json:"locale"`
}

// InstallResult is what a successful install reports.
type InstallResult struct {
	SiteID    string `json:"site_id"`
	Path      string `json:"path"`
	URL       string `json:"url"`
	Version   string `json:"installed_version"`
	RanAs     string `json:"ran_as"`
	Ownership string `json:"ownership"`
	AdminUser string `json:"admin_user"`
}

// InstallSite performs the real installation the panel previously only recorded.
func (s *WordPressService) InstallSite(ctx context.Context, tenantID, siteID uuid.UUID, req *InstallSiteRequest) (*InstallResult, error) {
	client, _, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	if err := client.Runner().Available(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWordPressRuntimeUnavailable, err)
	}

	site, err := s.GetByID(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}

	identity, err := wpcli.LookupIdentity(req.RunAsUser)
	if err != nil {
		return nil, err
	}
	group := req.RunAsGroup
	if group == "" {
		group = identity.Group
	}

	dir, err := wpcli.Path("site path", site.Path)
	if err != nil {
		return nil, err
	}
	if _, err := panelpaths.WithinWebRoot(dir); err != nil {
		return nil, fmt.Errorf("this site's path is outside the panel web root: %w", err)
	}

	// Record the identity before installing, so a failure halfway through
	// still leaves the panel knowing who owns the tree it created.
	runtime := &models.WordPressRuntime{
		SiteID: siteID, TenantID: tenantID,
		RunAsUser: identity.Name, RunAsGroup: group,
	}
	if err := runtimeRepo.UpsertRuntime(ctx, runtime); err != nil {
		return nil, err
	}

	if err := wpcli.EnsureDir(dir, identity); err != nil {
		return nil, err
	}

	target := wpcli.Site{Dir: dir, Identity: identity, URL: siteURLFor(site.Domain)}
	installReq := wpcli.InstallRequest{
		Site:          target,
		DBName:        site.DBName,
		DBUser:        site.DBUser,
		DBPassword:    site.DBPassword,
		DBHost:        site.DBHost,
		DBPrefix:      site.DBPrefix,
		AdminUser:     site.AdminUser,
		AdminPassword: site.AdminPassword,
		AdminEmail:    site.AdminEmail,
		SiteTitle:     req.SiteTitle,
		CoreVersion:   req.CoreVersion,
		Locale:        req.Locale,
	}
	if installReq.SiteTitle == "" {
		installReq.SiteTitle = site.Name
	}
	if err := client.Install(ctx, installReq); err != nil {
		return nil, err
	}

	// Ownership last, so that anything WP-CLI created along the way is caught.
	perms := wpcli.DefaultPermissions()
	if err := wpcli.EnsureUploads(dir, identity, perms); err != nil {
		return nil, err
	}
	if err := wpcli.ApplyOwnership(dir, identity, perms); err != nil {
		return nil, fmt.Errorf("WordPress was installed but its file ownership could not be "+
			"set, which leaves files the site cannot write: %w", err)
	}

	version, err := client.CoreVersionOf(ctx, target)
	if err != nil {
		version = "unknown"
	}
	_ = runtimeRepo.SetInstalledVersion(ctx, siteID, tenantID, version)
	s.record(ctx, tenantID, siteID, target, "core install")

	site.Version = version
	site.Status = "active"
	if err := s.repo.Update(ctx, site); err != nil {
		s.logger.Warn("WordPress installed but the site record could not be updated",
			zap.String("site_id", siteID.String()), zap.Error(err))
	}

	s.logger.Info("WordPress installed",
		zap.String("site_id", siteID.String()),
		zap.String("path", dir),
		zap.String("version", version),
		zap.String("ran_as", identity.String()))

	return &InstallResult{
		SiteID:    siteID.String(),
		Path:      dir,
		URL:       target.URL,
		Version:   version,
		RanAs:     identity.String(),
		AdminUser: site.AdminUser,
		Ownership: fmt.Sprintf("%s:%s, directories %o, files %o, wp-config.php %o, uploads %o",
			identity.Name, group, perms.DirMode, perms.FileMode, perms.ConfigMode, perms.UploadsDirMode),
	}, nil
}

// ---------------------------------------------------------------------------
// The live plugin, theme and core operations
// ---------------------------------------------------------------------------

// LivePlugins reports the plugins WP-CLI finds in the installation.
func (s *WordPressService) LivePlugins(ctx context.Context, tenantID, siteID uuid.UUID) ([]wpcli.Plugin, string, error) {
	client, _, _, err := s.runtime()
	if err != nil {
		return nil, "", err
	}
	_, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, "", err
	}
	plugins, err := client.ListPlugins(ctx, target)
	s.record(ctx, tenantID, siteID, target, "plugin list")
	return plugins, target.Identity.String(), err
}

// LiveThemes reports the themes WP-CLI finds in the installation.
func (s *WordPressService) LiveThemes(ctx context.Context, tenantID, siteID uuid.UUID) ([]wpcli.Theme, string, error) {
	client, _, _, err := s.runtime()
	if err != nil {
		return nil, "", err
	}
	_, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, "", err
	}
	themes, err := client.ListThemes(ctx, target)
	s.record(ctx, tenantID, siteID, target, "theme list")
	return themes, target.Identity.String(), err
}

// UpdateRequest names what to update. An empty Slugs means everything.
type UpdateRequest struct {
	Slugs []string `json:"slugs"`
}

// UpdateOutcome is what an update reports.
type UpdateOutcome struct {
	RanAs   string   `json:"ran_as"`
	Output  string   `json:"output"`
	Updated []string `json:"requested"`
}

// UpdatePluginsLive runs `wp plugin update`.
func (s *WordPressService) UpdatePluginsLive(ctx context.Context, tenantID, siteID uuid.UUID, req *UpdateRequest) (*UpdateOutcome, error) {
	client, _, _, err := s.runtime()
	if err != nil {
		return nil, err
	}
	_, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	result, err := client.UpdatePlugins(ctx, target, req.Slugs)
	s.record(ctx, tenantID, siteID, target, "plugin update")
	if err != nil {
		return nil, err
	}
	return &UpdateOutcome{RanAs: target.Identity.String(), Output: result.Stdout, Updated: req.Slugs}, nil
}

// UpdateThemesLive runs `wp theme update`.
func (s *WordPressService) UpdateThemesLive(ctx context.Context, tenantID, siteID uuid.UUID, req *UpdateRequest) (*UpdateOutcome, error) {
	client, _, _, err := s.runtime()
	if err != nil {
		return nil, err
	}
	_, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	result, err := client.UpdateThemes(ctx, target, req.Slugs)
	s.record(ctx, tenantID, siteID, target, "theme update")
	if err != nil {
		return nil, err
	}
	return &UpdateOutcome{RanAs: target.Identity.String(), Output: result.Stdout, Updated: req.Slugs}, nil
}

// CoreUpdateRequest is the body of POST /wordpress/:id/core/update.
type CoreUpdateRequest struct {
	Version string `json:"version"`
}

// UpdateCoreLive runs `wp core update` followed by `wp core update-db`.
func (s *WordPressService) UpdateCoreLive(ctx context.Context, tenantID, siteID uuid.UUID, req *CoreUpdateRequest) (*UpdateOutcome, error) {
	client, _, _, runtimeErr := s.runtime()
	if runtimeErr != nil {
		return nil, runtimeErr
	}
	_, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	result, err := client.UpdateCore(ctx, target, req.Version)
	s.record(ctx, tenantID, siteID, target, "core update")
	if err != nil {
		return nil, err
	}
	if version, verr := client.CoreVersionOf(ctx, target); verr == nil {
		if _, _, runtimeRepo, rerr := s.runtime(); rerr == nil {
			_ = runtimeRepo.SetInstalledVersion(ctx, siteID, tenantID, version)
		}
	}
	return &UpdateOutcome{RanAs: target.Identity.String(), Output: result.Stdout}, nil
}

// SearchReplaceRequest is the body of POST /wordpress/:id/search-replace.
//
// DryRun defaults to true through the handler, not here: this is the operation
// that rewrites every row of a customer's database, and the safe answer has to
// be the one you get by leaving the field out.
type SearchReplaceRequest struct {
	From   string `json:"from" binding:"required"`
	To     string `json:"to" binding:"required"`
	DryRun *bool  `json:"dry_run"`
}

// SearchReplaceLive runs a serialisation-safe search-replace.
func (s *WordPressService) SearchReplaceLive(ctx context.Context, tenantID, siteID uuid.UUID, req *SearchReplaceRequest) (*wpcli.SearchReplaceReport, string, error) {
	client, _, _, err := s.runtime()
	if err != nil {
		return nil, "", err
	}
	_, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, "", err
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	report, err := client.SearchReplace(ctx, target, req.From, req.To, dryRun)
	s.record(ctx, tenantID, siteID, target, "search-replace")
	return report, target.Identity.String(), err
}

// ResetPasswordRequest is the body of POST /wordpress/:id/users/password.
type ResetPasswordRequest struct {
	Login string `json:"login" binding:"required"`
	// Password is optional. When it is empty the panel generates one and
	// returns it once; it is never stored.
	Password string `json:"password"`
}

// ResetPasswordResult carries the new password back exactly once.
type ResetPasswordResult struct {
	Login string `json:"login"`
	// Password is present only when the panel generated it.
	Password string `json:"password,omitempty"`
	RanAs    string `json:"ran_as"`
}

// ResetUserPasswordLive resets a WordPress user's password.
func (s *WordPressService) ResetUserPasswordLive(ctx context.Context, tenantID, siteID uuid.UUID, req *ResetPasswordRequest) (*ResetPasswordResult, error) {
	client, _, _, err := s.runtime()
	if err != nil {
		return nil, err
	}
	_, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	generated := req.Password == ""
	password, err := client.ResetUserPassword(ctx, target, req.Login, req.Password)
	s.record(ctx, tenantID, siteID, target, "user update --user_pass")
	if err != nil {
		return nil, err
	}
	result := &ResetPasswordResult{Login: req.Login, RanAs: target.Identity.String()}
	if generated {
		result.Password = password
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Staging
// ---------------------------------------------------------------------------

// CreateStagingRequest is the body of POST /wordpress/:id/staging.
type CreateStagingRequest struct {
	// Subdomain is prepended to the production domain. It defaults to
	// "staging", producing staging.example.com.
	Subdomain string `json:"subdomain"`
	// DBName, DBUser and DBPassword are the staging database. They must differ
	// from production's; the service refuses them if they do not.
	DBName     string `json:"db_name" binding:"required"`
	DBUser     string `json:"db_user" binding:"required"`
	DBPassword string `json:"db_password" binding:"required"`
	DBHost     string `json:"db_host"`
	// BlockIndexing defaults to true. Two copies of a site in a search index
	// costs the customer their ranking.
	BlockIndexing *bool `json:"block_indexing"`
}

// StagingView is the API view of a staging environment.
type StagingView struct {
	ID      string                `json:"id"`
	Domain  string                `json:"staging_domain"`
	Path    string                `json:"staging_path"`
	URL     string                `json:"staging_url"`
	Status  string                `json:"status"`
	RanAs   string                `json:"ran_as,omitempty"`
	History models.StagingHistory `json:"history"`
	Clone   *wpcli.CloneResult    `json:"clone,omitempty"`
	Push    *wpcli.PushResult     `json:"push,omitempty"`
}

// CreateStaging clones production into a staging copy.
func (s *WordPressService) CreateStaging(ctx context.Context, tenantID, siteID uuid.UUID, req *CreateStagingRequest) (*StagingView, error) {
	_, staging, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	site, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}

	subdomain := req.Subdomain
	if subdomain == "" {
		subdomain = "staging"
	}
	if _, err := wpcli.Slug(subdomain); err != nil {
		return nil, fmt.Errorf("staging subdomain: %w", err)
	}
	stagingDomain := strings.ToLower(subdomain + "." + strings.TrimSpace(site.Domain))
	if err := panelpaths.ValidateDomain(stagingDomain); err != nil {
		return nil, err
	}
	stagingPath, err := panelpaths.SiteRoot(stagingDomain)
	if err != nil {
		return nil, err
	}
	if req.DBName == site.DBName {
		return nil, fmt.Errorf("the staging database may not be the production database (%q): "+
			"staging would be writing to the data it exists to protect", site.DBName)
	}

	record := &models.WordPressStaging{
		TenantID:          tenantID,
		ProductionSiteID:  siteID,
		StagingDomain:     stagingDomain,
		StagingPath:       stagingPath,
		StagingURL:        siteURLFor(stagingDomain),
		StagingDBName:     req.DBName,
		StagingDBUser:     req.DBUser,
		StagingDBPassword: req.DBPassword,
		StagingDBHost:     defaultHost(req.DBHost),
		Status:            "cloning",
		BlockIndexing:     req.BlockIndexing == nil || *req.BlockIndexing,
	}
	if err := runtimeRepo.CreateStaging(ctx, record); err != nil {
		return nil, err
	}

	// Staging runs as the SAME unix user as production. They belong to one
	// customer, and giving them separate users would mean a push has to cross
	// a privilege boundary - which can only be crossed as root.
	stagingTarget := wpcli.Site{
		Dir:      stagingPath,
		Identity: target.Identity,
		URL:      record.StagingURL,
	}
	if err := wpcli.EnsureDir(stagingPath, target.Identity); err != nil {
		_ = runtimeRepo.RecordStagingError(ctx, record.ID, tenantID, err.Error())
		return nil, err
	}

	result, err := staging.Clone(ctx, wpcli.CloneOptions{
		Production: wpcli.StagingSite{
			Site: target, DBName: site.DBName, DBUser: site.DBUser,
			DBPass: site.DBPassword, DBHost: site.DBHost,
		},
		Staging: wpcli.StagingSite{
			Site: stagingTarget, DBName: record.StagingDBName, DBUser: record.StagingDBUser,
			DBPass: record.StagingDBPassword, DBHost: record.StagingDBHost,
		},
		BlockIndexing: record.BlockIndexing,
	})
	if err != nil {
		_ = runtimeRepo.RecordStagingError(ctx, record.ID, tenantID, err.Error())
		return nil, err
	}
	if err := wpcli.ApplyOwnership(stagingPath, target.Identity, wpcli.DefaultPermissions()); err != nil {
		_ = runtimeRepo.RecordStagingError(ctx, record.ID, tenantID, err.Error())
		return nil, fmt.Errorf("staging was cloned but its file ownership could not be set: %w", err)
	}
	if err := runtimeRepo.RecordClone(ctx, record.ID, tenantID); err != nil {
		s.logger.Warn("the staging clone succeeded but could not be recorded", zap.Error(err))
	}
	s.record(ctx, tenantID, siteID, target, "staging clone")

	stored, err := runtimeRepo.GetStaging(ctx, siteID, tenantID)
	if err != nil {
		stored = record
	}
	return &StagingView{
		ID: stored.ID.String(), Domain: stored.StagingDomain, Path: stored.StagingPath,
		URL: stored.StagingURL, Status: stored.Status, RanAs: target.Identity.String(),
		History: stored.History(), Clone: result,
	}, nil
}

// GetStaging reports a site's staging environment.
func (s *WordPressService) GetStaging(ctx context.Context, tenantID, siteID uuid.UUID) (*StagingView, error) {
	_, _, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}
	if _, err := s.GetByID(ctx, tenantID, siteID); err != nil {
		return nil, err
	}
	stored, err := runtimeRepo.GetStaging(ctx, siteID, tenantID)
	if err != nil {
		return nil, err
	}
	return &StagingView{
		ID: stored.ID.String(), Domain: stored.StagingDomain, Path: stored.StagingPath,
		URL: stored.StagingURL, Status: stored.Status, History: stored.History(),
	}, nil
}

// PushStagingRequest is the body of POST /wordpress/:id/staging/push.
//
// Database has no default and no zero value that means anything. The JSON tag
// carries no omitempty and the service refuses an empty value with a message
// that lists the three choices, because a push that guesses is how a customer
// loses a week of orders.
type PushStagingRequest struct {
	Database string `json:"database"`
}

// PushStaging pushes staging back to production with an explicit database
// decision, having backed production up first.
func (s *WordPressService) PushStaging(ctx context.Context, tenantID, siteID uuid.UUID, req *PushStagingRequest) (*StagingView, error) {
	_, staging, runtimeRepo, err := s.runtime()
	if err != nil {
		return nil, err
	}

	action := wpcli.DatabaseAction(strings.TrimSpace(req.Database))
	if !action.Valid() {
		return nil, wpcli.ErrDatabaseChoiceRequired
	}

	site, _, target, err := s.resolveSite(ctx, tenantID, siteID)
	if err != nil {
		return nil, err
	}
	stored, err := runtimeRepo.GetStaging(ctx, siteID, tenantID)
	if err != nil {
		return nil, err
	}

	backupDir := filepath.Join(panelpaths.BackupRoot(), "staging", stored.ID.String())

	result, err := staging.Push(ctx, wpcli.PushOptions{
		Production: wpcli.StagingSite{
			Site: target, DBName: site.DBName, DBUser: site.DBUser,
			DBPass: site.DBPassword, DBHost: site.DBHost,
		},
		Staging: wpcli.StagingSite{
			Site: wpcli.Site{
				Dir: stored.StagingPath, Identity: target.Identity, URL: stored.StagingURL,
			},
			DBName: stored.StagingDBName, DBUser: stored.StagingDBUser,
			DBPass: stored.StagingDBPassword, DBHost: stored.StagingDBHost,
		},
		Database:  action,
		BackupDir: backupDir,
	})
	if err != nil {
		_ = runtimeRepo.RecordStagingError(ctx, stored.ID, tenantID, err.Error())
		return nil, err
	}

	if err := runtimeRepo.RecordPush(ctx, stored.ID, tenantID, string(action),
		result.BackupPath, result.DatabaseBackup); err != nil {
		s.logger.Warn("the staging push succeeded but could not be recorded", zap.Error(err))
	}
	s.record(ctx, tenantID, siteID, target, "staging push ("+string(action)+")")

	refreshed, err := runtimeRepo.GetStaging(ctx, siteID, tenantID)
	if err != nil {
		refreshed = stored
	}
	return &StagingView{
		ID: refreshed.ID.String(), Domain: refreshed.StagingDomain, Path: refreshed.StagingPath,
		URL: refreshed.StagingURL, Status: refreshed.Status, RanAs: target.Identity.String(),
		History: refreshed.History(), Push: result,
	}, nil
}

// DeleteStaging removes a staging record. It deliberately does not delete the
// staging files or database: a customer who asks to "remove staging" from a
// list has not asked to have a directory deleted, and the difference is not
// recoverable.
func (s *WordPressService) DeleteStaging(ctx context.Context, tenantID, siteID uuid.UUID) error {
	_, _, runtimeRepo, err := s.runtime()
	if err != nil {
		return err
	}
	if _, err := s.GetByID(ctx, tenantID, siteID); err != nil {
		return err
	}
	stored, err := runtimeRepo.GetStaging(ctx, siteID, tenantID)
	if err != nil {
		return err
	}
	if err := runtimeRepo.DeleteStaging(ctx, stored.ID, tenantID); err != nil {
		return err
	}
	s.logger.Info("staging environment record removed; its files and database were left in place",
		zap.String("site_id", siteID.String()),
		zap.String("staging_path", stored.StagingPath),
		zap.String("staging_db", stored.StagingDBName))
	return nil
}

func defaultHost(host string) string {
	if host == "" {
		return "localhost"
	}
	return host
}
