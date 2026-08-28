package backup

import (
	"context"
	"sync"
	"time"
)

// Phase names the stage a long-running operation is in. They are stable
// strings: the UI and the API both show them, so they are part of the contract.
const (
	PhaseQueued    = "queued"
	PhaseScanning  = "scanning"
	PhaseArchiving = "archiving"
	PhaseUploading = "uploading"
	PhaseDownload  = "downloading"
	PhaseExtract   = "extracting"
	PhaseVerifying = "verifying"
	PhasePruning   = "pruning"
	PhaseDone      = "done"
	PhaseFailed    = "failed"
	PhaseCancelled = "cancelled"
)

// Progress is a snapshot of a running operation. Every field is a plain value
// so it can be marshalled straight into an API response.
type Progress struct {
	Phase       string    `json:"phase"`
	Message     string    `json:"message"`
	FilesTotal  int       `json:"files_total"`
	FilesDone   int       `json:"files_done"`
	BytesTotal  int64     `json:"bytes_total"`
	BytesDone   int64     `json:"bytes_done"`
	Percent     float64   `json:"percent"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Cancellable bool      `json:"cancellable"`
}

// Tracker carries the progress of one operation and the means to stop it.
//
// Cancellation is a context, not a flag: every loop in this package that moves
// bytes checks ctx.Err(), so cancelling stops the work rather than marking it
// stopped and letting it finish. Callers that persist progress register a sink.
type Tracker struct {
	mu       sync.Mutex
	progress Progress
	sinks    []func(Progress)
	cancel   context.CancelFunc
	now      func() time.Time
}

// NewTracker returns a tracker and the context every stage of the operation
// must be run under.
func NewTracker(parent context.Context) (*Tracker, context.Context) {
	ctx, cancel := context.WithCancel(parent)
	t := &Tracker{
		cancel: cancel,
		now:    time.Now,
	}
	start := t.now()
	t.progress = Progress{
		Phase:       PhaseQueued,
		StartedAt:   start,
		UpdatedAt:   start,
		Cancellable: true,
	}
	return t, ctx
}

// OnUpdate registers a sink called on every change, under the tracker lock, so
// a persisting sink cannot observe two updates out of order.
func (t *Tracker) OnUpdate(fn func(Progress)) {
	if t == nil || fn == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sinks = append(t.sinks, fn)
}

// SetPhase moves the operation to a new phase and resets the counters that
// only make sense within a phase.
func (t *Tracker) SetPhase(phase, message string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress.Phase = phase
	t.progress.Message = message
	t.progress.BytesDone = 0
	t.progress.FilesDone = 0
	if phase == PhaseDone || phase == PhaseFailed || phase == PhaseCancelled {
		t.progress.Cancellable = false
		t.progress.Percent = 100
	}
	t.touchLocked()
}

// SetTotals declares how much work the current phase has.
func (t *Tracker) SetTotals(files int, bytes int64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress.FilesTotal = files
	t.progress.BytesTotal = bytes
	t.touchLocked()
}

// Advance records progress within the current phase.
func (t *Tracker) Advance(files int, bytes int64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress.FilesDone += files
	t.progress.BytesDone += bytes
	t.touchLocked()
}

func (t *Tracker) touchLocked() {
	t.progress.UpdatedAt = t.now()
	switch {
	case t.progress.Phase == PhaseDone || t.progress.Phase == PhaseFailed || t.progress.Phase == PhaseCancelled:
		t.progress.Percent = 100
	case t.progress.BytesTotal > 0:
		t.progress.Percent = percent(t.progress.BytesDone, t.progress.BytesTotal)
	case t.progress.FilesTotal > 0:
		t.progress.Percent = percent(int64(t.progress.FilesDone), int64(t.progress.FilesTotal))
	default:
		t.progress.Percent = 0
	}
	snapshot := t.progress
	for _, sink := range t.sinks {
		sink(snapshot)
	}
}

func percent(done, total int64) float64 {
	if total <= 0 {
		return 0
	}
	p := float64(done) / float64(total) * 100
	if p > 100 {
		return 100
	}
	if p < 0 {
		return 0
	}
	return p
}

// Snapshot returns the current progress.
func (t *Tracker) Snapshot() Progress {
	if t == nil {
		return Progress{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.progress
}

// Cancel stops the operation by cancelling its context.
//
// It deliberately takes no lock. Sinks are called with the tracker locked, so
// that a persisting sink cannot see two updates out of order, and the obvious
// reason to cancel an operation is something a sink noticed - an operator
// pressing the button, a quota exceeded. A Cancel that locked would deadlock
// against exactly that caller. The cancel function is written once, before the
// tracker is handed out, and never replaced, so reading it here needs no lock.
func (t *Tracker) Cancel() {
	if t == nil || t.cancel == nil {
		return
	}
	t.cancel()
}
