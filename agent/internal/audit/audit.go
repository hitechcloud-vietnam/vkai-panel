// Package audit is the agent's own record of what it was asked to do.
//
// # Why the agent keeps its own log
//
// The panel already logs the operations it sends. That log is not sufficient
// evidence for the machine the operations landed on, for the obvious reason: it
// is written by the party whose compromise is the thing an operator is trying to
// detect. Someone who owns the panel can send an agent anything the agent
// accepts and then write whatever they like into the panel's history of it.
//
// The record here is written by the agent, on the managed node, before and
// after the work happens, and it names the client certificate that asked. An
// operator can read it with nothing but ssh and cat, compare it against what
// the panel claims it sent, and see a divergence. That is the whole point of
// it, and it is why the entries are one JSON object per line: a format that
// survives being read by grep, jq, journald and a human in equal measure.
//
// # What is recorded
//
// Every operation the control channel dispatches, whether it succeeded, was
// refused, or failed - a refusal is often the more interesting line. Alongside
// it, the operations that execute a program or read a file record the exact
// argument vector or path they used, so the log says what was run and not
// merely what was asked for.
//
// Arguments are recorded verbatim, truncated. No operation in this agent takes
// a credential; if one is ever added, it must redact its own arguments before
// they reach here.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Outcomes an entry can record.
const (
	// OutcomeOK means the operation ran and returned a result.
	OutcomeOK = "ok"
	// OutcomeRefused means the agent declined before doing anything: an unknown
	// operation, or an argument that failed validation. Nothing ran.
	OutcomeRefused = "refused"
	// OutcomeFailed means the operation ran and did not succeed.
	OutcomeFailed = "failed"
	// OutcomeExecuted records the exact command or path an operation used. It
	// is written by the operation itself, in addition to the entry the control
	// channel writes for the request as a whole.
	OutcomeExecuted = "executed"
)

// Limits on what one entry may occupy. An operation argument object is capped
// at a megabyte by the HTTP layer, and a megabyte of it in every log line would
// fill a small VPS's disk faster than any legitimate use of the channel.
const (
	// MaxArgumentBytes is how much of an argument object is recorded.
	MaxArgumentBytes = 2048
	// MaxDetailBytes is how much of a command line or path list is recorded.
	MaxDetailBytes = 1024
	// MaxErrorBytes is how much of an error message is recorded.
	MaxErrorBytes = 1024
)

// Defaults for the log file itself.
const (
	// DefaultPath is where the record lives. It is outside the release
	// directory, like the agent's identity, so an upgrade does not take the
	// history with it.
	DefaultPath = "/vkai-panel/logs/agent-operations.log"
	// DefaultMaxBytes is the size at which the current file is rotated.
	DefaultMaxBytes = 16 << 20
	// DefaultKeep is how many rotated files are retained, so the record covers
	// roughly 80MB of history before the oldest is discarded.
	DefaultKeep = 4
)

// Actor is the party that asked for an operation. On the control channel it is
// the client certificate that completed the mutual TLS handshake; there is no
// other way to reach the channel, so there is no other kind of actor.
type Actor struct {
	// Name is the certificate's common name.
	Name string `json:"name"`
	// Serial identifies the exact certificate, which is what a revocation acts
	// on. Two certificates issued to the same panel differ here.
	Serial string `json:"serial,omitempty"`
	// Address is where the connection came from.
	Address string `json:"address,omitempty"`
}

// String renders an actor for a human-readable log line.
func (a Actor) String() string {
	switch {
	case a.Name == "" && a.Address == "":
		return "unidentified"
	case a.Serial == "":
		return fmt.Sprintf("%s from %s", a.Name, a.Address)
	default:
		return fmt.Sprintf("%s serial=%s from %s", a.Name, a.Serial, a.Address)
	}
}

// Entry is one line of the record.
type Entry struct {
	Time      time.Time `json:"time"`
	Actor     Actor     `json:"actor"`
	Operation string    `json:"operation"`
	Outcome   string    `json:"outcome"`
	// Arguments is the argument object as it arrived, truncated.
	Arguments string `json:"arguments,omitempty"`
	// Detail is what was actually done: the argument vector of a command, or
	// the resolved path of a file that was read.
	Detail string `json:"detail,omitempty"`
	Error  string `json:"error,omitempty"`
	// DurationMS is how long the operation took. It is absent on the entries an
	// operation writes about itself while it is still running.
	DurationMS int64  `json:"duration_ms,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`
}

// Options configures a Log.
type Options struct {
	Path     string
	MaxBytes int64
	Keep     int
	// AgentID is stamped on every entry so that a record collected from several
	// nodes stays attributable after it is merged.
	AgentID string
	// Fallback receives entries when the file cannot be written. It is the
	// process logger, which under systemd is the journal - degraded, because it
	// rotates on the journal's terms rather than this package's, but not
	// silence.
	Fallback *log.Logger
}

// Log is an append-only record on the local filesystem.
type Log struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int
	agentID  string
	fallback *log.Logger

	file *os.File
	size int64
	// degraded is set once the file has failed, so the reason is reported once
	// rather than on every operation.
	degraded bool
}

