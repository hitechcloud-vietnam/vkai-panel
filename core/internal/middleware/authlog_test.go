package middleware

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const (
	fail2banDir    = "../../../deploy/fail2ban"
	filterFileName = "vkai-panel-auth"
)

func sampleEvent() AuthEvent {
	return AuthEvent{
		Time:      time.Date(2026, 8, 28, 9, 14, 2, 0, time.UTC),
		Outcome:   AuthOutcomeFailure,
		Reason:    ReasonInvalidCredentials,
		IP:        "203.0.113.9",
		Account:   "admin",
		Scope:     ScopeLogin,
		Path:      "/api/v1/auth/login",
		RequestID: "8f2c1d4e",
	}
}

// TestAuthEventLine pins the exact wire format. The fail2ban filter in
// deploy/fail2ban parses these lines; if this test is updated without updating
// that filter, TestFail2banFilterMatchesAuthLogLines fails, which is the point.
func TestAuthEventLine(t *testing.T) {
	want := "2026-08-28T09:14:02Z vkai-auth outcome=failure reason=invalid_credentials " +
		"ip=203.0.113.9 account=admin scope=login path=/api/v1/auth/login request_id=8f2c1d4e"

	if got := sampleEvent().Line(); got != want {
		t.Fatalf("line format changed.\n got: %s\nwant: %s\n\n"+
			"If this change is intended, update the failregex in "+
			"deploy/fail2ban/filter.d/vkai-panel-auth.conf in the same commit.", got, want)
	}
}

func TestAuthEventLineIsAlwaysASingleLineWithAFixedFieldCount(t *testing.T) {
	events := []AuthEvent{
		sampleEvent(),
		{Outcome: AuthOutcomeSuccess, Reason: ReasonOK},
		{},
		{Outcome: AuthOutcomeBlocked, Reason: ReasonLocked, IP: "2001:db8::1", Account: strings.Repeat("a", 500)},
	}

	for _, event := range events {
		line := event.Line()
		if strings.ContainsAny(line, "\n\r") {
			t.Fatalf("line contains a newline: %q", line)
		}
		fields := strings.Fields(line)
		if len(fields) != 9 {
			t.Fatalf("line has %d fields, want 9 (timestamp, tag, and seven key=value pairs): %q",
				len(fields), line)
		}
		for _, field := range fields[2:] {
			if !strings.Contains(field, "=") {
				t.Fatalf("field %q is not a key=value pair in %q", field, line)
			}
		}
	}
}

// TestAuthEventLineResistsLogInjection is the reason every field is sanitised.
//
// The account name comes from the attacker. If it could carry a newline, an
// attacker would submit a username containing a forged failure line naming
// somebody else's address, and use the panel's own fail2ban jail to ban an
// address of their choosing - the operator's, for preference.
func TestAuthEventLineResistsLogInjection(t *testing.T) {
	forged := "victim\n2026-08-28T09:14:03Z vkai-auth outcome=failure reason=invalid_credentials ip=198.51.100.77 account=x"

	event := sampleEvent()
	event.Account = forged
	line := event.Line()

	if strings.Contains(line, "\n") {
		t.Fatal("a crafted account name broke out of its line")
	}
	if strings.Contains(line, "198.51.100.77") {
		t.Fatalf("a crafted account name smuggled an address into the line: %q", line)
	}

	failregex, _ := loadFail2banRegexes(t)
	matches := failregex.FindAllStringSubmatch(line, -1)
	if len(matches) != 1 {
		t.Fatalf("crafted line produced %d filter matches, want exactly 1", len(matches))
	}
	if host := hostFrom(failregex, matches[0]); host != "203.0.113.9" {
		t.Fatalf("filter extracted %q as the source address, want the real one 203.0.113.9", host)
	}
}

func TestSanitizeLogField(t *testing.T) {
	cases := map[string]string{
		"admin":              "admin",
		"":                   "-",
		"   ":                "-",
		"a b":                "a_b",
		"a\nb":               "a_b",
		"a=b":                "a_b",
		"user@example.com":   "user@example.com",
		"2001:db8::1":        "2001:db8::1",
		"/api/v1/auth/login": "/api/v1/auth/login",
		// Each non-ASCII rune collapses to a single underscore.
		"quản-trị":             "qu_n-tr_",
		strings.Repeat("x", 9): strings.Repeat("x", 9),
	}
	for input, want := range cases {
		if got := sanitizeLogField(input, 64); got != want {
			t.Errorf("sanitizeLogField(%q) = %q, want %q", input, got, want)
		}
	}

	if got := sanitizeLogField(strings.Repeat("x", 500), 64); len(got) != 64 {
		t.Errorf("long value not truncated: %d characters", len(got))
	}
}

