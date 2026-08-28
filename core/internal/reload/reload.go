// Package reload applies a new panel configuration to a running panel.
//
// The panel administers the machine it runs on, over the network, behind a
// secret entrance. Every setting it exposes is therefore also a way to lock its
// operator out of the only tool they have for fixing the mistake. That single
// fact shapes everything in this package:
//
//   - a change is prepared before any of it is committed, so a configuration
//     that cannot be applied is rejected without touching what is running;
//   - the new listener is bound and proven before the old one stops accepting,
//     because a panel that closed its only door after failing to open another
//     is a machine nobody can reach;
//   - the whole configuration is swapped behind one atomic pointer, so no
//     request is ever checked against half the old settings and half the new;
//   - a change that would cut the caller off is undone automatically, by this
//     process, within seconds. A confirmation dialogue is not protection: the
//     operator is being asked to confirm something they cannot evaluate.
//
// The same pipeline serves all four origins - the API, the state file, the
// environment file and SIGHUP - so there is exactly one place where a
// configuration becomes live, and exactly one place where that can be audited.
package reload

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
)

// Origin says who asked for a reload. It is recorded in the audit trail,
// because "the port changed" and "the port changed because somebody edited a
// file on disk at 03:00" are different incidents.
type Origin string

const (
	OriginAPI    Origin = "api"
	OriginFile   Origin = "file"
	OriginSignal Origin = "signal"
	OriginBoot   Origin = "boot"
)

// Request describes one reload attempt: who asked, from where, and what the
// panel must still be reachable as afterwards.
type Request struct {
	Origin   Origin
	Actor    string
	ClientIP string
	Host     string
	Detail   string

	// Caller, when set, is the point of view the lockout probe uses: the new
	// configuration has to admit a request from this address, for this host,
	// or the change is rolled back. It is empty for a reload that came from a
	// file or a signal, where there is no caller to be locked out.
	Caller *Caller
}

// Caller is the address and host a request arrived on.
type Caller struct {
	ClientIP string
	Host     string
}

// Change is one field that moved, with its value already redacted.
type Change struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// FieldStatus is the honest answer for one changed field: did it actually take
// effect in this process, or is it only saved.
type FieldStatus struct {
	Field           string `json:"field"`
	Applied         bool   `json:"applied"`
	RestartRequired bool   `json:"restart_required"`
	Detail          string `json:"detail"`
}

// Outcome is what a reload did. It is deliberately explicit about the
// difference between "saved" and "live": the failure this package exists to
// prevent is a panel that reports success for a change that never happened.
type Outcome struct {
	Applied         bool          `json:"applied"`
	Changes         []Change      `json:"changes"`
	Fields          []FieldStatus `json:"fields"`
	RestartRequired []string      `json:"restart_required"`
	Warnings        []string      `json:"warnings"`
	RolledBack      bool          `json:"rolled_back"`
	RollbackReason  string        `json:"rollback_reason"`
	AccessURL       string        `json:"access_url"`
	Origin          Origin        `json:"origin"`
	Duration        string        `json:"duration"`
}

// Applier applies one facet of a configuration to the running process.
//
// The split into Prepare and Commit is the whole point. Prepare does everything
// that can fail - binding a socket, loading a certificate, compiling an address
// matcher - and Commit does nothing but publish what Prepare already built. A
// Commit that could fail would leave the process half-reconfigured, which is
// the state this package must never produce.
type Applier interface {
	// Name identifies the applier in logs and in a rollback report.
	Name() string

	// Prepare builds everything the new configuration needs, without changing
	// what the process is currently doing. Returning nil means "nothing to do
	// for this change".
	Prepare(next *config.PanelAccessConfig) (Staged, error)
}

// Staged is a prepared change that has not been published yet.
type Staged interface {
	// Commit publishes the prepared change. It must not fail.
	Commit()

	// Rollback undoes a Commit, or releases a prepared change that was never
	// committed. It must be safe to call in either state and must leave the
	// process serving exactly what it served before Prepare.
	Rollback()

	// Retire releases what the previous generation held, after the new one has
	// been proven. This is where the old listener is drained.
	Retire()

	// Describe is one line for the log.
	Describe() string
}

// AuditFunc records a reload in the panel's audit trail. It is a function
// rather than a dependency so this package does not have to know about the
// service layer that owns the audit log.
type AuditFunc func(ctx context.Context, req Request, outcome *Outcome)

