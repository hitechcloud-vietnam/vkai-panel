package middleware

// The authentication event log.
//
// Everything a credential-accepting endpoint decides is written here as one
// line per event, in a fixed field order, with the source address in a fixed
// position. Two different readers depend on that shape:
//
//   - an operator grepping for who is being attacked;
//   - fail2ban, which parses these lines with the filter in
//     deploy/fail2ban/filter.d/vkai-panel-auth.conf and pushes the ban down to
//     the firewall, where a blocked attacker pays for a TCP connection instead
//     of for a request the panel has to process.
//
// The second reader is the reason the format is a stable contract rather than
// a convenience. A field reordered here is a jail that stops banning anybody,
// silently, and nothing else in the system would notice. TestAuthEventLine and
// TestFail2banFilterMatchesAuthLogLines assert the format and the shipped
// filter against each other so that a change to one fails the build of the
// other.
//
// The panel does not require fail2ban. If it is not installed, these lines are
// simply a log; the in-process guard is what actually stops the attack.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// AuthLogTag is the marker that starts every authentication event line. It
// exists so a filter can anchor on something that no other log line produces.
const AuthLogTag = "vkai-auth"

// EnvAuthLog overrides where the authentication log is written.
const EnvAuthLog = "VKAI_AUTH_LOG"

// Outcomes. These strings are part of the fail2ban contract.
const (
	// AuthOutcomeSuccess - the credential was correct.
	AuthOutcomeSuccess = "success"
	// AuthOutcomeFailure - the credential was presented and rejected.
	AuthOutcomeFailure = "failure"
	// AuthOutcomeBlocked - the attempt never reached the credential check
	// because the guard refused it.
	AuthOutcomeBlocked = "blocked"
)

// Reasons. Also part of the contract: an operator may narrow a jail to one of
// these, so they are stable identifiers rather than prose.
const (
	ReasonInvalidCredentials = "invalid_credentials"
	ReasonLocked             = "locked"
	ReasonThrottled          = "throttled"
	ReasonLimiterUnavailable = "limiter_unavailable"
	ReasonOK                 = "ok"
)

// AuthEvent is one authentication decision.
type AuthEvent struct {
	Time      time.Time
	Outcome   string
	Reason    string
	IP        string
	Account   string
	Scope     string
	Path      string
	RequestID string
	// Dimension records which limiter layer produced a block. It is written to
	// the structured logger for the operator but deliberately kept out of the
	// line format, which stays minimal so the filter stays simple.
	Dimension string
}

// Line renders the event in the format the fail2ban filter parses.
//
// The shape is:
//
//	<RFC3339 UTC> vkai-auth outcome=<o> reason=<r> ip=<a> account=<u> scope=<s> path=<p> request_id=<id>
//
// Every value is sanitised. That is not tidiness: the account field is
// attacker-supplied, and an account named so as to contain a newline and a
// forged "outcome=failure ip=<someone else>" would let an attacker use the
// panel's own jail to ban an arbitrary address - including the operator's.
func (e AuthEvent) Line() string {
	ts := e.Time
	if ts.IsZero() {
		ts = time.Now()
	}

	var b strings.Builder
	b.Grow(192)
	b.WriteString(ts.UTC().Format(time.RFC3339))
	b.WriteByte(' ')
	b.WriteString(AuthLogTag)
	b.WriteString(" outcome=")
	b.WriteString(sanitizeLogField(e.Outcome, 32))
	b.WriteString(" reason=")
	b.WriteString(sanitizeLogField(e.Reason, 48))
	b.WriteString(" ip=")
	b.WriteString(sanitizeLogField(e.IP, 64))
	b.WriteString(" account=")
	b.WriteString(sanitizeLogField(e.Account, 64))
	b.WriteString(" scope=")
	b.WriteString(sanitizeLogField(e.Scope, 32))
	b.WriteString(" path=")
	b.WriteString(sanitizeLogField(e.Path, 128))
	b.WriteString(" request_id=")
	b.WriteString(sanitizeLogField(e.RequestID, 64))
	return b.String()
}

