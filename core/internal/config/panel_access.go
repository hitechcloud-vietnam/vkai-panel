package config

// Panel access gate (aaPanel style).
//
// The panel must never share port 80/443 with the customer websites this
// server hosts: those ports belong to the vhosts, and anything reachable there
// is reachable by every scanner on the internet. The panel therefore listens on
// its own port (8888 by default, configurable, optionally randomised on first
// boot) behind a "security entrance" - a secret path prefix - plus optional IP
// allow-listing, host pinning and its own TLS certificate.
//
// Every value here can come from three places, in increasing priority:
//
//	built-in default  <  state file (/vkai-panel/etc/panel_access.json)  <  environment
//
// Whatever is still missing on the first boot is generated, written back to the
// state file and printed once, so the operator can copy the URL out of the
// console exactly like aaPanel does.

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultPanelPort is the aaPanel-compatible default. It is deliberately
	// not 80/443 and not the legacy API port.
	DefaultPanelPort = 8888

	// DefaultPanelBind listens on every interface. Set VKAI_PANEL_BIND to
	// 127.0.0.1 when a reverse proxy on the same host fronts the panel.
	DefaultPanelBind = "0.0.0.0"

	// DefaultPanelSessionTTL is how long the entrance cookie stays valid.
	DefaultPanelSessionTTL = 12 * time.Hour

	// PanelEntranceCookie is the cookie that records "this browser came in
	// through the correct entrance".
	PanelEntranceCookie = "vkai_entrance"

	// PanelRandomPortMin / PanelRandomPortMax bound the randomised port.
	PanelRandomPortMin = 8000
	PanelRandomPortMax = 65535
)

// DefaultPanelStateFile is where a generated port/entrance is persisted so the
// next restart keeps the same access URL. It follows VKAI_PANEL_ROOT, so a
// relocated installation keeps its state next to the rest of its configuration.
func DefaultPanelStateFile() string { return PanelStateFile() }

// entrancePattern is intentionally strict: the entrance ends up in a URL, in
// nginx configuration and in shell snippets in the documentation.
var entrancePattern = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9_.\-/]{2,63}$`)

// acmeProfilePattern matches an ACME profile name. The list of profiles is the
// CA's to change, so the name is not validated against a fixed set - only
// against the shape, which keeps a typo out of the order payload.
var acmeProfilePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// reservedPanelPorts are ports the panel refuses to take over. 80/443 belong to
// the hosted websites; the rest are the panel's own supporting services.
var reservedPanelPorts = map[int]string{
	80:   "HTTP cua website khach",
	443:  "HTTPS cua website khach",
	22:   "SSH",
	25:   "SMTP",
	3306: "MySQL",
	5432: "PostgreSQL",
	6379: "Redis",
}

// Panel TLS modes. The mode decides where the certificate on the panel port
// comes from; nothing else about the listener changes with it.
//
//	self-signed  generated here, carries the machine IP in its SAN list, warns
//	             in browsers, works before any DNS record or CA exists
//	letsencrypt  issued by an ACME CA over HTTP-01 and renewed in the
//	             background, for an installation that is reachable from the
//	             internet on port 80
//	custom       a file pair supplied by the operator, never rewritten by us
const (
	TLSModeSelfSigned  = "self-signed"
	TLSModeLetsEncrypt = "letsencrypt"
	TLSModeCustom      = "custom"
)

// ACME identifier types, spelled the way ACME itself spells them.
const (
	ACMEIdentifierDNS = "dns"
	ACMEIdentifierIP  = "ip"
)

// Certificate profiles published in the Let's Encrypt directory's
// meta.profiles. A certificate for a bare IP address is only issued under
// "shortlived", which lasts about six days - so nothing downstream may assume
// the ninety day lifetime of a classic certificate.
const (
	ACMEProfileShortLived = "shortlived"
	ACMEProfileTLSServer  = "tlsserver"
	ACMEProfileClassic    = "classic"
)

// PanelACMEConfig holds the settings used by the "letsencrypt" TLS mode.
type PanelACMEConfig struct {
	// Email is the ACME account contact. Issuance works without it, but it is
	// the only channel a CA has to warn that a certificate stopped renewing,
	// and a panel whose certificate silently expires locks its operator out of
	// the thing they would use to fix it.
	Email string `json:"email"`

	// UseStaging points issuance at the CA's staging environment, and defaults
	// to true. Production rate limits are per-account and per-identifier, and a
	// misconfigured install can burn a week of them in a few minutes of retry
	// loops; the operator opts into production once a staging run has proven
	// that the challenge is actually answerable from the internet.
	UseStaging bool `json:"use_staging"`

	// Profile is the ACME certificate profile. Empty means "derive it from the
	// identifier", which is the only correct default: an IP identifier exists
	// under the short-lived profile and nowhere else.
	Profile string `json:"profile"`

	// LastIssuedAt and LastError are display state, written after every
	// issuance attempt so the UI and the CLI can explain why the panel is
	// serving a self-signed certificate instead of the requested one.
	LastIssuedAt time.Time `json:"last_issued_at,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
}

// PanelTLSConfig describes the certificate the panel serves on its own port.
type PanelTLSConfig struct {
	Enabled  bool   `json:"enabled"`
	Mode     string `json:"mode"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`

	// SelfSigned predates Mode. It is still read (so a state file written by an
	// older build selects the right mode) and still written (so a rollback to
	// an older build keeps working), but it is kept in sync with Mode rather
	// than consulted directly: read EffectiveMode, not this field.
	SelfSigned bool `json:"self_signed"`

	ACME PanelACMEConfig `json:"acme"`
}

// EffectiveMode resolves the mode of a possibly zero-valued or legacy struct.
// An empty Mode is what every state file written before modes existed contains,
// so it falls back to what SelfSigned said.
func (t PanelTLSConfig) EffectiveMode() string {
	if mode := normalizeTLSMode(t.Mode); mode != "" {
		return mode
	}
	if t.SelfSigned {
		return TLSModeSelfSigned
	}
	return TLSModeCustom
}

// normalizeTLSMode maps the spellings operators actually type onto the three
// canonical modes, and returns "" for anything else.
func normalizeTLSMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "self-signed", "selfsigned", "self_signed", "self":
		return TLSModeSelfSigned
	case "letsencrypt", "lets-encrypt", "lets_encrypt", "acme", "le":
		return TLSModeLetsEncrypt
	case "custom", "file", "manual", "byo":
		return TLSModeCustom
	}
	return ""
}

// ParsePanelTLSMode validates operator input for VKAI_PANEL_TLS_MODE.
func ParsePanelTLSMode(raw string) (string, error) {
	if mode := normalizeTLSMode(raw); mode != "" {
		return mode, nil
	}
	return "", fmt.Errorf("panel access: VKAI_PANEL_TLS_MODE=%q is not one of %s, %s, %s",
		raw, TLSModeSelfSigned, TLSModeLetsEncrypt, TLSModeCustom)
}

// PanelIdentifier is the single ACME identifier the panel certificate covers.
type PanelIdentifier struct {
	Type  string
	Value string
}

// String renders the identifier the way ACME logs it, e.g. "ip:116.118.2.44".
func (i PanelIdentifier) String() string {
	if i.Type == "" && i.Value == "" {
		return ""
	}
	return i.Type + ":" + i.Value
}

// PanelAccessConfig is the complete access gate configuration.
type PanelAccessConfig struct {
	Enabled bool   `json:"enabled"`
	Bind    string `json:"bind"`
	Port    int    `json:"port"`

	// PublicPort is the port operators actually type, when a reverse proxy
	// publishes the panel instead of the API binding it directly. It changes
	// the printed URL and the firewall advice, never what the process binds.
	// Zero means "the panel is reached on Port".
	PublicPort   int    `json:"public_port"`
	PublicScheme string `json:"public_scheme"`

	Entrance          string         `json:"entrance"`
	EntranceEnabled   bool           `json:"entrance_enabled"`
	SessionTTLSeconds int            `json:"session_ttl_seconds"`
	AllowedIPs        []string       `json:"allowed_ips"`
	TrustedProxies    []string       `json:"trusted_proxies"`
	Domain            string         `json:"domain"`
	TLS               PanelTLSConfig `json:"tls"`
	UpdatedAt         time.Time      `json:"updated_at"`

	// StateFile is where Save() writes. Not persisted inside the file itself.
	StateFile string `json:"-"`

	// Generated lists the settings this process had to invent because nothing
	// supplied them. It drives the "write this down now" banner.
	Generated []string `json:"-"`

	// EnvOverrides lists settings whose value came from the environment. Those
	// cannot be changed by the CLI: the environment wins on the next start.
	EnvOverrides []string `json:"-"`

	// CertSource describes what actually produced the certificate currently on
	// the wire ("self-signed", "letsencrypt production", "custom file", ...).
	// The TLS manager sets it at startup, because the configured mode and the
	// served certificate can legitimately disagree: a CA that was unreachable
	// leaves the panel on the self-signed fallback, and the banner has to say
	// so rather than repeat what was requested.
	CertSource string `json:"-"`
}

