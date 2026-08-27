package utils

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Shared hardening helpers used by services that touch the shell, systemd unit
// files, SQL identifiers or the filesystem. Everything here is deny-by-default:
// input that does not match an explicit allowlist is rejected with an error.

var (
	// sqlIdentifierRe matches a conservative database / user name.
	sqlIdentifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,62}$`)

	// systemdUnitNameRe matches a systemd unit name without any path component.
	systemdUnitNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@-]{0,127}$`)

	// envKeyRe matches a shell/systemd environment variable name.
	envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	// safePathRe matches an absolute path built from a conservative charset.
	safePathRe = regexp.MustCompile(`^/[A-Za-z0-9/_.@+-]*$`)

	// domainNameRe matches a DNS host name (also used for vhost file names).
	domainNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*\.[a-z]{2,63}$`)

	// cronFieldRe matches a single cron schedule field.
	cronFieldRe = regexp.MustCompile(`^[0-9*/,\-]+$`)

	// nodeStartScriptRe matches a relative script path for a Node.js app.
	nodeStartScriptRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// ContainsControlChars reports whether s holds a byte that would let an
// attacker break out of a single line (unit files, cron files, config files).
func ContainsControlChars(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 && c != '\t' {
			return true
		}
		if c == 0x7f {
			return true
		}
	}
	return false
}

// ValidateSingleLine rejects any value carrying newlines or NUL bytes.
func ValidateSingleLine(value, fieldName string) error {
	if ContainsControlChars(value) {
		return fmt.Errorf("%s must not contain control characters", fieldName)
	}
	return nil
}

// ValidateSQLIdentifier checks a database or database-user name before it is
// interpolated into an administrative SQL statement.
func ValidateSQLIdentifier(value, fieldName string) error {
	if !sqlIdentifierRe.MatchString(value) {
		return fmt.Errorf("%s must match ^[a-zA-Z_][a-zA-Z0-9_]{0,62}$", fieldName)
	}
	return nil
}

// QuoteSQLIdentifier wraps an already validated identifier in double quotes for
// PostgreSQL, doubling any embedded quote as defence in depth.
func QuoteSQLIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// QuoteMySQLIdentifier wraps an already validated identifier in backticks.
func QuoteMySQLIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

// QuoteSQLLiteral renders a single-quoted SQL string literal. Backslashes are
// rejected rather than escaped because MySQL and PostgreSQL disagree on them.
func QuoteSQLLiteral(value string) (string, error) {
	if strings.ContainsAny(value, "\\\x00") || ContainsControlChars(value) {
		return "", fmt.Errorf("value contains characters that cannot be safely quoted")
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}

// ValidateCharsetName restricts MySQL charset / collation names.
func ValidateCharsetName(value, fieldName string) error {
	if value == "" {
		return nil
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`).MatchString(value) {
		return fmt.Errorf("%s must be alphanumeric", fieldName)
	}
	return nil
}

// ValidateServiceName checks a systemd unit name. It rejects path separators so
// the name can never escape /etc/systemd/system.
func ValidateServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name is required")
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("service name must not contain path separators")
	}
	if !systemdUnitNameRe.MatchString(name) {
		return fmt.Errorf("service name must match ^[a-zA-Z0-9][a-zA-Z0-9_.@-]{0,127}$")
	}
	return nil
}

// ValidateEnvKey checks an environment variable name.
func ValidateEnvKey(key string) error {
	if !envKeyRe.MatchString(key) {
		return fmt.Errorf("environment variable name %q is invalid", key)
	}
	return nil
}

// ValidateAbsolutePath checks that p is an absolute, clean path drawn from a
// conservative character set and free of traversal segments.
func ValidateAbsolutePath(p, fieldName string) error {
	if p == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s must be an absolute path", fieldName)
	}
	if ContainsControlChars(p) {
		return fmt.Errorf("%s must not contain control characters", fieldName)
	}
	if !safePathRe.MatchString(p) {
		return fmt.Errorf("%s contains characters that are not allowed in a path", fieldName)
	}
	if filepath.Clean(p) != strings.TrimRight(p, "/") && filepath.Clean(p) != p {
		return fmt.Errorf("%s must be a normalised path", fieldName)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("%s must not contain '..'", fieldName)
		}
	}
	return nil
}

// EnsureWithinRoot verifies that a cleaned path stays inside root.
func EnsureWithinRoot(root, p string) (string, error) {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(p)
	if cleanPath != cleanRoot && !strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes allowed root %q", p, cleanRoot)
	}
	return cleanPath, nil
}

// ValidateCommandArg rejects an argument that a CLI tool would read as an
// option. Callers should also pass "--" before user supplied operands.
func ValidateCommandArg(value, fieldName string) error {
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s must not start with '-'", fieldName)
	}
	if ContainsControlChars(value) {
		return fmt.Errorf("%s must not contain control characters", fieldName)
	}
	return nil
}

// ValidateHostname validates a DNS name used for vhost file names.
func ValidateHostname(domain, fieldName string) error {
	if domain == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	if len(domain) > 253 {
		return fmt.Errorf("%s must be at most 253 characters", fieldName)
	}
	if !domainNameRe.MatchString(strings.ToLower(domain)) {
		return fmt.Errorf("%s must be a valid domain name", fieldName)
	}
	return nil
}

// ValidateCronSchedule accepts the classic five field cron expression plus the
// common @-shortcuts. Anything else is refused before it reaches /etc/cron.d.
func ValidateCronSchedule(schedule string) error {
	s := strings.TrimSpace(schedule)
	if s == "" {
		return fmt.Errorf("schedule is required")
	}
	if ContainsControlChars(s) {
		return fmt.Errorf("schedule must not contain control characters")
	}
	switch s {
	case "@yearly", "@annually", "@monthly", "@weekly", "@daily", "@midnight", "@hourly":
		return nil
	}
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return fmt.Errorf("schedule must have exactly 5 fields")
	}
	for _, f := range fields {
		if !cronFieldRe.MatchString(f) {
			return fmt.Errorf("schedule field %q is invalid", f)
		}
	}
	return nil
}

// ValidateNodeStartScript restricts what may be appended to the node binary in
// a generated systemd ExecStart line.
func ValidateNodeStartScript(script, fieldName string) error {
	s := strings.TrimSpace(script)
	if s == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	if s == "npm start" {
		return nil
	}
	if strings.ContainsAny(s, " \t") {
		return fmt.Errorf("%s must be a single script path or the literal \"npm start\"", fieldName)
	}
	if !nodeStartScriptRe.MatchString(s) {
		return fmt.Errorf("%s must match ^[A-Za-z0-9._/-]+$", fieldName)
	}
	if strings.Contains(s, "..") {
		return fmt.Errorf("%s must not contain '..'", fieldName)
	}
	return nil
}

// ValidateSQLIdentifierOrUUID accepts either a conservative SQL identifier or a
// UUID string. Used where a resource id doubles as a database name.
func ValidateSQLIdentifierOrUUID(value, fieldName string) error {
	if sqlIdentifierRe.MatchString(value) {
		return nil
	}
	if regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`).MatchString(value) {
		return nil
	}
	return fmt.Errorf("%s is not a valid identifier", fieldName)
}