func TestAuthLoggerWritesOneLinePerEvent(t *testing.T) {
	var buffer bytes.Buffer
	logger := NewAuthLogger(&buffer, nil)

	logger.Log(sampleEvent())
	logger.Log(AuthEvent{Outcome: AuthOutcomeSuccess, Reason: ReasonOK, IP: "203.0.113.9", Account: "admin"})

	lines := strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("wrote %d lines, want 2:\n%s", len(lines), buffer.String())
	}
	for _, line := range lines {
		if !strings.Contains(line, AuthLogTag) {
			t.Fatalf("line is missing the %q tag the filter anchors on: %q", AuthLogTag, line)
		}
	}
}

// --------------------------------------------------------------------------
// The fail2ban contract
// --------------------------------------------------------------------------

// TestFail2banFilterMatchesAuthLogLines reads the shipped filter and matches it
// against lines the panel actually produces.
//
// Without this, the failure mode is silent and total: somebody reorders a field
// or renames an outcome, every test still passes, and the operator's jail
// quietly stops banning anybody. Nothing in a running system reports "my regex
// matched zero lines today".
func TestFail2banFilterMatchesAuthLogLines(t *testing.T) {
	failregex, ignoreregex := loadFail2banRegexes(t)

	shouldBan := []AuthEvent{
		sampleEvent(),
		{Outcome: AuthOutcomeFailure, Reason: ReasonInvalidCredentials, IP: "198.51.100.4", Account: "root", Scope: ScopeAPIKey, Path: "/api/v1/servers"},
		{Outcome: AuthOutcomeBlocked, Reason: ReasonLocked, IP: "192.0.2.15", Account: "admin", Scope: ScopeLogin, Path: "/api/v1/auth/login"},
		{Outcome: AuthOutcomeBlocked, Reason: ReasonThrottled, IP: "192.0.2.16", Account: "-", Scope: ScopeAgentEnrol, Path: "/api/v1/agent/register"},
		{Outcome: AuthOutcomeBlocked, Reason: ReasonLimiterUnavailable, IP: "2001:db8::dead", Account: "admin", Scope: ScopeTwoFactor, Path: "/api/v1/auth/2fa"},
		{Outcome: AuthOutcomeFailure, Reason: ReasonInvalidCredentials, IP: "2001:db8:1:1::5", Account: "fp_0011223344556677", Scope: ScopeRefresh, Path: "/api/v1/auth/refresh"},
	}

	for _, event := range shouldBan {
		line := event.Line()
		match := failregex.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("the shipped fail2ban filter does not match a line the panel emits:\n  %s\n"+
				"failregex: %s", line, failregex.String())
		}
		host := hostFrom(failregex, match)
		want := sanitizeLogField(event.IP, 64)
		if host != want {
			t.Fatalf("filter captured %q as <HOST>, want %q, from line:\n  %s", host, want, line)
		}
		if ignoreregex != nil && ignoreregex.MatchString(line) {
			t.Fatalf("ignoreregex swallows a line that should be banned:\n  %s", line)
		}
	}

	shouldNotBan := []AuthEvent{
		{Outcome: AuthOutcomeSuccess, Reason: ReasonOK, IP: "203.0.113.9", Account: "admin", Scope: ScopeLogin, Path: "/api/v1/auth/login"},
	}
	for _, event := range shouldNotBan {
		line := event.Line()
		if failregex.MatchString(line) {
			t.Fatalf("the filter would ban a successful authentication:\n  %s", line)
		}
		if ignoreregex != nil && !ignoreregex.MatchString(line) {
			t.Fatalf("ignoreregex should cover a successful authentication:\n  %s", line)
		}
	}
}

// TestFail2banFilterMatchesLinesWithTheDateStripped covers how fail2ban
// actually feeds a line to the failregex: it removes the portion matched by the
// datepattern first, leaving the rest with a leading space.
func TestFail2banFilterMatchesLinesWithTheDateStripped(t *testing.T) {
	failregex, _ := loadFail2banRegexes(t)

	full := sampleEvent().Line()
	stripped := " " + strings.SplitN(full, " ", 2)[1]

	match := failregex.FindStringSubmatch(stripped)
	if match == nil {
		t.Fatalf("the filter does not match once fail2ban has removed the timestamp:\n  %q", stripped)
	}
	if host := hostFrom(failregex, match); host != "203.0.113.9" {
		t.Fatalf("captured %q, want 203.0.113.9", host)
	}
}

