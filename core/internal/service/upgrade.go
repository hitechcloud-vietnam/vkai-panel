package service

// Panel self-upgrade, as the API and the UI see it.
//
// The privileged work - fetching a release, swapping /vkai-panel/current,
// restarting the units, rolling back when the new release will not come up -
// belongs to internal/upgrade and to /vkai-panel/bin/vkai-deploy. This file is
// the request-shaped layer in front of it, and it exists to solve three
// problems that the engine cannot solve on its own:
//
//   - The HTTP request must return immediately. An upgrade takes minutes and
//     the connection that started it does not survive the restart, so Start
//     hands back a job id and everything after that is polled.
//
//   - The API restarts in the middle of its own job. Progress held only in
//     memory disappears exactly when the operator needs it most, so the job is
//     mirrored to a small file under /vkai-panel/etc and reconciled against the
//     running version when the process comes back. The running version is the
//     ground truth: if the process that came back is the new version the
//     upgrade succeeded, if it is the old one the release was rolled back.
//
//   - Two upgrades at once would race for the release directory and the
//     symlink. One job may be in flight, process-wide and across a restart.
//
// The engine is reached through UpgradeEngine, a three-method interface
// declared here rather than imported. That keeps this package compiling on its
// own, and makes the eventual wiring a struct conversion instead of a rewrite:
// internal/upgrade's Status and Progress have the same fields in the same
// order as UpgradeStatus and UpgradeProgress, so
//
//	service.UpgradeEngineFuncs{
//	    CheckFunc: func(ctx context.Context) (service.UpgradeStatus, error) {
//	        s, err := engine.Check(ctx)
//	        return service.UpgradeStatus(s), err
//	    },
//	    ...
//	}
//
// is the whole adapter.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/version"
)

// ---------------------------------------------------------------------------
// Build identity
// ---------------------------------------------------------------------------

