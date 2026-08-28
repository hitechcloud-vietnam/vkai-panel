package wpcli

// Staging: clone a site, and push it back.
//
// The push is the dangerous half, and the requirement says exactly why:
// "push changes back with an explicit choice about the database, since pushing
// a staging database over production is how a customer loses a week of orders."
//
// So DatabaseAction has no safe default and no zero value that means anything.
// PushOptions.Database must be one of the three named constants; the zero value
// is DatabaseUnset and Push refuses it with a message that lists the choices.
// A caller that forgets the field gets an error, never "we picked one for you".
//
// The other half of that protection is that a push ALWAYS takes a backup of
// what it is about to overwrite, before it overwrites it, and the path to that
// backup is in the result. A customer who chooses wrong has a way back that
// does not involve a support ticket and a nightly tape.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// DatabaseAction is the explicit choice a push must carry.
type DatabaseAction string

const (
	// DatabaseUnset is the zero value. It is not a choice, and Push refuses it.
	DatabaseUnset DatabaseAction = ""
	// DatabaseKeepProduction copies files only and leaves the production
	// database completely untouched. This is the choice for a theme or plugin
	// change, and it is the one an operator should pick unless they know why
	// not.
	DatabaseKeepProduction DatabaseAction = "keep_production"
	// DatabaseOverwriteProduction replaces the production database with the
	// staging one. This is the choice that loses a week of orders when it is
	// made carelessly, so it is named for what it does rather than for what it
	// is for.
	DatabaseOverwriteProduction DatabaseAction = "overwrite_production"
	// DatabaseOnly replaces the production database and leaves production
	// files alone. For a content-only staging round.
	DatabaseOnly DatabaseAction = "database_only"
)

// Valid reports whether an action is one of the three real choices.
func (a DatabaseAction) Valid() bool {
	switch a {
	case DatabaseKeepProduction, DatabaseOverwriteProduction, DatabaseOnly:
		return true
	}
	return false
}

// ErrDatabaseChoiceRequired is returned when a push carries no database
// decision. It lists the options, because an operator reading it in an API
// response has nowhere else to look.
var ErrDatabaseChoiceRequired = errors.New(
	"a staging push must state what to do with the database: " +
		`"keep_production" (files only, production data untouched), ` +
		`"overwrite_production" (replace the production database with the staging one - ` +
		`this discards every order, comment and post made on production since the clone), or ` +
		`"database_only" (replace the database and leave production files alone). ` +
		"There is no default: choosing for you is how a week of orders is lost")

// StagingSite is a staging environment: a second site, a second database, a
// second directory, and the same system user as production.
//
// The same user is deliberate. Staging and production belong to one customer;
// giving them separate users would mean a push has to cross a privilege
// boundary, and the only way to cross it is as root.
type StagingSite struct {
	Site   Site
	DBName string
	DBUser string
	DBPass string
	DBHost string
}

// CloneOptions describes a clone from production to staging.
type CloneOptions struct {
	Production StagingSite
	Staging    StagingSite
	// BlockIndexing sets blog_public=0 so search engines do not index the
	// staging copy. Two copies of a site in an index costs the customer their
	// ranking, and it is the single most common complaint about staging.
	BlockIndexing bool
}

// PushOptions describes a push from staging back to production.
type PushOptions struct {
	Production StagingSite
	Staging    StagingSite
	// Database is the explicit choice. There is no default.
	Database DatabaseAction
	// BackupDir is where the pre-push backup of production is written. It is
	// required: a push with nowhere to put the backup is refused.
	BackupDir string
}

// CloneResult reports what a clone did.
type CloneResult struct {
	StagingDir  string               `json:"staging_dir"`
	StagingURL  string               `json:"staging_url"`
	Replacement *SearchReplaceReport `json:"url_rewrite"`
	RanAs       string               `json:"ran_as"`
}

