package wpcli

// Typed arguments for WP-CLI.
//
// Requirement five of this task, restated: "Commands take typed arguments,
// never a shell string built from user input. Site names, plugin slugs and
// paths are all attacker-controlled from this code's point of view."
//
// This file is the enforcement. Nothing in this package ever builds a command
// line as a string, and nothing ever calls sh. A WP-CLI invocation is a []string
// argv handed to exec.CommandContext, and every element of that slice comes
// from one of the constructors below, each of which validates against an
// anchored pattern and returns an error rather than escaping.
//
// Escaping is not used anywhere here on purpose. Escaping is a claim about
// somebody else's parser; validation is a claim about our own input, and only
// the second one can be tested exhaustively. The shell-metacharacter test in
// runner_test.go feeds every character a shell treats specially through every
// constructor and asserts a refusal.

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// A plugin or theme slug as WordPress.org publishes it, or a version
	// constraint appended by the caller. Slugs are lowercase alphanumerics
	// with dashes; some legacy ones contain underscores and dots.
	slugRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
	// A WordPress user login. WordPress itself allows a wide set, but a login
	// that reaches an argv must not start with a dash or contain a space.
	loginRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,59}$`)
	// A database table prefix.
	prefixRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,31}$`)
	// A MySQL identifier for a database or user name.
	identRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_$-]{0,63}$`)
	// A WordPress core version, or the word "latest".
	coreVersionRe = regexp.MustCompile(`^(latest|[0-9]{1,2}(\.[0-9]{1,3}){1,3})$`)
	// A unix user or group name.
	unixNameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}\$?$`)
)

// shellMetacharacters is every character that changes meaning in sh, plus the
// two that end a line and the one that ends a C string. No validated value may
// contain any of them.
//
// This is a belt-and-braces check: the anchored patterns above already exclude
// all of these. It is here because a future edit that widens one of those
// patterns "just to allow spaces in a site name" would otherwise silently
// widen it to allow `; rm -rf /` as well.
const shellMetacharacters = "`$&|;<>()[]{}!*?~#'\"\\ \t\n\r\v\f\x00"

// ErrMetacharacter is returned when a value contains a character a shell would
// treat specially. It names the character, because "invalid input" sends an
// operator to read source code.
type ErrMetacharacter struct {
	Field string
	Value string
	Char  rune
}

func (e *ErrMetacharacter) Error() string {
	return fmt.Sprintf("%s %q contains %q, which a shell would treat as syntax; "+
		"WP-CLI arguments are refused rather than escaped", e.Field, e.Value, string(e.Char))
}

// rejectMetacharacters is applied to every value before its own pattern.
func rejectMetacharacters(field, value string) error {
	if idx := strings.IndexAny(value, shellMetacharacters); idx >= 0 {
		return &ErrMetacharacter{Field: field, Value: value, Char: rune(value[idx])}
	}
	return nil
}

// rejectOptionInjection refuses a value that starts with a dash.
//
// This is the one that is easy to forget and expensive to miss. Even with no
// shell involved, an argv element of "--allow-root" in the position where a
// plugin slug was expected is a WP-CLI flag, not a slug - and --allow-root is
// exactly the flag this package exists to never pass.
func rejectOptionInjection(field, value string) error {
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s %q may not start with a dash: WP-CLI would read it as an option, "+
			"not as a value", field, value)
	}
	return nil
}

func validate(field, value string, pattern *regexp.Regexp, what string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if err := rejectMetacharacters(field, value); err != nil {
		return err
	}
	if err := rejectOptionInjection(field, value); err != nil {
		return err
	}
	if !pattern.MatchString(value) {
		return fmt.Errorf("invalid %s %q: expected %s", field, value, what)
	}
	return nil
}

// Slug validates a plugin or theme slug.
func Slug(value string) (string, error) {
	if err := validate("slug", value, slugRe, "a plugin or theme slug such as woocommerce"); err != nil {
		return "", err
	}
	return value, nil
}