// UpgradeBuildInfo is what GET /api/v1/version answers with, and nothing else.
//
// The values come from internal/version, which is the one place in the product
// that knows what release this binary is. What is deliberately *not* copied
// across is the rest of version.Info: the Go toolchain and the target platform
// are in that struct for the CLI banner and for the authenticated /api/v1/system
// route, and an unauthenticated endpoint that names the Go version hands a
// scanner the CVE list for this host for free. Three fields is what the upgrade
// flow needs, so three fields is what this answers.
type UpgradeBuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"build_date"`
}

// ResolveBuildInfo returns the identity of the running binary.
func ResolveBuildInfo() UpgradeBuildInfo {
	info := version.Get()
	return UpgradeBuildInfo{
		Version: info.Version,
		Commit:  info.Commit,
		Date:    info.BuildDate,
	}
}

// ---------------------------------------------------------------------------
// The engine contract
// ---------------------------------------------------------------------------

// UpgradeStatus is what a release check found. Field-compatible with
// upgrade.Status.
type UpgradeStatus struct {
	Current         string
	Latest          string
	UpdateAvailable bool
	CheckedAt       time.Time
	Changelog       string
}

// UpgradeProgress is where a running job has got to. Field-compatible with
// upgrade.Progress.
type UpgradeProgress struct {
	Step    string
	Percent int
	Message string
}

// UpgradeEngine performs the release work. Every method must be safe to call
// from more than one goroutine.
type UpgradeEngine interface {
	Check(ctx context.Context) (UpgradeStatus, error)
	Start(ctx context.Context, version string) (string, error)
	Progress(ctx context.Context, jobID string) (UpgradeProgress, error)
}

// UpgradeEngineFuncs adapts a foreign engine to UpgradeEngine without this
// package importing it. A nil field is treated as "not implemented" rather
// than panicking, because a half-wired engine must fail as an unavailable
// service and not as a crashed API.
type UpgradeEngineFuncs struct {
	CheckFunc    func(ctx context.Context) (UpgradeStatus, error)
	StartFunc    func(ctx context.Context, version string) (string, error)
	ProgressFunc func(ctx context.Context, jobID string) (UpgradeProgress, error)
}

func (e UpgradeEngineFuncs) Check(ctx context.Context) (UpgradeStatus, error) {
	if e.CheckFunc == nil {
		return UpgradeStatus{}, ErrUpgradeUnavailable
	}
	return e.CheckFunc(ctx)
}

func (e UpgradeEngineFuncs) Start(ctx context.Context, version string) (string, error) {
	if e.StartFunc == nil {
		return "", ErrUpgradeUnavailable
	}
	return e.StartFunc(ctx, version)
}

func (e UpgradeEngineFuncs) Progress(ctx context.Context, jobID string) (UpgradeProgress, error) {
	if e.ProgressFunc == nil {
		return UpgradeProgress{}, ErrUpgradeUnavailable
	}
	return e.ProgressFunc(ctx, jobID)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrUpgradeUnavailable means no release engine is wired into this build,
	// so the panel can report its version but cannot upgrade itself.
	ErrUpgradeUnavailable = errors.New("the upgrade engine is not available on this instance")

	// ErrUpgradeInProgress guards the release directory and the current
	// symlink: two jobs would fight over both.
	ErrUpgradeInProgress = errors.New("an upgrade is already running")

	// ErrUpgradeJobNotFound is a job id this panel has no record of. After a
	// restart only the most recent job is remembered, so an older id lands
	// here.
	ErrUpgradeJobNotFound = errors.New("no upgrade job with that id")

	// ErrUpgradeNoTarget means there is nothing newer to install and the
	// caller did not name a version explicitly.
	ErrUpgradeNoTarget = errors.New("there is no newer version to install")
)

// UpgradeValidationError is a rejected request field, reported to the caller
// rather than logged and swallowed.
type UpgradeValidationError struct {
	Field   string
	Message string
}

func (e *UpgradeValidationError) Error() string { return e.Message }

// ---------------------------------------------------------------------------
// Job state
// ---------------------------------------------------------------------------

// UpgradeJobState is the lifecycle of one upgrade.
type UpgradeJobState string

const (
	// UpgradeJobRunning: the release work is still going on, or the API is
	// restarting in the middle of it.
	UpgradeJobRunning UpgradeJobState = "running"

	// UpgradeJobSucceeded: the panel is serving the target version.
	UpgradeJobSucceeded UpgradeJobState = "succeeded"

	// UpgradeJobFailed: the upgrade stopped and the previous release was not
	// confirmed back in place.
	UpgradeJobFailed UpgradeJobState = "failed"

	// UpgradeJobRolledBack: the upgrade failed and the previous release is
	// serving again. Distinct from failed because the operator needs to know
	// the panel is healthy on the old version.
	UpgradeJobRolledBack UpgradeJobState = "rolled_back"
)

// Terminal reports whether no further transition is expected.
func (s UpgradeJobState) Terminal() bool { return s != UpgradeJobRunning }

const (
	// UpgradeAuditResource is the audit log resource for every upgrade action.
	UpgradeAuditResource = "panel_upgrade"

	// upgradeStateFileName is the job mirror, kept beside the other panel state
	// under /vkai-panel/etc. It holds one job - the most recent - because its
	// only purpose is to answer "what happened to the upgrade I just started?"
	// across the restart that upgrade caused. It is not an upgrade history.
	upgradeStateFileName = "upgrade_job.json"

	// upgradePollInterval is how often the watcher asks the engine where it is.
	// Fast enough that the UI's own polling never shows a stale step, slow
	// enough to be free.
	upgradePollInterval = time.Second

	// upgradeRestartGrace is how long a job found already running at startup is
	// given before the panel concludes the release did not take. It covers a
	// service restart plus the health check the deploy script waits on.
	upgradeRestartGrace = 10 * time.Minute

	// upgradeMaxDuration stops the watcher from following a job forever if the
	// engine never reports a terminal step.
	upgradeMaxDuration = 60 * time.Minute
)

// upgradeJob is both the in-memory job and the on-disk mirror. One struct, so
// there is no way for the two to describe different things.
type upgradeJob struct {
	JobID       string          `json:"job_id"`
	State       UpgradeJobState `json:"state"`
	FromVersion string          `json:"from_version"`
	ToVersion   string          `json:"to_version"`
	Step        string          `json:"step"`
	Percent     int             `json:"percent"`
	Message     string          `json:"message"`
	Error       string          `json:"error,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`

	// StartedByUser is recorded so the panel can say who started an upgrade
	// that is still running when another administrator logs in.
	StartedByUser string `json:"started_by_user,omitempty"`

	// Detached marks a job whose engine-side watcher is gone: the API process
	// that started it was replaced by the upgrade. Progress for such a job is
	// decided by the running version, not by the engine.
	Detached bool `json:"detached,omitempty"`
}

// ---------------------------------------------------------------------------
// Canonical steps
// ---------------------------------------------------------------------------

