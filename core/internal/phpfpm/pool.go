package phpfpm

// Rendering a PHP-FPM pool file.
//
// Everything in a pool file is attacker-controlled from this package's point of
// view: the pool name, the unix user, the memory limit and the extension names
// all arrive in an HTTP request body. A pool file is an INI-like format with no
// quoting and no escaping, so a value containing a newline is not a bad value,
// it is two directives - and the second one can be `user = root`.
//
// The answer is not to escape. It is to validate every field against a total,
// anchored pattern and refuse anything else, then render from typed fields with
// no string concatenation of caller data into directive names. A value that
// reaches Render has already been proved to contain no newline, no NUL and
// nothing outside its own grammar.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	// A pool name becomes a file name and a socket name, so it is one safe
	// path segment: lowercase alphanumerics, dot, dash and underscore, never
	// starting with a dot or a dash.
	poolNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
	// A unix user or group name, per the useradd(8) NAME_REGEX most
	// distributions ship.
	unixNameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}\$?$`)
	// A PHP memory size: a number with an optional K/M/G suffix, or -1 for
	// "no limit" on memory_limit.
	sizeRe = regexp.MustCompile(`^(-1|[0-9]{1,9}[KMGkmg]?)$`)
	// A PHP extension name as it appears in an .ini file and in a package
	// name. Underscores are real (pdo_mysql), dashes are not.
	extensionRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	// A process manager mode.
	pmModes = map[string]bool{"static": true, "dynamic": true, "ondemand": true}
)

// PoolSpec is one site's FPM pool, as typed fields. There is deliberately no
// "extra directives" string: an operator escape hatch here is an escape hatch
// straight to `user = root`.
type PoolSpec struct {
	// Name is the pool name, the pool file name and the socket name.
	Name string
	// Version is the PHP version this pool runs under. Changing it moves the
	// pool file between two directories and reloads two services.
	Version string
	// User and Group are the unix identity every request for this site runs
	// as. This is the site isolation boundary: two sites must never share one.
	User  string
	Group string
	// ListenOwner, ListenGroup and ListenMode govern who may talk to the
	// socket - in practice the web server's user.
	ListenOwner string
	ListenGroup string
	ListenMode  string

	// Process manager.
	PM                   string
	PMMaxChildren        int
	PMStartServers       int
	PMMinSpareServers    int
	PMMaxSpareServers    int
	PMMaxRequests        int
	PMProcessIdleTimeout string

	// The four settings the brief names, each one a php_admin_value so a site
	// cannot raise it back with ini_set() from its own PHP code. php_value
	// would let it; php_admin_value does not.
	MemoryLimit       string
	MaxExecutionTime  int
	MaxInputTime      int
	UploadMaxFilesize string
	PostMaxSize       string
	MaxFileUploads    int

	// Extensions is the set of extensions this site asked for. See the long
	// note in renderExtensions: PHP loads extensions in the FPM master, before
	// any pool exists, so this list is enforced at the version level and
	// recorded in the pool file rather than pretended to be per-pool.
	Extensions []string

	// Diagnostics.
	ErrorLog      string
	AccessLog     string
	SlowLog       string
	DisplayErrors bool
	Timezone      string

	// OpenBasedir confines the site to its own tree. Empty means "not set",
	// which is the correct default for a site that legitimately reads outside
	// its root; the service sets it for WordPress installs.
	OpenBasedir []string
	// DisabledFunctions is enforced per pool and really does take effect
	// there, unlike extension loading.
	DisabledFunctions []string

	// Env are extra environment variables for the pool's children.
	Env map[string]string

	// SocketPath is the absolute listen socket. It is filled in from the
	// Layout by the manager, never by a caller.
	SocketPath string
}

// Validate proves every field is renderable before a single byte is written.
// It returns the first problem, named, because an operator fixing a form field
// needs to know which field.
func (p *PoolSpec) Validate() error {
	if !poolNameRe.MatchString(p.Name) {
		return fmt.Errorf("invalid pool name %q: use lowercase letters, digits, dot, dash "+
			"or underscore, starting with a letter or digit", p.Name)
	}
	if err := ValidateVersion(p.Version); err != nil {
		return err
	}
	if !unixNameRe.MatchString(p.User) {
		return fmt.Errorf("invalid pool user %q", p.User)
	}
	if p.User == "root" {
		return fmt.Errorf("a pool may not run as root: every request to this site would " +
			"execute PHP with full privileges")
	}
	if !unixNameRe.MatchString(p.Group) {
		return fmt.Errorf("invalid pool group %q", p.Group)
	}
	if p.Group == "root" {
		return fmt.Errorf("a pool may not run in the root group")
	}
	if p.ListenOwner != "" && !unixNameRe.MatchString(p.ListenOwner) {
		return fmt.Errorf("invalid listen owner %q", p.ListenOwner)
	}
	if p.ListenGroup != "" && !unixNameRe.MatchString(p.ListenGroup) {
		return fmt.Errorf("invalid listen group %q", p.ListenGroup)
	}
	if p.ListenMode != "" && !regexp.MustCompile(`^0[0-7]{3}$`).MatchString(p.ListenMode) {
		return fmt.Errorf("invalid listen mode %q: expected a four-digit octal mode such as 0660", p.ListenMode)
	}
	if !pmModes[p.PM] {
		return fmt.Errorf("invalid process manager %q: expected static, dynamic or ondemand", p.PM)
	}
	if p.PMMaxChildren < 1 || p.PMMaxChildren > 10000 {
		return fmt.Errorf("pm.max_children must be between 1 and 10000, got %d", p.PMMaxChildren)
	}
	if p.PM == "dynamic" {
		switch {
		case p.PMStartServers < 1:
			return fmt.Errorf("pm.start_servers must be at least 1 for a dynamic pool")
		case p.PMMinSpareServers < 1:
			return fmt.Errorf("pm.min_spare_servers must be at least 1 for a dynamic pool")
		case p.PMMaxSpareServers < p.PMMinSpareServers:
			return fmt.Errorf("pm.max_spare_servers (%d) must not be below pm.min_spare_servers (%d)",
				p.PMMaxSpareServers, p.PMMinSpareServers)
		case p.PMStartServers < p.PMMinSpareServers || p.PMStartServers > p.PMMaxSpareServers:
			// FPM itself refuses to start on this, so catching it here is the
			// difference between a clear message and a site that is down.
			return fmt.Errorf("pm.start_servers (%d) must be between pm.min_spare_servers (%d) "+
				"and pm.max_spare_servers (%d); php-fpm refuses to start otherwise",
				p.PMStartServers, p.PMMinSpareServers, p.PMMaxSpareServers)
		case p.PMMaxSpareServers > p.PMMaxChildren:
			return fmt.Errorf("pm.max_spare_servers (%d) must not exceed pm.max_children (%d)",
				p.PMMaxSpareServers, p.PMMaxChildren)
		}
	}
	if p.PMMaxRequests < 0 {
		return fmt.Errorf("pm.max_requests must not be negative")
	}
	if p.PMProcessIdleTimeout != "" && !regexp.MustCompile(`^[0-9]{1,6}[smhd]?$`).MatchString(p.PMProcessIdleTimeout) {
		return fmt.Errorf("invalid pm.process_idle_timeout %q: expected a duration such as 10s", p.PMProcessIdleTimeout)
	}

	if p.MemoryLimit != "" && !sizeRe.MatchString(p.MemoryLimit) {
		return fmt.Errorf("invalid memory_limit %q: expected a size such as 256M, or -1", p.MemoryLimit)
	}
	if p.MaxExecutionTime < 0 || p.MaxExecutionTime > 86400 {
		return fmt.Errorf("max_execution_time must be between 0 and 86400 seconds, got %d", p.MaxExecutionTime)
	}
	if p.MaxInputTime < -1 || p.MaxInputTime > 86400 {
		return fmt.Errorf("max_input_time must be between -1 and 86400 seconds, got %d", p.MaxInputTime)
	}
	if p.UploadMaxFilesize != "" && !sizeRe.MatchString(p.UploadMaxFilesize) {
		return fmt.Errorf("invalid upload_max_filesize %q: expected a size such as 64M", p.UploadMaxFilesize)
	}
	if p.PostMaxSize != "" && !sizeRe.MatchString(p.PostMaxSize) {
		return fmt.Errorf("invalid post_max_size %q: expected a size such as 64M", p.PostMaxSize)
	}
	if p.MaxFileUploads < 0 || p.MaxFileUploads > 10000 {
		return fmt.Errorf("max_file_uploads must be between 0 and 10000, got %d", p.MaxFileUploads)
	}

	for _, ext := range p.Extensions {
		if !extensionRe.MatchString(ext) {
			return fmt.Errorf("invalid extension name %q: expected a lowercase name such as redis or pdo_mysql", ext)
		}
	}
	for _, fn := range p.DisabledFunctions {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`).MatchString(fn) {
			return fmt.Errorf("invalid function name %q in disable_functions", fn)
		}
	}
	if p.Timezone != "" && !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z0-9_+-]+){0,2}$`).MatchString(p.Timezone) {
		return fmt.Errorf("invalid timezone %q", p.Timezone)
	}

	for _, path := range append(append([]string{}, p.OpenBasedir...),
		p.ErrorLog, p.AccessLog, p.SlowLog, p.SocketPath) {
		if path == "" {
			continue
		}
		if err := validateConfigPath(path); err != nil {
			return err
		}
	}

	for key, value := range p.Env {
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`).MatchString(key) {
			return fmt.Errorf("invalid environment variable name %q", key)
		}
		if strings.ContainsAny(value, "\n\r\x00") {
			return fmt.Errorf("environment variable %q contains a newline, which would become a "+
				"second directive in the pool file", key)
		}
	}

	return nil
}

