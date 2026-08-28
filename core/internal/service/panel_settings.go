package service

// Panel access settings, editable from inside the panel.
//
// The access gate (port, security entrance, IP allow list, host pinning, TLS)
// is what stands between the internet and an admin API that reconfigures the
// host. Historically it could only be changed by editing a JSON file on disk
// and restarting, which meant nobody changed it. This service exposes the same
// settings to an administrator through the panel, with two rules that matter
// more than convenience:
//
//   - a change that would make the panel unreachable from where the caller is
//     sitting is refused until the caller acknowledges it and has been shown
//     the exact URL to use afterwards;
//   - every change is written to the audit log with the old and the new value,
//     the security entrance redacted, because the entrance is a credential.
//
// The authoritative configuration logic lives in internal/config: this service
// calls it (LoadPanelAccess, Validate, Save, AccessURL, EnsureTLSMaterial,
// RandomEntrance, ParseIPMatcher, CertFingerprint) and never reimplements it.
// What is added here is the request-shaped layer: partial updates, the caller's
// point of view, and the difference between "applied now" and "needs a
// restart".

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/reload"
)

const (
	// PanelSettingsMinPort keeps the panel out of the privileged range. Ports
	// below 1024 need root to bind and are where the services an operator
	// already runs live.
	PanelSettingsMinPort = 1024

	// PanelSettingsMaxPort is the end of the TCP port space.
	PanelSettingsMaxPort = 65535

	// PanelSettingsMinSessionTTL / PanelSettingsMaxSessionTTL bound the entrance
	// cookie lifetime. Below five minutes an operator is thrown out mid-task;
	// above thirty days the cookie outlives the reason it was issued.
	PanelSettingsMinSessionTTL = 5 * time.Minute
	PanelSettingsMaxSessionTTL = 30 * 24 * time.Hour

	// PanelSettingsAuditResource is the audit log resource name for every
	// change made through this service.
	PanelSettingsAuditResource = "panel_settings"

	// panelRedactedValue replaces the security entrance in audit details. The
	// entrance is a secret: an audit log that records it turns every reader of
	// the audit log into a holder of the panel's front door key.
	panelRedactedValue = "[redacted]"
)

// panelEntrancePattern is the entrance shape accepted from the panel UI. It is
// deliberately narrower than what the config package tolerates: an entrance
// typed into a form ends up in URLs, in nginx snippets and in documentation, so
// a single flat path segment of unambiguous characters is all that is offered.
var panelEntrancePattern = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9_-]{3,63}$`)

// panelHostnamePattern matches a DNS host name: labels of letters, digits and
// hyphens, hyphen never leading or trailing.
var panelHostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// panelForbiddenPorts are the ports the panel must never take over, whatever
// the caller asks for. 80 and 443 belong to the customer websites this server
// hosts; a panel answering there is reachable by every scanner on the internet
// and collides with the vhosts the panel exists to manage.
var panelForbiddenPorts = map[int]string{
	80:  "HTTP for hosted websites",
	443: "HTTPS for hosted websites",
}

// PanelSettingsCaller is the request's point of view. Every lockout decision is
// made against it: "unreachable" only means anything relative to somebody.
type PanelSettingsCaller struct {
	ClientIP  string
	Host      string
	UserAgent string
	UserID    uuid.UUID
	TenantID  uuid.UUID
}

// PanelACMEView is the automatic-issuance half of the TLS section.
type PanelACMEView struct {
	Email        string    `json:"email"`
	UseStaging   bool      `json:"use_staging"`
	Profile      string    `json:"profile"`
	LastIssuedAt time.Time `json:"last_issued_at"`
	LastError    string    `json:"last_error"`
}

// PanelTLSView describes the certificate the panel is serving.
//
// It carries everything an operator needs to decide whether the certificate is
// the right one - subject, issuer, expiry, fingerprint, the names it covers -
// and nothing that would let anybody impersonate the panel. There is
// deliberately no field for the private key, in this struct or in any struct it
// is built from: the key is written to disk once, at 0600, and is never read
// back into a response.
type PanelTLSView struct {
	Enabled    bool   `json:"enabled"`
	SelfSigned bool   `json:"self_signed"`
	Mode       string `json:"mode"`

	// Managed is true when this panel generates and renews the pair itself, so
	// the interface can hide the paste boxes for a certificate that would be
	// overwritten by the next renewal.
	Managed bool `json:"managed"`

	// Source is what actually produced the certificate on the wire, which is
	// not always the configured mode: an ACME order that failed leaves the
	// panel on a self-signed fallback, and saying "letsencrypt" then would be
	// the same silent lie the audit found.
	Source string `json:"source"`

	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	KeyPresent bool   `json:"key_present"`

	Fingerprint  string   `json:"fingerprint"`
	Subject      string   `json:"subject"`
	Issuer       string   `json:"issuer"`
	SerialNumber string   `json:"serial_number"`
	Hosts        []string `json:"hosts"`

	NotBefore     *time.Time `json:"not_before"`
	NotAfter      *time.Time `json:"not_after"`
	ExpiresInDays *int       `json:"expires_in_days"`
	Expired       bool       `json:"expired"`
	NotYetValid   bool       `json:"not_yet_valid"`
	ExpiringSoon  bool       `json:"expiring_soon"`
	Present       bool       `json:"present"`

	ChainLength   int    `json:"chain_length"`
	ChainComplete bool   `json:"chain_complete"`
	KeyType       string `json:"key_type"`
	KeyBits       int    `json:"key_bits"`

	Warnings []string `json:"warnings"`

	ACME PanelACMEView `json:"acme"`

	// Inconsistency is the configuration contradiction that shipped once and
	// produced no diagnostic: a certificate mode named while TLS is switched
	// off, so the whole certificate machinery is built and never asked to do
	// anything. Empty when there is none.
	Inconsistency string `json:"inconsistency"`
}

// PanelSettingsView is what GET /panel/settings returns: the stored settings
// plus everything derived from them that the UI would otherwise have to guess.
type PanelSettingsView struct {
	Enabled           bool     `json:"enabled"`
	Bind              string   `json:"bind"`
	Port              int      `json:"port"`
	PublicPort        int      `json:"public_port"`
	PublicScheme      string   `json:"public_scheme"`
	Entrance          string   `json:"entrance"`
	EntranceMasked    string   `json:"entrance_masked"`
	EntranceEnabled   bool     `json:"entrance_enabled"`
	SessionTTLSeconds int      `json:"session_ttl_seconds"`
	AllowedIPs        []string `json:"allowed_ips"`
	TrustedProxies    []string `json:"trusted_proxies"`
	Domain            string   `json:"domain"`

	TLS PanelTLSView `json:"tls"`

	AccessURL     string `json:"access_url"`
	AccessHost    string `json:"access_host"`
	EffectivePort int    `json:"effective_port"`
	Scheme        string `json:"scheme"`
	Proxied       bool   `json:"proxied"`

	// RestartPending and RestartReasons are what is saved but not live. With a
	// reloader attached this is normally empty - the port, the entrance, the
	// allow list and the certificate all take effect immediately - and what is
	// left in it is the honest remainder: database credentials, the Redis
	// address and anything else held by an object built once at start-up.
	RestartPending bool     `json:"restart_pending"`
	RestartReasons []string `json:"restart_reasons"`
	RunningPort    int      `json:"running_port"`
	RunningBind    string   `json:"running_bind"`

	// HotReload reports whether this process can apply a change without being
	// restarted, so the interface can stop promising something the build cannot
	// do. It is false only in a build where no reloader was wired.
	HotReload bool `json:"hot_reload"`

	// LastReloadAt is when a configuration last went live in this process.
	LastReloadAt *time.Time `json:"last_reload_at"`

	EnvOverrides []string  `json:"env_overrides"`
	StateFile    string    `json:"state_file"`
	UpdatedAt    time.Time `json:"updated_at"`

	// ClientIP is the address this request arrived from, so the UI can warn
	// before an allow list is saved that would exclude it.
	ClientIP string `json:"client_ip"`
}

// PanelSettingChange is one field the caller actually changed.
type PanelSettingChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// PanelSettingsUpdate is a partial update: a nil field is left alone, which is
// what lets the UI send only the section the operator edited.
type PanelSettingsUpdate struct {
	Port              *int      `json:"port"`
	Bind              *string   `json:"bind"`
	PublicPort        *int      `json:"public_port"`
	PublicScheme      *string   `json:"public_scheme"`
	Entrance          *string   `json:"entrance"`
	EntranceEnabled   *bool     `json:"entrance_enabled"`
	SessionTTLSeconds *int      `json:"session_ttl_seconds"`
	AllowedIPs        *[]string `json:"allowed_ips"`
	TrustedProxies    *[]string `json:"trusted_proxies"`
	Domain            *string   `json:"domain"`
	TLSEnabled        *bool     `json:"tls_enabled"`
	TLSSelfSigned     *bool     `json:"tls_self_signed"`
	TLSCertFile       *string   `json:"tls_cert_file"`
	TLSKeyFile        *string   `json:"tls_key_file"`

	// TLSMode selects where the certificate comes from: "self-signed",
	// "letsencrypt" (the panel obtains and renews it) or "custom" (the operator
	// supplies it). It is the field the interface should use; TLSSelfSigned is
	// kept for the older clients that predate modes.
	TLSMode *string `json:"tls_mode"`

	// TLSCertificate and TLSPrivateKey are PEM text pasted by an operator. They
	// are write-only: nothing ever returns them, and the key is redacted
	// everywhere it would otherwise be recorded.
	TLSCertificate *string `json:"tls_certificate"`
	TLSPrivateKey  *string `json:"tls_private_key"`

	// TLSAcceptRisk is the explicit override for the two certificate problems
	// an operator is allowed to insist past: a certificate that does not cover
	// the host the panel is reached as, and a chain browsers will not complete.
	// Neither is refused because it is unusual; both are refused because
	// accepting one silently is how a panel disappears from the network.
	TLSAcceptRisk bool `json:"tls_accept_risk"`

	ACMEEmail      *string `json:"acme_email"`
	ACMEUseStaging *bool   `json:"acme_use_staging"`
	ACMEProfile    *string `json:"acme_profile"`

	// Confirm acknowledges a change that moves or restricts the panel's own
	// entrance. Without it such a change is refused, never applied.
	Confirm bool `json:"confirm"`
}

// PanelSettingsResult is what a successful mutation returns.
//
// Applied and Fields are the part that matters: "saved" and "live" are
// different states, and a response that reported only the first would be the
// defect this endpoint was rewritten to remove.
type PanelSettingsResult struct {
	Settings       *PanelSettingsView   `json:"settings"`
	Changes        []PanelSettingChange `json:"changes"`
	AccessURL      string               `json:"access_url"`
	RestartPending bool                 `json:"restart_pending"`
	RestartReasons []string             `json:"restart_reasons"`
	Warnings       []string             `json:"warnings"`

	// Applied is true when every change in this request is live in the running
	// process. It is false when something was only written to disk.
	Applied bool `json:"applied"`

	// Fields is the per-field answer: what took effect and what is waiting for
	// a restart, with the reason.
	Fields []reload.FieldStatus `json:"fields"`

	// Certificate describes a certificate that was installed by this request.
	Certificate *config.CertificateInspection `json:"certificate,omitempty"`
}

// PanelSettingsValidationError is a rejected field with a message written for
// the operator, not for a log file.
type PanelSettingsValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`

	// Check names the specific test that rejected the value, for the fields
	// where "invalid" is not enough to act on. A pasted certificate can fail in
	// eight different ways and the operator has to know which one.
	Check string `json:"check,omitempty"`
}