// Supervisor owns the live configuration and the appliers that make it real.
type Supervisor struct {
	logger *zap.Logger

	// live is the configuration every reader sees. It is replaced whole, never
	// field by field: a request that reads it during a reload gets either the
	// entire old configuration or the entire new one.
	live atomic.Pointer[config.PanelAccessConfig]

	// mu serialises reloads against each other. It is never held by a reader.
	mu       sync.Mutex
	appliers []Applier

	subsMu sync.Mutex
	subs   []func(*config.PanelAccessConfig)

	audit AuditFunc

	// probe verifies that the panel is still reachable after a change. It is a
	// field so a test can substitute a deterministic one.
	probe Probe

	// boot is what this process started with. Comparing against it is how a
	// field that genuinely needs a restart is reported honestly.
	boot *config.PanelAccessConfig

	// restartRequired accumulates the settings that were changed on disk but
	// cannot take effect until the process restarts - database credentials and
	// the like. It is never cleared, because nothing but a restart clears it.
	restartMu       sync.Mutex
	restartRequired map[string]string

	lastApplied atomic.Int64

	// reloadCert rebuilds the certificate manager without a configuration
	// change. Reissuing a self-signed certificate replaces a file and moves no
	// setting, so the ordinary diff-driven path would correctly conclude that
	// nothing changed - and the panel would keep serving the certificate it
	// already had while the endpoint reported a new fingerprint.
	reloadCert atomic.Pointer[func() error]
}

// Options configures a Supervisor.
type Options struct {
	Config *config.PanelAccessConfig
	Logger *zap.Logger
	Audit  AuditFunc
	Probe  Probe
}

// New builds a Supervisor around the configuration this process started with.
func New(opts Options) *Supervisor {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.DefaultPanelAccess()
	}
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	s := &Supervisor{
		logger:          logger,
		audit:           opts.Audit,
		probe:           opts.Probe,
		boot:            Clone(cfg),
		restartRequired: map[string]string{},
	}
	s.live.Store(Clone(cfg))
	if s.probe == nil {
		s.probe = defaultProbe{}
	}

	return s
}

// Register adds an applier. Order matters: appliers are prepared and committed
// in registration order and rolled back in reverse, so register the cheapest to
// undo first and the listener last.
func (s *Supervisor) Register(a Applier) {
	if a == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appliers = append(s.appliers, a)
}

// SetCertificateReloader registers the hook that rebuilds the certificate
// manager in place.
func (s *Supervisor) SetCertificateReloader(fn func() error) {
	if fn == nil {
		s.reloadCert.Store(nil)
		return
	}
	s.reloadCert.Store(&fn)
}

// ReloadCertificate makes the certificate on disk the certificate on the wire.
// It returns an error when the panel has no way to do that, so the caller can
// say so rather than reporting a fingerprint nobody is being served.
func (s *Supervisor) ReloadCertificate() error {
	fn := s.reloadCert.Load()
	if fn == nil {
		return fmt.Errorf("reload: this process has no certificate manager to reload")
	}
	return (*fn)()
}

// SetAudit installs the audit hook after construction, which is what the API
// entry point needs: the audit service is built long after the supervisor.
func (s *Supervisor) SetAudit(fn AuditFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = fn
}

// Current is the live configuration. The returned pointer is a snapshot that is
// never mutated, so a reader may hold it for the length of a request.
func (s *Supervisor) Current() *config.PanelAccessConfig { return s.live.Load() }

// OnApplied registers a callback fired after a configuration goes live, so a
// component that keeps its own copy - the settings service, for one - learns
// about a change that came from a file or a signal rather than from itself.
//
// The callback runs on the goroutine that applied the change, inside the
// supervisor's lock, and must therefore return promptly and must not call back
// into Apply. A subscriber that also holds its own lock while asking for a
// change has to recognise the echo of that change rather than block on it; see
// the settings service, where getting this wrong hung the request until the
// drain timeout.
func (s *Supervisor) OnApplied(fn func(*config.PanelAccessConfig)) {
	if fn == nil {
		return
	}
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	s.subs = append(s.subs, fn)
}

// NoteRestartRequired records a setting that changed on disk and cannot be
// applied without restarting. It is reported by every later Outcome and by the
// settings endpoint, because a change that silently did not happen is precisely
// the failure this package exists to end.
func (s *Supervisor) NoteRestartRequired(field, reason string) {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()
	s.restartRequired[field] = reason
}