// validateConfigPath refuses anything that is not a plain absolute path. A
// newline here is a directive injection; a relative path is a file FPM writes
// somewhere nobody expects.
func validateConfigPath(path string) error {
	switch {
	case !strings.HasPrefix(path, "/"):
		return fmt.Errorf("path %q must be absolute", path)
	case strings.ContainsAny(path, "\n\r\x00"):
		return fmt.Errorf("path %q contains a control character", path)
	case strings.Contains(path, ".."):
		return fmt.Errorf("path %q must not contain ..", path)
	case len(path) > 4096:
		return fmt.Errorf("path is too long")
	}
	return nil
}

// Render produces the pool file. It validates first and returns an error
// rather than a partial file: a half-rendered pool file that FPM then reads is
// a site that is down.
func (p *PoolSpec) Render() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	var b strings.Builder
	line := func(format string, args ...any) {
		fmt.Fprintf(&b, format+"\n", args...)
	}

	line("; Managed by VKAI Panel. Do not edit by hand: this file is rewritten in full")
	line("; whenever the site's PHP version or pool settings change, and any manual")
	line("; change is lost at that point. Edit the site's PHP settings in the panel.")
	line(";")
	line("; pool: %s   php: %s   user: %s:%s", p.Name, p.Version, p.User, p.Group)
	line("")
	line("[%s]", p.Name)
	line("")
	line("; Identity. Every request to this site executes as this unix user, which is")
	line("; the isolation boundary between one customer and the next.")
	line("user = %s", p.User)
	line("group = %s", p.Group)
	line("")
	line("listen = %s", p.SocketPath)
	if p.ListenOwner != "" {
		line("listen.owner = %s", p.ListenOwner)
	}
	if p.ListenGroup != "" {
		line("listen.group = %s", p.ListenGroup)
	}
	if p.ListenMode != "" {
		line("listen.mode = %s", p.ListenMode)
	}
	line("")
	line("pm = %s", p.PM)
	line("pm.max_children = %d", p.PMMaxChildren)
	if p.PM == "dynamic" {
		line("pm.start_servers = %d", p.PMStartServers)
		line("pm.min_spare_servers = %d", p.PMMinSpareServers)
		line("pm.max_spare_servers = %d", p.PMMaxSpareServers)
	}
	if p.PM == "ondemand" && p.PMProcessIdleTimeout != "" {
		line("pm.process_idle_timeout = %s", p.PMProcessIdleTimeout)
	}
	if p.PMMaxRequests > 0 {
		line("pm.max_requests = %d", p.PMMaxRequests)
	}
	line("")

	if p.AccessLog != "" {
		line("access.log = %s", p.AccessLog)
	}
	if p.SlowLog != "" {
		line("slowlog = %s", p.SlowLog)
	}
	line("")

	line("; Per-site limits. These are php_admin_value, not php_value: a site cannot")
	line("; raise them again with ini_set() from its own PHP code.")
	if p.MemoryLimit != "" {
		line("php_admin_value[memory_limit] = %s", p.MemoryLimit)
	}
	if p.MaxExecutionTime > 0 {
		line("php_admin_value[max_execution_time] = %d", p.MaxExecutionTime)
	}
	if p.MaxInputTime != 0 {
		line("php_admin_value[max_input_time] = %d", p.MaxInputTime)
	}
	if p.UploadMaxFilesize != "" {
		line("php_admin_value[upload_max_filesize] = %s", p.UploadMaxFilesize)
	}
	if p.PostMaxSize != "" {
		line("php_admin_value[post_max_size] = %s", p.PostMaxSize)
	}
	if p.MaxFileUploads > 0 {
		line("php_admin_value[max_file_uploads] = %d", p.MaxFileUploads)
	}
	if p.Timezone != "" {
		line("php_admin_value[date.timezone] = %s", p.Timezone)
	}
	if p.ErrorLog != "" {
		line("php_admin_value[error_log] = %s", p.ErrorLog)
		line("php_admin_flag[log_errors] = on")
	}
	if p.DisplayErrors {
		line("php_admin_flag[display_errors] = on")
	} else {
		line("php_admin_flag[display_errors] = off")
	}
	if len(p.OpenBasedir) > 0 {
		line("php_admin_value[open_basedir] = %s", strings.Join(p.OpenBasedir, ":"))
	}
	if len(p.DisabledFunctions) > 0 {
		disabled := append([]string{}, p.DisabledFunctions...)
		sort.Strings(disabled)
		line("php_admin_value[disable_functions] = %s", strings.Join(disabled, ","))
	}
	line("")

	b.WriteString(p.renderExtensions())

	if len(p.Env) > 0 {
		line("")
		keys := make([]string, 0, len(p.Env))
		for key := range p.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			line("env[%s] = %s", key, p.Env[key])
		}
	}

	return []byte(b.String()), nil
}

