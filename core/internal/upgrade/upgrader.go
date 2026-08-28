package upgrade

// The Upgrader itself: its configuration, its injected dependencies, and the
// step sequence that Run walks.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Default service unit names. They match deploy/systemd.
var defaultServices = []string{"vkai-api", "vkai-ui", "vkai-agent"}

const (
	// DefaultRoot is the installation root. It mirrors
	// config.DefaultPanelRoot; it is repeated rather than imported so this
	// package has no dependency on the panel's configuration loader and can
	// be exercised entirely from a temporary directory.
	DefaultRoot = "/vkai-panel"

	// DefaultKeepReleases is how many release directories survive a prune.
	// Five is two upgrades' worth of history plus room for a bad week.
	DefaultKeepReleases = 5

	// DefaultHealthTimeout bounds how long the services get to come up
	// before the upgrade is treated as failed and rolled back.
	DefaultHealthTimeout = 90 * time.Second

	// DefaultHealthInterval is the gap between health probes.
	DefaultHealthInterval = 3 * time.Second

	// DefaultDownloadTimeout bounds the tarball download.
	DefaultDownloadTimeout = 15 * time.Minute

	// DefaultFeedTimeout bounds the release feed fetch. The feed is a small
	// JSON document, so this is short on purpose.
	DefaultFeedTimeout = 30 * time.Second

	// DefaultMaxDownloadBytes caps the compressed tarball.
	DefaultMaxDownloadBytes int64 = 1 << 30 // 1 GiB

	// DefaultMaxExtractBytes caps the uncompressed release, which is the
	// bound that matters against a decompression bomb.
	DefaultMaxExtractBytes int64 = 4 << 30 // 4 GiB

	// DefaultMaxArchiveEntries caps the number of members in the tarball.
	DefaultMaxArchiveEntries = 200000

	// DefaultDatabaseDumpEstimate is how much room preflight reserves for
	// the pre-upgrade dump when the caller does not say.
	DefaultDatabaseDumpEstimate int64 = 512 << 20 // 512 MiB

	// DefaultDiskSafetyMargin is headroom kept free on top of everything
	// the upgrade itself needs, so a full disk is not the outcome.
	DefaultDiskSafetyMargin int64 = 256 << 20 // 256 MiB

	// DefaultStaleLockAge is when a lock whose owner cannot be probed is
	// treated as abandoned.
	DefaultStaleLockAge = 6 * time.Hour

	// DefaultUserAgent identifies the panel to the release feed.
	DefaultUserAgent = "vkai-panel-upgrader"
)

// DatabaseBackupConfig describes how to take the pre-upgrade dump.
//
// The command is run through Deps.Runner, so it inherits the API process's
// environment - which is where PGPASSWORD from /vkai-panel/etc/.env lives. Two
// placeholders are substituted in Args: {{dest}} becomes the dump path and
// {{database}} becomes Name.
type DatabaseBackupConfig struct {
	// Enabled turns the dump on. When false the step is reported as skipped
	// and Result.DatabaseBackupPath stays empty.
	Enabled bool
	// Command is the dump binary, "pg_dump" by default.
	Command string
	// Args are its arguments, with {{dest}} and {{database}} substituted.
	Args []string
	// Name is the database to dump.
	Name string
	// Timeout bounds the dump.
	Timeout time.Duration
	// EstimateBytes is how much disk preflight reserves for the dump.
	EstimateBytes int64
}