// RestartRequired lists the settings still waiting for a restart.
func (s *Supervisor) RestartRequired() []string {
	s.restartMu.Lock()
	defer s.restartMu.Unlock()

	out := make([]string, 0, len(s.restartRequired))
	for field, reason := range s.restartRequired {
		if reason == "" {
			out = append(out, field)
			continue
		}
		out = append(out, field+": "+reason)
	}
	sort.Strings(out)
	return out
}

// LastApplied is when a configuration last went live.
func (s *Supervisor) LastApplied() time.Time {
	unix := s.lastApplied.Load()
	if unix == 0 {
		return time.Time{}
	}
	return time.Unix(unix, 0).UTC()
}

// Apply makes a configuration live.
//
// It is the only path by which the running panel changes, whether the change
// came from the API, from a file on disk or from SIGHUP. The sequence is:
//
//  1. validate the candidate in full - a configuration is applied whole or not
//     at all, so a single bad field stops the entire change here;
//  2. prepare every applier, which binds the new socket and loads the new
//     certificate without disturbing the running ones;
//  3. commit, which is a sequence of atomic pointer stores that no reader can
//     observe halfway through;
//  4. prove the panel is still reachable, from the caller's point of view where
//     there is a caller;
//  5. roll everything back if it is not, or retire the previous generation if
//     it is.
func (s *Supervisor) Apply(ctx context.Context, next *config.PanelAccessConfig, req Request) (*Outcome, error) {
	if next == nil {
		return nil, fmt.Errorf("reload: no configuration to apply")
	}

	started := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.live.Load()
	candidate := Clone(next)
	candidate.StateFile = current.StateFile

	// The whole configuration is validated before any of it is applied. This is
	// what "atomic from a reader's point of view" needs at the front: a
	// candidate that would be rejected halfway through the appliers must be
	// rejected before the first one runs.
	if err := candidate.Validate(); err != nil {
		return nil, fmt.Errorf("reload: the new configuration was rejected and nothing was changed: %w", err)
	}

	changes := Diff(current, candidate)
	outcome := &Outcome{
		Origin:    req.Origin,
		Changes:   changes,
		Fields:    []FieldStatus{},
		Warnings:  []string{},
		AccessURL: RedactURL(candidate),
	}

	if len(changes) == 0 {
		outcome.Applied = true
		outcome.RestartRequired = s.RestartRequired()
		outcome.Duration = time.Since(started).String()
		return outcome, nil
	}

	staged := make([]Staged, 0, len(s.appliers))
	names := make([]string, 0, len(s.appliers))

	rollback := func(reason string) {
		for i := len(staged) - 1; i >= 0; i-- {
			staged[i].Rollback()
		}
		s.logger.Warn("panel reload rolled back",
			zap.String("origin", string(req.Origin)),
			zap.String("reason", reason),
			zap.Strings("appliers", names),
		)
	}

	for _, applier := range s.appliers {
		prepared, err := applier.Prepare(candidate)
		if err != nil {
			rollback("prepare failed: " + applier.Name())
			return nil, fmt.Errorf("reload: %s could not be prepared, the panel is still running the previous configuration: %w",
				applier.Name(), err)
		}
		if prepared == nil {
			continue
		}
		staged = append(staged, prepared)
		names = append(names, applier.Name())
	}

	// Nothing below this line may fail without a rollback.
	for _, prepared := range staged {
		prepared.Commit()
	}
	s.live.Store(candidate)

	if err := s.probe.Verify(ctx, candidate, req); err != nil {
		s.live.Store(current)
		rollback("reachability probe failed: " + err.Error())

		outcome.RolledBack = true
		outcome.RollbackReason = err.Error()
		outcome.Applied = false
		outcome.AccessURL = RedactURL(current)
		outcome.Duration = time.Since(started).String()
		s.recordAudit(ctx, req, outcome)

		return outcome, &RollbackError{Reason: err.Error(), Changes: changes}
	}

	for _, prepared := range staged {
		prepared.Retire()
	}

	s.lastApplied.Store(time.Now().Unix())
	outcome.Applied = true
	outcome.Fields = s.fieldStatus(changes)
	outcome.RestartRequired = s.RestartRequired()
	outcome.Duration = time.Since(started).String()

	descriptions := make([]string, 0, len(staged))
	for _, prepared := range staged {
		if d := prepared.Describe(); d != "" {
			descriptions = append(descriptions, d)
		}
	}

	fields := make([]zap.Field, 0, len(changes)+4)
	fields = append(fields,
		zap.String("origin", string(req.Origin)),
		zap.String("actor", req.Actor),
		zap.String("client_ip", req.ClientIP),
		zap.Strings("applied", descriptions),
	)
	for _, change := range changes {
		fields = append(fields, zap.String(change.Field, change.Old+" -> "+change.New))
	}
	s.logger.Info("panel configuration reloaded", fields...)

	s.notify(candidate)
	s.recordAudit(ctx, req, outcome)

	return outcome, nil
}