// Open prepares the record. It never returns an error for a file it cannot
// create: an agent that refuses to work because its log directory is missing is
// an agent that stops answering the panel over a permissions problem. It falls
// back to the process logger instead and says so, loudly, once.
func Open(opts Options) *Log {
	if opts.Path == "" {
		opts.Path = DefaultPath
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	// Zero means "unset", not "keep nothing". A caller that passes no options at
	// all must get history, not a record that discards itself at every
	// rotation.
	if opts.Keep <= 0 {
		opts.Keep = DefaultKeep
	}
	if opts.Fallback == nil {
		opts.Fallback = log.Default()
	}
	l := &Log{
		path:     opts.Path,
		maxBytes: opts.MaxBytes,
		keep:     opts.Keep,
		agentID:  opts.AgentID,
		fallback: opts.Fallback,
	}
	if err := l.openFile(); err != nil {
		l.degraded = true
		l.fallback.Printf("WARNING: the local operation record at %s cannot be written (%v). "+
			"Every operation will still be logged, to this process log only. "+
			"An operator auditing this node independently of the panel needs the file: "+
			"create %s, owned by the user this agent runs as, mode 0700.",
			l.path, err, filepath.Dir(l.path))
	}
	return l
}

// Path reports where the record is being written, for the startup line.
func (l *Log) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.degraded {
		return ""
	}
	return l.path
}

func (l *Log) openFile() error {
	// 0700 on the directory and 0600 on the file: the record names which
	// certificate drove this host and which paths were read, and that is not
	// something every local account should be able to enumerate.
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	l.file = file
	l.size = info.Size()
	return nil
}

// Record writes one entry. It is safe for concurrent use and it never fails the
// caller: an operation is not abandoned because its audit line could not be
// written, it is performed and the failure to record it is itself reported.
func (l *Log) Record(entry Entry) {
	if l == nil {
		return
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	} else {
		entry.Time = entry.Time.UTC()
	}
	if entry.AgentID == "" {
		entry.AgentID = l.agentID
	}
	entry.Arguments = truncate(entry.Arguments, MaxArgumentBytes)
	entry.Detail = truncate(entry.Detail, MaxDetailBytes)
	entry.Error = truncate(entry.Error, MaxErrorBytes)

	line, err := json.Marshal(entry)
	if err != nil {
		l.fallback.Printf("audit: an entry for %s could not be encoded: %v", entry.Operation, err)
		return
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		l.writeFallbackLocked(entry)
		return
	}
	if l.size+int64(len(line)) > l.maxBytes {
		if rotateErr := l.rotateLocked(); rotateErr != nil {
			l.fallback.Printf("audit: the operation record could not be rotated: %v", rotateErr)
		}
	}
	written, writeErr := l.file.Write(line)
	l.size += int64(written)
	if writeErr != nil {
		if !l.degraded {
			l.degraded = true
			l.fallback.Printf("WARNING: writing to the operation record at %s failed (%v); "+
				"operations will be logged to this process log only from now on", l.path, writeErr)
		}
		_ = l.file.Close()
		l.file = nil
		l.writeFallbackLocked(entry)
	}
}

// writeFallbackLocked mirrors an entry into the process log in the same shape,
// so a degraded record is still greppable.
func (l *Log) writeFallbackLocked(entry Entry) {
	l.fallback.Printf("AUDIT operation=%s outcome=%s actor=%q args=%s detail=%q error=%q",
		entry.Operation, entry.Outcome, entry.Actor.String(), entry.Arguments, entry.Detail, entry.Error)
}

// rotateLocked moves the current file aside and starts a new one. The rotated
// files are numbered oldest-last, the way every other log on the box is.
func (l *Log) rotateLocked() error {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	oldest := fmt.Sprintf("%s.%d", l.path, l.keep)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return err
	}
	for n := l.keep - 1; n >= 1; n-- {
		from := fmt.Sprintf("%s.%d", l.path, n)
		to := fmt.Sprintf("%s.%d", l.path, n+1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return l.openFile()
}

// Close releases the file.
func (l *Log) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// ============================================================
// CARRYING THE ACTOR
// ============================================================

type actorKey struct{}

// WithActor puts the verified caller into a context. The control channel does
// this once, when the TLS handshake has already established who is calling, and
// the operations read it back out. Passing it this way rather than as an
// argument means an operation cannot claim to have been asked by someone else,
// and a new operation cannot forget to accept it.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ActorFrom reads the caller back out. An operation invoked from inside the
// agent - the periodic report collecting metrics, for instance - has no actor,
// and gets one that says so rather than an empty structure that reads like a
// missing field.
func ActorFrom(ctx context.Context) Actor {
	if actor, isActor := ctx.Value(actorKey{}).(Actor); isActor {
		return actor
	}
	return Actor{Name: "agent (local)"}
}

// truncate cuts a recorded value to a byte budget and says that it did, so
// nobody reads a clipped command line as the whole of it.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + fmt.Sprintf("... [truncated, %d bytes total]", len(value))
}

// Argv renders an argument vector for the Detail field. Quoting is applied so
// that an argument containing a space cannot be read as two arguments, which
// would make the record misleading about exactly the thing it exists to prove.
func Argv(name string, args ...string) string {
	parts := make([]string, 0, len(args)+1)
	for _, part := range append([]string{name}, args...) {
		if strings.ContainsAny(part, " \t\n\"'\\") || part == "" {
			parts = append(parts, fmt.Sprintf("%q", part))
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}