// Login validates a WordPress user login.
func Login(value string) (string, error) {
	if err := validate("user login", value, loginRe, "a WordPress login"); err != nil {
		return "", err
	}
	return value, nil
}

// TablePrefix validates a WordPress database table prefix.
func TablePrefix(value string) (string, error) {
	if err := validate("table prefix", value, prefixRe, "a prefix such as wp_"); err != nil {
		return "", err
	}
	return value, nil
}

// Identifier validates a database or database-user name.
func Identifier(field, value string) (string, error) {
	if err := validate(field, value, identRe, "a database identifier"); err != nil {
		return "", err
	}
	return value, nil
}

// CoreVersion validates a WordPress core version or "latest".
func CoreVersion(value string) (string, error) {
	if value == "" {
		return "latest", nil
	}
	if err := validate("core version", value, coreVersionRe, "a version such as 6.5.2, or latest"); err != nil {
		return "", err
	}
	return value, nil
}

// UnixName validates a system user or group name.
func UnixName(field, value string) (string, error) {
	if err := validate(field, value, unixNameRe, "a system user name"); err != nil {
		return "", err
	}
	return value, nil
}

// SiteURL validates a URL that will be handed to wp search-replace or wp core
// install. A URL is the one argument here that legitimately contains a colon
// and a slash, so it gets its own rule rather than a widened pattern.
func SiteURL(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("site url is required")
	}
	if len(value) > 2000 {
		return "", fmt.Errorf("site url is too long")
	}
	// Control characters and the shell's word-splitting characters are refused
	// even inside a URL; a legitimate site URL contains none of them.
	if strings.ContainsAny(value, "`$&|;<>()[]{}!*?'\"\\ \t\n\r\v\f\x00") {
		return "", fmt.Errorf("invalid site url %q: it contains a character that is not valid in a URL", value)
	}
	if err := rejectOptionInjection("site url", value); err != nil {
		return "", err
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid site url %q: %w", value, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid site url %q: expected an http:// or https:// URL", value)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid site url %q: it has no host", value)
	}
	return value, nil
}

// Path validates an absolute filesystem path that will become an argv element.
//
// It does NOT decide whether the path is one this panel may touch - that is
// Confinement.Resolve's job, and it is a separate question. This one only
// proves the string is a path and not shell syntax.
func Path(field, value string) (string, error) {
	switch {
	case value == "":
		return "", fmt.Errorf("%s is required", field)
	case !strings.HasPrefix(value, "/"):
		return "", fmt.Errorf("%s %q must be an absolute path", field, value)
	case len(value) > 4096:
		return "", fmt.Errorf("%s is too long", field)
	case strings.ContainsAny(value, "`$&|;<>()[]{}!*?\"'\\\n\r\t\x00"):
		return "", fmt.Errorf("%s %q contains a character that a shell would treat as syntax", field, value)
	}
	cleaned := filepath.Clean(value)
	if cleaned != value && cleaned+"/" != value {
		return "", fmt.Errorf("%s %q is not in canonical form (expected %q)", field, value, cleaned)
	}
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("%s %q must not contain ..", field, value)
	}
	return cleaned, nil
}

// FreeText is for the two arguments that genuinely are free text: the "from"
// and "to" of a search-replace, and an admin display name.
//
// They cannot be pattern-matched, so the rule is different: they may contain
// anything except a NUL (which would truncate the argv element) and they are
// length-bounded. This is safe precisely because there is no shell: a value of
// "; rm -rf /" is one argument to wp, which treats it as a literal string to
// search for. Passing the same value through a shell string would not be safe,
// which is why this package has no way to build one.
func FreeText(field, value string, maxLen int) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if len(value) > maxLen {
		return "", fmt.Errorf("%s is too long (max %d bytes)", field, maxLen)
	}
	if strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("%s contains a NUL byte", field)
	}
	if err := rejectOptionInjection(field, value); err != nil {
		return "", err
	}
	return value, nil
}