// fieldStatus turns the diff into the honest per-field answer.
func (s *Supervisor) fieldStatus(changes []Change) []FieldStatus {
	s.restartMu.Lock()
	pending := make(map[string]string, len(s.restartRequired))
	for k, v := range s.restartRequired {
		pending[k] = v
	}
	s.restartMu.Unlock()

	out := make([]FieldStatus, 0, len(changes))
	for _, change := range changes {
		status := FieldStatus{Field: change.Field, Applied: true}
		if reason, waiting := pending[change.Field]; waiting {
			status.Applied = false
			status.RestartRequired = true
			status.Detail = reason
		} else {
			status.Detail = LiveDetail(change.Field)
		}
		out = append(out, status)
	}
	return out
}

// LiveDetail says how a field takes effect, in the operator's terms.
func LiveDetail(field string) string {
	switch field {
	case "port", "bind":
		return "A new listener was opened on the new address and the old one stopped accepting; requests already in flight finished on it."
	case "entrance", "entrance_enabled", "allowed_ips", "trusted_proxies", "domain", "session_ttl_seconds", "enabled":
		return "The access gate was rebuilt and swapped in; the next request is checked against the new rules."
	case "tls.enabled", "tls.mode", "tls.self_signed", "tls.cert_file", "tls.key_file", "tls.certificate":
		return "The certificate manager was rebuilt; the next TLS handshake uses the new certificate."
	case "tls.acme.email", "tls.acme.use_staging", "tls.acme.profile":
		return "The renewal loop was restarted with the new ACME settings."
	case "public_port", "public_scheme":
		return "This only changes the URL the panel prints and reports; nothing binds it."
	default:
		return "Applied to the running process."
	}
}

func (s *Supervisor) notify(cfg *config.PanelAccessConfig) {
	s.subsMu.Lock()
	subs := make([]func(*config.PanelAccessConfig), len(s.subs))
	copy(subs, s.subs)
	s.subsMu.Unlock()

	for _, fn := range subs {
		fn(Clone(cfg))
	}
}

func (s *Supervisor) recordAudit(ctx context.Context, req Request, outcome *Outcome) {
	if s.audit == nil {
		return
	}
	s.audit(ctx, req, outcome)
}

// RollbackError is returned when a change was applied, failed its reachability
// check and was undone. It is a distinct type so the API can answer with the
// reason rather than a generic failure: the operator has to be told that their
// change was reverted, not merely that something went wrong.
type RollbackError struct {
	Reason  string
	Changes []Change
}

func (e *RollbackError) Error() string {
	return "the change was applied, could not be reached afterwards, and has been rolled back: " + e.Reason
}

// ---------------------------------------------------------------------------
// Diffing and redaction
// ---------------------------------------------------------------------------

// Redacted is what a secret looks like in a log, an audit entry or a response.
const Redacted = "[redacted]"