func (e *PanelSettingsValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

// PanelConfirmationReason explains one way the pending change would cut the
// caller off, and what to do about it.
type PanelConfirmationReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PanelSettingsConfirmationError is returned instead of applying a change that
// would move or restrict the panel's entrance. It carries the exact URL the
// caller must use afterwards, because "you are about to lock yourself out" is
// only actionable next to the address that still works.
type PanelSettingsConfirmationError struct {
	Reasons []PanelConfirmationReason `json:"reasons"`
	NewURL  string                    `json:"new_url"`
	Changes []PanelSettingChange      `json:"changes"`
}

func (e *PanelSettingsConfirmationError) Error() string {
	parts := make([]string, 0, len(e.Reasons))
	for _, r := range e.Reasons {
		parts = append(parts, r.Message)
	}
	return "confirmation required: " + strings.Join(parts, " ")
}

// PanelTLSRiskError is a certificate that would be served but should not be
// accepted without the operator saying so: it does not cover the host the panel
// is reached at, or its chain will not complete in a browser.
//
// It is a separate refusal from the entrance confirmation because it is a
// different question with a different answer. "Confirm" means "yes, move the
// panel and I have written down the new URL". This means "yes, I know browsers
// will complain about this certificate, and I know why". Conflating the two
// would train an operator to acknowledge one while reading about the other.
type PanelTLSRiskError struct {
	Check      string                        `json:"check"`
	Message    string                        `json:"message"`
	Inspection *config.CertificateInspection `json:"certificate"`
}

func (e *PanelTLSRiskError) Error() string { return e.Check + ": " + e.Message }

// PanelSettingsRollbackError is returned when a change was applied to the
// running panel, could not be reached afterwards, and was automatically undone.
//
// This is a success of the safety machinery and a failure of the request, and
// the response has to be both: the operator must be told that the panel is
// exactly as it was, so they do not go looking for a panel that moved.
type PanelSettingsRollbackError struct {
	Reason    string               `json:"reason"`
	Changes   []PanelSettingChange `json:"changes"`
	AccessURL string               `json:"access_url"`
}

func (e *PanelSettingsRollbackError) Error() string {
	return "the change was applied, the panel could not be reached afterwards, and it has been rolled back: " + e.Reason
}

// PanelSettingsApplyError is a change that was valid and could not be applied
// to the running panel: the port it asked for is taken, the certificate it
// named does not load, the state file is not writable.
//
// It is a distinct type because the operator has to be told two things at once,
// and a bare internal error tells them neither: what stopped the change, and
// that the panel is still running - and still reachable - exactly as it was.
type PanelSettingsApplyError struct {
	Reason    string               `json:"reason"`
	Changes   []PanelSettingChange `json:"changes"`
	AccessURL string               `json:"access_url"`
}

func (e *PanelSettingsApplyError) Error() string {
	return "the change was not applied and the panel is unchanged: " + e.Reason
}

// PanelReloader applies a configuration to the running panel.
//
// It is an interface rather than the concrete supervisor so this service can be
// driven without one - the CLI, and a build with no listener - and so a test
// can prove the service asks for the right thing. What it must never become is
// optional in production: a settings service with no reloader writes a file
// that nothing reads, which is exactly the shape of defect this code was
// rewritten to remove.
type PanelReloader interface {
	// Apply persists and applies the configuration, or returns an error having
	// changed nothing.
	Apply(ctx context.Context, next *config.PanelAccessConfig, req reload.Request) (*reload.Outcome, error)

	// Current is the configuration the process is running right now.
	Current() *config.PanelAccessConfig

	// RestartRequired lists settings that changed on disk and cannot take
	// effect without restarting the process.
	RestartRequired() []string

	// OnApplied registers a callback for a configuration that went live
	// somewhere else - a file edit, or SIGHUP.
	OnApplied(func(*config.PanelAccessConfig))

	// LastApplied is when a configuration last went live.
	LastApplied() time.Time

	// ReloadCertificate makes the certificate now on disk the certificate on
	// the wire, without a configuration change. A reissue moves no setting, so
	// nothing else in this interface would notice it.
	ReloadCertificate() error
}

// PanelSettingsService owns the panel's own access configuration at runtime.
type PanelSettingsService struct {
	logger *zap.Logger
	audit  *AuditService

	mu  sync.Mutex
	cfg *config.PanelAccessConfig

	// boot* is what this process started with. With a reloader attached these
	// are history rather than a restart trigger, and they stay because "you are
	// running on 8888, you saved 9000" is still the clearest way to describe a
	// change that could not be applied.
	bootPort         int
	bootBind         string
	bootPublicPort   int
	bootPublicScheme string
	bootTLSEnabled   bool
	bootTLSCertFile  string
	bootTLSKeyFile   string

	// stickyRestart holds reasons that no field comparison can detect. With a
	// reloader attached it stays empty; without one it is how an operator finds
	// out that a saved change is still only saved.
	stickyRestart map[string]bool

	// applying is true while this service is inside the reloader's Apply, which
	// it enters holding s.mu.
	//
	// Apply notifies its subscribers before it returns, and this service is one
	// of them. Without this flag the notification would come back to a callback
	// that takes s.mu on the goroutine that already holds it, and the request
	// would hang until the drain timeout - a deadlock that only appears when
	// the change is applied for real, which is exactly the class of defect a
	// component-level test never sees.
	applying atomic.Bool

	// reloader applies a saved configuration to the running process: the
	// listener, the access gate and the certificate manager. Without one the
	// settings are validated and persisted and take effect on the next start,
	// and every response says so rather than claiming otherwise.
	reloader PanelReloader
}

// NewPanelSettingsService takes the configuration this process started with.
//
// When the process has published a reloader (see internal/reload), the service
// attaches to it: the live configuration becomes the authority, changes made
// here are applied to the running panel, and changes made elsewhere - an edited
// file, a SIGHUP - are picked up here. That handshake happens in the
// constructor because the router builds this service positionally, out of reach
// of the entry point that owns the listener.
func NewPanelSettingsService(cfg *config.PanelAccessConfig, audit *AuditService, logger *zap.Logger) *PanelSettingsService {
	if cfg == nil {
		cfg = config.DefaultPanelAccess()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	running := panelCloneConfig(cfg)

	s := &PanelSettingsService{
		logger:           logger,
		audit:            audit,
		cfg:              running,
		bootPort:         running.Port,
		bootBind:         running.Bind,
		bootPublicPort:   running.PublicPort,
		bootPublicScheme: running.PublicScheme,
		bootTLSEnabled:   running.TLS.Enabled,
		bootTLSCertFile:  running.TLS.CertFile,
		bootTLSKeyFile:   running.TLS.KeyFile,
		stickyRestart:    map[string]bool{},
	}

	if supervisor := reload.Default(); supervisor != nil {
		s.SetReloader(supervisor)
	}

	return s
}

// SetReloader attaches the service to the running panel. The live configuration
// replaces whatever this service was constructed with, because the process is
// the authority on what it is actually serving.
func (s *PanelSettingsService) SetReloader(reloader PanelReloader) {
	if reloader == nil {
		return
	}

	s.mu.Lock()
	s.reloader = reloader
	if live := reloader.Current(); live != nil {
		s.cfg = panelCloneConfig(live)
		s.bootPort = live.Port
		s.bootBind = live.Bind
		s.bootPublicPort = live.PublicPort
		s.bootPublicScheme = live.PublicScheme
		s.bootTLSEnabled = live.TLS.Enabled
		s.bootTLSCertFile = live.TLS.CertFile
		s.bootTLSKeyFile = live.TLS.KeyFile
	}
	s.mu.Unlock()

	// A configuration that goes live somewhere else - an operator editing the
	// state file, an installer rewriting it, SIGHUP - has to be visible here
	// too, or the settings page would show what it last wrote instead of what
	// the panel is running.
	reloader.OnApplied(func(cfg *config.PanelAccessConfig) {
		if cfg == nil {
			return
		}
		if s.applying.Load() {
			// This is the echo of our own change, delivered on the goroutine
			// that is still holding s.mu. commitLocked stores the result
			// itself; taking the lock here would deadlock.
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.cfg = panelCloneConfig(cfg)
	})
}

// Get returns the current settings and everything derived from them.
func (s *PanelSettingsService) Get(caller PanelSettingsCaller) *PanelSettingsView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked(caller)
}

// Update applies a partial change. It validates first, then checks whether the
// result would cut the caller off, and only then writes anything: a rejected
// request leaves the state file untouched.
func (s *PanelSettingsService) Update(ctx context.Context, caller PanelSettingsCaller, req *PanelSettingsUpdate) (*PanelSettingsResult, error) {
	if req == nil {
		return nil, &PanelSettingsValidationError{Message: "Empty request body."}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := panelCloneConfig(s.cfg)
	if err := panelApplyUpdate(next, req); err != nil {
		return nil, err
	}
	if err := panelValidate(next); err != nil {
		return nil, err
	}
	// The config package owns the remaining invariants (reserved ports, bind
	// address, TLS material). Let it have the last word rather than guessing.
	if err := next.Validate(); err != nil {
		return nil, &PanelSettingsValidationError{Message: panelCleanConfigError(err)}
	}

	// A pasted certificate is proven and written before anything else is
	// decided, because installing it changes which files the configuration
	// points at and therefore what the rest of this request is about. Every
	// path out of here after this point either commits or calls
	// material.rollback().
	material, err := s.installPastedCertificateLocked(next, req)
	if err != nil {
		return nil, err
	}

	changes := panelDiff(s.cfg, next)
	if material != nil && material.installed {
		changes = append(changes, PanelSettingChange{
			Field: "tls.certificate",
			Old:   material.previousFingerprint,
			New:   material.inspection.Fingerprint,
		})
	}

	if len(changes) == 0 {
		return &PanelSettingsResult{
			Settings:       s.viewLocked(caller),
			Changes:        []PanelSettingChange{},
			AccessURL:      s.cfg.AccessURL(),
			RestartPending: s.restartPendingLocked(),
			RestartReasons: s.restartReasonsLocked(),
			Warnings:       []string{},
			Applied:        true,
			Fields:         []reload.FieldStatus{},
		}, nil
	}

	if err := panelEnvPinned(s.cfg, changes); err != nil {
		material.rollback()
		return nil, err
	}

	if reasons := panelLockoutReasons(s.cfg, next, caller); len(reasons) > 0 && !req.Confirm {
		material.rollback()
		return nil, &PanelSettingsConfirmationError{
			Reasons: reasons,
			NewURL:  next.AccessURL(),
			Changes: changes,
		}
	}

	return s.commitLocked(ctx, caller, next, changes, "panel.settings.update", material)
}

// panelTLSMaterial is a certificate pair this request wrote to disk, and the
// means to put back what was there before.
type panelTLSMaterial struct {
	installed           bool
	certFile            string
	keyFile             string
	backup              *config.CertificateBackup
	inspection          *config.CertificateInspection
	previousFingerprint string
}

// rollback restores the certificate pair that was on disk before this request.
// It is called on every failure path after the pair was written, because a
// change that was refused must leave nothing behind - least of all a
// certificate the panel is not configured to serve.
func (m *panelTLSMaterial) rollback() {
	if m == nil || !m.installed {
		return
	}
	m.backup.Restore(m.certFile, m.keyFile)
	m.installed = false
}

// installPastedCertificateLocked validates an operator-supplied certificate and
// key and writes them to disk, returning what it did so a later failure can
// undo it.
//
// The validation order is the order in which a failure makes the later checks
// meaningless, and every rejection names the check that produced it:
//
//  1. both blocks are PEM, and are what they claim to be;
//  2. the key is not passphrase-protected, because there is nowhere to keep a
//     passphrase across a restart;
//  3. the key belongs to the certificate. A mismatched pair loads and then
//     fails every handshake, which from outside looks like the panel is down;
//  4. the pair loads through crypto/tls, the same way the listener will load it;
//  5. the certificate is inside its validity period, with a warning inside the
//     last thirty days;
//  6. the chain reaches a trusted root, or the operator is told that
//     intermediates are missing rather than serving a chain browsers reject;
//  7. the names on the certificate cover the host the panel is actually reached
//     as.
//
// The last two are refusable-with-override: both can be deliberate, and neither
// may be accepted silently.
func (s *PanelSettingsService) installPastedCertificateLocked(next *config.PanelAccessConfig, req *PanelSettingsUpdate) (*panelTLSMaterial, error) {
	certPEM := panelTrimPEM(req.TLSCertificate)
	keyPEM := panelTrimPEM(req.TLSPrivateKey)

	if certPEM == "" && keyPEM == "" {
		return nil, nil
	}
	if certPEM == "" || keyPEM == "" {
		return nil, &PanelSettingsValidationError{
			Field:   "tls_certificate",
			Check:   config.CertCheckKeyPEM,
			Message: "A certificate and its private key have to be supplied together. Paste both, or neither to leave the current certificate in place.",
		}
	}

	inspection, err := config.InspectCertificatePair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		var certErr *config.CertificateError
		if errors.As(err, &certErr) {
			return nil, &PanelSettingsValidationError{
				Field:   panelCertField(certErr.Check),
				Check:   certErr.Check,
				Message: certErr.Message,
			}
		}
		return nil, err
	}

	// The host the operator actually reaches this panel on. A certificate for
	// the wrong name locks them out exactly the way a bad port change does: the
	// browser refuses to connect and the panel is the only tool that could fix
	// it.
	host := next.AccessHost()
	if !inspection.CoversHost(host) && !req.TLSAcceptRisk {
		return nil, &PanelTLSRiskError{
			Check: config.CertCheckHostnames,
			Message: fmt.Sprintf(
				"This certificate does not cover %s, which is the address this panel is reached at. It covers %s. A browser will refuse the connection, and the panel is the tool you would use to undo that.",
				host, panelCertNames(inspection)),
			Inspection: inspection,
		}
	}

	if !inspection.ChainComplete && !req.TLSAcceptRisk {
		return nil, &PanelTLSRiskError{
			Check:      config.CertCheckChain,
			Message:    panelChainMessage(inspection),
			Inspection: inspection,
		}
	}

	certFile, keyFile := panelCertPathsFor(next)

	backup, err := config.InstallCustomPair(certFile, keyFile, []byte(certPEM+"\n"), []byte(keyPEM+"\n"))
	if err != nil {
		return nil, err
	}

	previous := config.CertFingerprint(certFile)
	if backup != nil && !backup.Existed {
		previous = "(none)"
	}

	// A pasted pair is by definition the custom mode: the panel serves what it
	// was given and never regenerates it. Setting the mode here is also what
	// stops the ACME renewal loop, which would otherwise replace this file on a
	// timer weeks from now.
	next.TLS.Enabled = true
	next.TLS.Mode = config.TLSModeCustom
	next.TLS.SelfSigned = false
	next.TLS.CertFile = certFile
	next.TLS.KeyFile = keyFile

	return &panelTLSMaterial{
		installed:           true,
		certFile:            certFile,
		keyFile:             keyFile,
		backup:              backup,
		inspection:          inspection,
		previousFingerprint: previous,
	}, nil
}

// panelCertPathsFor decides where a pasted pair is stored.
//
// Never over the pair the panel generates and renews for itself: those files
// are rewritten without asking, and an operator's certificate landing there
// would be silently replaced by a renewal weeks later.
func panelCertPathsFor(cfg *config.PanelAccessConfig) (certFile, keyFile string) {
	certFile = strings.TrimSpace(cfg.TLS.CertFile)
	keyFile = strings.TrimSpace(cfg.TLS.KeyFile)

	if certFile == "" || keyFile == "" ||
		config.IsManagedCertPath(certFile) || config.IsManagedCertPath(keyFile) {
		return config.CustomPanelCertPaths()
	}
	return certFile, keyFile
}

// panelCertField maps a failed check onto the form field that produced it.
func panelCertField(check string) string {
	switch check {
	case config.CertCheckKeyPEM, config.CertCheckKeyEncrypted:
		return "tls_private_key"
	case config.CertCheckKeyMatch, config.CertCheckLoad:
		return "tls_private_key"
	default:
		return "tls_certificate"
	}
}

func panelCertNames(inspection *config.CertificateInspection) string {
	names := append([]string{}, inspection.DNSNames...)
	names = append(names, inspection.IPAddresses...)
	if len(names) == 0 {
		return "no host names at all"
	}
	return strings.Join(names, ", ")
}

func panelChainMessage(inspection *config.CertificateInspection) string {
	if inspection.SelfSigned {
		return "This certificate signed itself. No browser will trust it until it is installed by hand on every machine that reaches the panel, which is a decision to make deliberately rather than by accident."
	}
	return fmt.Sprintf(
		"The chain is incomplete: this certificate was issued by %q and that issuer's certificate was not pasted with it. Browsers that do not already hold the intermediate will reject the connection. Paste the full chain your certificate authority supplied - the certificate first, then each intermediate below it.",
		inspection.Issuer)
}

// panelTrimPEM normalises pasted PEM: the text areas an operator uses add
// leading and trailing whitespace, and a stray blank line is not a reason to
// refuse a certificate.
func panelTrimPEM(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// RegenerateEntrance replaces the security entrance with a fresh random one.
// It is a lockout by construction - the old URL stops working the moment it is
// saved - so it always needs confirmation.
func (s *PanelSettingsService) RegenerateEntrance(ctx context.Context, caller PanelSettingsCaller, confirm bool) (*PanelSettingsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cfg.EntranceEnabled {
		return nil, &PanelSettingsValidationError{
			Field:   "entrance",
			Message: "The security entrance is switched off. Turn it on before generating a new one.",
		}
	}

	entrance, err := config.RandomEntrance()
	if err != nil {
		return nil, err
	}

	next := panelCloneConfig(s.cfg)
	next.Entrance = config.NormalizeEntrance(entrance)
	if err := panelValidate(next); err != nil {
		return nil, err
	}
	if err := next.Validate(); err != nil {
		return nil, &PanelSettingsValidationError{Message: panelCleanConfigError(err)}
	}

	changes := panelDiff(s.cfg, next)
	if !confirm {
		return nil, &PanelSettingsConfirmationError{
			Reasons: []PanelConfirmationReason{{
				Code:    "entrance_changed",
				Message: "A new security entrance replaces the current one immediately. The old panel URL stops working. Save the new URL before continuing.",
			}},
			NewURL:  next.AccessURL(),
			Changes: changes,
		}
	}

	return s.commitLocked(ctx, caller, next, changes, "panel.settings.entrance.regenerate", nil)
}

// ReissueCertificate throws away the panel's self-signed certificate and makes
// a new one for the hosts the panel is currently reachable as.
//
// The running listener read the old certificate off disk when it started, so
// the new one is served after a restart. Saying so is the point: an operator
// who compares the fingerprint in the browser against a certificate that is not
// being served yet learns nothing.
func (s *PanelSettingsService) ReissueCertificate(ctx context.Context, caller PanelSettingsCaller) (*PanelSettingsResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cfg.TLS.Enabled {
		return nil, &PanelSettingsValidationError{
			Field:   "tls",
			Message: "TLS is switched off for the panel. Enable it before reissuing a certificate.",
		}
	}
	if !s.cfg.TLS.SelfSigned {
		return nil, &PanelSettingsValidationError{
			Field:   "tls",
			Message: "The panel is serving an operator-supplied certificate. Replace the certificate and key files, or switch to a self-signed certificate first.",
		}
	}

	next := panelCloneConfig(s.cfg)
	oldFingerprint := config.CertFingerprint(next.TLS.CertFile)

	// Removing the pair is what forces a reissue: EnsureTLSMaterial keeps an
	// existing certificate that still covers every host.
	if path := strings.TrimSpace(next.TLS.CertFile); path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("panel settings: cannot remove the old certificate %s: %w", path, err)
		}
	}
	if path := strings.TrimSpace(next.TLS.KeyFile); path != "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("panel settings: cannot remove the old private key %s: %w", path, err)
		}
	}

	if _, _, err := next.EnsureTLSMaterial(); err != nil {
		return nil, err
	}

	newFingerprint := config.CertFingerprint(next.TLS.CertFile)

	// The certificate is on disk; making it the one on the wire is a separate
	// step, because it moves no setting and the configuration diff would
	// correctly find nothing to apply. Reporting a new fingerprint while the
	// listener still serves the old certificate would be worse than useless:
	// an operator comparing it against what their browser shows would conclude
	// they were being intercepted.
	if s.reloader != nil {
		if err := s.reloader.ReloadCertificate(); err != nil {
			s.stickyRestart["tls_certificate"] = true
			s.logger.Error("panel settings: the reissued certificate is on disk but could not be served",
				zap.Error(err))
		} else {
			delete(s.stickyRestart, "tls_certificate")
		}
	} else {
		s.stickyRestart["tls_certificate"] = true
	}

	changes := []PanelSettingChange{{
		Field: "tls.fingerprint",
		Old:   oldFingerprint,
		New:   newFingerprint,
	}}

	return s.commitLocked(ctx, caller, next, changes, "panel.settings.tls.reissue", nil)
}