// PushResult reports what a push did, including where the backup went.
type PushResult struct {
	Database       DatabaseAction       `json:"database_action"`
	FilesCopied    bool                 `json:"files_copied"`
	DatabaseCopied bool                 `json:"database_copied"`
	BackupPath     string               `json:"backup_path"`
	DatabaseBackup string               `json:"database_backup_path"`
	Replacement    *SearchReplaceReport `json:"url_rewrite"`
	RanAs          string               `json:"ran_as"`
}

// Staging performs clone and push.
type Staging struct {
	client *Client
	logger *zap.Logger
	// rsyncPath is the copy program. It is a field so an operator can point it
	// at a different rsync and so a test can point it at a stub.
	rsyncPath string
	// copy is the directory copier. It is a field for the same reason
	// Runner.exec is: the real one drops privileges to the site user before
	// exec, and a test process cannot exec anything as uid 1201. Substituting
	// it is how the ORDER of a clone or a push - which is the whole safety
	// property - gets tested at all.
	copy func(ctx context.Context, site Site, from, to string) error
}

// NewStaging builds a staging driver over a WP-CLI client.
func NewStaging(client *Client, logger *zap.Logger) *Staging {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &Staging{client: client, logger: logger, rsyncPath: "rsync"}
	s.copy = s.rsyncTree
	return s
}

// Clone copies production to staging: files, then database, then rewrite every
// production URL in the staging database to the staging URL.
//
// The order matters. Copying the database before the files leaves a window
// where staging's wp-config.php points at production's database; a `wp
// search-replace` in that window rewrites PRODUCTION. So: files first (which
// brings staging's own wp-config.php into place), then the database, then the
// rewrite - and the rewrite runs with --path pointing at staging.
func (s *Staging) Clone(ctx context.Context, opts CloneOptions) (*CloneResult, error) {
	if err := s.validatePair(opts.Production, opts.Staging); err != nil {
		return nil, err
	}

	// 1. Files.
	if err := s.copyTree(ctx, opts.Production.Site, opts.Production.Site.Dir, opts.Staging.Site.Dir); err != nil {
		return nil, fmt.Errorf("copying the site files to staging: %w", err)
	}

	// 2. Point staging's wp-config.php at the staging database, BEFORE any
	// database command runs against the staging path.
	if err := s.pointConfigAt(ctx, opts.Staging); err != nil {
		return nil, err
	}

	// 3. Database.
	dump, err := s.exportDatabase(ctx, opts.Production.Site)
	if err != nil {
		return nil, fmt.Errorf("exporting the production database: %w", err)
	}
	defer os.Remove(dump)
	if err := s.importDatabase(ctx, opts.Staging.Site, dump); err != nil {
		return nil, fmt.Errorf("importing into the staging database: %w", err)
	}

	// 4. Rewrite URLs in the staging database. Serialisation-safe; see
	// Client.SearchReplace.
	report, err := s.client.SearchReplace(ctx, opts.Staging.Site,
		opts.Production.Site.URL, opts.Staging.Site.URL, false)
	if err != nil {
		return nil, fmt.Errorf("rewriting URLs in the staging database: %w", err)
	}

	// 5. Keep it out of the search index.
	if opts.BlockIndexing {
		if _, err := s.client.run(ctx, opts.Staging.Site, time.Minute,
			"option", "update", "blog_public", "0"); err != nil {
			return nil, fmt.Errorf("staging was cloned but could not be hidden from search "+
				"engines, which would put two copies of this site in the index: %w", err)
		}
	}

	s.logger.Info("staging site cloned",
		zap.String("production", opts.Production.Site.Dir),
		zap.String("staging", opts.Staging.Site.Dir),
		zap.String("ran_as", opts.Staging.Site.Identity.String()),
		zap.Int("url_replacements", report.Replacements))

	return &CloneResult{
		StagingDir:  opts.Staging.Site.Dir,
		StagingURL:  opts.Staging.Site.URL,
		Replacement: report,
		RanAs:       opts.Staging.Site.Identity.String(),
	}, nil
}