// DefaultPanelAccess returns the configuration used when neither a state file
// nor an environment variable says otherwise.
func DefaultPanelAccess() *PanelAccessConfig {
	return &PanelAccessConfig{
		Enabled:           true,
		Bind:              DefaultPanelBind,
		Port:              DefaultPanelPort,
		Entrance:          "",
		EntranceEnabled:   true,
		SessionTTLSeconds: int(DefaultPanelSessionTTL / time.Second),
		AllowedIPs:        []string{},
		TrustedProxies:    []string{},
		StateFile:         PanelStateFilePath(),
		// HTTPS is on by default, self-signed. A panel that logs an operator in
		// as root over plain HTTP leaks the session cookie to anyone on the path,
		// and waiting for a domain to exist before enabling TLS means the most
		// exposed period - first install, default credentials - is the unencrypted
		// one. The generated certificate carries the machine's IP in its SAN list,
		// so it works before any DNS record exists. Browsers will warn: that is
		// what the fingerprint printed in the startup banner is for.
		// Mode is deliberately left empty here rather than set to
		// TLSModeSelfSigned: a state file written before modes existed carries
		// self_signed without a mode, and a non-empty default would win over it
		// and silently re-enable self-signed for an operator who had configured
		// their own certificate. EffectiveMode derives "self-signed" from the
		// SelfSigned flag below, and fillGenerated writes the resolved mode
		// back so that a loaded config always carries a concrete one.
		TLS: PanelTLSConfig{
			Enabled:    true,
			SelfSigned: true,
			ACME: PanelACMEConfig{
				// Staging until an operator opts into production. See the field
				// comment: the cost of guessing wrong here is a week-long
				// rate-limit lockout, and the cost of guessing wrong the other
				// way is one extra environment variable.
				UseStaging: true,
			},
		},
	}
}

// PanelStateFilePath resolves where the generated access settings live.
func PanelStateFilePath() string {
	if v := envString("PANEL_CONFIG_FILE", "PANEL_STATE_FILE"); v != "" {
		return v
	}
	return DefaultPanelStateFile()
}

// PanelAccessSource says where one load reads from. The zero value is the
// production case: the process environment and the default state file.
//
// It exists because the configuration is loaded more than once now. The first
// load builds the process; every later one re-derives the same configuration
// after the state file or the environment file changed on disk, and that later
// load must be able to read an environment that is not this process's own -
// editing /vkai-panel/etc/.env does not change the variables a running process
// already inherited.
type PanelAccessSource struct {
	// Env resolves one variable. Nil means the process environment.
	Env EnvLookup

	// StateFile overrides where the persisted settings are read from and
	// written back to. Empty means the location PanelStateFilePath resolves.
	StateFile string

	// NoPersist stops a generated value from being written back. A reload that
	// is only inspecting a candidate configuration must not have side effects
	// on disk.
	NoPersist bool
}

// LoadPanelAccess builds the effective configuration from defaults, the state
// file and the environment, generating whatever is still missing.
//
// A generated value is persisted when the state file is writable. It is not an
// error when it is not (containers frequently run with a read-only /etc): the
// panel still starts, it just prints the values again on the next boot.
func LoadPanelAccess() (*PanelAccessConfig, error) {
	return LoadPanelAccessFrom(PanelAccessSource{})
}

// LoadPanelAccessFrom is LoadPanelAccess against an explicit source.
func LoadPanelAccessFrom(src PanelAccessSource) (*PanelAccessConfig, error) {
	env := envSource{lookup: src.Env}
	cfg := DefaultPanelAccess()
	if path := strings.TrimSpace(src.StateFile); path != "" {
		cfg.StateFile = path
	} else if path := env.get("PANEL_CONFIG_FILE", "PANEL_STATE_FILE"); path != "" {
		cfg.StateFile = path
	}

	if err := cfg.loadStateFile(); err != nil {
		return nil, err
	}
	if err := cfg.applyEnv(env); err != nil {
		return nil, err
	}
	if err := cfg.fillGenerated(env); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if len(cfg.Generated) > 0 && !src.NoPersist {
		// Best effort: a read-only state file must not stop the panel booting.
		_ = cfg.Save()
	}

	return cfg, nil
}

// loadStateFile overlays the persisted settings. A missing file is normal on a
// first run; a corrupt one is not, and is reported rather than silently reset.
func (p *PanelAccessConfig) loadStateFile() error {
	path := p.StateFile
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		if os.IsPermission(err) {
			// Running unprivileged: fall back to defaults + environment.
			return nil
		}
		return fmt.Errorf("panel access: khong doc duoc %s: %w", path, err)
	}

	stored := *p
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("panel access: %s khong phai JSON hop le: %w", path, err)
	}
	stored.StateFile = path
	stored.Generated = nil
	stored.EnvOverrides = nil
	*p = stored

	return nil
}

// applyEnv overlays the environment. Both the VKAI_ prefixed name and the bare
// name are accepted, so PANEL_PORT works as documented by aaPanel-style guides
// while VKAI_PANEL_PORT stays consistent with the rest of this panel.
func (p *PanelAccessConfig) applyEnv(env envSource) error {
	if v, ok := env.boolOK("PANEL_ENABLED"); ok {
		p.Enabled = v
		p.markEnv("enabled")
	}
	if v := env.get("PANEL_BIND", "PANEL_HOST"); v != "" {
		p.Bind = v
		p.markEnv("bind")
	}
	if v := env.get("PANEL_PORT"); v != "" {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("panel access: PANEL_PORT=%q khong phai so", v)
		}
		p.Port = port
		p.markEnv("port")
	}
	if v := env.get("PANEL_PUBLIC_PORT"); v != "" {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("panel access: PANEL_PUBLIC_PORT=%q khong phai so", v)
		}
		p.PublicPort = port
		p.markEnv("public_port")
	}
	if v := env.get("PANEL_PUBLIC_SCHEME"); v != "" {
		scheme := strings.ToLower(strings.TrimSpace(v))
		if scheme != "http" && scheme != "https" {
			return fmt.Errorf("panel access: PANEL_PUBLIC_SCHEME=%q chi nhan http hoac https", v)
		}
		p.PublicScheme = scheme
		p.markEnv("public_scheme")
	}
	if v := env.get("PANEL_ENTRANCE"); v != "" {
		if strings.EqualFold(strings.TrimSpace(v), "random") {
			entrance, err := RandomEntrance()
			if err != nil {
				return err
			}
			p.Entrance = entrance
		} else {
			p.Entrance = NormalizeEntrance(v)
		}
		p.markEnv("entrance")
	}
	if v, ok := env.boolOK("PANEL_ENTRANCE_ENABLED"); ok {
		p.EntranceEnabled = v
		p.markEnv("entrance_enabled")
	}
	if v := env.get("PANEL_SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("panel access: VKAI_PANEL_SESSION_TTL=%q khong hop le (vi du 12h)", v)
		}
		if d <= 0 {
			return fmt.Errorf("panel access: VKAI_PANEL_SESSION_TTL phai lon hon 0")
		}
		p.SessionTTLSeconds = int(d / time.Second)
		p.markEnv("session_ttl")
	}
	if v, ok := env.listOK("PANEL_ALLOWED_IPS", "PANEL_ALLOW_IPS"); ok {
		p.AllowedIPs = v
		p.markEnv("allowed_ips")
	}
	if v, ok := env.listOK("PANEL_TRUSTED_PROXIES"); ok {
		p.TrustedProxies = v
		p.markEnv("trusted_proxies")
	}
	if v := env.get("PANEL_DOMAIN"); v != "" {
		p.Domain = strings.ToLower(strings.TrimSpace(v))
		p.markEnv("domain")
	}
	// VKAI_PANEL_TLS_MODE is read here but applied last: it is the explicit
	// statement of intent, so none of the older single-purpose variables below
	// may end up contradicting it.
	modeEnv := env.get("PANEL_TLS_MODE", "PANEL_SSL_MODE")

	if v := env.get("PANEL_TLS_CERT", "PANEL_TLS_CERT_FILE"); v != "" {
		p.TLS.CertFile = v
		p.TLS.Enabled = true
		p.adoptCustomModeFromFiles(modeEnv)
		p.markEnv("tls_cert")
	}
	if v := env.get("PANEL_TLS_KEY", "PANEL_TLS_KEY_FILE"); v != "" {
		p.TLS.KeyFile = v
		p.TLS.Enabled = true
		p.adoptCustomModeFromFiles(modeEnv)
		p.markEnv("tls_key")
	}
	if v, ok := env.boolOK("PANEL_TLS_SELF_SIGNED"); ok {
		p.TLS.SelfSigned = v
		if modeEnv == "" {
			// The legacy flag still moves the mode, so an existing deployment
			// that sets it keeps behaving exactly as it did.
			if v {
				p.TLS.Mode = TLSModeSelfSigned
			} else if p.TLSMode() == TLSModeSelfSigned {
				p.TLS.Mode = TLSModeCustom
			}
		}
		if v {
			p.TLS.Enabled = true
		}
		p.markEnv("tls_self_signed")
	}
	if v, ok := env.boolOK("PANEL_TLS_ENABLED", "PANEL_SSL"); ok {
		p.TLS.Enabled = v
		p.markEnv("tls_enabled")
	}

	if modeEnv != "" {
		mode, err := ParsePanelTLSMode(modeEnv)
		if err != nil {
			return err
		}
		p.TLS.Mode = mode
		p.TLS.SelfSigned = mode == TLSModeSelfSigned
		// Naming a mode means asking for TLS, unless TLS was switched off in
		// the same environment - in which case that wins, it is the more
		// specific instruction.
		if !p.hasEnv("tls_enabled") {
			p.TLS.Enabled = true
		}
		p.markEnv("tls_mode")
	}

	if v := env.get("PANEL_ACME_EMAIL", "PANEL_LETSENCRYPT_EMAIL", "PANEL_LE_EMAIL"); v != "" {
		p.TLS.ACME.Email = strings.TrimSpace(v)
		p.markEnv("acme_email")
	}
	if v, ok := env.boolOK("PANEL_ACME_STAGING", "PANEL_LETSENCRYPT_STAGING"); ok {
		p.TLS.ACME.UseStaging = v
		p.markEnv("acme_staging")
	}
	if v := env.get("PANEL_ACME_PROFILE", "PANEL_LETSENCRYPT_PROFILE"); v != "" {
		p.TLS.ACME.Profile = strings.ToLower(strings.TrimSpace(v))
		p.markEnv("acme_profile")
	}

	return nil
}