// renderExtensions writes the extension manifest.
//
// This deserves the honesty the rest of this task asks for. PHP loads extension
// modules in the FPM MASTER process, from php.ini and the scanned conf.d
// directory, before a single pool exists. `php_admin_value[extension]` is
// accepted by the pool parser and then does nothing, because the extension
// directive is handled by the ini parser at startup and not by the per-request
// ini override machinery. A panel that writes it into a pool file and reports
// "redis enabled for this site" is lying.
//
// So the enabled set is enforced where it can be - at the PHP version level, by
// Manager.EnsureExtensions, which installs the distribution package and writes
// the conf.d file - and recorded here, in the pool file, so that the pool file
// remains a complete description of what this site asked for and an operator
// reading it can see why an extension is present.
func (p *PoolSpec) renderExtensions() string {
	var b strings.Builder
	b.WriteString("; Extensions requested by this site.\n")
	b.WriteString("; PHP loads extensions in the php-fpm master process, before any pool exists,\n")
	b.WriteString("; so an extension cannot be switched on for one pool and off for another on the\n")
	b.WriteString("; same PHP version. The panel enforces this list at the PHP version level (it\n")
	b.WriteString("; installs the distribution package and writes the conf.d file) and records it\n")
	b.WriteString("; here so this file describes the whole request. A site that needs a different\n")
	b.WriteString("; extension set needs a different PHP version.\n")
	if len(p.Extensions) == 0 {
		b.WriteString(";   (none beyond the version's defaults)\n")
		return b.String()
	}
	extensions := append([]string{}, p.Extensions...)
	sort.Strings(extensions)
	for _, ext := range extensions {
		fmt.Fprintf(&b, ";   extension = %s\n", ext)
	}
	return b.String()
}
