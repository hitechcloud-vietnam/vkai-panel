package wpcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// recordingRunner captures every WP-CLI argv and answers each one, so a whole
// clone or push can be driven and then inspected command by command.
type recordingRunner struct {
	Runner
	commands []string
	fail     map[string]error
}

func newRecordingClient(t *testing.T) (*Client, *Staging, *recordingRunner) {
	t.Helper()
	rec := &recordingRunner{fail: map[string]error{}}
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(_ context.Context, cmd *exec.Cmd) (*Result, error) {
		argv := strings.Join(cmd.Args[1:], " ")
		rec.commands = append(rec.commands, argv)
		for marker, err := range rec.fail {
			if strings.Contains(argv, marker) {
				return &Result{Stderr: "simulated"}, err
			}
		}
		return &Result{Stdout: "[]"}, nil
	}
	client := NewClient(runner, zap.NewNop())
	return client, NewStaging(client, zap.NewNop()), rec
}

func (r *recordingRunner) ran(marker string) bool {
	for _, command := range r.commands {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func (r *recordingRunner) indexOf(marker string) int {
	for i, command := range r.commands {
		if strings.Contains(command, marker) {
			return i
		}
	}
	return -1
}

// stagingPair builds a production/staging pair rooted in a temporary directory,
// with rsync stubbed by a script so a real copy happens without needing rsync's
// exact flags to be supported by the test environment.
func stagingPair(t *testing.T) (StagingSite, StagingSite, string) {
	t.Helper()
	root := t.TempDir()
	prodDir := filepath.Join(root, "production")
	stagingDir := filepath.Join(root, "staging")
	for _, dir := range []string{prodDir, stagingDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(prodDir, "index.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	identity := nonRoot()
	production := StagingSite{
		Site:   Site{Dir: prodDir, Identity: identity, URL: "https://example.com"},
		DBName: "wp_prod", DBUser: "wp_prod", DBPass: "p1", DBHost: "localhost",
	}
	staging := StagingSite{
		Site:   Site{Dir: stagingDir, Identity: identity, URL: "https://staging.example.com"},
		DBName: "wp_stg", DBUser: "wp_stg", DBPass: "p2", DBHost: "localhost",
	}
	return production, staging, root
}

// copyLog records every directory copy a clone or push performs, and under
// which identity. The real copier drops privileges to the site user before
// exec, which a test process cannot do, so this substitutes for it - exactly
// the way Runner.exec is substituted for the WP-CLI process.
//
// Crucially it appends into the SAME command list the WP-CLI recorder uses, so
// the file copies and the database commands form one ordered timeline. The
// order of those two is the whole safety property of a clone, and it cannot be
// asserted from two separate lists.
type copyLog struct {
	runner     *recordingRunner
	identities []Identity
	fail       map[string]error
}

// installCopyLog puts the recorder in place of rsync.
func installCopyLog(staging *Staging, rec *recordingRunner) *copyLog {
	log := &copyLog{runner: rec, fail: map[string]error{}}
	staging.copy = func(_ context.Context, site Site, from, to string) error {
		rec.commands = append(rec.commands, "COPY "+from+" -> "+to)
		log.identities = append(log.identities, site.Identity)
		for marker, err := range log.fail {
			if strings.Contains(to, marker) || strings.Contains(from, marker) {
				return err
			}
		}
		return os.MkdirAll(to, 0o750)
	}
	return log
}

func (c *copyLog) copiedInto(dest string) bool {
	return c.runner.ran("COPY ") && c.runner.indexOf("-> "+dest) >= 0
}

func (c *copyLog) copies() []string {
	var out []string
	for _, command := range c.runner.commands {
		if strings.HasPrefix(command, "COPY ") {
			out = append(out, strings.TrimPrefix(command, "COPY "))
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The database decision
// ---------------------------------------------------------------------------

// TestAPushWithNoDatabaseDecisionIsRefused is the test the third WordPress
// requirement asks for. There is no default, because "pushing a staging
// database over production is how a customer loses a week of orders".
func TestAPushWithNoDatabaseDecisionIsRefused(t *testing.T) {
	_, staging, rec := newRecordingClient(t)
	production, stagingSite, root := stagingPair(t)
	installCopyLog(staging, rec)

	for _, action := range []DatabaseAction{DatabaseUnset, DatabaseAction("yes"),
		DatabaseAction("true"), DatabaseAction("both"), DatabaseAction(" ")} {
		_, err := staging.Push(context.Background(), PushOptions{
			Production: production, Staging: stagingSite,
			Database:  action,
			BackupDir: filepath.Join(root, "backup"),
		})
		if !errors.Is(err, ErrDatabaseChoiceRequired) {
			t.Fatalf("Push with database=%q returned %v, want ErrDatabaseChoiceRequired", action, err)
		}
		if len(rec.commands) != 0 {
			t.Fatalf("a push with no database decision still ran commands: %v", rec.commands)
		}
	}

	// The refusal has to list the choices: an operator reading it in an API
	// response has nowhere else to look.
	message := ErrDatabaseChoiceRequired.Error()
	for _, choice := range []string{"keep_production", "overwrite_production", "database_only"} {
		if !strings.Contains(message, choice) {
			t.Errorf("the refusal does not offer %q", choice)
		}
	}
	if !strings.Contains(message, "week of orders") {
		t.Error("the refusal does not say what is at stake")
	}
}

// TestKeepProductionNeverTouchesTheProductionDatabase is the choice an operator
// should pick unless they know why not, and the one whose implementation must
// be provably inert on the database.
func TestKeepProductionNeverTouchesTheProductionDatabase(t *testing.T) {
	_, staging, rec := newRecordingClient(t)
	production, stagingSite, root := stagingPair(t)
	installCopyLog(staging, rec)

	result, err := staging.Push(context.Background(), PushOptions{
		Production: production, Staging: stagingSite,
		Database:  DatabaseKeepProduction,
		BackupDir: filepath.Join(root, "backup"),
	})
	if err != nil {
		t.Fatalf("a files-only push failed: %v", err)
	}
	if !result.FilesCopied {
		t.Error("a files-only push did not copy the files")
	}
	if result.DatabaseCopied {
		t.Fatal("a files-only push reported that it copied the database")
	}

	// The only database command allowed is the backup export of production.
	// db import and db reset must never appear.
	for _, forbidden := range []string{"db import", "db reset"} {
		if rec.ran(forbidden) {
			t.Fatalf("a keep_production push ran %q against production: %v", forbidden, rec.commands)
		}
	}
	if !rec.ran("db export") {
		t.Fatal("production was not backed up before the push")
	}
	if result.DatabaseBackup == "" || result.BackupPath == "" {
		t.Fatal("the push did not report where production was backed up")
	}
}

// TestOverwriteProductionBacksUpBeforeItOverwrites. The choice is destructive
// by definition; the recovery path is the backup, and it must exist BEFORE the
// import runs, not after.
func TestOverwriteProductionBacksUpBeforeItOverwrites(t *testing.T) {
	_, staging, rec := newRecordingClient(t)
	production, stagingSite, root := stagingPair(t)
	installCopyLog(staging, rec)

	result, err := staging.Push(context.Background(), PushOptions{
		Production: production, Staging: stagingSite,
		Database:  DatabaseOverwriteProduction,
		BackupDir: filepath.Join(root, "backup"),
	})
	if err != nil {
		t.Fatalf("the push failed: %v", err)
	}
	if !result.DatabaseCopied {
		t.Fatal("an overwrite_production push did not copy the database")
	}

	backupAt := rec.indexOf("db export " + result.DatabaseBackup)
	importAt := rec.indexOf("db import")
	resetAt := rec.indexOf("db reset")
	if backupAt < 0 {
		t.Fatalf("production was never backed up: %v", rec.commands)
	}
	if importAt < 0 {
		t.Fatalf("the staging database was never imported: %v", rec.commands)
	}
	if backupAt > importAt {
		t.Fatal("production was backed up AFTER it was overwritten; the backup is of the " +
			"staging data, and the customer's orders are gone")
	}
	if resetAt > importAt {
		t.Fatal("the target database was emptied after the import rather than before it")
	}

	// After importing staging's database, production's URLs must be rewritten
	// back to production - otherwise every link on the live site points at
	// staging.
	if !rec.ran("search-replace https://staging.example.com https://example.com") {
		t.Fatalf("the URLs were not rewritten back to production: %v", rec.commands)
	}
	// And production must not stay hidden from search engines.
	if !rec.ran("option update blog_public 1") {
		t.Fatal("production was left with blog_public=0, so it drops out of the search index")
	}
}

// TestDatabaseOnlyLeavesProductionFilesAlone.
func TestDatabaseOnlyLeavesProductionFilesAlone(t *testing.T) {
	_, staging, rec := newRecordingClient(t)
	production, stagingSite, root := stagingPair(t)
	copies := installCopyLog(staging, rec)

	result, err := staging.Push(context.Background(), PushOptions{
		Production: production, Staging: stagingSite,
		Database:  DatabaseOnly,
		BackupDir: filepath.Join(root, "backup"),
	})
	if err != nil {
		t.Fatalf("a database_only push failed: %v", err)
	}
	if result.FilesCopied {
		t.Fatal("a database_only push reported that it copied files")
	}
	if !result.DatabaseCopied {
		t.Fatal("a database_only push did not copy the database")
	}
	if copies.copiedInto(production.Site.Dir) {
		t.Fatalf("a database_only push copied files INTO production: %v", copies.copies())
	}
	// The pre-push backup of production's files still has to happen: a
	// database_only push that goes wrong is recovered by rolling both halves
	// back together.
	made := copies.copies()
	if len(made) != 1 || !strings.HasPrefix(made[0], production.Site.Dir+" -> ") {
		t.Fatalf("production's files were not backed up before the push: %v", made)
	}
}

// TestAPushWithNowhereToBackUpToIsRefused. A push whose backup cannot be
// written has no recovery path, so it does not run.
func TestAPushWithNowhereToBackUpToIsRefused(t *testing.T) {
	_, staging, rec := newRecordingClient(t)
	production, stagingSite, _ := stagingPair(t)
	installCopyLog(staging, rec)

	for _, dir := range []string{"", "relative/backup", "/tmp/../etc"} {
		if _, err := staging.Push(context.Background(), PushOptions{
			Production: production, Staging: stagingSite,
			Database: DatabaseKeepProduction, BackupDir: dir,
		}); err == nil {
			t.Fatalf("a push with backup dir %q was accepted", dir)
		}
	}
	if len(rec.commands) != 0 {
		t.Fatalf("commands ran despite the refusal: %v", rec.commands)
	}
}

// ---------------------------------------------------------------------------
// Clone
// ---------------------------------------------------------------------------

// TestCloneCopiesFilesBeforeTheDatabaseAndRewritesStagingsUrls.
//
// The order is the whole safety property: copying the database before the files
// leaves a window where staging's wp-config.php still points at production's
// database, and a search-replace in that window rewrites PRODUCTION.
func TestCloneCopiesFilesBeforeTheDatabaseAndRewritesStagingsUrls(t *testing.T) {
	_, staging, rec := newRecordingClient(t)
	production, stagingSite, _ := stagingPair(t)
	copies := installCopyLog(staging, rec)

	result, err := staging.Clone(context.Background(), CloneOptions{
		Production: production, Staging: stagingSite, BlockIndexing: true,
	})
	if err != nil {
		t.Fatalf("the clone failed: %v", err)
	}
	if result.StagingURL != "https://staging.example.com" {
		t.Fatalf("the clone reports the staging URL as %q", result.StagingURL)
	}

	// The files land BEFORE the database. Copying the database first leaves a
	// window in which staging's wp-config.php still points at production's
	// database.
	copyAt := rec.indexOf("COPY " + production.Site.Dir + " -> " + stagingSite.Site.Dir)
	if copyAt < 0 {
		t.Fatalf("production's files were never copied to staging: %v", rec.commands)
	}
	if !copies.copiedInto(stagingSite.Site.Dir) {
		t.Fatal("nothing was copied into the staging directory")
	}

	// wp-config.php is pointed at the staging database BEFORE any database
	// command runs against the staging path.
	configAt := rec.indexOf("config set DB_NAME wp_stg")
	importAt := rec.indexOf("db import")
	replaceAt := rec.indexOf("search-replace https://example.com https://staging.example.com")
	if configAt < 0 {
		t.Fatalf("staging's wp-config.php was never pointed at the staging database: %v", rec.commands)
	}
	if importAt < 0 {
		t.Fatalf("the database was never imported into staging: %v", rec.commands)
	}
	if copyAt > configAt {
		t.Fatal("staging's wp-config.php was repointed before the files were copied; the copy " +
			"would then overwrite it with production's, and the next database command would " +
			"run against production")
	}
	if configAt > importAt {
		t.Fatal("the database was imported before staging's wp-config.php was repointed; the " +
			"import ran against production")
	}
	if replaceAt < 0 {
		t.Fatalf("staging's URLs were never rewritten: %v", rec.commands)
	}
	if replaceAt < importAt {
		t.Fatal("the URL rewrite ran before the database was imported, so it rewrote " +
			"production's data")
	}

	// The rewrite runs against staging's path, never production's.
	for _, command := range rec.commands {
		if strings.Contains(command, "search-replace") &&
			strings.Contains(command, "--path="+production.Site.Dir) {
			t.Fatalf("a search-replace ran against the PRODUCTION path: %q", command)
		}
	}

	if !rec.ran("option update blog_public 0") {
		t.Fatal("the staging copy was not hidden from search engines, so two copies of the " +
			"site end up in the index")
	}
}

// TestCloneRefusesAPairThatWouldDestroyTheSite covers every way the two sites
// can be the same thing.
func TestCloneRefusesAPairThatWouldDestroyTheSite(t *testing.T) {
	_, staging, rec := newRecordingClient(t)
	production, stagingSite, root := stagingPair(t)
	installCopyLog(staging, rec)

	cases := []struct {
		name   string
		mutate func(*StagingSite, *StagingSite)
		want   string
	}{
		{"the same directory", func(p, s *StagingSite) {
			s.Site.Dir = p.Site.Dir
		}, "are the same"},
		{"staging nested inside production", func(p, s *StagingSite) {
			s.Site.Dir = filepath.Join(p.Site.Dir, "staging")
		}, "nested"},
		{"production nested inside staging", func(p, s *StagingSite) {
			p.Site.Dir = filepath.Join(s.Site.Dir, "production")
		}, "nested"},
		{"the same URL", func(p, s *StagingSite) {
			s.Site.URL = p.Site.URL
		}, "same URL"},
		{"the same database", func(p, s *StagingSite) {
			s.DBName = p.DBName
		}, "share the database"},
		{"a root identity", func(p, s *StagingSite) {
			s.Site.Identity = Identity{Name: "root"}
		}, "refusing to run WP-CLI"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec.commands = nil
			p, s := production, stagingSite
			tc.mutate(&p, &s)

			if _, err := staging.Clone(context.Background(), CloneOptions{Production: p, Staging: s}); err == nil {
				t.Fatal("the clone was accepted")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refused with %q, want a message containing %q", err, tc.want)
			}
			if _, err := staging.Push(context.Background(), PushOptions{
				Production: p, Staging: s, Database: DatabaseKeepProduction,
				BackupDir: filepath.Join(root, "backup"),
			}); err == nil {
				t.Fatal("the push was accepted")
			}
			if len(rec.commands) != 0 {
				t.Fatalf("commands ran despite the refusal: %v", rec.commands)
			}
		})
	}
}

// TestEveryStagingCommandRunsAsTheSiteUser. Staging and production share one
// unix user on purpose; what must never happen is either of them running as
// root.
func TestEveryStagingCommandRunsAsTheSiteUser(t *testing.T) {
	var identities []Identity
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(_ context.Context, cmd *exec.Cmd) (*Result, error) {
		if cmd.SysProcAttr == nil || cmd.SysProcAttr.Credential == nil {
			t.Fatal("a staging command was launched with no credential")
		}
		credential := cmd.SysProcAttr.Credential
		identities = append(identities, Identity{UID: credential.Uid, GID: credential.Gid})
		return &Result{Stdout: "[]"}, nil
	}
	client := NewClient(runner, zap.NewNop())
	staging := NewStaging(client, zap.NewNop())
	staging.copy = func(_ context.Context, site Site, _, to string) error {
		identities = append(identities, site.Identity)
		return os.MkdirAll(to, 0o750)
	}
	production, stagingSite, _ := stagingPair(t)

	if _, err := staging.Clone(context.Background(), CloneOptions{
		Production: production, Staging: stagingSite, BlockIndexing: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(identities) == 0 {
		t.Fatal("no commands ran")
	}
	for _, identity := range identities {
		if identity.UID == 0 || identity.GID == 0 {
			t.Fatalf("a staging command ran as uid %d gid %d", identity.UID, identity.GID)
		}
		if identity.UID != nonRoot().UID {
			t.Fatalf("a staging command ran as uid %d, want the site user %d",
				identity.UID, nonRoot().UID)
		}
	}
}

// TestDatabaseActionValidity is the small guard on the enum itself.
func TestDatabaseActionValidity(t *testing.T) {
	valid := []DatabaseAction{DatabaseKeepProduction, DatabaseOverwriteProduction, DatabaseOnly}
	for _, action := range valid {
		if !action.Valid() {
			t.Errorf("%q should be valid", action)
		}
	}
	if DatabaseUnset.Valid() {
		t.Fatal("the zero value must never be a valid choice: a caller that forgets the field " +
			"would get whichever behaviour the zero value happens to mean")
	}
}