// adoptCustomModeFromFiles switches an untouched default (self-signed) over to
// "custom" when the operator points at a certificate file pair. Without it the
// self-signed generator would overwrite the file they just configured.
func (p *PanelAccessConfig) adoptCustomModeFromFiles(modeEnv string) {
	if modeEnv != "" {
		return
	}
	if p.TLSMode() == TLSModeSelfSigned {
		p.TLS.Mode = TLSModeCustom
		p.TLS.SelfSigned = false
	}
}

// fillGenerated invents the values nobody supplied: a random port when
// PANEL_RANDOM_PORT asks for one, and an entrance whenever the security
// entrance is on but no path has ever been chosen.
func (p *PanelAccessConfig) fillGenerated(env envSource) error {
	randomPort, _ := env.boolOK("PANEL_RANDOM_PORT")
	if randomPort && !p.hasEnv("port") {
		port, err := RandomPanelPort()
		if err != nil {
			return err
		}
		p.Port = port
		p.Generated = append(p.Generated, "port")
	}

	if p.Port == 0 {
		p.Port = DefaultPanelPort
	}

	if p.EntranceEnabled && strings.TrimSpace(p.Entrance) == "" {
		entrance, err := RandomEntrance()
		if err != nil {
			return err
		}
		p.Entrance = entrance
		p.Generated = append(p.Generated, "entrance")
	}

	// Resolve the mode once, here, so every later reader sees a concrete value
	// and the state file we write back is unambiguous for the next build.
	p.TLS.Mode = p.TLS.EffectiveMode()
	p.TLS.SelfSigned = p.TLS.Mode == TLSModeSelfSigned

	// Both locally generated and CA-issued certificates land on the same pair
	// of paths: whatever produced it, this is the certificate the panel serves,
	// and keeping one location means a mode change never leaves a stale file
	// behind that a later mode change would pick back up.
	if p.TLS.Enabled && p.TLS.Mode != TLSModeCustom {
		if strings.TrimSpace(p.TLS.CertFile) == "" {
			p.TLS.CertFile = filepath.Join(PanelSSLDir(), "panel.crt")
		}
		if strings.TrimSpace(p.TLS.KeyFile) == "" {
			p.TLS.KeyFile = filepath.Join(PanelSSLDir(), "panel.key")
		}
	}

	if p.SessionTTLSeconds <= 0 {
		p.SessionTTLSeconds = int(DefaultPanelSessionTTL / time.Second)
	}
	if strings.TrimSpace(p.Bind) == "" {
		p.Bind = DefaultPanelBind
	}

	return nil
}

// Validate rejects a configuration that would either fail to bind or quietly
// give up the isolation the panel port exists to provide.
// TLSInconsistency reports a configuration that asks for a certificate and
// switches TLS off in the same breath.
//
// This combination shipped on a real install: VKAI_PANEL_TLS_MODE=letsencrypt
// together with VKAI_PANEL_TLS_ENABLED=false. The resolution rule preferred the
// explicit "off", which is defensible in isolation, and the consequence was
// silence: the ACME client, its renewal loop and its hot certificate reload were
// all constructed and then never asked to do anything. The panel served a
// self-signed certificate generated once at install time that nothing would ever
// renew, and no log line said so.
//
// Returning the mismatch as a value lets the caller decide - the API logs it at
// startup and the settings endpoint shows it to an operator - without failing a
// panel that is already running this way.
func (p *PanelAccessConfig) TLSInconsistency() string {
	if p.TLS.Enabled {
		return ""
	}
	switch p.TLSMode() {
	case TLSModeLetsEncrypt:
		return "TLS mode is 'letsencrypt' but TLS is disabled for the panel: " +
			"no certificate will be requested and none will be renewed. " +
			"Set VKAI_PANEL_TLS_ENABLED=true so the panel terminates TLS itself, " +
			"or set VKAI_PANEL_TLS_MODE=none if something in front of it does."
	case TLSModeSelfSigned:
		return "TLS mode is 'self-signed' but TLS is disabled for the panel: " +
			"the generated certificate is never served and never renewed."
	}
	return ""
}

func (p *PanelAccessConfig) Validate() error {
	if !p.Enabled {
		return nil
	}

	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("panel access: cong %d nam ngoai khoang 1-65535", p.Port)
	}
	if name, reserved := reservedPanelPorts[p.Port]; reserved {
		return fmt.Errorf("panel access: cong %d da danh cho %s, panel phai dung cong rieng (mac dinh %d)",
			p.Port, name, DefaultPanelPort)
	}

	if p.PublicPort != 0 {
		if p.PublicPort < 1 || p.PublicPort > 65535 {
			return fmt.Errorf("panel access: cong cong khai %d nam ngoai khoang 1-65535", p.PublicPort)
		}
		if name, reserved := reservedPanelPorts[p.PublicPort]; reserved {
			return fmt.Errorf("panel access: cong cong khai %d da danh cho %s, panel phai dung cong rieng",
				p.PublicPort, name)
		}
	}

	if ip := strings.TrimSpace(p.Bind); ip != "" && ip != "0.0.0.0" && ip != "::" {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("panel access: VKAI_PANEL_BIND=%q khong phai dia chi IP hop le", p.Bind)
		}
	}

	if p.EntranceEnabled {
		entrance := NormalizeEntrance(p.Entrance)
		if err := ValidateEntrance(entrance); err != nil {
			return err
		}
		p.Entrance = entrance
	}

	for _, cidr := range p.AllowedIPs {
		if _, err := ParseIPMatcher(cidr); err != nil {
			return fmt.Errorf("panel access: VKAI_PANEL_ALLOWED_IPS chua gia tri khong hop le %q", cidr)
		}
	}
	for _, cidr := range p.TrustedProxies {
		if _, err := ParseIPMatcher(cidr); err != nil {
			return fmt.Errorf("panel access: VKAI_PANEL_TRUSTED_PROXIES chua gia tri khong hop le %q", cidr)
		}
	}

	if d := strings.TrimSpace(p.Domain); d != "" {
		if strings.ContainsAny(d, "/ :") {
			return fmt.Errorf("panel access: VKAI_PANEL_DOMAIN=%q chi duoc chua ten mien, khong co scheme hay cong", p.Domain)
		}
	}

	if p.TLS.Enabled {
		if err := p.validateTLS(); err != nil {
			return err
		}
	}

	return nil
}

// validateTLS rejects a TLS configuration that could not possibly produce a
// certificate, before the listener is built rather than after.
func (p *PanelAccessConfig) validateTLS() error {
	if normalizeTLSMode(p.TLS.Mode) == "" && strings.TrimSpace(p.TLS.Mode) != "" {
		return fmt.Errorf("panel access: TLS mode %q is not one of %s, %s, %s",
			p.TLS.Mode, TLSModeSelfSigned, TLSModeLetsEncrypt, TLSModeCustom)
	}

	if p.TLSMode() == TLSModeCustom {
		if strings.TrimSpace(p.TLS.CertFile) == "" || strings.TrimSpace(p.TLS.KeyFile) == "" {
			return fmt.Errorf("panel access: bat TLS thi phai dat ca VKAI_PANEL_TLS_CERT va VKAI_PANEL_TLS_KEY (hoac VKAI_PANEL_TLS_SELF_SIGNED=true)")
		}
	}

	if email := strings.TrimSpace(p.TLS.ACME.Email); email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			return fmt.Errorf("panel access: VKAI_PANEL_ACME_EMAIL=%q is not a valid address", p.TLS.ACME.Email)
		}
	}

	if profile := strings.TrimSpace(p.TLS.ACME.Profile); profile != "" {
		if !acmeProfilePattern.MatchString(profile) {
			return fmt.Errorf("panel access: VKAI_PANEL_ACME_PROFILE=%q is not a profile name (letters, digits and '-' only)", p.TLS.ACME.Profile)
		}
	}

	return nil
}