// Push moves staging back to production, with the database decision the caller
// made explicitly.
func (s *Staging) Push(ctx context.Context, opts PushOptions) (*PushResult, error) {
	// The refusal that this whole file exists for, checked before anything is
	// read, copied or backed up.
	if !opts.Database.Valid() {
		return nil, ErrDatabaseChoiceRequired
	}
	if err := s.validatePair(opts.Production, opts.Staging); err != nil {
		return nil, err
	}
	backupDir, err := Path("backup directory", opts.BackupDir)
	if err != nil {
		return nil, fmt.Errorf("a staging push needs somewhere to back production up to: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return nil, fmt.Errorf("cannot create the backup directory %s: %w", backupDir, err)
	}
	if err := os.Chown(backupDir, int(opts.Production.Site.Identity.UID),
		int(opts.Production.Site.Identity.GID)); err != nil {
		return nil, fmt.Errorf("cannot give the backup directory to %s: %w",
			opts.Production.Site.Identity.Name, err)
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	result := &PushResult{
		Database: opts.Database,
		RanAs:    opts.Production.Site.Identity.String(),
	}

	// Always back production up first, whatever the choice. The files backup
	// is taken even for a database-only push, because a database-only push
	// that goes wrong is still recovered by rolling both halves back together.
	filesBackup := filepath.Join(backupDir, "production-files-"+stamp)
	if err := s.copyTree(ctx, opts.Production.Site, opts.Production.Site.Dir, filesBackup); err != nil {
		return nil, fmt.Errorf("refusing to push: production could not be backed up first: %w", err)
	}
	result.BackupPath = filesBackup

	dbBackup := filepath.Join(backupDir, "production-database-"+stamp+".sql")
	if _, err := s.client.run(ctx, opts.Production.Site, 30*time.Minute,
		"db", "export", dbBackup, "--add-drop-table"); err != nil {
		return nil, fmt.Errorf("refusing to push: the production database could not be backed up "+
			"first: %w", err)
	}
	result.DatabaseBackup = dbBackup

	// Files.
	if opts.Database != DatabaseOnly {
		if err := s.copyTree(ctx, opts.Staging.Site, opts.Staging.Site.Dir, opts.Production.Site.Dir); err != nil {
			return nil, fmt.Errorf("copying staging files over production: %w (production files "+
				"are backed up at %s)", err, filesBackup)
		}
		// The copy has just overwritten production's wp-config.php with
		// staging's, which points at the staging database. Put it back before
		// anything touches the database.
		if err := s.pointConfigAt(ctx, opts.Production); err != nil {
			return nil, fmt.Errorf("staging files were copied to production but production's "+
				"wp-config.php still points at the staging database: %w (production files are "+
				"backed up at %s)", err, filesBackup)
		}
		result.FilesCopied = true
	}

	// Database, only on an explicit instruction to replace it.
	switch opts.Database {
	case DatabaseKeepProduction:
		s.logger.Info("staging push kept the production database",
			zap.String("production", opts.Production.Site.Dir))

	case DatabaseOverwriteProduction, DatabaseOnly:
		dump, err := s.exportDatabase(ctx, opts.Staging.Site)
		if err != nil {
			return nil, fmt.Errorf("exporting the staging database: %w", err)
		}
		defer os.Remove(dump)
		if err := s.importDatabase(ctx, opts.Production.Site, dump); err != nil {
			return nil, fmt.Errorf("importing the staging database over production: %w "+
				"(the production database is backed up at %s)", err, dbBackup)
		}
		report, err := s.client.SearchReplace(ctx, opts.Production.Site,
			opts.Staging.Site.URL, opts.Production.Site.URL, false)
		if err != nil {
			return nil, fmt.Errorf("the staging database was imported but its URLs still point "+
				"at staging: %w (the production database is backed up at %s)", err, dbBackup)
		}
		result.Replacement = report
		result.DatabaseCopied = true
		// A production site that came from staging inherits blog_public=0 and
		// would drop out of the search index. Put it back.
		if _, err := s.client.run(ctx, opts.Production.Site, time.Minute,
			"option", "update", "blog_public", "1"); err != nil {
			return nil, fmt.Errorf("the database was pushed but production is still hidden from "+
				"search engines: %w", err)
		}
	}

	s.logger.Warn("staging pushed to production",
		zap.String("production", opts.Production.Site.Dir),
		zap.String("staging", opts.Staging.Site.Dir),
		zap.String("database_action", string(opts.Database)),
		zap.Bool("files_copied", result.FilesCopied),
		zap.Bool("database_copied", result.DatabaseCopied),
		zap.String("files_backup", result.BackupPath),
		zap.String("database_backup", result.DatabaseBackup),
		zap.String("ran_as", opts.Production.Site.Identity.String()))

	return result, nil
}

// validatePair proves both sites are usable and, above all, distinct. A clone
// whose source and destination are the same directory deletes the site.
func (s *Staging) validatePair(production, staging StagingSite) error {
	prodDir, err := Path("production directory", production.Site.Dir)
	if err != nil {
		return err
	}
	stagingDir, err := Path("staging directory", staging.Site.Dir)
	if err != nil {
		return err
	}
	if prodDir == stagingDir {
		return fmt.Errorf("the production and staging directories are the same (%s)", prodDir)
	}
	if strings.HasPrefix(stagingDir+"/", prodDir+"/") || strings.HasPrefix(prodDir+"/", stagingDir+"/") {
		return fmt.Errorf("the staging directory (%s) and the production directory (%s) are "+
			"nested; a copy between them would recurse into itself", stagingDir, prodDir)
	}
	if _, err := SiteURL(production.Site.URL); err != nil {
		return fmt.Errorf("production: %w", err)
	}
	if _, err := SiteURL(staging.Site.URL); err != nil {
		return fmt.Errorf("staging: %w", err)
	}
	if production.Site.URL == staging.Site.URL {
		return fmt.Errorf("production and staging have the same URL (%s); the URL rewrite would "+
			"have nothing to rewrite and visitors could not tell them apart", production.Site.URL)
	}
	if production.DBName != "" && production.DBName == staging.DBName {
		return fmt.Errorf("production and staging share the database %q: staging would be writing "+
			"to the production data it was created to protect", production.DBName)
	}
	for _, identity := range []Identity{production.Site.Identity, staging.Site.Identity} {
		if identity.UID == 0 || identity.GID == 0 {
			return &ErrWouldRunAsRoot{Requested: identity.Name, UID: identity.UID, GID: identity.GID}
		}
	}
	return nil
}

// pointConfigAt rewrites a site's wp-config.php database constants.
func (s *Staging) pointConfigAt(ctx context.Context, target StagingSite) error {
	dbName, err := Identifier("database name", target.DBName)
	if err != nil {
		return err
	}
	dbUser, err := Identifier("database user", target.DBUser)
	if err != nil {
		return err
	}
	host := target.DBHost
	if host == "" {
		host = "localhost"
	}
	if err := validateDBHost(host); err != nil {
		return err
	}
	settings := [][2]string{
		{"DB_NAME", dbName},
		{"DB_USER", dbUser},
		{"DB_PASSWORD", target.DBPass},
		{"DB_HOST", host},
	}
	for _, setting := range settings {
		if _, err := s.client.run(ctx, target.Site, time.Minute,
			"config", "set", setting[0], setting[1], "--type=constant"); err != nil {
			return fmt.Errorf("cannot point wp-config.php at the %s database (%s): %w",
				setting[0], target.Site.Dir, err)
		}
	}
	return nil
}

func (s *Staging) exportDatabase(ctx context.Context, site Site) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(site.Dir), ".vkai-wpdump-*.sql")
	if err != nil {
		return "", err
	}
	path := file.Name()
	file.Close()
	// The dump is written by the site user, so it has to own the file.
	if err := os.Chown(path, int(site.Identity.UID), int(site.Identity.GID)); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("cannot give the dump file to %s: %w", site.Identity.Name, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		os.Remove(path)
		return "", err
	}
	if _, err := s.client.run(ctx, site, 30*time.Minute, "db", "export", path, "--add-drop-table"); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func (s *Staging) importDatabase(ctx context.Context, site Site, dumpPath string) error {
	if _, err := Path("dump file", dumpPath); err != nil {
		return err
	}
	// The importing user has to be able to read the dump the exporting user
	// wrote. Both are the same customer's user in every path this package
	// takes, but a clone across two users would silently fail here otherwise.
	if err := os.Chown(dumpPath, int(site.Identity.UID), int(site.Identity.GID)); err != nil {
		return fmt.Errorf("cannot give the dump file to %s: %w", site.Identity.Name, err)
	}
	// `wp db reset` before the import, so rows that staging deleted do not
	// survive in production. --yes because there is no terminal to confirm at.
	if _, err := s.client.run(ctx, site, 10*time.Minute, "db", "reset", "--yes"); err != nil {
		return fmt.Errorf("cannot empty the target database before importing: %w", err)
	}
	if _, err := s.client.run(ctx, site, 30*time.Minute, "db", "import", dumpPath); err != nil {
		return err
	}
	return nil
}

// copyTree is the entry point every copy goes through. It validates the pair
// and then delegates to the installed copier.
func (s *Staging) copyTree(ctx context.Context, site Site, from, to string) error {
	if s.copy == nil {
		s.copy = s.rsyncTree
	}
	return s.copy(ctx, site, from, to)
}

// rsyncTree copies one directory to another as the site user.
//
// rsync is invoked as an argv, as everything here is, and it runs with the
// site's credential rather than root's - so a symlink inside the site tree
// pointing at /etc cannot be followed into a file the site user cannot read.
// --safe-links drops such links outright rather than copying their targets.
func (s *Staging) rsyncTree(ctx context.Context, site Site, from, to string) error {
	source, err := Path("copy source", from)
	if err != nil {
		return err
	}
	dest, err := Path("copy destination", to)
	if err != nil {
		return err
	}
	if source == dest {
		return fmt.Errorf("refusing to copy %s onto itself", source)
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("cannot read the source directory %s: %w", source, err)
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("cannot create %s: %w", dest, err)
	}
	if err := os.Chown(dest, int(site.Identity.UID), int(site.Identity.GID)); err != nil {
		return fmt.Errorf("cannot give %s to %s: %w", dest, site.Identity.Name, err)
	}

	args := []string{
		"--archive",
		"--delete",
		"--safe-links",
		"--no-owner", "--no-group",
		// Never copy the panel's own scratch, WP-CLI's cache, or a nested
		// staging copy.
		"--exclude=.wp-cli/",
		"--exclude=.vkai-wpdump-*",
		"--exclude=wp-content/cache/",
		source + "/", dest + "/",
	}
	cmd := exec.CommandContext(ctx, s.rsyncPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:    site.Identity.UID,
			Gid:    site.Identity.GID,
			Groups: []uint32{site.Identity.GID},
		},
		Setpgid: true,
	}
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync %s -> %s failed as %s: %w: %s",
			source, dest, site.Identity, err, firstLines("", string(out)))
	}
	return nil
}

// SetCopierForTest replaces the directory copier.
//
// The real one drops privileges to the site user before exec, which a test
// process cannot do, so without this the ORDER of a clone or a push - which is
// the entire safety property of both - could not be asserted anywhere.
func (s *Staging) SetCopierForTest(copier func(ctx context.Context, site Site, from, to string) error) {
	s.copy = copier
}