// commitLocked makes a validated configuration real. The caller must hold s.mu.
//
// With a reloader attached, persisting and applying are one operation and it
// belongs to the reloader: it writes the state file, rebinds the listener,
// swaps the access gate and replaces the certificate manager as a single
// prepare/commit/verify sequence, and puts every one of them back if the panel
// turns out to be unreachable afterwards. Saving here first would defeat that -
// a rolled-back change would leave a state file describing a port this process
// is not listening on, and the next restart would move the panel to it.
//
// Without a reloader the settings are still validated and still persisted; the
// response then says plainly that they are saved and not yet live.
func (s *PanelSettingsService) commitLocked(
	ctx context.Context,
	caller PanelSettingsCaller,
	next *config.PanelAccessConfig,
	changes []PanelSettingChange,
	action string,
	material *panelTLSMaterial,
) (*PanelSettingsResult, error) {
	next.StateFile = s.cfg.StateFile
	warnings := make([]string, 0, 4)

	// A pinned domain or a new bind address changes which hosts the self-signed
	// certificate has to cover. EnsureTLSMaterial reissues it when it no longer
	// matches; failing to do so must not roll back a saved setting, so it is
	// reported as a warning instead. A pasted certificate is never touched: it
	// is already on disk and already proven.
	if next.TLS.Enabled && next.TLSMode() != config.TLSModeCustom {
		if _, _, err := next.EnsureTLSMaterial(); err != nil {
			warnings = append(warnings, "Settings saved, but the panel TLS material could not be prepared: "+err.Error())
		}
	}

	var (
		outcome *reload.Outcome
		applied bool
	)

	if s.reloader != nil {
		s.applying.Store(true)
		result, err := s.reloader.Apply(ctx, next, reload.Request{
			Origin:   reload.OriginAPI,
			Actor:    caller.UserID.String(),
			ClientIP: caller.ClientIP,
			Host:     caller.Host,
			Detail:   action,
			Caller:   &reload.Caller{ClientIP: caller.ClientIP, Host: caller.Host},
		})
		s.applying.Store(false)
		if err != nil {
			// Nothing was applied and nothing was persisted. Put back any
			// certificate files this request replaced, so the disk agrees with
			// the process again.
			material.rollback()

			var rolledBack *reload.RollbackError
			if errors.As(err, &rolledBack) {
				s.auditFailureLocked(ctx, caller, action, changes, rolledBack.Reason)
				return nil, &PanelSettingsRollbackError{
					Reason:    rolledBack.Reason,
					Changes:   changes,
					AccessURL: s.cfg.AccessURL(),
				}
			}
			s.auditFailureLocked(ctx, caller, action, changes, err.Error())
			return nil, &PanelSettingsApplyError{
				Reason:    panelCleanReloadError(err),
				Changes:   changes,
				AccessURL: s.cfg.AccessURL(),
			}
		}

		outcome = result
		applied = result.Applied
		warnings = append(warnings, result.Warnings...)
		if live := s.reloader.Current(); live != nil {
			next = panelCloneConfig(live)
		}
	} else {
		if err := next.Save(); err != nil {
			material.rollback()
			return nil, err
		}
		s.stickyRestart["not_hot_reloadable"] = true
		warnings = append(warnings,
			"Settings saved. This build has no reloader attached, so they take effect the next time the panel service starts.")
	}

	s.cfg = next

	s.auditLocked(ctx, caller, action, changes, applied)

	view := s.viewLocked(caller)
	if view.RestartPending {
		warnings = append(warnings,
			"Some of these settings are saved but not live; see restart_reasons for which, and why.")
	}

	result := &PanelSettingsResult{
		Settings:       view,
		Changes:        changes,
		AccessURL:      view.AccessURL,
		RestartPending: view.RestartPending,
		RestartReasons: view.RestartReasons,
		Warnings:       warnings,
		Applied:        applied || s.reloader == nil,
		Fields:         []reload.FieldStatus{},
	}
	if outcome != nil {
		result.Fields = outcome.Fields
		result.Applied = outcome.Applied
	}
	if material != nil {
		result.Certificate = material.inspection
		if material.inspection != nil {
			warnings = append(warnings, material.inspection.Warnings...)
			result.Warnings = warnings
		}
	}

	return result, nil
}