// sanitizeLogField reduces a value to characters that cannot break a line or
// forge a field, and bounds its length. An empty or fully stripped value
// becomes "-" so the field count never changes.
func sanitizeLogField(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}

	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if b.Len() >= max {
			break
		}
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-' || r == '@' || r == ':' || r == '/' || r == '+':
			b.WriteRune(r)
		default:
			// Everything else - spaces, newlines, quotes, equals signs, any
			// non-ASCII - collapses to a single underscore.
			b.WriteByte('_')
		}
	}

	out := b.String()
	if out == "" {
		return "-"
	}
	return out
}

// AuthLogger writes AuthEvents to the fail2ban-readable log and mirrors them
// into the panel's structured logger.
type AuthLogger struct {
	mu     sync.Mutex
	out    io.Writer
	logger *zap.Logger
}

// NewAuthLogger writes events to w. A nil w discards the line format and keeps
// only the structured mirror, which is what tests that only care about zap use.
func NewAuthLogger(w io.Writer, logger *zap.Logger) *AuthLogger {
	return &AuthLogger{out: w, logger: logger}
}

var (
	defaultAuthLoggerOnce sync.Once
	defaultAuthLogger     *AuthLogger
)

// DefaultAuthLogger returns the process-wide authentication logger, opening
// the log file on first use. If the file cannot be opened the events go to
// standard error instead: losing the fail2ban feed is bad, losing the events
// entirely is worse.
func DefaultAuthLogger(logger *zap.Logger) *AuthLogger {
	defaultAuthLoggerOnce.Do(func() {
		defaultAuthLogger = NewAuthLogger(openAuthLog(logger), logger)
	})
	if defaultAuthLogger.logger == nil && logger != nil {
		defaultAuthLogger.mu.Lock()
		defaultAuthLogger.logger = logger
		defaultAuthLogger.mu.Unlock()
	}
	return defaultAuthLogger
}

// AuthLogPath is where authentication events are written.
func AuthLogPath() string {
	if override := strings.TrimSpace(os.Getenv(EnvAuthLog)); override != "" {
		return override
	}
	return filepath.Join(config.LogRoot(), "auth.log")
}

func openAuthLog(logger *zap.Logger) io.Writer {
	path := AuthLogPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		if logger != nil {
			logger.Error("authentication log directory unavailable, falling back to stderr",
				zap.String("path", path), zap.Error(err))
		}
		return os.Stderr
	}

	// Probe once so a permission problem is reported at start-up rather than
	// discovered as silence months later.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		if logger != nil {
			logger.Error("authentication log unavailable, falling back to stderr",
				zap.String("path", path), zap.Error(err))
		}
		return os.Stderr
	}
	_ = f.Close()

	// Rotation is bounded but generous: fail2ban only reads the live file, so
	// rotating too eagerly would drop events out from under a jail whose
	// findtime has not elapsed yet.
	return &lumberjack.Logger{
		Filename:   path,
		MaxSize:    50,
		MaxBackups: 10,
		MaxAge:     30,
		Compress:   true,
	}
}

// Log records one event.
func (l *AuthLogger) Log(event AuthEvent) {
	if l == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}

	line := event.Line()

	l.mu.Lock()
	out := l.out
	logger := l.logger
	l.mu.Unlock()

	if out != nil {
		// A failed write must not take the request down with it.
		_, _ = io.WriteString(out, line+"\n")
	}

	if logger == nil {
		return
	}

	fields := []zap.Field{
		zap.String("outcome", event.Outcome),
		zap.String("reason", event.Reason),
		zap.String("ip", event.IP),
		zap.String("account", sanitizeLogField(event.Account, 64)),
		zap.String("scope", event.Scope),
		zap.String("path", event.Path),
		zap.String("request_id", event.RequestID),
	}
	if event.Dimension != "" {
		fields = append(fields, zap.String("dimension", event.Dimension))
	}

	switch event.Outcome {
	case AuthOutcomeSuccess:
		logger.Info("authentication event", fields...)
	default:
		logger.Warn("authentication event", fields...)
	}
}