// Save writes the configuration back to the state file with 0600 permissions:
// the entrance path is a secret, and a world-readable copy of it defeats the
// point of having one.
func (p *PanelAccessConfig) Save() error {
	path := p.StateFile
	if path == "" {
		path = PanelStateFilePath()
		p.StateFile = path
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("panel access: khong tao duoc thu muc %s: %w", filepath.Dir(path), err)
	}

	p.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// Write through a temporary file so a crash cannot leave a half written
	// state file that would lock the operator out on the next boot.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("panel access: khong ghi duoc %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("panel access: khong thay duoc %s: %w", path, err)
	}

	return nil
}

// SessionTTL is the entrance cookie lifetime.
func (p *PanelAccessConfig) SessionTTL() time.Duration {
	if p.SessionTTLSeconds <= 0 {
		return DefaultPanelSessionTTL
	}
	return time.Duration(p.SessionTTLSeconds) * time.Second
}

// ListenAddr is the address the panel HTTP server binds.
func (p *PanelAccessConfig) ListenAddr() string {
	return net.JoinHostPort(p.Bind, strconv.Itoa(p.Port))
}

// TLSMode is the resolved certificate mode: self-signed, letsencrypt or custom.
func (p *PanelAccessConfig) TLSMode() string { return p.TLS.EffectiveMode() }

// UsesACME reports whether the panel certificate is expected to come from a CA.
func (p *PanelAccessConfig) UsesACME() bool {
	return p.TLS.Enabled && p.TLSMode() == TLSModeLetsEncrypt
}

// ACMEIdentifier is the single identifier the panel certificate is requested
// for: the pinned domain when there is one, otherwise this machine's own IPv4.
//
// It returns an error rather than a best guess whenever the address cannot be
// validated from the internet. Ordering an unvalidatable identifier does not
// merely fail - it consumes the "failed validation" rate limit, which is per
// account and per hour, so a retry loop over a private address locks the
// installation out of issuing anything at all.
func (p *PanelAccessConfig) ACMEIdentifier() (PanelIdentifier, error) {
	if d := strings.ToLower(strings.TrimSpace(p.Domain)); d != "" {
		return PanelIdentifier{Type: ACMEIdentifierDNS, Value: d}, nil
	}

	raw := primaryIPv4()
	if raw == "" {
		return PanelIdentifier{}, fmt.Errorf(
			"panel tls: this host has no non-loopback IPv4 address and VKAI_PANEL_DOMAIN is unset, so there is no identifier to request a certificate for")
	}

	ip := net.ParseIP(raw)
	if reason := unroutableIPv4Reason(ip); reason != "" {
		return PanelIdentifier{}, fmt.Errorf(
			"panel tls: %s is %s, which a public CA cannot reach or validate; set VKAI_PANEL_DOMAIN to a public name that resolves here, or keep VKAI_PANEL_TLS_MODE=%s",
			raw, reason, TLSModeSelfSigned)
	}

	return PanelIdentifier{Type: ACMEIdentifierIP, Value: ip.To4().String()}, nil
}

// ACMEProfileFor is the profile an order should carry: whatever the operator
// pinned, otherwise the only profile that can serve this identifier type.
func (p *PanelAccessConfig) ACMEProfileFor(identifierType string) string {
	if v := strings.TrimSpace(p.TLS.ACME.Profile); v != "" {
		return strings.ToLower(v)
	}
	return DefaultACMEProfile(identifierType)
}

// DefaultACMEProfile maps an identifier type onto a Let's Encrypt profile.
// Certificates for a bare IP address are only issued under "shortlived"; a DNS
// identifier gets the ordinary TLS server profile.
func DefaultACMEProfile(identifierType string) string {
	if strings.EqualFold(strings.TrimSpace(identifierType), ACMEIdentifierIP) {
		return ACMEProfileShortLived
	}
	return ACMEProfileTLSServer
}

// RecordACMEResult stores the outcome of an issuance attempt for display. It
// only mutates the in-memory config; persisting it is the caller's decision,
// because a state file write failing must never turn into a failed renewal.
func (p *PanelAccessConfig) RecordACMEResult(at time.Time, err error) {
	if err != nil {
		p.TLS.ACME.LastError = err.Error()
		return
	}
	p.TLS.ACME.LastIssuedAt = at.UTC()
	p.TLS.ACME.LastError = ""
}

// unroutableIPv4Reason names why an address cannot be used as an ACME
// identifier, or returns "" when it is a public unicast IPv4 address.
//
// The blocks below are exactly the ones a hosting panel actually meets: a NAT
// lab (RFC1918), a container bridge (RFC1918), a machine behind a carrier NAT
// (RFC6598 100.64/10, common on cheap VPS and on mobile uplinks), a cloud
// metadata or auto-configured interface (169.254/16) and localhost (127/8).
func unroutableIPv4Reason(ip net.IP) string {
	if ip == nil {
		return "not a valid IP address"
	}
	v4 := ip.To4()
	if v4 == nil {
		// Only IPv4 is handled here: the panel advertises an A record, and an
		// AAAA-only install needs a domain anyway.
		return "not an IPv4 address"
	}

	switch {
	case v4[0] == 0:
		return "in the unspecified block 0.0.0.0/8"
	case v4.IsLoopback():
		return "a loopback address (127.0.0.0/8)"
	case v4.IsLinkLocalUnicast() || v4.IsLinkLocalMulticast():
		return "a link-local address (169.254.0.0/16)"
	case v4[0] == 10,
		v4[0] == 172 && v4[1]&0xf0 == 16,
		v4[0] == 192 && v4[1] == 168:
		return "a private address (RFC1918)"
	case v4[0] == 100 && v4[1]&0xc0 == 64:
		return "inside the carrier-grade NAT block (RFC6598 100.64.0.0/10)"
	case v4[0] == 192 && v4[1] == 0 && v4[2] == 2,
		v4[0] == 198 && v4[1] == 51 && v4[2] == 100,
		v4[0] == 203 && v4[1] == 0 && v4[2] == 113:
		return "a documentation address (RFC5737)"
	case v4[0] == 198 && v4[1]&0xfe == 18:
		return "a benchmarking address (RFC2544)"
	case v4.IsMulticast():
		return "a multicast address (224.0.0.0/4)"
	case v4[0] >= 240:
		return "in the reserved block 240.0.0.0/4"
	}

	return ""
}

// IsPubliclyRoutableIPv4 reports whether an address can be used as an ACME "ip"
// identifier at all.
func IsPubliclyRoutableIPv4(ip net.IP) bool { return unroutableIPv4Reason(ip) == "" }

// Scheme is https as soon as the panel serves its own certificate.
func (p *PanelAccessConfig) Scheme() string {
	if p.TLS.Enabled {
		return "https"
	}
	return "http"
}

// EffectivePort is the port an operator connects to: the reverse proxy's port
// when there is one, otherwise the port this process binds.
func (p *PanelAccessConfig) EffectivePort() int {
	if p.PublicPort > 0 {
		return p.PublicPort
	}
	return p.Port
}

// EffectiveScheme is what the browser sees, which differs from Scheme() when a
// reverse proxy terminates TLS in front of a plain HTTP panel.
func (p *PanelAccessConfig) EffectiveScheme() string {
	if s := strings.TrimSpace(p.PublicScheme); s != "" {
		return s
	}
	return p.Scheme()
}

// IsProxied reports whether the panel is published by something else.
func (p *PanelAccessConfig) IsProxied() bool {
	return p.PublicPort > 0 && p.PublicPort != p.Port
}

// AccessHost is the host an operator types in the browser: the pinned domain
// when there is one, the bind address when it is a concrete interface, and
// otherwise a best guess at the primary address of this machine.
func (p *PanelAccessConfig) AccessHost() string {
	if d := strings.TrimSpace(p.Domain); d != "" {
		return d
	}
	bind := strings.TrimSpace(p.Bind)
	if bind != "" && bind != "0.0.0.0" && bind != "::" {
		if ip := net.ParseIP(bind); ip != nil && ip.To4() == nil {
			return "[" + bind + "]"
		}
		return bind
	}
	if ip := primaryIPv4(); ip != "" {
		return ip
	}
	return "IP-MAY-CHU"
}

// AccessURL is the full URL including the security entrance.
func (p *PanelAccessConfig) AccessURL() string {
	scheme, port := p.EffectiveScheme(), p.EffectivePort()
	// Omit the port when it is the scheme default: "https://panel.example.com:443/"
	// is valid but reads as a misconfiguration to an operator pasting it around.
	url := fmt.Sprintf("%s://%s", scheme, p.AccessHost())
	if !((scheme == "http" && port == 80) || (scheme == "https" && port == 443)) {
		url = fmt.Sprintf("%s:%d", url, port)
	}
	if p.EntranceEnabled && p.Entrance != "" {
		url += p.Entrance
	}
	return url + "/"
}

