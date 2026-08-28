package reload

// Configuration files change without anybody calling the API.
//
// The state file and the environment file are edited by hand, by the installer
// and by upgrade scripts. An operator who edits one of them expects it to
// matter; today it matters at the next restart, which is a restart they have to
// remember to perform and, on a panel, one they may be reluctant to perform
// remotely. So the files are watched.
//
// Two things make watching a file harder than it looks:
//
//	a file is written in several syscalls. An editor that truncates and then
//	writes leaves a window in which the file is valid JSON describing nothing,
//	or invalid JSON, or empty. Applying that window would take the panel down
//	over a save that was still in progress. So a change has to hold still for a
//	moment before it is believed;
//
//	a rejected file must change nothing. The running configuration is kept and
//	exactly what was rejected is logged, because the operator's next move is to
//	fix the file and they need to know which line.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

const (
	// DefaultPollInterval is how often the watched files are examined. A stat
	// and a read of two small files, twice a second, is not a cost worth
	// optimising and it keeps the reaction time under a second.
	DefaultPollInterval = 500 * time.Millisecond

	// DefaultQuietPeriod is how long a file has to stop changing before it is
	// believed. It is the debounce: an editor writing in three syscalls
	// produces three different contents inside this window and only the last
	// one is ever applied.
	DefaultQuietPeriod = 1500 * time.Millisecond
)

// WatcherOptions configures the file watcher.
type WatcherOptions struct {
	// StateFile is the panel access state file. Empty means "the default".
	StateFile string

	// EnvFile is the environment file the installer writes. Empty disables
	// environment watching.
	EnvFile string

	PollInterval time.Duration
	QuietPeriod  time.Duration

	Logger *zap.Logger
}

// Watcher reloads the panel when a configuration file changes.
type Watcher struct {
	sup    *Supervisor
	opts   WatcherOptions
	logger *zap.Logger

	mu sync.Mutex
	// applied is the fingerprint of the content this process has already acted
	// on, so a file that is rewritten with the same bytes is not reapplied.
	applied map[string]string
	// pending is the fingerprint currently waiting out the quiet period.
	pending      map[string]string
	pendingSince map[string]time.Time
	// rejected remembers the fingerprint of content that was refused, so the
	// same broken file is not re-reported every poll.
	rejected map[string]string

	// bootEnv is the environment file as it was when the process started. The
	// restart-required report is a comparison against it.
	bootEnv map[string]string
}

// NewWatcher builds a watcher for a supervisor.
func NewWatcher(sup *Supervisor, opts WatcherOptions) *Watcher {
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if opts.QuietPeriod <= 0 {
		opts.QuietPeriod = DefaultQuietPeriod
	}
	if opts.Logger == nil {
		opts.Logger = zap.NewNop()
	}
	if strings.TrimSpace(opts.StateFile) == "" {
		opts.StateFile = config.PanelStateFilePath()
	}

	w := &Watcher{
		sup:          sup,
		opts:         opts,
		logger:       opts.Logger,
		applied:      map[string]string{},
		pending:      map[string]string{},
		pendingSince: map[string]time.Time{},
		rejected:     map[string]string{},
	}

	// Record the current contents as already applied: this process was built
	// from them, and a watcher that fired once at start-up would reload a
	// configuration that is already live.
	for _, path := range w.watched() {
		w.applied[path] = fingerprintFile(path)
	}
	if w.opts.EnvFile != "" {
		w.bootEnv, _ = config.ParseEnvFile(w.opts.EnvFile)
	}

	return w
}

func (w *Watcher) watched() []string {
	paths := make([]string, 0, 2)
	if w.opts.StateFile != "" {
		paths = append(paths, w.opts.StateFile)
	}
	if w.opts.EnvFile != "" {
		paths = append(paths, w.opts.EnvFile)
	}
	return paths
}

// Run polls until the context ends. It is meant to be started in a goroutine.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.opts.PollInterval)
	defer ticker.Stop()

	w.logger.Info("watching the panel configuration files for changes",
		zap.Strings("files", w.watched()),
		zap.Duration("poll_interval", w.opts.PollInterval),
		zap.Duration("quiet_period", w.opts.QuietPeriod))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll(ctx, time.Now())
		}
	}
}

// poll examines the watched files once. A file that has changed starts a quiet
// period; a file that has held still for the whole quiet period is applied.
func (w *Watcher) poll(ctx context.Context, now time.Time) {
	w.mu.Lock()

	ready := make([]string, 0, 2)
	for _, path := range w.watched() {
		current := fingerprintFile(path)

		if current == w.applied[path] {
			delete(w.pending, path)
			delete(w.pendingSince, path)
			continue
		}
		if current == w.rejected[path] {
			// Already reported. The operator has to change the file again for
			// this to be looked at once more.
			continue
		}

		if w.pending[path] != current {
			w.pending[path] = current
			w.pendingSince[path] = now
			continue
		}
		if now.Sub(w.pendingSince[path]) >= w.opts.QuietPeriod {
			ready = append(ready, path)
		}
	}

	w.mu.Unlock()

	if len(ready) == 0 {
		return
	}

	w.apply(ctx, Request{
		Origin: OriginFile,
		Actor:  "file",
		Detail: strings.Join(ready, ", "),
	}, ready)
}