// UpgradeStep is one entry in the step list the UI draws. The engine reports a
// free-form step name; this list gives that name a place in an ordered sequence
// so an operator can see what is done and what is still to come.
type UpgradeStep struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// UpgradeSteps is the sequence an upgrade goes through. It mirrors the steps
// the release engine emits (internal/upgrade.Step), so an operator watching this
// list is watching the real thing rather than an illustration of it.
//
// It is advisory in one direction only: a step the engine reports that is not in
// this list is still shown, as the active one, rather than being dropped.
var UpgradeSteps = []UpgradeStep{
	{Key: "lock", Label: "Take the upgrade lock"},
	{Key: "check", Label: "Check the release feed"},
	{Key: "download", Label: "Download the release"},
	{Key: "verify", Label: "Verify the package"},
	{Key: "stage", Label: "Unpack into a staging directory"},
	{Key: "preflight", Label: "Run the pre-flight checks"},
	{Key: "backup", Label: "Back up the database"},
	{Key: "switch", Label: "Switch to the new release"},
	{Key: "restart", Label: "Restart the panel services"},
	{Key: "health", Label: "Check the panel came back"},
	{Key: "cleanup", Label: "Clean up old releases"},
}

// upgradeStepAliases maps the words an engine may use onto the canonical keys,
// so "Downloading release", "download_release" and "download" all land on the
// same row. The engine's own step identifiers are all in here; the extra
// synonyms cost nothing and stop a renamed step from blanking the list.
var upgradeStepAliases = map[string]string{
	"lock":        "lock",
	"locking":     "lock",
	"check":       "check",
	"checking":    "check",
	"feed":        "check",
	"download":    "download",
	"downloading": "download",
	"fetch":       "download",
	"verify":      "verify",
	"verifying":   "verify",
	"checksum":    "verify",
	"signature":   "verify",
	"stage":       "stage",
	"staging":     "stage",
	"extract":     "stage",
	"unpack":      "stage",
	"preflight":   "preflight",
	"backup":      "backup",
	"snapshot":    "backup",
	"dump":        "backup",
	"switch":      "switch",
	"promote":     "switch",
	"symlink":     "switch",
	"activate":    "switch",
	"restart":     "restart",
	"restarting":  "restart",
	"services":    "restart",
	"health":      "health",
	"healthy":     "health",
	"cleanup":     "cleanup",
	"prune":       "cleanup",
	"finalize":    "cleanup",
}

// upgradeStepSplit breaks a reported step name into words on anything that is
// not a letter or a digit, so casing and separators stop mattering.
var upgradeStepSplit = regexp.MustCompile(`[^a-z0-9]+`)

// The words that end a job.
//
// Matching is positional, not "any word anywhere", because the engine's step
// names and its per-step outcome may arrive concatenated - "check succeeded",
// "restart failed", "rollback succeeded". Treating "succeeded" as terminal
// wherever it appears would call an upgrade finished at its second step.
//
//   - a failure word LAST ends the job as failed: "restart failed".
//   - a rollback word anywhere, with no failure word last, means the previous
//     release was put back: "rollback", "rolled_back", "rollback succeeded".
//     "rollback failed" is therefore a failure, which is right - the rollback
//     itself did not work.
//   - a success word FIRST ends the job as succeeded: "done", "completed",
//     "done succeeded". "check succeeded" is not a success: "check" is first.
var (
	upgradeTerminalSuccess = map[string]bool{
		"done": true, "complete": true, "completed": true,
		"finished": true, "success": true, "succeeded": true,
	}
	upgradeTerminalFailure = map[string]bool{
		"failed": true, "failure": true, "error": true,
		"aborted": true, "cancelled": true, "canceled": true,
	}
	upgradeTerminalRollback = map[string]bool{
		"rollback": true, "rolledback": true, "rolled": true,
		"rollingback": true, "revert": true, "reverted": true, "reverting": true,
	}
)