// Banner renders the aaPanel-style access table printed once at startup.
func (p *PanelAccessConfig) Banner() string {
	entrance := p.Entrance
	if !p.EntranceEnabled {
		entrance = "(tat - khong khuyen nghi)"
	}

	allowed := "tat ca IP"
	if len(p.AllowedIPs) > 0 {
		allowed = strings.Join(p.AllowedIPs, ", ")
	}

	domain := p.Domain
	if domain == "" {
		domain = "(khong rang buoc)"
	}

	// The banner reports the certificate the panel is actually serving, not the
	// one that was requested: an operator who asked for Let's Encrypt and got
	// the self-signed fallback because port 80 was busy needs to see that here,
	// not discover it from a browser warning an hour later.
	tls := "off (HTTP)"
	fingerprint := ""
	if p.TLS.Enabled {
		source := strings.TrimSpace(p.CertSource)
		if source == "" {
			source = p.TLSMode()
			if source == TLSModeLetsEncrypt && p.TLS.ACME.UseStaging {
				source += " (staging)"
			}
		}
		tls = "on (HTTPS) - " + source
		if strings.Contains(source, TLSModeSelfSigned) {
			// Only a self-signed certificate needs the fingerprint: a CA-issued
			// one is verified by the browser without the operator doing
			// anything, and printing a fingerprint they never check trains them
			// to click through the one that matters.
			fingerprint = CertFingerprint(p.TLS.CertFile)
		}
		if p.TLSMode() == TLSModeLetsEncrypt && p.TLS.ACME.LastError != "" {
			tls += " [last ACME error: " + p.TLS.ACME.LastError + "]"
		}
	}

	rows := [][2]string{
		{"URL truy cap", p.AccessURL()},
		{"Cong panel", strconv.Itoa(p.EffectivePort())},
	}
	if p.IsProxied() {
		rows = append(rows, [2]string{"Cong noi bo", fmt.Sprintf("%d (sau reverse proxy)", p.Port)})
	}
	rows = append(rows, [][2]string{
		{"Dia chi bind", p.Bind},
		{"Loi vao an toan", entrance},
		{"Gioi han IP", allowed},
		{"Ten mien", domain},
		{"TLS", tls},
		{"Goc cai dat", PanelRoot()},
		{"Goc website", WebRoot()},
		{"Sao luu", BackupRoot()},
		{"Nhat ky", LogRoot()},
		{"File cau hinh", p.StateFile},
	}...)
	if fingerprint != "" {
		// Printed so the operator can compare it with what the browser shows.
		// Clicking through a certificate warning without checking anything is
		// how an interception goes unnoticed.
		rows = append(rows, [2]string{"Van tay chung chi", fingerprint})
	}

	var b strings.Builder
	const width = 78
	line := strings.Repeat("=", width)

	b.WriteString("\n" + line + "\n")
	b.WriteString("  VKAI PANEL - HiTechCloud (hitechcloud.vn)\n")
	b.WriteString("  THONG TIN TRUY CAP (khong dung cong 80/443)\n")
	b.WriteString(line + "\n")
	for _, row := range rows {
		b.WriteString(fmt.Sprintf("  %-18s : %s\n", row[0], row[1]))
	}
	b.WriteString(line + "\n")
	b.WriteString("  MO TUONG LUA cho cong panel truoc khi dong console:\n")
	b.WriteString(fmt.Sprintf("    ufw allow %d/tcp        # Ubuntu / Debian\n", p.EffectivePort()))
	b.WriteString(fmt.Sprintf("    firewall-cmd --permanent --add-port=%d/tcp && firewall-cmd --reload   # RHEL\n", p.EffectivePort()))
	b.WriteString("  Cong 80/443 danh RIENG cho website khach - khong dung de vao panel.\n")
	if p.EntranceEnabled {
		b.WriteString("  Truy cap sai duong dan se tra ve 404 trung tinh. Hay luu lai URL o tren.\n")
	}
	if len(p.Generated) > 0 {
		b.WriteString(fmt.Sprintf("  Gia tri vua duoc sinh tu dong: %s (da luu vao file cau hinh).\n",
			strings.Join(p.Generated, ", ")))
	}
	b.WriteString(line + "\n")

	return b.String()
}

// NormalizeEntrance turns user input into the canonical form: exactly one
// leading slash, no trailing slash, no empty segments.
func NormalizeEntrance(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "/")
	if s == "" {
		return ""
	}
	// Collapse repeated separators so "/a//b" and "/a/b" cannot both exist.
	parts := make([]string, 0, 4)
	for _, seg := range strings.Split(s, "/") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return "/" + strings.Join(parts, "/")
}

// ValidateEntrance rejects paths that would be unusable, ambiguous or unsafe.
func ValidateEntrance(entrance string) error {
	if entrance == "" {
		return fmt.Errorf("panel access: loi vao an toan dang bat nhung duong dan rong")
	}
	if !entrancePattern.MatchString(entrance) {
		return fmt.Errorf("panel access: loi vao %q khong hop le (chi dung chu, so, - _ . / va dai 4-64 ky tu)", entrance)
	}
	if strings.Contains(entrance, "..") {
		return fmt.Errorf("panel access: loi vao %q khong duoc chua '..'", entrance)
	}
	// Reserved prefixes: an entrance that shadows these would make the API and
	// the health probes unreachable.
	for _, reserved := range []string{"/api", "/health", "/ready", "/live", "/ws", "/_next", "/static"} {
		if entrance == reserved || strings.HasPrefix(entrance+"/", reserved+"/") {
			return fmt.Errorf("panel access: loi vao %q trung voi duong dan he thong %q", entrance, reserved)
		}
	}
	return nil
}

// RandomEntrance produces an aaPanel-style secret path, e.g. /vkai_a1b2c3d4e5f60718.
//
// aaPanel ships 8 hex characters (32 bits). That is small enough that a patient
// scanner sweeping one host can walk the whole space, and the entrance is the
// only thing standing between the internet and a login form running as root.
// 8 random bytes (64 bits) costs nothing to type once and removes the class of
// attack entirely.
func RandomEntrance() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("panel access: khong sinh duoc loi vao ngau nhien: %w", err)
	}
	return "/vkai_" + hex.EncodeToString(buf), nil
}

// RandomPanelPort picks a free-looking port in the configured range, skipping
// the ports the panel must never take over.
func RandomPanelPort() (int, error) {
	span := int64(PanelRandomPortMax - PanelRandomPortMin + 1)
	for attempt := 0; attempt < 64; attempt++ {
		n, err := rand.Int(rand.Reader, big.NewInt(span))
		if err != nil {
			return 0, fmt.Errorf("panel access: khong sinh duoc cong ngau nhien: %w", err)
		}
		port := PanelRandomPortMin + int(n.Int64())
		if _, reserved := reservedPanelPorts[port]; reserved {
			continue
		}
		if port == 3000 || port == 30110 || port == 30111 {
			continue
		}
		return port, nil
	}
	return DefaultPanelPort, nil
}

// ParseIPMatcher accepts either a bare IP or a CIDR block and returns the
// network it matches.
func ParseIPMatcher(value string) (*net.IPNet, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil, fmt.Errorf("gia tri rong")
	}
	if strings.Contains(v, "/") {
		_, network, err := net.ParseCIDR(v)
		if err != nil {
			return nil, err
		}
		return network, nil
	}
	ip := net.ParseIP(v)
	if ip == nil {
		return nil, fmt.Errorf("%q khong phai IP hay CIDR", value)
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
}

// EnsureTLSMaterial makes sure the certificate and key exist, generating a
// self-signed pair when the operator asked for one. It returns the paths the
// HTTP server should use.
func (p *PanelAccessConfig) EnsureTLSMaterial() (certFile, keyFile string, err error) {
	if !p.TLS.Enabled {
		return "", "", nil
	}

	certFile = strings.TrimSpace(p.TLS.CertFile)
	keyFile = strings.TrimSpace(p.TLS.KeyFile)
	if certFile == "" || keyFile == "" {
		return "", "", fmt.Errorf("panel access: thieu duong dan chung chi TLS cho panel")
	}

	mode := p.TLSMode()

	if fileExists(certFile) && fileExists(keyFile) {
		if mode != TLSModeSelfSigned {
			// A custom pair belongs to the operator and a CA-issued pair
			// belongs to the TLS manager's renewal loop. Neither is ever
			// regenerated from here.
			return certFile, keyFile, nil
		}
		// A self-signed certificate pins the hosts it was issued for. When the
		// machine changes address or a domain is pinned later, the old file stops
		// matching what operators now type and every visit turns into a warning
		// they are trained to click through. Reissue instead.
		if certCovers(certFile, p.certHosts()) {
			return certFile, keyFile, nil
		}
		_ = os.Remove(certFile)
		_ = os.Remove(keyFile)
	}

	if mode == TLSModeCustom {
		return "", "", fmt.Errorf("panel access: khong tim thay chung chi %s hoac khoa %s", certFile, keyFile)
	}

	// The letsencrypt mode bootstraps from a self-signed pair too. The panel has
	// to answer HTTPS on its very first request, long before an ACME order can
	// finish, and a listener that waits for a CA is a listener that never comes
	// up on the day the CA is unreachable.
	if err := generateSelfSignedCert(certFile, keyFile, p.certHosts()); err != nil {
		return "", "", err
	}
	p.Generated = append(p.Generated, "tls_self_signed")

	return certFile, keyFile, nil
}