// auditLocked records who changed what, and whether it actually took effect.
//
// "Applied" is part of the record on purpose. An audit trail that says the port
// changed, when the change was written to a file and never reached the running
// process, is worse than no entry at all: it is evidence for something that did
// not happen. It never fails the request - losing the audit entry is bad,
// refusing a change that is already live is worse.
func (s *PanelSettingsService) auditLocked(ctx context.Context, caller PanelSettingsCaller, action string, changes []PanelSettingChange, applied bool) {
	s.auditEntryLocked(ctx, caller, action, changes, applied, "success", "")
}

// auditFailureLocked records a change that was refused or rolled back. It
// matters as much as a success: an operator investigating why the panel is on
// its old port needs to find the attempt, not silence.
func (s *PanelSettingsService) auditFailureLocked(ctx context.Context, caller PanelSettingsCaller, action string, changes []PanelSettingChange, reason string) {
	s.auditEntryLocked(ctx, caller, action, changes, false, "failure", reason)
}

func (s *PanelSettingsService) auditEntryLocked(
	ctx context.Context,
	caller PanelSettingsCaller,
	action string,
	changes []PanelSettingChange,
	applied bool,
	status string,
	reason string,
) {
	redacted := make([]map[string]string, 0, len(changes))
	for _, change := range changes {
		redacted = append(redacted, map[string]string{
			"field": change.Field,
			"old":   panelRedactValue(change.Field, change.Old),
			"new":   panelRedactValue(change.Field, change.New),
		})
	}

	if s.audit != nil {
		details := models.JSONMap{
			"changes":      redacted,
			"access_url":   panelRedactURL(s.cfg),
			"client_ip":    caller.ClientIP,
			"state_file":   s.cfg.StateFile,
			"change_count": len(changes),
			"applied":      applied,
			"origin":       string(reload.OriginAPI),
		}
		if reason != "" {
			details["reason"] = reason
		}

		var userID *uuid.UUID
		if caller.UserID != uuid.Nil {
			id := caller.UserID
			userID = &id
		}

		if err := s.audit.Log(ctx, caller.TenantID, userID, action, PanelSettingsAuditResource, nil,
			details, caller.ClientIP, caller.UserAgent, status); err != nil {
			s.logger.Error("panel settings: audit log write failed",
				zap.String("action", action),
				zap.Error(err),
			)
		}
	}

	// The audit trail is the record of record, but an operator reading the
	// service log during an incident should not have to open the database.
	fields := make([]zap.Field, 0, len(changes)+5)
	fields = append(fields,
		zap.String("action", action),
		zap.String("client_ip", caller.ClientIP),
		zap.Bool("applied", applied),
		zap.String("status", status),
	)
	if reason != "" {
		fields = append(fields, zap.String("reason", reason))
	}
	for _, change := range changes {
		fields = append(fields, zap.String(change.Field,
			panelRedactValue(change.Field, change.Old)+" -> "+panelRedactValue(change.Field, change.New)))
	}

	if status == "success" {
		s.logger.Info("panel access settings changed", fields...)
		return
	}
	s.logger.Warn("panel access settings change refused", fields...)
}

