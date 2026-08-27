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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
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

// PanelTLSConfig describes the certificate the panel serves on its own port.
type PanelTLSConfig struct {
	Enabled    bool   `json:"enabled"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	SelfSigned bool   `json:"self_signed"`
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
	}
}

// PanelStateFilePath resolves where the generated access settings live.
func PanelStateFilePath() string {
	if v := envString("PANEL_CONFIG_FILE", "PANEL_STATE_FILE"); v != "" {
		return v
	}
	return DefaultPanelStateFile()
}

// LoadPanelAccess builds the effective configuration from defaults, the state
// file and the environment, generating whatever is still missing.
//
// A generated value is persisted when the state file is writable. It is not an
// error when it is not (containers frequently run with a read-only /etc): the
// panel still starts, it just prints the values again on the next boot.
func LoadPanelAccess() (*PanelAccessConfig, error) {
	cfg := DefaultPanelAccess()

	if err := cfg.loadStateFile(); err != nil {
		return nil, err
	}
	if err := cfg.applyEnv(); err != nil {
		return nil, err
	}
	if err := cfg.fillGenerated(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if len(cfg.Generated) > 0 {
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
func (p *PanelAccessConfig) applyEnv() error {
	if v, ok := envBoolOK("PANEL_ENABLED"); ok {
		p.Enabled = v
		p.markEnv("enabled")
	}
	if v := envString("PANEL_BIND", "PANEL_HOST"); v != "" {
		p.Bind = v
		p.markEnv("bind")
	}
	if v := envString("PANEL_PORT"); v != "" {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("panel access: PANEL_PORT=%q khong phai so", v)
		}
		p.Port = port
		p.markEnv("port")
	}
	if v := envString("PANEL_PUBLIC_PORT"); v != "" {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("panel access: PANEL_PUBLIC_PORT=%q khong phai so", v)
		}
		p.PublicPort = port
		p.markEnv("public_port")
	}
	if v := envString("PANEL_PUBLIC_SCHEME"); v != "" {
		scheme := strings.ToLower(strings.TrimSpace(v))
		if scheme != "http" && scheme != "https" {
			return fmt.Errorf("panel access: PANEL_PUBLIC_SCHEME=%q chi nhan http hoac https", v)
		}
		p.PublicScheme = scheme
		p.markEnv("public_scheme")
	}
	if v := envString("PANEL_ENTRANCE"); v != "" {
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
	if v, ok := envBoolOK("PANEL_ENTRANCE_ENABLED"); ok {
		p.EntranceEnabled = v
		p.markEnv("entrance_enabled")
	}
	if v := envString("PANEL_SESSION_TTL"); v != "" {
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
	if v, ok := envListOK("PANEL_ALLOWED_IPS", "PANEL_ALLOW_IPS"); ok {
		p.AllowedIPs = v
		p.markEnv("allowed_ips")
	}
	if v, ok := envListOK("PANEL_TRUSTED_PROXIES"); ok {
		p.TrustedProxies = v
		p.markEnv("trusted_proxies")
	}
	if v := envString("PANEL_DOMAIN"); v != "" {
		p.Domain = strings.ToLower(strings.TrimSpace(v))
		p.markEnv("domain")
	}
	if v := envString("PANEL_TLS_CERT", "PANEL_TLS_CERT_FILE"); v != "" {
		p.TLS.CertFile = v
		p.TLS.Enabled = true
		p.markEnv("tls_cert")
	}
	if v := envString("PANEL_TLS_KEY", "PANEL_TLS_KEY_FILE"); v != "" {
		p.TLS.KeyFile = v
		p.TLS.Enabled = true
		p.markEnv("tls_key")
	}
	if v, ok := envBoolOK("PANEL_TLS_SELF_SIGNED"); ok {
		p.TLS.SelfSigned = v
		if v {
			p.TLS.Enabled = true
		}
		p.markEnv("tls_self_signed")
	}
	if v, ok := envBoolOK("PANEL_TLS_ENABLED", "PANEL_SSL"); ok {
		p.TLS.Enabled = v
		p.markEnv("tls_enabled")
	}

	return nil
}

// fillGenerated invents the values nobody supplied: a random port when
// PANEL_RANDOM_PORT asks for one, and an entrance whenever the security
// entrance is on but no path has ever been chosen.
func (p *PanelAccessConfig) fillGenerated() error {
	randomPort, _ := envBoolOK("PANEL_RANDOM_PORT")
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

	if p.TLS.Enabled && p.TLS.SelfSigned {
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

	if p.TLS.Enabled && !p.TLS.SelfSigned {
		if strings.TrimSpace(p.TLS.CertFile) == "" || strings.TrimSpace(p.TLS.KeyFile) == "" {
			return fmt.Errorf("panel access: bat TLS thi phai dat ca VKAI_PANEL_TLS_CERT va VKAI_PANEL_TLS_KEY (hoac VKAI_PANEL_TLS_SELF_SIGNED=true)")
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
	url := fmt.Sprintf("%s://%s:%d", p.EffectiveScheme(), p.AccessHost(), p.EffectivePort())
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

	tls := "tat (HTTP)"
	if p.TLS.Enabled {
		tls = "bat (HTTPS)"
		if p.TLS.SelfSigned {
			tls += " - chung chi tu ky"
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

	var b strings.Builder
	const width = 78
	line := strings.Repeat("=", width)

	b.WriteString("\n" + line + "\n")
	b.WriteString("  VKAI PANEL - HiTech Cloud (hitechcloud.vn)\n")
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

// RandomEntrance produces an aaPanel-style secret path, e.g. /vkai_a1b2c3d4.
func RandomEntrance() (string, error) {
	buf := make([]byte, 4)
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

	certExists := fileExists(certFile)
	keyExists := fileExists(keyFile)
	if certExists && keyExists {
		return certFile, keyFile, nil
	}

	if !p.TLS.SelfSigned {
		return "", "", fmt.Errorf("panel access: khong tim thay chung chi %s hoac khoa %s", certFile, keyFile)
	}

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
			Organization: []string{"HiTech Cloud"},
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

// envString returns the first non-empty value among VKAI_<name> and <name>.
func envString(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv("VKAI_" + name)); v != "" {
			return v
		}
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

func envBoolOK(names ...string) (bool, bool) {
	raw := envString(names...)
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

// envListOK reports whether the variable was set at all, so that an explicit
// empty value ("allow everyone") can be distinguished from "not configured".
func envListOK(names ...string) ([]string, bool) {
	for _, name := range names {
		for _, key := range []string{"VKAI_" + name, name} {
			raw, ok := os.LookupEnv(key)
			if !ok {
				continue
			}
			return splitList(raw), true
		}
	}
	return nil, false
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
