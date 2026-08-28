package upgrade

// Progress reporting.
//
// The CLI renders a spinner and the API streams status to the browser, and
// neither can do that from log lines: they need to know which of a known set of
// steps is running and whether it finished. So the steps are a typed enum, the
// caller supplies one callback, and log output stays a separate concern.

import "time"

// Step names one stage of an upgrade. The zero value is StepNone so that an
// unset Step in an Event is obviously unset rather than silently "lock".
type Step int

const (
	// StepNone is the zero value: no step is running.
	StepNone Step = iota
	// StepLock takes the upgrade lock, recovering one left by a dead process.
	StepLock
	// StepCheck fetches the feed and decides whether an upgrade applies.
	StepCheck
	// StepDownload fetches the release tarball into the panel's tmp directory.
	StepDownload
	// StepVerify checks the tarball's sha256 against the manifest.
	StepVerify
	// StepStage extracts the verified tarball into a staging directory.
	StepStage
	// StepPreflight runs the last checks before anything is switched.
	StepPreflight
	// StepBackupDatabase dumps the database and records where it went.
	StepBackupDatabase
	// StepSwitch promotes the staging directory and repoints the symlink.
	StepSwitch
	// StepRestart restarts the panel services.
	StepRestart
	// StepHealthCheck waits for the services to report healthy.
	StepHealthCheck
	// StepRollback puts the previous release back. Only reached on failure.
	StepRollback
	// StepPrune removes releases beyond the retention limit.
	StepPrune
	// StepCleanup removes the downloaded tarball and any staging leftovers.
	StepCleanup
	// StepDone is emitted once, last, when the upgrade finished.
	StepDone
)

// String returns a stable machine-readable identifier for the step. It is used
// in API payloads as well as in messages, so it must not be prettified.
func (s Step) String() string {
	switch s {
	case StepNone:
		return "none"
	case StepLock:
		return "lock"
	case StepCheck:
		return "check"
	case StepDownload:
		return "download"
	case StepVerify:
		return "verify"
	case StepStage:
		return "stage"
	case StepPreflight:
		return "preflight"
	case StepBackupDatabase:
		return "backup_database"
	case StepSwitch:
		return "switch"
	case StepRestart:
		return "restart"
	case StepHealthCheck:
		return "health_check"
	case StepRollback:
		return "rollback"
	case StepPrune:
		return "prune"
	case StepCleanup:
		return "cleanup"
	case StepDone:
		return "done"
	default:
		return "unknown"
	}
}

// Description is the human-readable line a CLI or the UI shows next to the step.
func (s Step) Description() string {
	switch s {
	case StepLock:
		return "Acquiring the upgrade lock"
	case StepCheck:
		return "Checking for a new release"
	case StepDownload:
		return "Downloading the release"
	case StepVerify:
		return "Verifying the download checksum"
	case StepStage:
		return "Staging the new release"
	case StepPreflight:
		return "Running preflight checks"
	case StepBackupDatabase:
		return "Backing up the database"
	case StepSwitch:
		return "Switching to the new release"
	case StepRestart:
		return "Restarting services"
	case StepHealthCheck:
		return "Waiting for services to become healthy"
	case StepRollback:
		return "Rolling back to the previous release"
	case StepPrune:
		return "Pruning old releases"
	case StepCleanup:
		return "Cleaning up temporary files"
	case StepDone:
		return "Upgrade complete"
	default:
		return "Idle"
	}
}

// MarshalText makes Step serialise as its identifier in JSON, so an API client
// sees {"step":"health_check"} and not {"step":10}, which would change meaning
// the day a step is inserted.
func (s Step) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Status is what happened to a step.
type Status string

const (
	// StatusStarted is emitted when a step begins.
	StatusStarted Status = "started"
	// StatusSucceeded is emitted when a step completes.
	StatusSucceeded Status = "succeeded"
	// StatusFailed is emitted when a step aborts the upgrade.
	StatusFailed Status = "failed"
	// StatusSkipped is emitted for a step that did not apply, such as the
	// database backup when no database is configured.
	StatusSkipped Status = "skipped"
)

// Event is one progress notification. Err is set only when Status is
// StatusFailed.
type Event struct {
	Step    Step      `json:"step"`
	Status  Status    `json:"status"`
	Message string    `json:"message,omitempty"`
	At      time.Time `json:"at"`
	Err     error     `json:"-"`
	// Error is the rendered form of Err, so an API response carries the
	// reason without the caller having to reach into a Go error value.
	Error string `json:"error,omitempty"`
}

// ProgressFunc receives every Event. It is called synchronously on the
// goroutine running the upgrade, so an implementation that blocks stalls the
// upgrade; write to a buffered channel rather than doing work here.
type ProgressFunc func(Event)