// viewLocked renders the response body. The caller must hold s.mu.
func (s *PanelSettingsService) viewLocked(caller PanelSettingsCaller) *PanelSettingsView {
	cfg := s.cfg
	reasons := s.restartReasonsLocked()

	view := &PanelSettingsView{
		Enabled:           cfg.Enabled,
		Bind:              cfg.Bind,
		Port:              cfg.Port,
		PublicPort:        cfg.PublicPort,
		PublicScheme:      cfg.PublicScheme,
		Entrance:          cfg.Entrance,
		EntranceMasked:    panelMaskEntrance(cfg.Entrance),
		EntranceEnabled:   cfg.EntranceEnabled,
		SessionTTLSeconds: cfg.SessionTTLSeconds,
		AllowedIPs:        panelCopyList(cfg.AllowedIPs),
		TrustedProxies:    panelCopyList(cfg.TrustedProxies),
		Domain:            cfg.Domain,
		TLS:               panelTLSView(cfg),
		AccessURL:         cfg.AccessURL(),
		AccessHost:        cfg.AccessHost(),
		EffectivePort:     cfg.EffectivePort(),
		Scheme:            cfg.EffectiveScheme(),
		Proxied:           cfg.IsProxied(),
		RestartPending:    len(reasons) > 0,
		RestartReasons:    reasons,
		RunningPort:       s.bootPort,
		RunningBind:       s.bootBind,
		HotReload:         s.reloader != nil,
		EnvOverrides:      panelCopyList(cfg.EnvOverrides),
		StateFile:         cfg.StateFile,
		UpdatedAt:         cfg.UpdatedAt,
		ClientIP:          caller.ClientIP,
	}

	if s.reloader != nil {
		// With a reloader the port is live, so what is running is what is
		// configured - reporting the boot values would be stale the moment the
		// first rebind happened.
		view.RunningPort = cfg.Port
		view.RunningBind = cfg.Bind
		if at := s.reloader.LastApplied(); !at.IsZero() {
			applied := at
			view.LastReloadAt = &applied
		}
	}

	return view
}