// normaliseUpgradeStep reduces a reported step name to its lowercase words.
func normaliseUpgradeStep(step string) []string {
	words := upgradeStepSplit.Split(strings.ToLower(strings.TrimSpace(step)), -1)
	out := make([]string, 0, len(words))
	for _, w := range words {
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// upgradeStepKey maps a reported step onto a canonical key, or "" when the step
// is one this panel does not recognise.
func upgradeStepKey(step string) string {
	for _, word := range normaliseUpgradeStep(step) {
		if key, ok := upgradeStepAliases[word]; ok {
			return key
		}
	}
	return ""
}

// upgradeStepIndex is the position of a reported step in UpgradeSteps, or -1.
func upgradeStepIndex(step string) int {
	key := upgradeStepKey(step)
	if key == "" {
		return -1
	}
	for i, s := range UpgradeSteps {
		if s.Key == key {
			return i
		}
	}
	return -1
}

// classifyUpgradeStep decides whether a reported step ends the job, following
// the positional rules documented above.
func classifyUpgradeStep(step string, percent int) UpgradeJobState {
	words := normaliseUpgradeStep(step)
	if len(words) == 0 {
		if percent >= 100 {
			return UpgradeJobSucceeded
		}
		return UpgradeJobRunning
	}

	if upgradeTerminalFailure[words[len(words)-1]] {
		return UpgradeJobFailed
	}

	for _, w := range words {
		if upgradeTerminalRollback[w] {
			return UpgradeJobRolledBack
		}
	}

	if upgradeTerminalSuccess[words[0]] {
		return UpgradeJobSucceeded
	}

	if percent >= 100 {
		return UpgradeJobSucceeded
	}

	return UpgradeJobRunning
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// UpgradeJobView is one job as the API reports it.
type UpgradeJobView struct {
	JobID       string          `json:"job_id"`
	State       UpgradeJobState `json:"state"`
	Running     bool            `json:"running"`
	FromVersion string          `json:"from_version"`
	ToVersion   string          `json:"to_version"`
	Step        string          `json:"step"`
	StepKey     string          `json:"step_key"`
	StepIndex   int             `json:"step_index"`
	Percent     int             `json:"percent"`
	Message     string          `json:"message"`
	Error       string          `json:"error,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
	Detached    bool            `json:"detached"`
	Steps       []UpgradeStep   `json:"steps"`
}

// UpgradeStatusView is what the status and check endpoints answer with.
type UpgradeStatusView struct {
	Current         string          `json:"current"`
	Latest          string          `json:"latest"`
	UpdateAvailable bool            `json:"update_available"`
	CheckedAt       *time.Time      `json:"checked_at"`
	Changelog       string          `json:"changelog"`
	Commit          string          `json:"commit"`
	BuildDate       string          `json:"build_date"`
	Available       bool            `json:"available"`
	Unavailable     string          `json:"unavailable_reason,omitempty"`
	Job             *UpgradeJobView `json:"job"`
	Steps           []UpgradeStep   `json:"steps"`
}

// UpgradeStartResult is what the start endpoint answers with. It carries the
// version the panel is leaving as well as the one it is going to, because the
// UI needs both to tell a successful upgrade from a rollback once the API
// comes back.
type UpgradeStartResult struct {
	JobID       string        `json:"job_id"`
	FromVersion string        `json:"from_version"`
	ToVersion   string        `json:"to_version"`
	StartedAt   time.Time     `json:"started_at"`
	Steps       []UpgradeStep `json:"steps"`
}

// UpgradeCaller is who asked and from where, for the audit trail.
type UpgradeCaller struct {
	ClientIP  string
	UserAgent string
	UserID    uuid.UUID
	TenantID  uuid.UUID
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// UpgradeService owns the job lifecycle, the persisted mirror and the audit
// trail. The engine owns everything that touches the disk outside
// /vkai-panel/etc.
type UpgradeService struct {
	engine    UpgradeEngine
	audit     *AuditService
	logger    *zap.Logger
	build     UpgradeBuildInfo
	statePath string

	mu       sync.Mutex
	job      *upgradeJob
	lastSeen *UpgradeStatus
	now      func() time.Time

	// starting is held for the length of the engine's Start call, which is
	// made outside the mutex. Without it two requests could both pass the
	// "no job is running" check before either had a job to be seen.
	starting bool
}

// NewUpgradeService builds the service. A nil engine is allowed and is not an
// error: the panel still reports its version honestly and answers the upgrade
// endpoints with 503 rather than pretending an upgrade path exists.
func NewUpgradeService(engine UpgradeEngine, audit *AuditService, logger *zap.Logger) *UpgradeService {
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &UpgradeService{
		engine:    engine,
		audit:     audit,
		logger:    logger,
		build:     ResolveBuildInfo(),
		statePath: filepath.Join(config.EtcRoot(), upgradeStateFileName),
		now:       time.Now,
	}

	// A job still marked running at construction time was started by a process
	// that no longer exists: this one. That is the normal end of a successful
	// upgrade, so it is reconciled rather than reported as an anomaly.
	s.loadState()
	s.mu.Lock()
	s.reconcileLocked()
	s.mu.Unlock()

	return s
}

// Build returns the running binary's identity for GET /api/v1/version.
func (s *UpgradeService) Build() UpgradeBuildInfo {
	if s == nil {
		return ResolveBuildInfo()
	}
	return s.build
}

// Status returns the last known release status without contacting the release
// source, plus the current job. It is the endpoint the UI opens on, so it must
// be cheap: a settings page that blocks on a network fetch is a settings page
// that looks broken behind a firewall.
func (s *UpgradeService) Status() UpgradeStatusView {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.reconcileLocked()
	return s.statusViewLocked()
}

// Check contacts the release source and replaces the cached status.
func (s *UpgradeService) Check(ctx context.Context, caller UpgradeCaller) (UpgradeStatusView, error) {
	if s.engine == nil {
		return UpgradeStatusView{}, ErrUpgradeUnavailable
	}

	status, err := s.engine.Check(ctx)
	if err != nil {
		s.auditEvent(ctx, caller, "upgrade.check", "failure", models.JSONMap{
			"current": s.build.Version,
			"error":   err.Error(),
		})
		return UpgradeStatusView{}, err
	}

	// The status view reports the version this process is running, not the one
	// the engine found on disk. They differ exactly when a release was staged
	// but the service has not been restarted yet, and the running one is the
	// one the operator is looking at.
	if status.CheckedAt.IsZero() {
		status.CheckedAt = s.now()
	}

	s.mu.Lock()
	s.lastSeen = &status
	s.reconcileLocked()
	view := s.statusViewLocked()
	s.mu.Unlock()

	s.auditEvent(ctx, caller, "upgrade.check", "success", models.JSONMap{
		"current":          view.Current,
		"latest":           view.Latest,
		"update_available": view.UpdateAvailable,
	})

	return view, nil
}

// Start begins an upgrade and returns as soon as the engine has accepted it.
// Everything after that is polled: the connection that started the upgrade will
// not survive it.
func (s *UpgradeService) Start(ctx context.Context, caller UpgradeCaller, version string) (UpgradeStartResult, error) {
	if s.engine == nil {
		return UpgradeStartResult{}, ErrUpgradeUnavailable
	}

	version = strings.TrimSpace(version)
	if version != "" {
		if err := validateUpgradeVersion(version); err != nil {
			return UpgradeStartResult{}, err
		}
	}

	s.mu.Lock()
	s.reconcileLocked()
	if s.starting || (s.job != nil && !s.job.State.Terminal()) {
		running := ""
		if s.job != nil {
			running = s.job.JobID
		}
		s.mu.Unlock()
		s.auditEvent(ctx, caller, "upgrade.start", "denied", models.JSONMap{
			"reason":         "an upgrade is already running",
			"running_job_id": running,
		})
		return UpgradeStartResult{}, ErrUpgradeInProgress
	}

	// No version named means "install what the last check found".
	target := version
	if target == "" {
		if s.lastSeen == nil || !s.lastSeen.UpdateAvailable || strings.TrimSpace(s.lastSeen.Latest) == "" {
			s.mu.Unlock()
			return UpgradeStartResult{}, ErrUpgradeNoTarget
		}
		target = strings.TrimSpace(s.lastSeen.Latest)
	}
	s.starting = true
	s.mu.Unlock()

	// The engine call is made outside the lock: it may reach the network, and
	// holding the mutex across it would block the status endpoint the UI is
	// polling every couple of seconds. The starting latch keeps the guard above
	// honest for the length of the call.
	jobID, err := s.engine.Start(ctx, target)
	if err != nil {
		s.mu.Lock()
		s.starting = false
		s.mu.Unlock()

		s.auditEvent(ctx, caller, "upgrade.start", "failure", models.JSONMap{
			"from":  s.build.Version,
			"to":    target,
			"error": err.Error(),
		})
		return UpgradeStartResult{}, err
	}
	if strings.TrimSpace(jobID) == "" {
		// An engine that accepts a job without naming it leaves nothing to
		// poll. Give the job an id here so the UI still has something to
		// follow, and say so in the log.
		jobID = uuid.New().String()
		s.logger.Warn("upgrade engine returned an empty job id; using a locally generated one",
			zap.String("job_id", jobID))
	}

	started := s.now()
	job := &upgradeJob{
		JobID:         jobID,
		State:         UpgradeJobRunning,
		FromVersion:   s.build.Version,
		ToVersion:     target,
		Step:          UpgradeSteps[0].Key,
		Percent:       0,
		Message:       "The upgrade has been queued.",
		StartedAt:     started,
		UpdatedAt:     started,
		StartedByUser: userIDString(caller.UserID),
	}

	s.mu.Lock()
	s.job = job
	s.starting = false
	s.persistLocked()
	view := jobView(job)
	s.mu.Unlock()

	s.auditEvent(ctx, caller, "upgrade.start", "success", models.JSONMap{
		"job_id": jobID,
		"from":   job.FromVersion,
		"to":     job.ToVersion,
	})

	// The watcher outlives the request by design, so it gets a context of its
	// own: cancelling the HTTP request must not cancel the upgrade.
	go s.watch(jobID)

	return UpgradeStartResult{
		JobID:       view.JobID,
		FromVersion: view.FromVersion,
		ToVersion:   view.ToVersion,
		StartedAt:   view.StartedAt,
		Steps:       UpgradeSteps,
	}, nil
}

// Progress reports where a job has got to. It answers from the mirrored state
// rather than from the engine, so it keeps answering after the restart that
// destroyed the engine's own record.
func (s *UpgradeService) Progress(ctx context.Context, jobID string) (UpgradeJobView, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return UpgradeJobView{}, ErrUpgradeJobNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.reconcileLocked()

	if s.job == nil || s.job.JobID != jobID {
		return UpgradeJobView{}, ErrUpgradeJobNotFound
	}

	// A job whose watcher died with the previous process is refreshed here
	// instead: the engine in this process knows nothing about it, so the
	// running version decides, which reconcileLocked has just done.
	if !s.job.Detached && !s.job.State.Terminal() && s.engine != nil {
		s.refreshFromEngineLocked(ctx)
	}

	return jobView(s.job), nil
}

// ---------------------------------------------------------------------------
// The watcher
// ---------------------------------------------------------------------------

// watch mirrors the engine's progress into the persisted job until the job
// ends, the process is replaced by the upgrade it is watching, or the job runs
// out of time.
func (s *UpgradeService) watch(jobID string) {
	ticker := time.NewTicker(upgradePollInterval)
	defer ticker.Stop()

	deadline := s.now().Add(upgradeMaxDuration)

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		progress, err := s.engine.Progress(ctx, jobID)
		cancel()

		s.mu.Lock()
		if s.job == nil || s.job.JobID != jobID || s.job.State.Terminal() {
			s.mu.Unlock()
			return
		}

		if err != nil {
			// An engine that has forgotten the job while this process is still
			// alive is a real failure, but a transient error is not. Only the
			// deadline below ends the job, so a blip costs one poll.
			s.job.Error = err.Error()
			s.job.UpdatedAt = s.now()
			s.persistLocked()
			s.mu.Unlock()

			if s.now().After(deadline) {
				s.finish(jobID, UpgradeJobFailed, "The upgrade did not report a result in time.", err.Error())
				return
			}
			continue
		}

		s.applyProgressLocked(progress)
		state := s.job.State
		s.persistLocked()
		s.mu.Unlock()

		if state.Terminal() {
			s.logFinish(jobID, state)
			return
		}

		if s.now().After(deadline) {
			s.finish(jobID, UpgradeJobFailed, "The upgrade did not report a result in time.", "")
			return
		}
	}
}

// refreshFromEngineLocked pulls one progress sample synchronously. Used by the
// progress endpoint so a UI poll is never a whole tick behind the engine.
func (s *UpgradeService) refreshFromEngineLocked(ctx context.Context) {
	progress, err := s.engine.Progress(ctx, s.job.JobID)
	if err != nil {
		return
	}
	s.applyProgressLocked(progress)
	s.persistLocked()
}

// applyProgressLocked folds one engine sample into the job.
func (s *UpgradeService) applyProgressLocked(progress UpgradeProgress) {
	if s.job == nil {
		return
	}

	if step := strings.TrimSpace(progress.Step); step != "" {
		s.job.Step = step
	}
	if progress.Percent > 0 {
		s.job.Percent = clampPercent(progress.Percent)
	}
	if msg := strings.TrimSpace(progress.Message); msg != "" {
		s.job.Message = msg
	}
	s.job.UpdatedAt = s.now()

	if state := classifyUpgradeStep(progress.Step, progress.Percent); state.Terminal() {
		s.job.State = state
		finished := s.now()
		s.job.FinishedAt = &finished
		if state == UpgradeJobSucceeded {
			s.job.Percent = 100
		}
	}
}

// finish forces a job to a terminal state, for the cases the engine never
// reports one.
func (s *UpgradeService) finish(jobID string, state UpgradeJobState, message, errText string) {
	s.mu.Lock()
	if s.job == nil || s.job.JobID != jobID || s.job.State.Terminal() {
		s.mu.Unlock()
		return
	}
	s.job.State = state
	s.job.Message = message
	if errText != "" {
		s.job.Error = errText
	}
	finished := s.now()
	s.job.FinishedAt = &finished
	s.job.UpdatedAt = finished
	s.persistLocked()
	s.mu.Unlock()

	s.logFinish(jobID, state)
}

func (s *UpgradeService) logFinish(jobID string, state UpgradeJobState) {
	s.logger.Info("upgrade job finished",
		zap.String("job_id", jobID),
		zap.String("state", string(state)),
	)
}

// ---------------------------------------------------------------------------
// Reconciliation against the running version
// ---------------------------------------------------------------------------

// reconcileLocked settles a job whose watcher is gone. The panel restarts
// itself in the middle of its own upgrade, so the process that answers the next
// poll is usually not the one that started the job. What that process is
// running is the only trustworthy signal left:
//
//   - running the target version  -> the upgrade took.
//   - running the previous one    -> either the release has not been switched
//     yet, or it was switched, failed its health check and was rolled back.
//     The two are told apart by time: past the restart grace, a panel still on
//     the old version is a panel that was rolled back.
func (s *UpgradeService) reconcileLocked() {
	job := s.job
	if job == nil || job.State.Terminal() {
		return
	}

	// A job this process started is watched by this process, and the version
	// this process runs is by definition the one being upgraded away from.
	// There is nothing for the version comparison to settle until the API has
	// been replaced.
	if !job.Detached {
		return
	}

	running := s.build.Version

	if versionsEqual(running, job.ToVersion) {
		job.State = UpgradeJobSucceeded
		job.Percent = 100
		job.Step = UpgradeSteps[len(UpgradeSteps)-1].Key
		job.Message = fmt.Sprintf("The panel is running version %s.", running)
		finished := s.now()
		job.FinishedAt = &finished
		job.UpdatedAt = finished
		s.persistLocked()
		return
	}

	if s.now().Sub(job.UpdatedAt) < upgradeRestartGrace {
		return
	}

	finished := s.now()
	job.FinishedAt = &finished
	job.UpdatedAt = finished
	if versionsEqual(running, job.FromVersion) {
		job.State = UpgradeJobRolledBack
		job.Message = fmt.Sprintf(
			"The upgrade to %s did not complete. The panel was restored to version %s and is running normally.",
			job.ToVersion, job.FromVersion)
	} else {
		job.State = UpgradeJobFailed
		job.Message = fmt.Sprintf(
			"The upgrade to %s did not complete and the panel is running version %s. Check the upgrade log on the server.",
			job.ToVersion, running)
	}
	s.persistLocked()
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// loadState reads the mirrored job. A missing or unreadable file is not an
// error: it means no upgrade has been started from this install, which is the
// normal case.
func (s *UpgradeService) loadState() {
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("upgrade: cannot read the job state file",
				zap.String("path", s.statePath), zap.Error(err))
		}
		return
	}

	var job upgradeJob
	if err := json.Unmarshal(data, &job); err != nil {
		s.logger.Warn("upgrade: the job state file is not valid JSON; ignoring it",
			zap.String("path", s.statePath), zap.Error(err))
		return
	}
	if strings.TrimSpace(job.JobID) == "" {
		return
	}

	// The process that wrote this file is not this one, so nothing in this
	// process is watching the job any more.
	if !job.State.Terminal() {
		job.Detached = true
	}
	s.job = &job
}

// persistLocked writes the job mirror. The file sits next to the other panel
// state under /vkai-panel/etc, is written through a temporary file so a crash
// mid-write cannot leave a truncated job, and is mode 0600 because it records
// what an administrator did and when.
func (s *UpgradeService) persistLocked() {
	if s.job == nil {
		return
	}

	data, err := json.MarshalIndent(s.job, "", "  ")
	if err != nil {
		s.logger.Error("upgrade: cannot encode the job state", zap.Error(err))
		return
	}

	dir := filepath.Dir(s.statePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		s.logger.Warn("upgrade: cannot create the state directory",
			zap.String("path", dir), zap.Error(err))
		return
	}

	tmp, err := os.CreateTemp(dir, ".upgrade_job-*.json")
	if err != nil {
		s.logger.Warn("upgrade: cannot write the job state file",
			zap.String("path", s.statePath), zap.Error(err))
		return
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		s.logger.Warn("upgrade: cannot write the job state file",
			zap.String("path", s.statePath), zap.Error(err))
		return
	}
	if err := tmp.Chmod(0o600); err != nil {
		s.logger.Warn("upgrade: cannot restrict the job state file", zap.Error(err))
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		s.logger.Warn("upgrade: cannot close the job state file", zap.Error(err))
		return
	}
	if err := os.Rename(tmpName, s.statePath); err != nil {
		os.Remove(tmpName)
		s.logger.Warn("upgrade: cannot replace the job state file",
			zap.String("path", s.statePath), zap.Error(err))
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// statusViewLocked assembles the status answer from the cached check and the
// running build.
func (s *UpgradeService) statusViewLocked() UpgradeStatusView {
	view := UpgradeStatusView{
		Current:   s.build.Version,
		Latest:    s.build.Version,
		Commit:    s.build.Commit,
		BuildDate: s.build.Date,
		Available: s.engine != nil,
		Steps:     UpgradeSteps,
	}
	if s.engine == nil {
		view.Unavailable = ErrUpgradeUnavailable.Error()
	}

	if s.lastSeen != nil {
		if latest := strings.TrimSpace(s.lastSeen.Latest); latest != "" {
			view.Latest = latest
		}
		view.Changelog = s.lastSeen.Changelog
		view.UpdateAvailable = s.lastSeen.UpdateAvailable && !versionsEqual(view.Latest, view.Current)
		if !s.lastSeen.CheckedAt.IsZero() {
			checked := s.lastSeen.CheckedAt
			view.CheckedAt = &checked
		}
	}

	// An upgrade in flight is never also an upgrade on offer: the button it
	// would enable is the one guarded against a second job.
	if s.job != nil {
		job := jobView(s.job)
		view.Job = &job
		if !s.job.State.Terminal() {
			view.UpdateAvailable = false
		}
	}

	return view
}

func jobView(job *upgradeJob) UpgradeJobView {
	return UpgradeJobView{
		JobID:       job.JobID,
		State:       job.State,
		Running:     !job.State.Terminal(),
		FromVersion: job.FromVersion,
		ToVersion:   job.ToVersion,
		Step:        job.Step,
		StepKey:     upgradeStepKey(job.Step),
		StepIndex:   upgradeStepIndex(job.Step),
		Percent:     clampPercent(job.Percent),
		Message:     job.Message,
		Error:       job.Error,
		StartedAt:   job.StartedAt,
		UpdatedAt:   job.UpdatedAt,
		FinishedAt:  job.FinishedAt,
		Detached:    job.Detached,
		Steps:       UpgradeSteps,
	}
}

// upgradeVersionPattern is deliberately strict: the version string reaches a
// root-owned release script, so only what a release tag can contain is
// accepted. Anything a shell would find interesting is outside the set.
var upgradeVersionPattern = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){0,3}(-[A-Za-z0-9._-]{1,32})?$`)

func validateUpgradeVersion(version string) error {
	if len(version) > 64 {
		return &UpgradeValidationError{Field: "version", Message: "The version is too long."}
	}
	if !upgradeVersionPattern.MatchString(version) {
		return &UpgradeValidationError{
			Field:   "version",
			Message: "The version must look like 1.2.3 or 1.2.3-rc1.",
		}
	}
	return nil
}

// versionsEqual compares two release strings ignoring a leading "v" and
// surrounding space, so "v1.2.0" and "1.2.0" are one version.
func versionsEqual(a, b string) bool {
	na := strings.TrimPrefix(strings.TrimSpace(a), "v")
	nb := strings.TrimPrefix(strings.TrimSpace(b), "v")
	return na != "" && na == nb
}

func clampPercent(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func userIDString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

// auditEvent writes one upgrade action to the audit trail. Reads are not
// recorded: the status and progress endpoints are polled every couple of
// seconds during an upgrade, and an audit log filled with those is an audit log
// nobody reads.
func (s *UpgradeService) auditEvent(ctx context.Context, caller UpgradeCaller, action, status string, details models.JSONMap) {
	if s.audit == nil {
		return
	}

	var userID *uuid.UUID
	if caller.UserID != uuid.Nil {
		id := caller.UserID
		userID = &id
	}

	if err := s.audit.Log(ctx, caller.TenantID, userID, action, UpgradeAuditResource, nil,
		details, caller.ClientIP, caller.UserAgent, status); err != nil {
		s.logger.Error("upgrade: audit log write failed",
			zap.String("action", action), zap.Error(err))
	}
}