// Config is everything about this installation that the upgrader needs.
type Config struct {
	// Root is the installation root, /vkai-panel by default. Every other
	// path is derived from it, which is what makes the tests able to run
	// against a temporary directory.
	Root string

	// FeedURL is where the release manifest is published.
	FeedURL string

	// CurrentVersion is the version running now. When empty it is read from
	// the state file, and failing that from the current symlink's target.
	CurrentVersion string

	// Services are the systemd units to restart and health-check.
	Services []string

	// HealthURL, when set, is probed with a GET after the restart in
	// addition to systemctl is-active. It should be the panel's own health
	// endpoint on its own port.
	HealthURL string

	// KeepReleases is how many release directories to keep on prune.
	KeepReleases int

	// HealthTimeout bounds the wait for the services to become healthy.
	HealthTimeout time.Duration
	// HealthInterval is the gap between health probes.
	HealthInterval time.Duration
	// DownloadTimeout bounds the tarball download.
	DownloadTimeout time.Duration
	// FeedTimeout bounds the release feed fetch.
	FeedTimeout time.Duration

	// MaxDownloadBytes caps the compressed download.
	MaxDownloadBytes int64
	// MaxExtractBytes caps the uncompressed release.
	MaxExtractBytes int64
	// MaxArchiveEntries caps the number of tarball members.
	MaxArchiveEntries int

	// DiskSafetyMargin is free space kept over and above what the upgrade
	// needs.
	DiskSafetyMargin int64

	// StaleLockAge is when a lock file with an unprobeable owner is treated
	// as abandoned.
	StaleLockAge time.Duration

	// UserAgent is sent on feed and tarball requests.
	UserAgent string

	// Database configures the pre-upgrade dump.
	Database DatabaseBackupConfig

	// Progress receives every step transition. Optional.
	Progress ProgressFunc

	// HealthCheck overrides how health is determined. The default probes
	// each service with systemctl is-active and then, if HealthURL is set,
	// issues a GET expecting a 2xx.
	HealthCheck func(ctx context.Context) error
}

// Doer is the slice of *http.Client the upgrader uses.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// CommandRunner runs an external command. Everything privileged the upgrader
// does - systemctl, pg_dump - goes through here, which is what lets the tests
// assert on the exact command line without a systemd on the machine.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Clock is time, injected. Sleep is part of it because the health check waits,
// and a test that waits ninety real seconds is a test nobody runs.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// Deps are the outside-world dependencies. Any field left nil gets a real
// implementation, so production callers pass Deps{} and tests fill in what
// they want to control.
type Deps struct {
	// HTTP fetches the feed and the tarball.
	HTTP Doer
	// Runner runs systemctl and the database dump.
	Runner CommandRunner
	// Clock supplies Now and Sleep.
	Clock Clock
	// DiskFree reports the bytes available to a non-root writer at path.
	DiskFree func(path string) (uint64, error)
	// ProcessAlive reports whether a pid is still running. It decides
	// whether a lock file is held or merely abandoned.
	ProcessAlive func(pid int) bool
	// PID is the pid written into the lock file.
	PID int
}

// Upgrader upgrades one installation. It is not safe for concurrent use by
// several goroutines, and the on-disk lock stops several processes; both are
// deliberate, since two upgrades of one machine is exactly the situation this
// package exists to prevent.
type Upgrader struct {
	cfg  Config
	deps Deps

	// events is the transcript of the run in progress. It is reset at the
	// start of Run and copied into the Result, so a caller that supplied no
	// Progress callback still gets the whole sequence back.
	events []Event
}

