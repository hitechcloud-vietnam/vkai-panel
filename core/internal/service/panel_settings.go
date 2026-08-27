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
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/hitechcloud-vietnam/vkai-panel/internal/models"
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

// PanelTLSView describes the certificate the panel is serving.
type PanelTLSView struct {
	Enabled       bool       `json:"enabled"`
	SelfSigned    bool       `json:"self_signed"`
	Mode          string     `json:"mode"`
	CertFile      string     `json:"cert_file"`
	KeyFile       string     `json:"key_file"`
	Fingerprint   string     `json:"fingerprint"`
	Subject       string     `json:"subject"`
	Hosts         []string   `json:"hosts"`
	NotBefore     *time.Time `json:"not_before"`
	NotAfter      *time.Time `json:"not_after"`
	ExpiresInDays *int       `json:"expires_in_days"`
	Expired       bool       `json:"expired"`
	Present       bool       `json:"present"`
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

	RestartPending bool     `json:"restart_pending"`
	RestartReasons []string `json:"restart_reasons"`
	RunningPort    int      `json:"running_port"`
	RunningBind    string   `json:"running_bind"`

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

	// Confirm acknowledges a change that moves or restricts the panel's own
	// entrance. Without it such a change is refused, never applied.
	Confirm bool `json:"confirm"`
}

// PanelSettingsResult is what a successful mutation returns.
type PanelSettingsResult struct {
	Settings       *PanelSettingsView   `json:"settings"`
	Changes        []PanelSettingChange `json:"changes"`
	AccessURL      string               `json:"access_url"`
	RestartPending bool                 `json:"restart_pending"`
	RestartReasons []string             `json:"restart_reasons"`
	Warnings       []string             `json:"warnings"`
}

// PanelSettingsValidationError is a rejected field with a message written for
// the operator, not for a log file.
type PanelSettingsValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
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

// PanelSettingsService owns the panel's own access configuration at runtime.
type PanelSettingsService struct {
	logger *zap.Logger
	audit  *AuditService

	mu  sync.Mutex
	cfg *config.PanelAccessConfig

	// boot* is what this process is actually serving on. Comparing the stored
	// configuration against it is what "restart pending" means: the flag clears
	// itself on restart because the snapshot is taken at start-up.
	bootPort         int
	bootBind         string
	bootPublicPort   int
	bootPublicScheme string
	bootTLSEnabled   bool
	bootTLSCertFile  string
	bootTLSKeyFile   string

	// stickyRestart holds reasons that no field comparison can detect, such as
	// a certificate reissued on disk after the listener already read the old
	// one. It is in-memory only, so a restart clears it.
	stickyRestart map[string]bool

	// reload, when set, hands the new configuration to whoever owns the live
	// request guard so the entrance, allow list, host pinning and session TTL
	// take effect without a restart. The listener socket is not reloadable, so
	// the port is reported as restart-pending regardless.
	reload func(*config.PanelAccessConfig) error
}