func (s *PanelSettingsService) restartPendingLocked() bool {
	return len(s.restartReasonsLocked()) > 0
}

// restartReasonsLocked lists the settings that are saved but not yet live.
//
// With a reloader attached this is normally empty. The panel rebinds its
// listener, rebuilds its access gate and replaces its certificate manager
// without stopping, so none of those settings is ever merely "saved". What can
// still be in here is the honest remainder - database credentials, the Redis
// address, the signing secret, the filesystem layout - each of which is held by
// an object built once at start-up and reported with the reason it cannot move.
//
// Without a reloader every listener setting is in here, because without one
// none of them is live.
func (s *PanelSettingsService) restartReasonsLocked() []string {
	cfg := s.cfg

	if s.reloader != nil {
		reasons := s.reloader.RestartRequired()
		sticky := s.stickyReasonsLocked()
		return append(reasons, sticky...)
	}

	reasons := make([]string, 0, 6)
	if cfg.Port != s.bootPort {
		reasons = append(reasons, fmt.Sprintf("port (running on %d, saved as %d)", s.bootPort, cfg.Port))
	}
	if cfg.Bind != s.bootBind {
		reasons = append(reasons, fmt.Sprintf("bind address (running on %s, saved as %s)", s.bootBind, cfg.Bind))
	}
	if cfg.PublicPort != s.bootPublicPort {
		reasons = append(reasons, "public port")
	}
	if cfg.PublicScheme != s.bootPublicScheme {
		reasons = append(reasons, "public scheme")
	}
	if cfg.TLS.Enabled != s.bootTLSEnabled {
		reasons = append(reasons, "TLS mode")
	}
	if cfg.TLS.CertFile != s.bootTLSCertFile || cfg.TLS.KeyFile != s.bootTLSKeyFile {
		reasons = append(reasons, "TLS certificate paths")
	}

	return append(reasons, s.stickyReasonsLocked()...)
}

func (s *PanelSettingsService) stickyReasonsLocked() []string {
	sticky := make([]string, 0, len(s.stickyRestart))
	for reason := range s.stickyRestart {
		switch reason {
		case "tls_certificate":
			sticky = append(sticky, "TLS certificate (a new certificate is on disk; the listener is still serving the old one)")
		case "access_gate":
			sticky = append(sticky, "access gate (the running guard could not be reloaded)")
		case "not_hot_reloadable":
			sticky = append(sticky, "every setting here (this build has no reloader attached, so nothing takes effect until the panel service restarts)")
		default:
			sticky = append(sticky, reason)
		}
	}
	sort.Strings(sticky)
	return sticky
}

// ---------------------------------------------------------------------------
// Update application and validation
// ---------------------------------------------------------------------------

// panelApplyUpdate copies the supplied fields onto a candidate configuration,
// normalising as it goes so validation and comparison both see canonical values.
func panelApplyUpdate(cfg *config.PanelAccessConfig, req *PanelSettingsUpdate) error {
	if req.Port != nil {
		cfg.Port = *req.Port
	}
	if req.Bind != nil {
		cfg.Bind = strings.TrimSpace(*req.Bind)
	}
	if req.PublicPort != nil {
		cfg.PublicPort = *req.PublicPort
	}
	if req.PublicScheme != nil {
		cfg.PublicScheme = strings.ToLower(strings.TrimSpace(*req.PublicScheme))
	}
	if req.EntranceEnabled != nil {
		cfg.EntranceEnabled = *req.EntranceEnabled
	}
	if req.Entrance != nil {
		cfg.Entrance = config.NormalizeEntrance(*req.Entrance)
	}
	if req.SessionTTLSeconds != nil {
		cfg.SessionTTLSeconds = *req.SessionTTLSeconds
	}
	if req.AllowedIPs != nil {
		cfg.AllowedIPs = panelNormalizeList(*req.AllowedIPs)
	}
	if req.TrustedProxies != nil {
		cfg.TrustedProxies = panelNormalizeList(*req.TrustedProxies)
	}
	if req.Domain != nil {
		cfg.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(*req.Domain), "."))
	}
	if req.TLSEnabled != nil {
		cfg.TLS.Enabled = *req.TLSEnabled
	}
	if req.TLSSelfSigned != nil {
		cfg.TLS.SelfSigned = *req.TLSSelfSigned
		// The legacy flag still moves the mode, so a client that predates modes
		// keeps behaving as it did instead of setting a flag nothing reads.
		if *req.TLSSelfSigned {
			cfg.TLS.Mode = config.TLSModeSelfSigned
		} else if cfg.TLSMode() == config.TLSModeSelfSigned {
			cfg.TLS.Mode = config.TLSModeCustom
		}
	}
	if req.TLSMode != nil {
		mode, err := config.ParsePanelTLSMode(*req.TLSMode)
		if err != nil {
			return &PanelSettingsValidationError{
				Field: "tls_mode",
				Message: fmt.Sprintf("The certificate mode must be one of %q (the panel generates its own), %q (the panel obtains and renews one automatically) or %q (you supply one).",
					config.TLSModeSelfSigned, config.TLSModeLetsEncrypt, config.TLSModeCustom),
			}
		}
		cfg.TLS.Mode = mode
		cfg.TLS.SelfSigned = mode == config.TLSModeSelfSigned
		// Naming a mode is asking for TLS, unless the same request switched it
		// off - in which case that is the more specific instruction and wins.
		if req.TLSEnabled == nil {
			cfg.TLS.Enabled = true
		}
		if mode != config.TLSModeCustom && !config.IsManagedCertPath(cfg.TLS.CertFile) {
			// Moving back to a mode the panel manages means moving back to the
			// files it manages. Leaving the pasted paths in place would have the
			// generator or the renewal loop overwrite the operator's own files.
			cfg.TLS.CertFile, cfg.TLS.KeyFile = "", ""
		}
	}
	if req.TLSCertFile != nil {
		cfg.TLS.CertFile = strings.TrimSpace(*req.TLSCertFile)
	}
	if req.TLSKeyFile != nil {
		cfg.TLS.KeyFile = strings.TrimSpace(*req.TLSKeyFile)
	}
	if req.ACMEEmail != nil {
		cfg.TLS.ACME.Email = strings.TrimSpace(*req.ACMEEmail)
	}
	if req.ACMEUseStaging != nil {
		cfg.TLS.ACME.UseStaging = *req.ACMEUseStaging
	}
	if req.ACMEProfile != nil {
		cfg.TLS.ACME.Profile = strings.ToLower(strings.TrimSpace(*req.ACMEProfile))
	}

	// A mode the panel manages needs somewhere to keep the pair it produces.
	// fillGenerated does this at start-up; doing it here means a mode change
	// through the API lands on the same paths as a mode change in the file.
	if cfg.TLS.Enabled && cfg.TLSMode() != config.TLSModeCustom {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			cfg.TLS.CertFile, cfg.TLS.KeyFile = config.ManagedPanelCertPaths()
		}
	}

	return nil
}