// New builds an Upgrader, filling in defaults for everything the caller left
// unset. It fails only on configuration that could not possibly work.
func New(cfg Config, deps Deps) (*Upgrader, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		cfg.Root = DefaultRoot
	}
	cfg.Root = filepath.Clean(cfg.Root)
	if !filepath.IsAbs(cfg.Root) {
		return nil, fmt.Errorf("upgrade root %q must be an absolute path", cfg.Root)
	}
	if len(cfg.Services) == 0 {
		cfg.Services = append([]string(nil), defaultServices...)
	}
	if cfg.KeepReleases <= 0 {
		cfg.KeepReleases = DefaultKeepReleases
	}
	if cfg.HealthTimeout <= 0 {
		cfg.HealthTimeout = DefaultHealthTimeout
	}
	if cfg.HealthInterval <= 0 {
		cfg.HealthInterval = DefaultHealthInterval
	}
	if cfg.DownloadTimeout <= 0 {
		cfg.DownloadTimeout = DefaultDownloadTimeout
	}
	if cfg.FeedTimeout <= 0 {
		cfg.FeedTimeout = DefaultFeedTimeout
	}
	if cfg.MaxDownloadBytes <= 0 {
		cfg.MaxDownloadBytes = DefaultMaxDownloadBytes
	}
	if cfg.MaxExtractBytes <= 0 {
		cfg.MaxExtractBytes = DefaultMaxExtractBytes
	}
	if cfg.MaxArchiveEntries <= 0 {
		cfg.MaxArchiveEntries = DefaultMaxArchiveEntries
	}
	if cfg.DiskSafetyMargin <= 0 {
		cfg.DiskSafetyMargin = DefaultDiskSafetyMargin
	}
	if cfg.StaleLockAge <= 0 {
		cfg.StaleLockAge = DefaultStaleLockAge
	}
	if strings.TrimSpace(cfg.UserAgent) == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.Database.Enabled {
		if strings.TrimSpace(cfg.Database.Command) == "" {
			cfg.Database.Command = "pg_dump"
		}
		if len(cfg.Database.Args) == 0 {
			cfg.Database.Args = []string{
				"--no-owner", "--no-acl",
				"--format=custom",
				"--file={{dest}}",
				"{{database}}",
			}
		}
		if cfg.Database.Timeout <= 0 {
			cfg.Database.Timeout = 30 * time.Minute
		}
	}
	if cfg.Database.EstimateBytes <= 0 {
		cfg.Database.EstimateBytes = DefaultDatabaseDumpEstimate
	}

	if deps.HTTP == nil {
		deps.HTTP = &http.Client{Timeout: 0}
	}
	if deps.Runner == nil {
		deps.Runner = ExecRunner{}
	}
	if deps.Clock == nil {
		deps.Clock = SystemClock{}
	}
	if deps.DiskFree == nil {
		deps.DiskFree = diskFree
	}
	if deps.ProcessAlive == nil {
		deps.ProcessAlive = processAlive
	}
	if deps.PID == 0 {
		deps.PID = os.Getpid()
	}

	return &Upgrader{cfg: cfg, deps: deps}, nil
}

// Config returns a copy of the resolved configuration, defaults filled in.
func (u *Upgrader) Config() Config { return u.cfg }

// ---------------------------------------------------------------- paths

// ReleasesDir is /vkai-panel/releases.
func (u *Upgrader) ReleasesDir() string { return filepath.Join(u.cfg.Root, "releases") }

// CurrentLink is /vkai-panel/current, the symlink the systemd units point at.
func (u *Upgrader) CurrentLink() string { return filepath.Join(u.cfg.Root, "current") }

// EtcDir is /vkai-panel/etc.
func (u *Upgrader) EtcDir() string { return filepath.Join(u.cfg.Root, "etc") }

// TmpDir is /vkai-panel/tmp, where downloads land.
func (u *Upgrader) TmpDir() string { return filepath.Join(u.cfg.Root, "tmp") }

// LockFile is /vkai-panel/etc/upgrade.lock.
func (u *Upgrader) LockFile() string { return filepath.Join(u.EtcDir(), "upgrade.lock") }

// StateFile is /vkai-panel/etc/upgrade_state.json. It is a plain file rather
// than a database row on purpose: the upgrader has to work when the database is
// exactly the thing that is down.
func (u *Upgrader) StateFile() string { return filepath.Join(u.EtcDir(), "upgrade_state.json") }

// DatabaseBackupDir is /vkai-panel/www/backup/databases, matching
// config.DatabaseBackupDir.
func (u *Upgrader) DatabaseBackupDir() string {
	return filepath.Join(u.cfg.Root, "www", "backup", "databases")
}

// ReleaseDir is the final directory of one release.
func (u *Upgrader) ReleaseDir(version string) string {
	return filepath.Join(u.ReleasesDir(), version)
}