// certHosts is the SAN list of the generated certificate.
func (p *PanelAccessConfig) certHosts() []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if d := strings.TrimSpace(p.Domain); d != "" {
		hosts = append(hosts, d)
	}
	if bind := strings.TrimSpace(p.Bind); bind != "" && bind != "0.0.0.0" && bind != "::" {
		hosts = append(hosts, bind)
	}
	if ip := primaryIPv4(); ip != "" {
		hosts = append(hosts, ip)
	}
	return hosts
}

// certCovers reports whether the certificate on disk still lists every host the
// panel is reachable as.
func certCovers(certFile string, hosts []string) bool {
	raw, err := os.ReadFile(certFile)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	if time.Now().After(cert.NotAfter) {
		return false
	}
	for _, h := range hosts {
		if cert.VerifyHostname(h) != nil {
			return false
		}
	}
	return true
}

// CertFingerprint returns the SHA-256 fingerprint an operator can compare
// against what the browser shows before accepting a self-signed certificate.
// Without this, "click through the warning" is the only option on offer, which
// is indistinguishable from accepting an interception.
func CertFingerprint(certFile string) string {
	raw, err := os.ReadFile(certFile)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return ""
	}
	sum := sha256.Sum256(block.Bytes)
	parts := make([]string, 0, len(sum))
	for _, b := range sum {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return "SHA256:" + strings.Join(parts, ":")
}

func generateSelfSignedCert(certFile, keyFile string, hosts []string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("panel access: khong sinh duoc khoa RSA: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return fmt.Errorf("panel access: khong sinh duoc serial chung chi: %w", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "VKAI Panel",
			Organization: []string{"HiTechCloud"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, h)
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("panel access: khong tao duoc chung chi tu ky: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(certFile), 0o750); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o750); err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return fmt.Errorf("panel access: khong ghi duoc %s: %w", certFile, err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	// The private key is never group or world readable.
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return fmt.Errorf("panel access: khong ghi duoc %s: %w", keyFile, err)
	}

	return nil
}

func (p *PanelAccessConfig) markEnv(name string) {
	for _, existing := range p.EnvOverrides {
		if existing == name {
			return
		}
	}
	p.EnvOverrides = append(p.EnvOverrides, name)
}

func (p *PanelAccessConfig) hasEnv(name string) bool {
	for _, existing := range p.EnvOverrides {
		if existing == name {
			return true
		}
	}
	return false
}

// IsEnvOverridden reports whether a setting is pinned by the environment, in
// which case editing the state file has no effect on the next start.
func (p *PanelAccessConfig) IsEnvOverridden(name string) bool {
	return p.hasEnv(name)
}

// EnvLookup resolves one environment variable by name, the way os.LookupEnv
// does. It is a parameter rather than a call to os so that a configuration can
// be re-derived from an environment file that this process never inherited:
// editing /vkai-panel/etc/.env does not change os.Environ of a running panel,
// and a reload that read os.Environ would report "nothing changed" for a file
// the operator just edited.
type EnvLookup func(name string) (string, bool)

// OSEnvLookup is the process environment.
func OSEnvLookup(name string) (string, bool) { return os.LookupEnv(name) }

// EnvFileLookup layers the contents of an environment file over a fallback,
// which is what a reload reads: the file is the operator's intent, and anything
// it does not mention keeps the value this process started with.
//
// A key that was in the file at boot and has since been deleted from it is NOT
// unset by this - the process still carries it - which is why the reload path
// reports such a key as needing a restart instead of pretending it applied.
func EnvFileLookup(vars map[string]string, fallback EnvLookup) EnvLookup {
	if fallback == nil {
		fallback = OSEnvLookup
	}
	return func(name string) (string, bool) {
		if v, ok := vars[name]; ok {
			return v, true
		}
		return fallback(name)
	}
}

// envSource reads variables through an EnvLookup, accepting both the VKAI_
// prefixed name and the bare one.
type envSource struct{ lookup EnvLookup }

func (e envSource) raw(name string) (string, bool) {
	if e.lookup == nil {
		return os.LookupEnv(name)
	}
	return e.lookup(name)
}

// get returns the first non-empty value among VKAI_<name> and <name>.
func (e envSource) get(names ...string) string {
	for _, name := range names {
		if v, ok := e.raw("VKAI_" + name); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
		if v, ok := e.raw(name); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

func (e envSource) boolOK(names ...string) (bool, bool) {
	raw := e.get(names...)
	if raw == "" {
		return false, false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on", "enabled":
		return true, true
	case "0", "false", "no", "off", "disabled":
		return false, true
	}
	return false, false
}

// listOK reports whether the variable was set at all, so that an explicit
// empty value ("allow everyone") can be distinguished from "not configured".
func (e envSource) listOK(names ...string) ([]string, bool) {
	for _, name := range names {
		for _, key := range []string{"VKAI_" + name, name} {
			raw, ok := e.raw(key)
			if !ok {
				continue
			}
			return splitList(raw), true
		}
	}
	return nil, false
}

// envString returns the first non-empty value among VKAI_<name> and <name> in
// the process environment.
func envString(names ...string) string { return envSource{}.get(names...) }

// ParseEnvFile reads a shell-style environment file - the format systemd's
// EnvironmentFile= and the installer both write - into a map.
//
// It is deliberately forgiving about what it skips (comments, blank lines,
// lines with no '=') and strict about what it accepts: a line it cannot make
// sense of is skipped rather than turning the whole file into an error, because
// the alternative is a panel that refuses to reload its configuration over a
// stray comment. A missing file is not an error either; it is the normal state
// of an installation that configures everything through the state file.
func ParseEnvFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("panel access: cannot read the environment file %s: %w", path, err)
	}

	vars := make(map[string]string, 32)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		// Strip one layer of matching quotes, which is what a shell would do.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		vars[key] = value
	}

	return vars, nil
}

func splitList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// primaryIPv4 reports the first non-loopback IPv4 address of this host. It is
// only used to build a human readable URL, never for an access decision.
func primaryIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		network, ok := addr.(*net.IPNet)
		if !ok || network.IP.IsLoopback() {
			continue
		}
		if v4 := network.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Operator-supplied certificates
// ---------------------------------------------------------------------------
//
// An operator pastes a certificate and a private key into the panel. What
// arrives is text, and every way it can be wrong ends the same way: the panel
// stops answering HTTPS and the person who could fix it is on the other side of
// the connection that just broke. So the pair is proven here, before anything
// is written, and each failure names the check that rejected it.

// Certificate check names. They travel to the UI in the error payload so the
// interface can point at the field the operator has to fix.
const (
	CertCheckCertPEM      = "certificate_pem"
	CertCheckKeyPEM       = "private_key_pem"
	CertCheckKeyEncrypted = "private_key_encrypted"
	CertCheckKeyMatch     = "key_matches_certificate"
	CertCheckValidity     = "validity_period"
	CertCheckChain        = "chain_complete"
	CertCheckHostnames    = "hostname_coverage"
	CertCheckLoad         = "tls_load"
)

// CertExpiryWarningWindow is how far ahead an expiry is worth warning about.
// Thirty days is long enough to obtain a replacement by any means, including
// one that involves a human at a certificate authority.
const CertExpiryWarningWindow = 30 * 24 * time.Hour

// CertificateError is a rejected certificate or key, with the check that
// rejected it. Check is a stable identifier; Message is written for a person.
type CertificateError struct {
	Check   string
	Message string
	// Overridable marks a rejection an operator is allowed to insist past -
	// hostname coverage and chain completeness, both of which can be
	// deliberately unusual. A pair that does not parse or whose key does not
	// match is never overridable: it cannot serve a single handshake.
	Overridable bool
}

func (e *CertificateError) Error() string { return e.Check + ": " + e.Message }

// CertificateInspection is everything the panel can say about a certificate
// pair without holding the key. It is what GET /panel/settings returns for the
// TLS section, and it deliberately has no field that could carry key material.
type CertificateInspection struct {
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	SerialNumber string    `json:"serial_number"`
	Fingerprint  string    `json:"fingerprint"`
	NotBefore    time.Time `json:"not_before"`
	NotAfter     time.Time `json:"not_after"`

	ExpiresInDays int  `json:"expires_in_days"`
	Expired       bool `json:"expired"`
	NotYetValid   bool `json:"not_yet_valid"`
	ExpiringSoon  bool `json:"expiring_soon"`

	DNSNames    []string `json:"dns_names"`
	IPAddresses []string `json:"ip_addresses"`

	SelfSigned    bool `json:"self_signed"`
	ChainLength   int  `json:"chain_length"`
	ChainComplete bool `json:"chain_complete"`

	KeyType string `json:"key_type"`
	KeyBits int    `json:"key_bits"`

	// Warnings are conditions that do not stop the pair being served.
	Warnings []string `json:"warnings"`
}

