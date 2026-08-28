package wpcli

import (
	"strings"
	"testing"
)

// TestEveryShellMetacharacterIsRefusedByEveryConstructor is the exhaustive
// version of the task's fifth requirement. Rather than picking a few nasty
// strings, it feeds every character a shell treats specially through every
// argument constructor and asserts a refusal.
//
// The point is not that any single one of these would be exploitable today -
// this package never invokes a shell, so it would not be. The point is that the
// day somebody adds a convenience that does build a command line, these
// constructors are already the wall, and this test is what stops the wall being
// quietly lowered to "allow spaces in site names".
func TestEveryShellMetacharacterIsRefusedByEveryConstructor(t *testing.T) {
	constructors := map[string]func(string) (string, error){
		"Slug":        Slug,
		"Login":       Login,
		"TablePrefix": TablePrefix,
		"CoreVersion": func(v string) (string, error) { return CoreVersion(v) },
		"Identifier":  func(v string) (string, error) { return Identifier("database name", v) },
		"UnixName":    func(v string) (string, error) { return UnixName("site user", v) },
	}

	// Every character that changes meaning in sh, plus the line terminators and
	// NUL. The base is a value each constructor would otherwise accept.
	metacharacters := "`$&|;<>()[]{}!*?~#'\"\\ \t\n\r\v\f\x00"

	for name, construct := range constructors {
		for _, char := range metacharacters {
			for _, candidate := range []string{
				"woocommerce" + string(char),
				string(char) + "woocommerce",
				"woo" + string(char) + "commerce",
			} {
				if _, err := construct(candidate); err == nil {
					t.Errorf("%s accepted %q, which contains the shell metacharacter %q",
						name, candidate, string(char))
				}
			}
		}
	}
}

// TestOptionInjectionIsRefused is the attack that survives having no shell.
// An argv element of "--allow-root" where a plugin slug was expected is a
// WP-CLI flag - and --allow-root is precisely the flag this package exists
// never to pass.
func TestOptionInjectionIsRefused(t *testing.T) {
	dangerous := []string{
		"--allow-root",
		"--path=/etc",
		"-a",
		"--skip-plugins",
		"--require=/tmp/evil.php",
		"--ssh=root@example.com",
	}
	for _, value := range dangerous {
		if _, err := Slug(value); err == nil {
			t.Errorf("Slug accepted %q; WP-CLI would read it as an option, not a value", value)
		}
		if _, err := Login(value); err == nil {
			t.Errorf("Login accepted %q", value)
		}
		if _, err := FreeText("search-replace from", value, 2000); err == nil {
			t.Errorf("FreeText accepted %q as a leading option", value)
		}
		if _, err := SiteURL(value); err == nil {
			t.Errorf("SiteURL accepted %q", value)
		}
	}
}

// TestLegitimateValuesAreStillAccepted guards against a validator so strict
// that the feature does not work. Every value here is one a real customer has.
func TestLegitimateValuesAreStillAccepted(t *testing.T) {
	for _, slug := range []string{
		"woocommerce", "wordpress-seo", "wp-super-cache", "classic-editor",
		"advanced-custom-fields", "akismet", "w3-total-cache", "elementor",
		"twentytwentyfour", "really-simple-ssl", "wp_rocket", "gtranslate",
	} {
		if _, err := Slug(slug); err != nil {
			t.Errorf("Slug refused the real plugin slug %q: %v", slug, err)
		}
	}
	for _, login := range []string{"admin", "j.smith", "editor_1", "owner@example.com", "Anna-Marie"} {
		if _, err := Login(login); err != nil {
			t.Errorf("Login refused %q: %v", login, err)
		}
	}
	for _, url := range []string{
		"https://example.com", "http://example.com", "https://staging.example.com",
		"https://example.com/blog", "https://example.co.uk:8443", "https://xn--e1afmkfd.xn--p1ai",
	} {
		if _, err := SiteURL(url); err != nil {
			t.Errorf("SiteURL refused the real URL %q: %v", url, err)
		}
	}
	for _, prefix := range []string{"wp_", "wp5f3_", "myprefix_"} {
		if _, err := TablePrefix(prefix); err != nil {
			t.Errorf("TablePrefix refused %q: %v", prefix, err)
		}
	}
	for _, version := range []string{"latest", "6.5.2", "6.4", "6.5.2.1", ""} {
		if _, err := CoreVersion(version); err != nil {
			t.Errorf("CoreVersion refused %q: %v", version, err)
		}
	}
}

// TestPathValidationRefusesTraversalAndSyntax covers the argument that is both
// a path and an argv element.
func TestPathValidationRefusesTraversalAndSyntax(t *testing.T) {
	bad := []string{
		"", "relative/path", "../../etc/passwd", "/var/www/../../etc",
		"/var/www/$(id)", "/var/www/`id`", "/var/www/a;rm -rf /",
		"/var/www/a\nb", "/var/www/a\x00b", "/var/www/site/../..",
	}
	for _, value := range bad {
		if _, err := Path("site path", value); err == nil {
			t.Errorf("Path accepted %q", value)
		}
	}
	good := map[string]string{
		"/vkai-panel/www/domains/example.com": "/vkai-panel/www/domains/example.com",
		"/vkai-panel/www/domains/site-1":      "/vkai-panel/www/domains/site-1",
		"/vkai-panel/www/domains/x/":          "/vkai-panel/www/domains/x",
	}
	for value, want := range good {
		got, err := Path("site path", value)
		if err != nil {
			t.Errorf("Path refused %q: %v", value, err)
		} else if got != want {
			t.Errorf("Path(%q) = %q, want %q", value, got, want)
		}
	}
}

// TestFreeTextAllowsWhatItMustAndRefusesWhatItMustNot. search-replace operands
// are genuinely free text - a customer replacing "Ltd." with "Limited" is
// ordinary - and that is only safe because there is no shell anywhere in this
// package for them to reach.
func TestFreeTextAllowsWhatItMustAndRefusesWhatItMustNot(t *testing.T) {
	allowed := []string{
		"https://example.com", "Acme Ltd.", "O'Brien & Sons", "50% off!",
		"http://old.example.com/wp-content", "café", "价格",
	}
	for _, value := range allowed {
		if _, err := FreeText("search-replace from", value, 2000); err != nil {
			t.Errorf("FreeText refused the legitimate value %q: %v", value, err)
		}
	}
	if _, err := FreeText("search-replace from", "a\x00b", 2000); err == nil {
		t.Error("FreeText accepted a NUL byte, which would truncate the argv element")
	}
	if _, err := FreeText("search-replace from", strings.Repeat("x", 3000), 2000); err == nil {
		t.Error("FreeText accepted an over-long value")
	}
	if _, err := FreeText("search-replace from", "", 2000); err == nil {
		t.Error("FreeText accepted an empty value")
	}
}

// TestTheMetacharacterErrorNamesTheCharacter: "invalid input" sends an operator
// to read source code.
func TestTheMetacharacterErrorNamesTheCharacter(t *testing.T) {
	_, err := Slug("woo;commerce")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), `";"`) {
		t.Fatalf("the error does not name the offending character: %v", err)
	}
	if !strings.Contains(err.Error(), "refused rather than escaped") {
		t.Fatalf("the error does not say why it was refused rather than escaped: %v", err)
	}
}