// Diff lists the fields that changed, in a stable order, with the security
// entrance redacted. The entrance is a credential: an audit log that records it
// turns every reader of the audit log into a holder of the panel's front door
// key.
func Diff(current, next *config.PanelAccessConfig) []Change {
	changes := make([]Change, 0, 12)

	add := func(field, oldValue, newValue string) {
		if oldValue == newValue {
			return
		}
		if IsSecretField(field) {
			oldValue, newValue = redactPresence(oldValue), redactPresence(newValue)
		}
		changes = append(changes, Change{Field: field, Old: oldValue, New: newValue})
	}

	add("enabled", strconv.FormatBool(current.Enabled), strconv.FormatBool(next.Enabled))
	add("port", strconv.Itoa(current.Port), strconv.Itoa(next.Port))
	add("bind", current.Bind, next.Bind)
	add("public_port", strconv.Itoa(current.PublicPort), strconv.Itoa(next.PublicPort))
	add("public_scheme", current.PublicScheme, next.PublicScheme)
	add("entrance", current.Entrance, next.Entrance)
	add("entrance_enabled", strconv.FormatBool(current.EntranceEnabled), strconv.FormatBool(next.EntranceEnabled))
	add("session_ttl_seconds", strconv.Itoa(current.SessionTTLSeconds), strconv.Itoa(next.SessionTTLSeconds))
	add("allowed_ips", strings.Join(current.AllowedIPs, ", "), strings.Join(next.AllowedIPs, ", "))
	add("trusted_proxies", strings.Join(current.TrustedProxies, ", "), strings.Join(next.TrustedProxies, ", "))
	add("domain", current.Domain, next.Domain)
	add("tls.enabled", strconv.FormatBool(current.TLS.Enabled), strconv.FormatBool(next.TLS.Enabled))
	add("tls.mode", current.TLSMode(), next.TLSMode())
	add("tls.cert_file", current.TLS.CertFile, next.TLS.CertFile)
	add("tls.key_file", current.TLS.KeyFile, next.TLS.KeyFile)
	add("tls.acme.email", current.TLS.ACME.Email, next.TLS.ACME.Email)
	add("tls.acme.use_staging", strconv.FormatBool(current.TLS.ACME.UseStaging), strconv.FormatBool(next.TLS.ACME.UseStaging))
	add("tls.acme.profile", current.TLS.ACME.Profile, next.TLS.ACME.Profile)

	return changes
}

// IsSecretField reports whether a field's value must never be written down.
func IsSecretField(field string) bool {
	switch field {
	case "entrance", "tls.certificate", "tls.private_key", "tls.acme.account_key":
		return true
	}
	return strings.Contains(field, "password") || strings.Contains(field, "secret") ||
		strings.Contains(field, "private_key")
}

// redactPresence keeps the one fact about a secret that is safe to record and
// worth recording: whether there was one.
func redactPresence(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return Redacted
}

// RedactURL is the access URL with the entrance removed, for anything that is
// written down.
func RedactURL(cfg *config.PanelAccessConfig) string {
	url := cfg.AccessURL()
	if cfg.EntranceEnabled && cfg.Entrance != "" {
		url = strings.Replace(url, cfg.Entrance, "/"+Redacted, 1)
	}
	return url
}

// Clone is a deep enough copy that the original cannot be mutated through it.
// Every configuration handed across a boundary in this package is a clone, so
// that the live snapshot readers hold is genuinely immutable.
func Clone(cfg *config.PanelAccessConfig) *config.PanelAccessConfig {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	clone.AllowedIPs = copyStrings(cfg.AllowedIPs)
	clone.TrustedProxies = copyStrings(cfg.TrustedProxies)
	clone.Generated = copyStrings(cfg.Generated)
	clone.EnvOverrides = copyStrings(cfg.EnvOverrides)
	return &clone
}

func copyStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// ---------------------------------------------------------------------------
// Process-wide handoff
// ---------------------------------------------------------------------------

// defaultSupervisor is how the settings service finds the supervisor.
//
// The router builds the settings handler itself, positionally, out of reach of
// the API entry point that owns the listener - so there is no constructor
// parameter to pass this through. A process-wide pointer, set once before the
// router is built, is the honest version of that constraint: it is explicit,
// it is documented, and the alternative is the settings service saving a file
// that nothing reads, which is the defect this work exists to remove.
var defaultSupervisor atomic.Pointer[Supervisor]

// SetDefault publishes the supervisor for components that cannot be handed one.
func SetDefault(s *Supervisor) { defaultSupervisor.Store(s) }

// Default is the published supervisor, or nil when the process has none - a
// unit test, or a build that never starts a listener.
func Default() *Supervisor { return defaultSupervisor.Load() }

// Adopt replaces the live snapshot without applying anything.
//
// It exists for exactly one moment: start-up, after the certificate manager has
// resolved the paths and the source it actually ended up with. Everything built
// before that point holds a configuration that is correct except for those
// fields, and adopting is how they learn the difference - through the same
// subscription a later reload would use, rather than through a second copy of
// the configuration that drifts.
func (s *Supervisor) Adopt(cfg *config.PanelAccessConfig) {
	if cfg == nil {
		return
	}
	s.mu.Lock()
	snapshot := Clone(cfg)
	s.boot = Clone(cfg)
	s.live.Store(snapshot)
	s.mu.Unlock()

	s.notify(snapshot)
}