// apply re-derives the configuration from disk and hands it to the supervisor.
func (w *Watcher) apply(ctx context.Context, req Request, changed []string) {
	next, envVars, err := w.load()
	if err != nil {
		w.reject(changed, err.Error())
		return
	}

	w.reportRestartRequired(envVars)

	if _, err := w.sup.Apply(ctx, next, req); err != nil {
		w.reject(changed, err.Error())
		return
	}

	w.mu.Lock()
	for _, path := range changed {
		w.applied[path] = fingerprintFile(path)
		delete(w.pending, path)
		delete(w.pendingSince, path)
		delete(w.rejected, path)
	}
	w.mu.Unlock()

	w.logger.Info("panel configuration reloaded from disk",
		zap.Strings("files", changed),
		zap.String("origin", string(req.Origin)))
}

// reject keeps the running configuration and says exactly what was refused.
func (w *Watcher) reject(changed []string, reason string) {
	w.mu.Lock()
	for _, path := range changed {
		w.rejected[path] = fingerprintFile(path)
		delete(w.pending, path)
		delete(w.pendingSince, path)
	}
	w.mu.Unlock()

	w.logger.Error("the panel configuration on disk was rejected and NOT applied; "+
		"the panel is still running the configuration it had",
		zap.Strings("files", changed),
		zap.String("rejected_because", reason))
}

// load re-derives the panel access configuration the way the process derived it
// at start-up, except that the environment comes from the environment file
// rather than from this process's own environment - editing a file does not
// change the variables a running process already inherited.
func (w *Watcher) load() (*config.PanelAccessConfig, map[string]string, error) {
	lookup := config.OSEnvLookup
	var envVars map[string]string

	if w.opts.EnvFile != "" {
		parsed, err := config.ParseEnvFile(w.opts.EnvFile)
		if err != nil {
			return nil, nil, err
		}
		envVars = parsed
		lookup = config.EnvFileLookup(parsed, config.OSEnvLookup)
	}

	cfg, err := config.LoadPanelAccessFrom(config.PanelAccessSource{
		Env:       lookup,
		StateFile: w.opts.StateFile,
		NoPersist: true,
	})
	if err != nil {
		return nil, envVars, err
	}

	return cfg, envVars, nil
}

// reportRestartRequired records the environment variables that changed and
// cannot take effect without restarting the process.
//
// This is the honest half of hot reload. Database credentials, the Redis
// address and the signing secret are held by connections and by objects built
// once at start-up; a reload cannot reach them, and a reload that said it could
// would be the same defect as before in the opposite direction.
func (w *Watcher) reportRestartRequired(envVars map[string]string) {
	if envVars == nil || w.bootEnv == nil {
		return
	}

	for key, reason := range config.RestartRequiredEnvKeys() {
		before, hadBefore := w.bootEnv[key]
		after, hasAfter := envVars[key]

		switch {
		case hadBefore && !hasAfter:
			w.sup.NoteRestartRequired(key,
				reason+" It was removed from the environment file, and this process still holds the value it started with until it is restarted.")
			w.logger.Warn("an environment variable was removed but the running process still holds its old value",
				zap.String("variable", key))
		case before != after && hasAfter:
			w.sup.NoteRestartRequired(key, reason)
			w.logger.Warn("an environment variable changed and cannot be applied without a restart",
				zap.String("variable", key),
				zap.String("reason", reason))
		}
	}
}

// fingerprintFile is the content hash of a file, or a marker for its absence.
// Hashing the content rather than trusting the modification time means a file
// that was rewritten with identical bytes - which is what most installers do -
// is correctly seen as unchanged.
func fingerprintFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "absent"
		}
		return "unreadable:" + err.Error()
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// SIGHUP
// ---------------------------------------------------------------------------

// WatchSignals reloads on SIGHUP.
//
// It is here because it is the first thing every operator tries, and because it
// is what an init system sends on "systemctl reload". A panel that ignored it
// would be teaching its operators that reloading does not work.
func (w *Watcher) WatchSignals(ctx context.Context) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			w.logger.Info("SIGHUP received, reloading the panel configuration from disk")
			w.reloadNow(ctx, Request{
				Origin: OriginSignal,
				Actor:  "SIGHUP",
				Detail: strings.Join(w.watched(), ", "),
			})
		}
	}
}

// reloadNow re-reads everything and applies it, bypassing the quiet period: a
// signal is an explicit statement that the files are finished being written.
func (w *Watcher) reloadNow(ctx context.Context, req Request) {
	next, envVars, err := w.load()
	if err != nil {
		w.logger.Error("the panel configuration on disk was rejected and NOT applied; "+
			"the panel is still running the configuration it had",
			zap.String("origin", string(req.Origin)),
			zap.String("rejected_because", err.Error()))
		return
	}

	w.reportRestartRequired(envVars)

	outcome, err := w.sup.Apply(ctx, next, req)
	if err != nil {
		w.logger.Error("the panel configuration on disk could not be applied; "+
			"the panel is still running the configuration it had",
			zap.String("origin", string(req.Origin)),
			zap.Error(err))
		return
	}

	w.mu.Lock()
	for _, path := range w.watched() {
		w.applied[path] = fingerprintFile(path)
		delete(w.rejected, path)
		delete(w.pending, path)
		delete(w.pendingSince, path)
	}
	w.mu.Unlock()

	w.logger.Info("panel configuration reloaded",
		zap.String("origin", string(req.Origin)),
		zap.Int("changes", len(outcome.Changes)),
		zap.Strings("restart_required", outcome.RestartRequired))
}

// Reload applies whatever is on disk right now. It is exported so the CLI and
// the tests can drive the same path a signal drives.
func (w *Watcher) Reload(ctx context.Context, req Request) { w.reloadNow(ctx, req) }
