package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestSecretDoesNotPrintItself covers the ways a string normally escapes: a
// format verb, a JSON encode, and the %#v that a debugging session reaches
// for. The test asserts on the produced text rather than on the intent.
func TestSecretDoesNotPrintItself(t *testing.T) {
	const password = "hunter2-correct-horse"
	secret := Secret(password)

	for _, format := range []string{"%v", "%s", "%#v", "%+v"} {
		got := fmt.Sprintf(format, secret)
		if strings.Contains(got, password) {
			t.Errorf("Sprintf(%q, secret) leaked the value: %s", format, got)
		}
		if got != Redacted {
			t.Errorf("Sprintf(%q, secret) = %q, want %q", format, got, Redacted)
		}
	}

	encoded, err := json.Marshal(struct {
		Password Secret `json:"password"`
	}{Password: secret})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), password) {
		t.Errorf("JSON encoding leaked the value: %s", encoded)
	}

	if secret.Reveal() != password {
		t.Errorf("Reveal() = %q, want the real value back", secret.Reveal())
	}

	// An unset secret must read as unset, not as a configured-but-hidden one.
	if got := Secret("").String(); got != "" {
		t.Errorf("empty Secret printed %q, want an empty string", got)
	}
}

func TestIsSecretKey(t *testing.T) {
	secret := []string{
		"password", "smtp_password", "smtpPassword", "SMTP PASSWORD",
		"bot_token", "access_token", "refresh_token", "token",
		"secret", "webhook_secret", "api_key", "apiKey",
		"private_key", "credentials", "authorization", "bearer_token",
		"signing_key", "passphrase",
	}
	for _, key := range secret {
		if !IsSecretKey(key) {
			t.Errorf("IsSecretKey(%q) = false, want true", key)
		}
	}

	// Keys that merely mention a word must not be swept up, or a channel
	// config becomes unreadable in the UI.
	notSecret := []string{"host", "port", "from", "to", "chat_id", "user_id", "security", "helo", "method", "dedup_key", "parse_mode"}
	for _, key := range notSecret {
		if IsSecretKey(key) {
			t.Errorf("IsSecretKey(%q) = true, want false", key)
		}
	}
}

func TestRedactConfigReplacesEverySecret(t *testing.T) {
	cfg := map[string]interface{}{
		"host":           "smtp.example.vn",
		"port":           587,
		"username":       "alerts@example.vn",
		"password":       "s3cr3t-smtp-password",
		"bot_token":      "123456:AAH-super-secret-bot-token",
		"url":            "https://hooks.example.vn/services/T000/B111/XXXXsecretXXXX",
		"headers":        map[string]interface{}{"Authorization": "Bearer abcdef123456", "X-Env": "prod"},
		"access_token":   "zalo-oa-access-token-value",
		"empty_password": "",
	}

	redacted := RedactConfig(cfg)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(encoded)

	for _, leak := range []string{
		"s3cr3t-smtp-password",
		"123456:AAH-super-secret-bot-token",
		"XXXXsecretXXXX",
		"Bearer abcdef123456",
		"zalo-oa-access-token-value",
	} {
		if strings.Contains(body, leak) {
			t.Errorf("RedactConfig leaked %q in %s", leak, body)
		}
	}

	// The parts an operator needs in order to recognise the channel survive.
	for _, keep := range []string{"smtp.example.vn", "alerts@example.vn", "hooks.example.vn", "prod"} {
		if !strings.Contains(body, keep) {
			t.Errorf("RedactConfig dropped %q, which an operator needs: %s", keep, body)
		}
	}

	// A password that is not set must read as not set, so the UI can tell the
	// difference between "no credential" and "credential, hidden".
	if redacted["empty_password"] != "" {
		t.Errorf("empty secret became %q, want an empty string", redacted["empty_password"])
	}

	// The input is untouched: the caller still needs it to build a sender.
	if cfg["password"] != "s3cr3t-smtp-password" {
		t.Errorf("RedactConfig modified its input")
	}
}