// TestFail2banJailPointsAtTheShippedFilter catches the other half of the
// contract: a jail naming a filter that is not installed silently does nothing.
func TestFail2banJailPointsAtTheShippedFilter(t *testing.T) {
	jail := readFail2banFile(t, filepath.Join(fail2banDir, "jail.d", "vkai-panel.conf"))

	settings := parseFail2banSettings(jail)
	if got := settings["filter"]; got != filterFileName {
		t.Fatalf("jail uses filter %q, but the shipped filter file is %s.conf", got, filterFileName)
	}
	if settings["enabled"] != "true" {
		t.Fatalf("the shipped jail is not enabled: %q", settings["enabled"])
	}
	if settings["logpath"] == "" {
		t.Fatal("the jail names no log path")
	}
	if settings["maxretry"] == "" || settings["findtime"] == "" || settings["bantime"] == "" {
		t.Fatalf("the jail is missing one of maxretry/findtime/bantime: %+v", settings)
	}
}

// TestFail2banFilterHasADatePattern guards the setting that decides whether
// findtime means anything at all.
func TestFail2banFilterHasADatePattern(t *testing.T) {
	filter := readFail2banFile(t, filepath.Join(fail2banDir, "filter.d", filterFileName+".conf"))
	if !strings.Contains(filter, "datepattern") {
		t.Fatal("the filter sets no datepattern, so fail2ban would guess at the timestamp " +
			"and findtime would be unreliable")
	}
	// The panel writes RFC3339. Assert the pattern is for that shape rather
	// than a syslog one left over from a copied filter.
	if !strings.Contains(filter, "%%Y-%%m-%%dT%%H:%%M:%%S") {
		t.Fatal("the datepattern does not describe the RFC3339 timestamps the panel writes")
	}
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func readFail2banFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	return string(data)
}

// hostRegex stands in for fail2ban's built-in <HOST>. The real expansion
// differs between fail2ban versions; what this test is asserting is that the
// literal text around <HOST> matches the panel's lines and that <HOST> lands on
// the source address, not that a particular fail2ban release spells the address
// pattern one way or another.
const hostRegex = `(?P<host>[0-9A-Za-z:._-]+)`

func loadFail2banRegexes(t *testing.T) (failregex, ignoreregex *regexp.Regexp) {
	t.Helper()

	filter := readFail2banFile(t, filepath.Join(fail2banDir, "filter.d", filterFileName+".conf"))
	settings := parseFail2banSettings(filter)

	rawFail := settings["failregex"]
	if rawFail == "" {
		t.Fatal("the filter defines no failregex")
	}
	if !strings.Contains(rawFail, "<HOST>") {
		t.Fatalf("the failregex has no <HOST> placeholder, so fail2ban would never learn "+
			"which address to ban: %s", rawFail)
	}

	compiled, err := regexp.Compile(strings.ReplaceAll(rawFail, "<HOST>", hostRegex))
	if err != nil {
		t.Fatalf("failregex does not compile: %v\n%s", err, rawFail)
	}

	if rawIgnore := settings["ignoreregex"]; rawIgnore != "" {
		ignoreregex, err = regexp.Compile(strings.ReplaceAll(rawIgnore, "<HOST>", hostRegex))
		if err != nil {
			t.Fatalf("ignoreregex does not compile: %v\n%s", err, rawIgnore)
		}
	}

	return compiled, ignoreregex
}

// parseFail2banSettings reads the "key = value" pairs out of a fail2ban
// configuration file, joining indented continuation lines and dropping
// comments. It is deliberately small: it only has to understand the files this
// repository ships.
func parseFail2banSettings(content string) map[string]string {
	settings := make(map[string]string)
	current := ""

	for _, raw := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			current = ""
			continue
		}

		indented := raw != strings.TrimLeft(raw, " \t")
		if indented && current != "" {
			settings[current] += "\n" + trimmed
			continue
		}

		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			current = ""
			continue
		}
		current = strings.TrimSpace(key)
		settings[current] = strings.TrimSpace(value)
	}

	return settings
}

func hostFrom(re *regexp.Regexp, match []string) string {
	for i, name := range re.SubexpNames() {
		if name == "host" && i < len(match) {
			return match[i]
		}
	}
	return ""
}