// stagingDir is where a release is extracted before preflight promotes it. It
// lives beside the final directory so the promotion is a rename within one
// filesystem, and it is named so that a leftover cannot be mistaken for a
// release: prune and version listing both ignore anything that is not a plain
// version.
func (u *Upgrader) stagingDir(version string) string {
	return filepath.Join(u.ReleasesDir(), fmt.Sprintf(".staging-%s-%d", version, u.deps.PID))
}

// ---------------------------------------------------------------- progress

func (u *Upgrader) emit(step Step, status Status, msg string, err error) {
	ev := Event{
		Step:    step,
		Status:  status,
		Message: msg,
		At:      u.deps.Clock.Now(),
		Err:     err,
	}
	if err != nil {
		ev.Error = err.Error()
	}
	u.events = append(u.events, ev)
	if u.cfg.Progress != nil {
		u.cfg.Progress(ev)
	}
}

func (u *Upgrader) started(step Step) { u.emit(step, StatusStarted, step.Description(), nil) }

func (u *Upgrader) succeeded(step Step, msg string) { u.emit(step, StatusSucceeded, msg, nil) }

func (u *Upgrader) skipped(step Step, msg string) { u.emit(step, StatusSkipped, msg, nil) }

// failed emits the failure and returns the error unchanged, so call sites read
// "return u.failed(StepX, err)".
func (u *Upgrader) failed(step Step, err error) error {
	u.emit(step, StatusFailed, step.Description()+" failed", err)
	return err
}

// ---------------------------------------------------------------- results

// CheckResult is what a check-for-updates call reports.
type CheckResult struct {
	// CurrentVersion is what is running.
	CurrentVersion string `json:"current_version"`
	// LatestVersion is the newest version the feed lists, whether or not it
	// can be installed from here.
	LatestVersion string `json:"latest_version"`
	// UpdateAvailable is true when Target is set and installable.
	UpdateAvailable bool `json:"update_available"`
	// Target is the release Run would install. Nil when up to date or when
	// the jump is blocked.
	Target *Manifest `json:"target,omitempty"`
	// Blocked is true when a newer release exists but min_upgrade_from
	// refuses the jump.
	Blocked bool `json:"blocked"`
	// InstallFirst names the version to install before the blocked release.
	InstallFirst string `json:"install_first,omitempty"`
}

// Result is the outcome of Run. It is returned even when the upgrade failed,
// because the operator needs to know how far it got and where the database dump
// went.
type Result struct {
	// FromVersion is the version that was running when Run started.
	FromVersion string `json:"from_version"`
	// ToVersion is the version that was being installed.
	ToVersion string `json:"to_version"`
	// Manifest is the release that was acted on.
	Manifest Manifest `json:"manifest"`
	// ReleaseDir is where the new release was staged and promoted to.
	ReleaseDir string `json:"release_dir,omitempty"`
	// PreviousRelease is the directory the current symlink pointed at
	// before the switch, and therefore the rollback target.
	PreviousRelease string `json:"previous_release,omitempty"`
	// DatabaseBackupPath is where the pre-upgrade dump was written. Empty
	// when the database backup was not configured or was never reached.
	DatabaseBackupPath string `json:"database_backup_path,omitempty"`
	// Switched is true once the current symlink pointed at the new release,
	// whether or not it stayed there.
	Switched bool `json:"switched"`
	// Succeeded is true only when the new release is live and healthy.
	Succeeded bool `json:"succeeded"`
	// RolledBack is true when the previous release was restored and is
	// healthy again.
	RolledBack bool `json:"rolled_back"`
	// NeedsManualIntervention is true when the rollback itself failed. This
	// is the field a monitoring integration alerts on.
	NeedsManualIntervention bool `json:"needs_manual_intervention"`
	// Pruned lists the release directories removed by the prune step.
	Pruned []string `json:"pruned,omitempty"`
	// Events is every progress event, in order.
	Events []Event `json:"events,omitempty"`
	// StartedAt and FinishedAt bound the run.
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}