func TestRedactURLKeepsTheHostAndDropsTheToken(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://hooks.slack.com/services/T00/B11/SECRETTOKEN", "https://hooks.slack.com/" + Redacted},
		{"https://example.vn", "https://example.vn"},
		{"https://example.vn/", "https://example.vn"},
		{"https://example.vn/hook?key=secret", "https://example.vn/" + Redacted},
		{"not a url at all", Redacted},
		{"", ""},
	}
	for _, tc := range cases {
		if got := RedactURL(tc.in); got != tc.want {
			t.Errorf("RedactURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestMergeConfigKeepsStoredSecrets is the regression test for the way
// redaction goes wrong: a UI reads a channel, edits an unrelated field, and
// writes the whole object back with the placeholder still in it.
func TestMergeConfigKeepsStoredSecrets(t *testing.T) {
	stored := map[string]interface{}{
		"host":     "smtp.old.vn",
		"password": "the-real-password",
		"url":      "https://hooks.example.vn/services/SECRET",
		"headers":  map[string]interface{}{"Authorization": "Bearer real-token"},
	}
	incoming := map[string]interface{}{
		"host":     "smtp.new.vn",
		"password": Redacted,
		"url":      "https://hooks.example.vn/" + Redacted,
		"headers":  map[string]interface{}{"Authorization": Redacted},
	}

	merged := MergeConfig(stored, incoming)

	if merged["host"] != "smtp.new.vn" {
		t.Errorf("host = %v, want the edited value", merged["host"])
	}
	if merged["password"] != "the-real-password" {
		t.Errorf("password = %v, want the stored credential kept", merged["password"])
	}
	if merged["url"] != "https://hooks.example.vn/services/SECRET" {
		t.Errorf("url = %v, want the stored URL kept", merged["url"])
	}
	headers, ok := merged["headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("headers = %#v, want a nested map", merged["headers"])
	}
	if headers["Authorization"] != "Bearer real-token" {
		t.Errorf("Authorization = %v, want the stored credential kept", headers["Authorization"])
	}

	// A genuinely new secret still replaces the old one.
	replaced := MergeConfig(stored, map[string]interface{}{"password": "a-new-password"})
	if replaced["password"] != "a-new-password" {
		t.Errorf("password = %v, want the new credential written", replaced["password"])
	}

	// A placeholder with nothing stored behind it must not be saved as the
	// literal placeholder.
	fresh := MergeConfig(map[string]interface{}{}, map[string]interface{}{"password": Redacted})
	if _, present := fresh["password"]; present {
		t.Errorf("password = %v, want the placeholder dropped entirely", fresh["password"])
	}
}

func TestScrubberRemovesSecretsFromText(t *testing.T) {
	scrub := NewScrubber("super-secret-token", "smtp-password-value", "", "ab")

	text := "dial https://api.telegram.org/botsuper-secret-token/sendMessage failed; auth smtp-password-value rejected"
	got := scrub.Scrub(text)

	if strings.Contains(got, "super-secret-token") || strings.Contains(got, "smtp-password-value") {
		t.Errorf("Scrub left a secret behind: %s", got)
	}
	if !strings.Contains(got, "api.telegram.org") {
		t.Errorf("Scrub removed the part that makes the error readable: %s", got)
	}

	// A two-character value is not scrubbed: it would mangle every message
	// without protecting a credential that short.
	if strings.Contains(scrub.Scrub("about"), Redacted) {
		t.Errorf("Scrub replaced a value shorter than the minimum")
	}
}

// TestScrubErrorDoesNotLeakThroughUnwrap is the subtle half: wrapping a dirty
// error keeps the dirty text reachable through %+v and errors.Unwrap, which is
// exactly how it gets back into a log.
func TestScrubErrorDoesNotLeakThroughUnwrap(t *testing.T) {
	const token = "123456:AAH-secret-bot-token"
	scrub := NewScrubber(token)

	dirty := fmt.Errorf("post to https://api.telegram.org/bot%s/sendMessage: connection refused", token)
	clean := scrub.ScrubError(dirty)

	if strings.Contains(clean.Error(), token) {
		t.Errorf("ScrubError left the token in the message: %s", clean)
	}
	if strings.Contains(fmt.Sprintf("%+v", clean), token) {
		t.Errorf("ScrubError left the token reachable through %%+v: %+v", clean)
	}
	if inner := errors.Unwrap(clean); inner != nil && strings.Contains(inner.Error(), token) {
		t.Errorf("ScrubError left the token reachable through Unwrap: %v", inner)
	}

	// The permanent marking has to survive scrubbing, or a misconfigured
	// channel would be retried five times instead of dead-lettered at once.
	permanent := scrub.ScrubError(Permanent(dirty))
	if !IsPermanent(permanent) {
		t.Errorf("ScrubError dropped the permanent marking")
	}
	if strings.Contains(permanent.Error(), token) {
		t.Errorf("ScrubError left the token in a permanent error: %s", permanent)
	}
}

func TestScrubberForConfigFindsEverySecret(t *testing.T) {
	scrub := ScrubberForConfig(map[string]interface{}{
		"host":     "smtp.example.vn",
		"password": "smtp-password-here",
		"url":      "https://hooks.example.vn/services/WEBHOOKSECRETPATH?key=querysecret",
		"headers":  map[string]interface{}{"Authorization": "Bearer nested-token-value"},
	})

	text := "failed: smtp-password-here / services/WEBHOOKSECRETPATH / key=querysecret / Bearer nested-token-value at smtp.example.vn"
	got := scrub.Scrub(text)

	for _, leak := range []string{"smtp-password-here", "WEBHOOKSECRETPATH", "querysecret", "nested-token-value"} {
		if strings.Contains(got, leak) {
			t.Errorf("ScrubberForConfig missed %q: %s", leak, got)
		}
	}
	if !strings.Contains(got, "smtp.example.vn") {
		t.Errorf("ScrubberForConfig removed the hostname, which is not a secret: %s", got)
	}
}
