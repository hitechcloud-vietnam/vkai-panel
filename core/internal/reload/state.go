package reload

// Persisting the configuration as part of applying it.
//
// The state file is written here, inside the same prepare/commit/rollback
// sequence as everything else, rather than by whoever asked for the change.
// That is not tidiness: it is the only way a rolled-back change leaves the file
// on disk agreeing with what the process is running. A settings endpoint that
// saved first and applied second would, on a rollback, leave a state file
// describing a port the panel is not listening on - and the next restart would
// move the panel to it, unattended, hours later.

import (
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// StateFile persists the panel access configuration.
//
// Writing the file during a reload that came from that same file is harmless
// and deliberate: the watcher records the fingerprint of what it applied
// immediately afterwards, so the rewrite is seen as already-applied rather than
// as a new change. The alternative - skipping the write - would leave the file
// holding whatever the operator typed instead of the normalised configuration
// the panel is actually running.
type StateFile struct {
	logger *zap.Logger
}

// NewStateFile builds the persistence applier.
func NewStateFile(logger *zap.Logger) *StateFile {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &StateFile{logger: logger}
}

// Name implements Applier.
func (s *StateFile) Name() string { return "state file" }

// Prepare writes the new configuration, keeping the old bytes so the write can
// be undone. It runs first among the appliers: a state file that cannot be
// written is the cheapest failure to recover from, because nothing else has
// been touched yet.
func (s *StateFile) Prepare(next *config.PanelAccessConfig) (Staged, error) {
	path := strings.TrimSpace(next.StateFile)
	if path == "" {
		return nil, nil
	}
	previous, existed := readIfPresent(path)

	if err := next.Save(); err != nil {
		return nil, err
	}

	return &stateStaged{
		logger:   s.logger,
		path:     path,
		previous: previous,
		existed:  existed,
	}, nil
}

type stateStaged struct {
	logger   *zap.Logger
	path     string
	previous []byte
	existed  bool
}

// Commit does nothing: Prepare already wrote the file, because a write is the
// step that can fail and Commit is the step that may not.
func (s *stateStaged) Commit() {}

// Rollback restores the previous file, so the configuration on disk keeps
// describing the configuration in memory.
func (s *stateStaged) Rollback() {
	if !s.existed {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			s.logger.Error("could not remove the state file written for a change that was rolled back",
				zap.String("path", s.path), zap.Error(err))
		}
		return
	}
	if err := os.WriteFile(s.path, s.previous, 0o600); err != nil {
		s.logger.Error("could not restore the previous state file after a rollback: "+
			"the file on disk now describes a configuration this process is not running",
			zap.String("path", s.path), zap.Error(err))
	}
}

func (s *stateStaged) Retire() {}

func (s *stateStaged) Describe() string { return "state file written" }

func readIfPresent(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}