// panelValidate is the panel-facing rule set. Every message names the field and
// says what would be accepted, because the operator cannot read the source.
func panelValidate(cfg *config.PanelAccessConfig) error {
	// The reserved ports are checked first: 80 and 443 are also below the
	// minimum, and "that port belongs to the hosted websites" tells an operator
	// far more than "out of range" does.
	if name, forbidden := panelForbiddenPorts[cfg.Port]; forbidden {
		return &PanelSettingsValidationError{
			Field:   "port",
			Message: fmt.Sprintf("Port %d is reserved for %s. The panel must keep its own port so it never competes with a hosted website.", cfg.Port, name),
		}
	}
	if cfg.Port < PanelSettingsMinPort || cfg.Port > PanelSettingsMaxPort {
		return &PanelSettingsValidationError{
			Field: "port",
			Message: fmt.Sprintf("Port must be between %d and %d. Ports below %d are privileged and already belong to the services running on this host.",
				PanelSettingsMinPort, PanelSettingsMaxPort, PanelSettingsMinPort),
		}
	}

	if cfg.PublicPort != 0 {
		if name, forbidden := panelForbiddenPorts[cfg.PublicPort]; forbidden {
			return &PanelSettingsValidationError{
				Field:   "public_port",
				Message: fmt.Sprintf("Public port %d is reserved for %s.", cfg.PublicPort, name),
			}
		}
		if cfg.PublicPort < PanelSettingsMinPort || cfg.PublicPort > PanelSettingsMaxPort {
			return &PanelSettingsValidationError{
				Field:   "public_port",
				Message: fmt.Sprintf("Public port must be between %d and %d, or 0 to publish the panel on its own port.", PanelSettingsMinPort, PanelSettingsMaxPort),
			}
		}
	}

	if scheme := cfg.PublicScheme; scheme != "" && scheme != "http" && scheme != "https" {
		return &PanelSettingsValidationError{
			Field:   "public_scheme",
			Message: "Public scheme must be either http or https.",
		}
	}

	if bind := strings.TrimSpace(cfg.Bind); bind != "" && bind != "0.0.0.0" && bind != "::" {
		if net.ParseIP(bind) == nil {
			return &PanelSettingsValidationError{
				Field:   "bind",
				Message: fmt.Sprintf("Bind address %q is not a valid IP address. Use 0.0.0.0 to listen on every interface.", cfg.Bind),
			}
		}
	}

	if cfg.EntranceEnabled {
		if !panelEntrancePattern.MatchString(cfg.Entrance) {
			return &PanelSettingsValidationError{
				Field:   "entrance",
				Message: "The security entrance must be a single path starting with / followed by 4 to 64 letters, digits, hyphens or underscores, for example /vkai_a1b2c3d4.",
			}
		}
		if err := config.ValidateEntrance(cfg.Entrance); err != nil {
			return &PanelSettingsValidationError{
				Field:   "entrance",
				Message: fmt.Sprintf("The security entrance %q collides with a reserved path used by the API.", cfg.Entrance),
			}
		}
	}

	for _, entry := range cfg.AllowedIPs {
		if _, err := config.ParseIPMatcher(entry); err != nil {
			return &PanelSettingsValidationError{
				Field:   "allowed_ips",
				Message: fmt.Sprintf("%q is neither an IP address nor a CIDR block. Use 203.0.113.7 or 203.0.113.0/24.", entry),
			}
		}
	}
	for _, entry := range cfg.TrustedProxies {
		if _, err := config.ParseIPMatcher(entry); err != nil {
			return &PanelSettingsValidationError{
				Field:   "trusted_proxies",
				Message: fmt.Sprintf("%q is neither an IP address nor a CIDR block. Use 10.0.0.5 or 10.0.0.0/8.", entry),
			}
		}
	}

	if domain := cfg.Domain; domain != "" {
		if len(domain) > 253 || !panelHostnamePattern.MatchString(domain) {
			return &PanelSettingsValidationError{
				Field:   "domain",
				Message: fmt.Sprintf("%q is not a valid host name. Enter a bare host name such as panel.example.com, with no scheme, port or path.", cfg.Domain),
			}
		}
	}

	ttl := time.Duration(cfg.SessionTTLSeconds) * time.Second
	if ttl < PanelSettingsMinSessionTTL || ttl > PanelSettingsMaxSessionTTL {
		return &PanelSettingsValidationError{
			Field: "session_ttl_seconds",
			Message: fmt.Sprintf("Session lifetime must be between %d seconds (5 minutes) and %d seconds (30 days).",
				int(PanelSettingsMinSessionTTL/time.Second), int(PanelSettingsMaxSessionTTL/time.Second)),
		}
	}

	if cfg.TLS.Enabled && cfg.TLSMode() == config.TLSModeCustom {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			return &PanelSettingsValidationError{
				Field:   "tls",
				Message: "A supplied certificate needs both a certificate and a private key. Paste both, or choose the self-signed or automatic mode to have the panel produce its own.",
			}
		}
	}

	if cfg.TLS.Enabled && cfg.TLSMode() == config.TLSModeLetsEncrypt {
		// The identifier is what an order is placed for, and an order that
		// cannot be validated does not merely fail: it spends a per-account
		// rate limit shared with every future order from this installation.
		if _, err := cfg.ACMEIdentifier(); err != nil {
			return &PanelSettingsValidationError{
				Field:   "tls_mode",
				Message: "Automatic issuance cannot be used here: " + err.Error(),
			}
		}
	}

	return nil
}

// panelFieldEnvMarkers maps a changed field onto the marker the config package
// records when that setting came from the environment.
var panelFieldEnvMarkers = map[string]string{
	"enabled":              "enabled",
	"port":                 "port",
	"bind":                 "bind",
	"public_port":          "public_port",
	"public_scheme":        "public_scheme",
	"entrance":             "entrance",
	"entrance_enabled":     "entrance_enabled",
	"session_ttl_seconds":  "session_ttl",
	"allowed_ips":          "allowed_ips",
	"trusted_proxies":      "trusted_proxies",
	"domain":               "domain",
	"tls.enabled":          "tls_enabled",
	"tls.mode":             "tls_mode",
	"tls.self_signed":      "tls_self_signed",
	"tls.cert_file":        "tls_cert",
	"tls.key_file":         "tls_key",
	"tls.acme.email":       "acme_email",
	"tls.acme.use_staging": "acme_staging",
	"tls.acme.profile":     "acme_profile",
}

// panelEnvPinned refuses a change to a setting the environment already pins.
//
// The environment wins over the state file, by design and at every load. So a
// change to a pinned setting can be applied to the running process and written
// to disk and still be undone the moment anything re-derives the configuration
// - the next restart, or the file watcher, seconds later. That is not a change
// that half-worked; it is a change that reverts itself while the operator is
// still reading the success message.
//
// Refusing it, and naming the variable to remove, is the only honest answer.
func panelEnvPinned(cfg *config.PanelAccessConfig, changes []PanelSettingChange) error {
	for _, change := range changes {
		marker, known := panelFieldEnvMarkers[change.Field]
		if !known || !cfg.IsEnvOverridden(marker) {
			continue
		}

		variable := config.PanelEnvVariable(marker)
		if variable == "" {
			variable = "the environment"
		}

		return &PanelSettingsValidationError{
			Field: change.Field,
			Check: "environment_override",
			Message: fmt.Sprintf(
				"%s is pinned by %s in the environment, which wins over anything saved here. The panel would apply this change and then undo it the next time it reads its configuration. Remove %s from %s and try again.",
				change.Field, variable, variable, config.EnvFile()),
		}
	}
	return nil
}

// panelLockoutReasons lists every way the pending change would make the panel
// unreachable from where the caller is right now.
func panelLockoutReasons(current, next *config.PanelAccessConfig, caller PanelSettingsCaller) []PanelConfirmationReason {
	reasons := make([]PanelConfirmationReason, 0, 3)

	if next.EntranceEnabled && next.Entrance != current.Entrance {
		reasons = append(reasons, PanelConfirmationReason{
			Code:    "entrance_changed",
			Message: "The security entrance changes. The current panel URL stops working as soon as this is saved.",
		})
	}

	if len(next.AllowedIPs) > 0 && !panelAllowListCovers(next.AllowedIPs, caller.ClientIP) {
		reasons = append(reasons, PanelConfirmationReason{
			Code: "allow_list_excludes_caller",
			Message: fmt.Sprintf("The IP allow list does not include your current address %s. Saving it locks this browser out of the panel.",
				panelDisplayIP(caller.ClientIP)),
		})
	}

	if next.Domain != "" && next.Domain != current.Domain && !panelHostMatches(caller.Host, next.Domain) {
		reasons = append(reasons, PanelConfirmationReason{
			Code: "domain_excludes_caller",
			Message: fmt.Sprintf("The panel will only answer requests for %s. You are connected as %s, which stops working.",
				next.Domain, panelDisplayHost(caller.Host)),
		})
	}

	return reasons
}