// CoversHost reports whether the certificate is valid for a host name or IP
// address the panel is actually reached as.
func (i *CertificateInspection) CoversHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return true
	}
	host = strings.TrimSuffix(host, ".")
	if ip := net.ParseIP(host); ip != nil {
		for _, candidate := range i.IPAddresses {
			if parsed := net.ParseIP(candidate); parsed != nil && parsed.Equal(ip) {
				return true
			}
		}
		// A certificate may also carry a bare address as a DNS name. It is not
		// how a browser matches, so it does not count here.
		return false
	}
	for _, name := range i.DNSNames {
		if matchDNSName(name, host) {
			return true
		}
	}
	return false
}

// matchDNSName compares one SAN entry against a host, honouring the single
// leading wildcard label that X.509 allows and nothing else.
func matchDNSName(pattern, host string) bool {
	pattern = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pattern), "."))
	host = strings.ToLower(host)
	if pattern == host {
		return true
	}
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	suffix := pattern[1:] // ".example.com"
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	label := host[:len(host)-len(suffix)]
	return label != "" && !strings.Contains(label, ".")
}

// InspectCertificatePair validates a pasted certificate and private key and
// reports what they are. Every return of a *CertificateError names the check
// that failed; the inspection is returned alongside whenever enough of the
// certificate parsed to describe it, so the UI can show the operator what they
// pasted next to the reason it was refused.
//
// The order of the checks is the order in which a failure makes the later ones
// meaningless, so the first message an operator sees is the cause and not a
// consequence.
func InspectCertificatePair(certPEM, keyPEM []byte) (*CertificateInspection, error) {
	chain, err := decodeCertificateChain(certPEM)
	if err != nil {
		return nil, err
	}
	leaf := chain[0]

	key, keyType, keyBits, err := decodePrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}

	inspection := describeCertificate(chain)
	inspection.KeyType = keyType
	inspection.KeyBits = keyBits

	// The key must belong to this certificate. A mismatched pair loads from
	// disk and then fails every single handshake, which from outside is
	// indistinguishable from the panel being down.
	if !publicKeyMatches(leaf.PublicKey, key) {
		return inspection, &CertificateError{
			Check:   CertCheckKeyMatch,
			Message: "The private key does not belong to this certificate. Paste the key that was generated with this certificate's signing request.",
		}
	}

	// Proven the way the listener will prove it, so nothing can pass here and
	// fail in crypto/tls afterwards.
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return inspection, &CertificateError{
			Check:   CertCheckLoad,
			Message: "The certificate and key parse individually but cannot be loaded together: " + err.Error(),
		}
	}

	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return inspection, &CertificateError{
			Check: CertCheckValidity,
			Message: fmt.Sprintf("This certificate is not valid until %s. Check the clock on this machine, or wait until it becomes valid.",
				leaf.NotBefore.UTC().Format(time.RFC3339)),
		}
	}
	if now.After(leaf.NotAfter) {
		return inspection, &CertificateError{
			Check: CertCheckValidity,
			Message: fmt.Sprintf("This certificate expired on %s. Browsers refuse an expired certificate, so serving it would take the panel off the network.",
				leaf.NotAfter.UTC().Format(time.RFC3339)),
		}
	}

	return inspection, nil
}

// decodeCertificateChain parses every CERTIFICATE block, leaf first.
func decodeCertificateChain(certPEM []byte) ([]*x509.Certificate, error) {
	rest := certPEM
	chain := make([]*x509.Certificate, 0, 3)

	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, &CertificateError{
				Check: CertCheckCertPEM,
				Message: fmt.Sprintf("The certificate box contains a %q block. Paste the certificate itself, which begins with -----BEGIN CERTIFICATE-----.",
					block.Type),
			}
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, &CertificateError{
				Check:   CertCheckCertPEM,
				Message: "A certificate block is not a valid X.509 certificate: " + err.Error(),
			}
		}
		chain = append(chain, cert)
	}

	if len(chain) == 0 {
		return nil, &CertificateError{
			Check:   CertCheckCertPEM,
			Message: "No certificate was found. Paste the PEM text, including the -----BEGIN CERTIFICATE----- and -----END CERTIFICATE----- lines.",
		}
	}

	return chain, nil
}

// decodePrivateKey parses an RSA, EC or PKCS#8 private key and names what it
// found, so the UI can say "RSA 2048" rather than "a key".
func decodePrivateKey(keyPEM []byte) (any, string, int, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, "", 0, &CertificateError{
			Check:   CertCheckKeyPEM,
			Message: "No private key was found. Paste the PEM text, including the -----BEGIN ... PRIVATE KEY----- and -----END ... PRIVATE KEY----- lines.",
		}
	}

	// An encrypted key would parse as garbage and produce an unreadable error.
	// Both the legacy header form and the PKCS#8 block type are named here
	// because "decrypt it first" is the only useful thing to say about either.
	if _, encrypted := block.Headers["DEK-Info"]; encrypted || block.Type == "ENCRYPTED PRIVATE KEY" {
		return nil, "", 0, &CertificateError{
			Check:   CertCheckKeyEncrypted,
			Message: "This private key is passphrase-protected. The panel has nowhere to hold a passphrase across a restart, so decrypt the key first (openssl rsa -in key.pem -out key-decrypted.pem) and paste the decrypted key.",
		}
	}

	switch block.Type {
	case "RSA PRIVATE KEY", "EC PRIVATE KEY", "PRIVATE KEY":
	default:
		return nil, "", 0, &CertificateError{
			Check: CertCheckKeyPEM,
			Message: fmt.Sprintf("The private key box contains a %q block, which is not a private key. A certificate belongs in the certificate box.",
				block.Type),
		}
	}

	var (
		key any
		err error
	)
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, "", 0, &CertificateError{
			Check:   CertCheckKeyPEM,
			Message: "The private key could not be parsed: " + err.Error(),
		}
	}

	switch typed := key.(type) {
	case *rsa.PrivateKey:
		return key, "RSA", typed.N.BitLen(), nil
	case *ecdsa.PrivateKey:
		return key, "ECDSA " + typed.Curve.Params().Name, typed.Curve.Params().BitSize, nil
	case ed25519.PrivateKey:
		return key, "Ed25519", 256, nil
	default:
		return nil, "", 0, &CertificateError{
			Check:   CertCheckKeyPEM,
			Message: "The private key is of a type this panel cannot serve TLS with. Use an RSA or ECDSA key.",
		}
	}
}

// publicKeyMatches reports whether a private key belongs to a certificate.
func publicKeyMatches(pub any, priv any) bool {
	type equaler interface{ Equal(x crypto.PublicKey) bool }

	var candidate crypto.PublicKey
	switch typed := priv.(type) {
	case *rsa.PrivateKey:
		candidate = &typed.PublicKey
	case *ecdsa.PrivateKey:
		candidate = &typed.PublicKey
	case ed25519.PrivateKey:
		candidate = typed.Public()
	default:
		return false
	}

	if cmp, ok := pub.(equaler); ok {
		return cmp.Equal(candidate)
	}
	return false
}

// describeCertificate builds the inspection from a parsed chain, including the
// chain completeness verdict.
func describeCertificate(chain []*x509.Certificate) *CertificateInspection {
	leaf := chain[0]

	inspection := &CertificateInspection{
		Subject:      leaf.Subject.String(),
		Issuer:       leaf.Issuer.String(),
		SerialNumber: leaf.SerialNumber.String(),
		Fingerprint:  fingerprintDER(leaf.Raw),
		NotBefore:    leaf.NotBefore,
		NotAfter:     leaf.NotAfter,
		DNSNames:     append([]string{}, leaf.DNSNames...),
		IPAddresses:  []string{},
		SelfSigned:   isSelfIssued(leaf),
		ChainLength:  len(chain),
		Warnings:     []string{},
	}
	for _, ip := range leaf.IPAddresses {
		inspection.IPAddresses = append(inspection.IPAddresses, ip.String())
	}

	now := time.Now()
	inspection.Expired = now.After(leaf.NotAfter)
	inspection.NotYetValid = now.Before(leaf.NotBefore)
	inspection.ExpiresInDays = int(leaf.NotAfter.Sub(now).Hours() / 24)
	inspection.ExpiringSoon = !inspection.Expired && leaf.NotAfter.Sub(now) < CertExpiryWarningWindow

	if inspection.ExpiringSoon {
		inspection.Warnings = append(inspection.Warnings, fmt.Sprintf(
			"This certificate expires in %d days, on %s. Replace it before then.",
			inspection.ExpiresInDays, leaf.NotAfter.UTC().Format("2006-01-02")))
	}

	inspection.ChainComplete = chainIsComplete(chain)
	if !inspection.ChainComplete {
		inspection.Warnings = append(inspection.Warnings, ChainIncompleteMessage(chain))
	}

	return inspection
}