// NewPanelSettingsService takes the configuration this process started with.
// Passing the same pointer the server was built from is intentional: the
// snapshot of "what is running" has to be taken before anything is edited.
func NewPanelSettingsService(cfg *config.PanelAccessConfig, audit *AuditService, logger *zap.Logger) *PanelSettingsService {
	if cfg == nil {
		cfg = config.DefaultPanelAccess()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	running := panelCloneConfig(cfg)

	return &PanelSettingsService{
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
}

// SetReloader registers the hot-reload hook for the live access guard. It is
// optional: without it the settings are still persisted and validated, they
// simply reach the running guard on the next restart.
func (s *PanelSettingsService) SetReloader(fn func(*config.PanelAccessConfig) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reload = fn
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

	changes := panelDiff(s.cfg, next)
	if len(changes) == 0 {
		return &PanelSettingsResult{
			Settings:       s.viewLocked(caller),
			Changes:        []PanelSettingChange{},
			AccessURL:      s.cfg.AccessURL(),
			RestartPending: s.restartPendingLocked(),
			RestartReasons: s.restartReasonsLocked(),
			Warnings:       []string{},
		}, nil
	}

	if reasons := panelLockoutReasons(s.cfg, next, caller); len(reasons) > 0 && !req.Confirm {
		return nil, &PanelSettingsConfirmationError{
			Reasons: reasons,
			NewURL:  next.AccessURL(),
			Changes: changes,
		}
	}

	return s.commitLocked(ctx, caller, next, changes, "panel.settings.update")
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

	return s.commitLocked(ctx, caller, next, changes, "panel.settings.entrance.regenerate")
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
	s.stickyRestart["tls_certificate"] = true

	changes := []PanelSettingChange{{
		Field: "tls.fingerprint",
		Old:   oldFingerprint,
		New:   newFingerprint,
	}}

	return s.commitLocked(ctx, caller, next, changes, "panel.settings.tls.reissue")
}

// commitLocked persists, hands the new configuration to the live guard when a
// reloader is registered, records the audit entry and builds the response. The
// caller must hold s.mu.
func (s *PanelSettingsService) commitLocked(
	ctx context.Context,
	caller PanelSettingsCaller,
	next *config.PanelAccessConfig,
	changes []PanelSettingChange,
	action string,
) (*PanelSettingsResult, error) {
	next.StateFile = s.cfg.StateFile
	if err := next.Save(); err != nil {
		return nil, err
	}

	// A pinned domain or a new bind address changes which hosts the self-signed
	// certificate has to cover. EnsureTLSMaterial reissues it when it no longer
	// matches; failing to do so must not roll back a saved setting, so it is
	// reported as a warning instead.
	warnings := make([]string, 0, 2)
	if next.TLS.Enabled {
		if _, _, err := next.EnsureTLSMaterial(); err != nil {
			warnings = append(warnings, "Settings saved, but the panel TLS material could not be prepared: "+err.Error())
		}
	}

	s.cfg = next

	if s.reload != nil {
		if err := s.reload(panelCloneConfig(next)); err != nil {
			warnings = append(warnings, "Settings saved, but the running access gate did not reload: "+err.Error()+" Restart the panel service to apply them.")
			s.stickyRestart["access_gate"] = true
		}
	}

	s.auditLocked(ctx, caller, action, changes)

	view := s.viewLocked(caller)
	if view.RestartPending {
		warnings = append(warnings, "A panel service restart is required before the new listener settings take effect.")
	}

	return &PanelSettingsResult{
		Settings:       view,
		Changes:        changes,
		AccessURL:      view.AccessURL,
		RestartPending: view.RestartPending,
		RestartReasons: view.RestartReasons,
		Warnings:       warnings,
	}, nil
}

// auditLocked records who changed what. It never fails the request: losing the
// audit entry is bad, refusing a change that already reached disk is worse.
func (s *PanelSettingsService) auditLocked(ctx context.Context, caller PanelSettingsCaller, action string, changes []PanelSettingChange) {
	if s.audit == nil {
		return
	}

	redacted := make([]map[string]string, 0, len(changes))
	for _, change := range changes {
		redacted = append(redacted, map[string]string{
			"field": change.Field,
			"old":   panelRedactValue(change.Field, change.Old),
			"new":   panelRedactValue(change.Field, change.New),
		})
	}

	details := models.JSONMap{
		"changes":      redacted,
		"access_url":   panelRedactURL(s.cfg),
		"client_ip":    caller.ClientIP,
		"state_file":   s.cfg.StateFile,
		"change_count": len(changes),
	}

	var userID *uuid.UUID
	if caller.UserID != uuid.Nil {
		id := caller.UserID
		userID = &id
	}

	if err := s.audit.Log(ctx, caller.TenantID, userID, action, PanelSettingsAuditResource, nil,
		details, caller.ClientIP, caller.UserAgent, "success"); err != nil {
		s.logger.Error("panel settings: audit log write failed",
			zap.String("action", action),
			zap.Error(err),
		)
	}

	// The audit trail is the record of record, but an operator reading the
	// service log during an incident should not have to open the database.
	fields := make([]zap.Field, 0, len(changes)+2)
	fields = append(fields,
		zap.String("action", action),
		zap.String("client_ip", caller.ClientIP),
	)
	for _, change := range changes {
		fields = append(fields, zap.String(change.Field,
			panelRedactValue(change.Field, change.Old)+" -> "+panelRedactValue(change.Field, change.New)))
	}
	s.logger.Info("panel access settings changed", fields...)
}

// viewLocked renders the response body. The caller must hold s.mu.
func (s *PanelSettingsService) viewLocked(caller PanelSettingsCaller) *PanelSettingsView {
	cfg := s.cfg
	reasons := s.restartReasonsLocked()

	return &PanelSettingsView{
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
		EnvOverrides:      panelCopyList(cfg.EnvOverrides),
		StateFile:         cfg.StateFile,
		UpdatedAt:         cfg.UpdatedAt,
		ClientIP:          caller.ClientIP,
	}
}

func (s *PanelSettingsService) restartPendingLocked() bool {
	return len(s.restartReasonsLocked()) > 0
}

// restartReasonsLocked lists the settings that are saved but not yet live. Only
// the ones the listener socket was built from can be in here: everything else
// is either reloadable or has no effect on how the panel is reached.
func (s *PanelSettingsService) restartReasonsLocked() []string {
	reasons := make([]string, 0, 4)
	cfg := s.cfg

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

	sticky := make([]string, 0, len(s.stickyRestart))
	for reason := range s.stickyRestart {
		switch reason {
		case "tls_certificate":
			sticky = append(sticky, "TLS certificate (a new certificate is on disk; the listener is still serving the old one)")
		case "access_gate":
			sticky = append(sticky, "access gate (the running guard could not be reloaded)")
		default:
			sticky = append(sticky, reason)
		}
	}
	sort.Strings(sticky)

	return append(reasons, sticky...)
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
	}
	if req.TLSCertFile != nil {
		cfg.TLS.CertFile = strings.TrimSpace(*req.TLSCertFile)
	}
	if req.TLSKeyFile != nil {
		cfg.TLS.KeyFile = strings.TrimSpace(*req.TLSKeyFile)
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

	if cfg.TLS.Enabled && !cfg.TLS.SelfSigned {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			return &PanelSettingsValidationError{
				Field:   "tls",
				Message: "A custom certificate needs both a certificate file and a key file. Switch to a self-signed certificate to have the panel generate its own.",
			}
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
	add("tls.self_signed", strconv.FormatBool(current.TLS.SelfSigned), strconv.FormatBool(next.TLS.SelfSigned))
	add("tls.cert_file", current.TLS.CertFile, next.TLS.CertFile)
	add("tls.key_file", current.TLS.KeyFile, next.TLS.KeyFile)

	return changes
}

// panelRedactValue removes the security entrance from anything that is written
// down. Every other field is safe to record verbatim.
func panelRedactValue(field, value string) string {
	if value == "" {
		return ""
	}
	if field == "entrance" {
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
func panelTLSView(cfg *config.PanelAccessConfig) PanelTLSView {
	view := PanelTLSView{
		Enabled:    cfg.TLS.Enabled,
		SelfSigned: cfg.TLS.SelfSigned,
		CertFile:   cfg.TLS.CertFile,
		KeyFile:    cfg.TLS.KeyFile,
		Hosts:      []string{},
	}

	switch {
	case !cfg.TLS.Enabled:
		view.Mode = "off"
	case cfg.TLS.SelfSigned:
		view.Mode = "self_signed"
	default:
		view.Mode = "custom"
	}

	if !cfg.TLS.Enabled {
		return view
	}

	view.Fingerprint = config.CertFingerprint(cfg.TLS.CertFile)

	cert, err := panelReadCertificate(cfg.TLS.CertFile)
	if err != nil || cert == nil {
		return view
	}

	view.Present = true
	view.Subject = cert.Subject.CommonName
	notBefore := cert.NotBefore
	notAfter := cert.NotAfter
	view.NotBefore = &notBefore
	view.NotAfter = &notAfter
	view.Expired = time.Now().After(notAfter)

	days := int(time.Until(notAfter).Hours() / 24)
	view.ExpiresInDays = &days

	hosts := make([]string, 0, len(cert.DNSNames)+len(cert.IPAddresses))
	hosts = append(hosts, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		hosts = append(hosts, ip.String())
	}
	view.Hosts = hosts

	return view
}

// panelReadCertificate parses the leaf certificate off disk. A missing or
// unreadable file is not an error worth failing a request over: the view simply
// reports that no certificate is present.
func panelReadCertificate(path string) (*x509.Certificate, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("panel settings: %s does not contain a PEM certificate", path)
	}
	return x509.ParseCertificate(block.Bytes)
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

// panelCleanConfigError turns a config package error into something an operator
// can act on. The config package prefixes its messages with "panel access: ".
func panelCleanConfigError(err error) string {
	message := strings.TrimSpace(strings.TrimPrefix(err.Error(), "panel access:"))
	if message == "" {
		return "The configuration was rejected."
	}
	return "The configuration was rejected: " + message
}