// panelAllowListCovers reports whether an allow list still admits an address.
// An address that cannot be parsed is never covered: failing closed here means
// asking for a confirmation, never skipping one.
func panelAllowListCovers(list []string, clientIP string) bool {
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return false
	}
	for _, entry := range list {
		network, err := config.ParseIPMatcher(entry)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// panelHostMatches compares the Host header the request arrived on against a
// pinned domain, ignoring the port and a trailing dot.
func panelHostMatches(requestHost, domain string) bool {
	host := strings.TrimSpace(requestHost)
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	return host == strings.ToLower(domain)
}

// ---------------------------------------------------------------------------
// Diffing, masking and derived values
// ---------------------------------------------------------------------------

// panelDiff lists the fields that actually changed, in a stable order so the
// audit log reads the same way every time.
func panelDiff(current, next *config.PanelAccessConfig) []PanelSettingChange {
	changes := make([]PanelSettingChange, 0, 8)

	add := func(field, oldValue, newValue string) {
		if oldValue != newValue {
			changes = append(changes, PanelSettingChange{Field: field, Old: oldValue, New: newValue})
		}
	}

	add("port", strconv.Itoa(current.Port), strconv.Itoa(next.Port))
	add("bind", current.Bind, next.Bind)
	add("public_port", strconv.Itoa(current.PublicPort), strconv.Itoa(next.PublicPort))
	add("public_scheme", current.PublicScheme, next.PublicScheme)
	add("entrance", current.Entrance, next.Entrance)
	add("entrance_enabled", strconv.FormatBool(current.EntranceEnabled), strconv.FormatBool(next.EntranceEnabled))
	add("session_ttl_seconds", strconv.Itoa(current.SessionTTLSeconds), strconv.Itoa(next.SessionTTLSeconds))
	add("allowed_ips", panelJoinList(current.AllowedIPs), panelJoinList(next.AllowedIPs))
	add("trusted_proxies", panelJoinList(current.TrustedProxies), panelJoinList(next.TrustedProxies))
	add("domain", current.Domain, next.Domain)
	add("tls.enabled", strconv.FormatBool(current.TLS.Enabled), strconv.FormatBool(next.TLS.Enabled))
	add("tls.mode", current.TLSMode(), next.TLSMode())
	add("tls.self_signed", strconv.FormatBool(current.TLS.SelfSigned), strconv.FormatBool(next.TLS.SelfSigned))
	add("tls.cert_file", current.TLS.CertFile, next.TLS.CertFile)
	add("tls.key_file", current.TLS.KeyFile, next.TLS.KeyFile)
	add("tls.acme.email", current.TLS.ACME.Email, next.TLS.ACME.Email)
	add("tls.acme.use_staging", strconv.FormatBool(current.TLS.ACME.UseStaging), strconv.FormatBool(next.TLS.ACME.UseStaging))
	add("tls.acme.profile", current.TLS.ACME.Profile, next.TLS.ACME.Profile)

	return changes
}

// panelRedactValue removes the security entrance from anything that is written
// down. Every other field is safe to record verbatim.
func panelRedactValue(field, value string) string {
	if value == "" {
		return ""
	}
	if field == "entrance" || strings.Contains(field, "private_key") {
		return panelRedactedValue
	}
	return value
}

// panelRedactURL is the access URL with the entrance stripped, for the audit
// details where the full URL would carry the secret.
func panelRedactURL(cfg *config.PanelAccessConfig) string {
	url := cfg.AccessURL()
	if cfg.EntranceEnabled && cfg.Entrance != "" {
		url = strings.Replace(url, cfg.Entrance, "/"+panelRedactedValue, 1)
	}
	return url
}

// panelMaskEntrance keeps enough of the entrance to recognise it in a list and
// not enough to use it.
func panelMaskEntrance(entrance string) string {
	trimmed := strings.TrimPrefix(entrance, "/")
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 4 {
		return "/" + strings.Repeat("*", len(trimmed))
	}
	return "/" + trimmed[:2] + strings.Repeat("*", len(trimmed)-2)
}

// panelTLSView reads the certificate on disk so the UI can show what is
// actually being served, not what was configured.
//
// Everything here comes from the certificate, which is public by construction.
// The private key is named - so an operator can see where it lives and check
// its permissions - and never read: there is no path from this function to the
// contents of the key file, and there must not be one, because everything this
// function returns ends up in a browser.
func panelTLSView(cfg *config.PanelAccessConfig) PanelTLSView {
	view := PanelTLSView{
		Enabled:       cfg.TLS.Enabled,
		SelfSigned:    cfg.TLS.SelfSigned,
		CertFile:      cfg.TLS.CertFile,
		KeyFile:       cfg.TLS.KeyFile,
		Hosts:         []string{},
		Warnings:      []string{},
		Source:        cfg.CertSource,
		Inconsistency: cfg.TLSInconsistency(),
		ACME: PanelACMEView{
			Email:        cfg.TLS.ACME.Email,
			UseStaging:   cfg.TLS.ACME.UseStaging,
			Profile:      cfg.ACMEProfileFor(panelACMEIdentifierType(cfg)),
			LastIssuedAt: cfg.TLS.ACME.LastIssuedAt,
			LastError:    cfg.TLS.ACME.LastError,
		},
	}

	if !cfg.TLS.Enabled {
		view.Mode = "off"
		return view
	}

	view.Mode = cfg.TLSMode()
	view.Managed = cfg.TLSMode() != config.TLSModeCustom
	view.KeyPresent = panelFileExists(cfg.TLS.KeyFile)

	if view.Source == "" {
		view.Source = cfg.TLSMode()
	}

	view.Fingerprint = config.CertFingerprint(cfg.TLS.CertFile)

	inspection, err := config.InspectCertificateFile(cfg.TLS.CertFile)
	if err != nil || inspection == nil {
		if err != nil && !os.IsNotExist(err) {
			view.Warnings = append(view.Warnings,
				"The certificate file could not be read: "+err.Error())
		}
		return view
	}

	view.Present = true
	view.Subject = inspection.Subject
	view.Issuer = inspection.Issuer
	view.SerialNumber = inspection.SerialNumber
	view.Fingerprint = inspection.Fingerprint
	view.ChainLength = inspection.ChainLength
	view.ChainComplete = inspection.ChainComplete
	view.Expired = inspection.Expired
	view.NotYetValid = inspection.NotYetValid
	view.ExpiringSoon = inspection.ExpiringSoon
	view.Warnings = append(view.Warnings, inspection.Warnings...)

	notBefore, notAfter := inspection.NotBefore, inspection.NotAfter
	view.NotBefore = &notBefore
	view.NotAfter = &notAfter
	days := inspection.ExpiresInDays
	view.ExpiresInDays = &days

	hosts := make([]string, 0, len(inspection.DNSNames)+len(inspection.IPAddresses))
	hosts = append(hosts, inspection.DNSNames...)
	hosts = append(hosts, inspection.IPAddresses...)
	view.Hosts = hosts

	// A certificate that does not name the address operators actually type is
	// worth saying out loud here and not only when one is being installed: a
	// pinned domain added later can invalidate a certificate that was correct
	// when it was saved.
	if host := cfg.AccessHost(); host != "" && !inspection.CoversHost(host) {
		view.Warnings = append(view.Warnings, fmt.Sprintf(
			"This certificate does not cover %s, which is the address the panel is reached at.", host))
	}

	return view
}

// panelACMEIdentifierType is the identifier kind an order would carry, which is
// what decides the default profile. An identifier that cannot be derived leaves
// it empty, and the profile falls back to the DNS default.
func panelACMEIdentifierType(cfg *config.PanelAccessConfig) string {
	id, err := cfg.ACMEIdentifier()
	if err != nil {
		return ""
	}
	return id.Type
}

func panelFileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func panelCloneConfig(cfg *config.PanelAccessConfig) *config.PanelAccessConfig {
	clone := *cfg
	clone.AllowedIPs = panelCopyList(cfg.AllowedIPs)
	clone.TrustedProxies = panelCopyList(cfg.TrustedProxies)
	clone.Generated = panelCopyList(cfg.Generated)
	clone.EnvOverrides = panelCopyList(cfg.EnvOverrides)
	return &clone
}

func panelCopyList(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// panelNormalizeList trims, drops blanks and removes duplicates while keeping
// the order the operator typed.
func panelNormalizeList(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		value := strings.TrimSpace(raw)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func panelJoinList(in []string) string {
	return strings.Join(in, ", ")
}

func panelDisplayIP(ip string) string {
	if strings.TrimSpace(ip) == "" {
		return "(unknown)"
	}
	return ip
}

func panelDisplayHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "(unknown host)"
	}
	return host
}

// panelCleanReloadError strips the package prefix off a reload failure so the
// operator reads the cause and not the call stack it came from.
func panelCleanReloadError(err error) string {
	message := strings.TrimSpace(strings.TrimPrefix(err.Error(), "reload:"))
	if message == "" {
		return "The change could not be applied."
	}
	return message
}

// panelCleanConfigError turns a config package error into something an operator
// can act on. The config package prefixes its messages with "panel access: ".
func panelCleanConfigError(err error) string {
	message := strings.TrimSpace(strings.TrimPrefix(err.Error(), "panel access:"))
	if message == "" {
		return "The configuration was rejected."
	}
	return "The configuration was rejected: " + message
}