// ChainIncompleteMessage explains, in the operator's terms, what is missing.
func ChainIncompleteMessage(chain []*x509.Certificate) string {
	leaf := chain[0]
	if isSelfIssued(leaf) {
		return "This certificate signed itself, so no browser will trust it without the operator installing it manually. That is fine for a private deployment and wrong for a public one."
	}
	return fmt.Sprintf(
		"The chain is incomplete: this certificate was issued by %q and that issuer's certificate was not pasted with it. Browsers that do not already hold the intermediate will reject the connection. Paste the full chain your certificate authority supplied - the certificate first, then each intermediate.",
		leaf.Issuer.CommonName)
}

// chainIsComplete reports whether the pasted blocks lead to a trusted root.
//
// It verifies against the system trust store with the pasted certificates
// (other than the leaf) offered as intermediates. A self-issued leaf can never
// pass: it is trusted only by whoever installed it, which is a decision the
// operator makes explicitly rather than one this function makes for them.
func chainIsComplete(chain []*x509.Certificate) bool {
	leaf := chain[0]
	if isSelfIssued(leaf) {
		return false
	}

	intermediates := x509.NewCertPool()
	for _, cert := range chain[1:] {
		intermediates.AddCert(cert)
	}

	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		// Without a trust store there is nothing to verify against. Fall back to
		// the structural question - is the issuer of each certificate present -
		// rather than claiming a completeness that was never checked.
		return issuerPresent(chain)
	}

	_, err = leaf.Verify(x509.VerifyOptions{
		Intermediates: intermediates,
		Roots:         roots,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// The name is checked separately, against the host the panel answers
		// as, which is a different question from whether the chain builds.
	})
	return err == nil
}

// issuerPresent is the structural fallback: every certificate but the last has
// its issuer in the chain.
func issuerPresent(chain []*x509.Certificate) bool {
	if len(chain) < 2 {
		return false
	}
	for i := 0; i < len(chain)-1; i++ {
		if !bytes.Equal(chain[i].RawIssuer, chain[i+1].RawSubject) {
			return false
		}
	}
	return true
}

func isSelfIssued(cert *x509.Certificate) bool {
	return bytes.Equal(cert.RawIssuer, cert.RawSubject)
}

func fingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, 0, len(sum))
	for _, b := range sum {
		parts = append(parts, fmt.Sprintf("%02X", b))
	}
	return "SHA256:" + strings.Join(parts, ":")
}

// InspectCertificateFile describes the certificate on disk, without the key.
// It is what the settings endpoint reports for a certificate the panel is
// already serving, whoever produced it.
func InspectCertificateFile(certFile string) (*CertificateInspection, error) {
	if strings.TrimSpace(certFile) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	chain, err := decodeCertificateChain(raw)
	if err != nil {
		return nil, err
	}
	return describeCertificate(chain), nil
}

// CustomPanelCertPaths is where a pasted certificate pair is stored.
//
// It is deliberately NOT the panel.crt/panel.key pair that the self-signed
// generator and the ACME renewal loop both write: those files are rewritten
// without asking, and an operator's pasted certificate landing there would be
// silently replaced by a renewal weeks later - the kind of defect that is only
// noticed when somebody asks why the certificate changed back.
func CustomPanelCertPaths() (certFile, keyFile string) {
	dir := PanelSSLDir()
	return filepath.Join(dir, "panel-custom.crt"), filepath.Join(dir, "panel-custom.key")
}

// IsManagedCertPath reports whether a path belongs to the pair this panel
// generates and renews for itself.
func IsManagedCertPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	dir := PanelSSLDir()
	return path == filepath.Join(dir, "panel.crt") || path == filepath.Join(dir, "panel.key")
}

// InstallCustomPair writes a validated certificate pair to disk.
//
// The key is written first and at 0600, before the certificate that points at
// it, and both go through a temporary file and a rename: a crash in the middle
// must never leave a certificate that does not match the key on disk, because
// that combination fails every handshake on the next start - when nobody is
// watching and the operator's way back in is the thing that broke.
//
// The previous pair, if any, is returned so the caller can put it back.
func InstallCustomPair(certFile, keyFile string, certPEM, keyPEM []byte) (previous *CertificateBackup, err error) {
	if strings.TrimSpace(certFile) == "" || strings.TrimSpace(keyFile) == "" {
		return nil, fmt.Errorf("panel access: no path to store the certificate pair at")
	}
	if err := os.MkdirAll(filepath.Dir(certFile), 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o750); err != nil {
		return nil, err
	}

	previous = backupPair(certFile, keyFile)

	if err := writePanelFileAtomic(keyFile, keyPEM, 0o600); err != nil {
		return previous, err
	}
	if err := writePanelFileAtomic(certFile, certPEM, 0o644); err != nil {
		// Put the key back: a new key beside an old certificate is exactly the
		// mismatch this function exists to prevent.
		previous.Restore(certFile, keyFile)
		return previous, err
	}

	return previous, nil
}

// CertificateBackup is the pair that was on disk before a replacement, held in
// memory so a failed change can be undone without another disk read.
type CertificateBackup struct {
	CertPEM  []byte
	KeyPEM   []byte
	CertMode os.FileMode
	KeyMode  os.FileMode
	Existed  bool
}

// Restore puts the previous pair back, or removes the files when there was no
// previous pair. It reports no error: it runs on a path that is already
// failing, and there is nothing left to fall back to.
func (b *CertificateBackup) Restore(certFile, keyFile string) {
	if b == nil || !b.Existed {
		_ = os.Remove(certFile)
		_ = os.Remove(keyFile)
		return
	}
	_ = writePanelFileAtomic(keyFile, b.KeyPEM, b.KeyMode)
	_ = writePanelFileAtomic(certFile, b.CertPEM, b.CertMode)
}

func backupPair(certFile, keyFile string) *CertificateBackup {
	backup := &CertificateBackup{CertMode: 0o644, KeyMode: 0o600}

	certPEM, certErr := os.ReadFile(certFile)
	keyPEM, keyErr := os.ReadFile(keyFile)
	if certErr != nil || keyErr != nil {
		return backup
	}

	backup.Existed = true
	backup.CertPEM, backup.KeyPEM = certPEM, keyPEM
	if info, err := os.Stat(certFile); err == nil {
		backup.CertMode = info.Mode().Perm()
	}
	if info, err := os.Stat(keyFile); err == nil {
		backup.KeyMode = info.Mode().Perm()
	}
	return backup
}

func writePanelFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("panel access: cannot write %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("panel access: cannot replace %s: %w", path, err)
	}
	return nil
}

// FingerprintDER is the SHA-256 fingerprint of a DER-encoded certificate, in
// the form an operator sees in a browser.
func FingerprintDER(der []byte) string { return fingerprintDER(der) }

// ManagedPanelCertPaths is where the panel keeps the certificate pair it
// produces and renews for itself, in the self-signed and automatic modes.
func ManagedPanelCertPaths() (certFile, keyFile string) {
	dir := PanelSSLDir()
	return filepath.Join(dir, "panel.crt"), filepath.Join(dir, "panel.key")
}

// panelEnvVariables maps the marker recorded in EnvOverrides onto the
// environment variable an operator would actually edit.
var panelEnvVariables = map[string]string{
	"enabled":          "VKAI_PANEL_ENABLED",
	"bind":             "VKAI_PANEL_BIND",
	"port":             "VKAI_PANEL_PORT",
	"public_port":      "VKAI_PANEL_PUBLIC_PORT",
	"public_scheme":    "VKAI_PANEL_PUBLIC_SCHEME",
	"entrance":         "VKAI_PANEL_ENTRANCE",
	"entrance_enabled": "VKAI_PANEL_ENTRANCE_ENABLED",
	"session_ttl":      "VKAI_PANEL_SESSION_TTL",
	"allowed_ips":      "VKAI_PANEL_ALLOWED_IPS",
	"trusted_proxies":  "VKAI_PANEL_TRUSTED_PROXIES",
	"domain":           "VKAI_PANEL_DOMAIN",
	"tls_enabled":      "VKAI_PANEL_TLS_ENABLED",
	"tls_mode":         "VKAI_PANEL_TLS_MODE",
	"tls_self_signed":  "VKAI_PANEL_TLS_SELF_SIGNED",
	"tls_cert":         "VKAI_PANEL_TLS_CERT",
	"tls_key":          "VKAI_PANEL_TLS_KEY",
	"acme_email":       "VKAI_PANEL_ACME_EMAIL",
	"acme_staging":     "VKAI_PANEL_ACME_STAGING",
	"acme_profile":     "VKAI_PANEL_ACME_PROFILE",
}

// PanelEnvVariable is the environment variable that pins a setting, or "" when
// the marker is not one this package knows.
func PanelEnvVariable(marker string) string { return panelEnvVariables[marker] }
