package notify

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"
)

// Redacted is what replaces a secret everywhere one would otherwise be
// printed. It is a fixed string rather than an empty one so that a config
// coming back from the API still shows an operator that a password is set.
const Redacted = "[REDACTED]"

// Secret is a string that refuses to print itself.
//
// Every credential read out of a channel config is held in this type, so that
// the ordinary ways a value leaks - a %v in a log line, a struct dumped into
// an error, a handler returning the struct as JSON - all produce the
// placeholder. Reveal is the single deliberate way out, and every call to it
// is a place worth reading twice.
type Secret string

// String implements fmt.Stringer, which is what %v and %s reach for.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return Redacted
}

// GoString implements fmt.GoStringer, which is what %#v reaches for.
func (s Secret) GoString() string { return s.String() }

// MarshalJSON keeps a Secret out of any response body it is embedded in.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// Reveal returns the underlying value. Call it at the point of use - handing
// the password to the SMTP client - and never to build a string.
func (s Secret) Reveal() string { return string(s) }

// Empty reports whether the secret is unset.
func (s Secret) Empty() bool { return s == "" }

// secretKeyMarkers are the substrings that make a config key a credential.
// They are matched against the key with separators removed, so "smtp_password",
// "smtpPassword" and "SMTP PASSWORD" all match.
var secretKeyMarkers = []string{
	"password",
	"passwd",
	"passphrase",
	"token",
	"secret",
	"apikey",
	"privatekey",
	"accesskey",
	"credential",
	"authorization",
	"bearer",
	"signingkey",
}

// urlKeyMarkers are the config keys whose value is a URL that may itself carry
// a credential in its path or query - a Slack or Discord webhook URL is a
// token with a hostname in front of it.
var urlKeyMarkers = []string{"url", "endpoint", "webhook", "callback"}

// normalizeKey lowercases a key and strips the separators people vary on.
func normalizeKey(key string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsSecretKey reports whether a config key holds a credential.
func IsSecretKey(key string) bool {
	n := normalizeKey(key)
	for _, marker := range secretKeyMarkers {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

// IsURLKey reports whether a config key holds a URL worth trimming on the way
// out. It is checked after IsSecretKey, so "webhook_secret" is treated as a
// secret and not as a URL.
func IsURLKey(key string) bool {
	n := normalizeKey(key)
	for _, marker := range urlKeyMarkers {
		if strings.Contains(n, marker) {
			return true
		}
	}
	return false
}

// RedactURL keeps the part of a URL an operator needs in order to recognise
// which endpoint is configured - the scheme and the host - and drops the path
// and query, which is where the credential lives.
func RedactURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Not something we can trim safely, so nothing survives.
		return Redacted
	}
	out := u.Scheme + "://" + u.Host
	if u.Path == "" || u.Path == "/" {
		if u.RawQuery == "" && u.User == nil {
			return out
		}
	}
	return out + "/" + Redacted
}

// RedactConfig returns a copy of a channel config safe to hand to an API
// client or a log. The input is not modified: the caller usually still needs
// the real values to build a sender.
func RedactConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	out := make(map[string]interface{}, len(cfg))
	for key, value := range cfg {
		switch {
		case IsSecretKey(key):
			// A secret is replaced whatever its type: a number used as a token
			// is still a token, and an absent one stays absent so the UI can
			// tell "not configured" from "configured, hidden".
			if isEmptyValue(value) {
				out[key] = ""
			} else {
				out[key] = Redacted
			}
		case IsURLKey(key):
			if s, ok := value.(string); ok {
				out[key] = RedactURL(s)
			} else {
				out[key] = value
			}
		default:
			switch v := value.(type) {
			case map[string]interface{}:
				// Headers maps are the common nesting, and Authorization
				// lives in one.
				out[key] = RedactConfig(v)
			default:
				out[key] = value
			}
		}
	}
	return out
}

// isEmptyValue reports whether a config value is absent for redaction
// purposes.
func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

// MergeConfig folds a client-supplied config onto the stored one.
//
// It exists because of a specific way redaction goes wrong: the API hands out
// a config with "smtp_password": "[REDACTED]", an operator edits the SMTP host
// in the UI, the UI sends the whole object back, and the stored password
// becomes the literal string "[REDACTED]" - a channel that silently stops
// working, discovered during the next incident. Any value that is the
// placeholder, or a redacted URL, means "leave what is stored alone".
func MergeConfig(stored, incoming map[string]interface{}) map[string]interface{} {
	if incoming == nil {
		return stored
	}
	out := make(map[string]interface{}, len(incoming))
	for key, value := range incoming {
		if isRedactedValue(value) {
			if old, ok := stored[key]; ok {
				out[key] = old
				continue
			}
			// Nothing stored to keep: drop the placeholder rather than saving
			// it, so the channel reads as unconfigured instead of misconfigured.
			continue
		}
		if nested, ok := value.(map[string]interface{}); ok {
			if oldNested, ok := stored[key].(map[string]interface{}); ok {
				out[key] = MergeConfig(oldNested, nested)
				continue
			}
		}
		out[key] = value
	}
	return out
}

// isRedactedValue reports whether a value came back from a redacted response
// rather than from an operator.
func isRedactedValue(v interface{}) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	return s == Redacted || strings.HasSuffix(s, "/"+Redacted)
}

// Scrubber removes known secret values from arbitrary text.
//
// It is the second line of defence, and the one that catches what the type
// system cannot: an SMTP server that echoes a credential back in its refusal,
// or a Telegram transport error that carries the bot token because the token
// is part of the request URL. Errors and log lines pass through here before
// they are written anywhere durable.
type Scrubber struct {
	values []string
}

// NewScrubber builds a scrubber over a set of secret values. Empty and very
// short values are ignored: replacing every occurrence of a two-character
// password would mangle the message without protecting anything.
func NewScrubber(values ...string) *Scrubber {
	s := &Scrubber{}
	for _, v := range values {
		s.Add(v)
	}
	return s
}

// minScrubLength is the shortest value worth replacing. Below it the
// replacement does more damage to the message than the leak does to the
// operator, and a credential that short is already lost.
const minScrubLength = 5

// Add registers another value to remove.
func (s *Scrubber) Add(value string) {
	value = strings.TrimSpace(value)
	if len(value) < minScrubLength {
		return
	}
	for _, existing := range s.values {
		if existing == value {
			return
		}
	}
	s.values = append(s.values, value)
	// Longest first, so a token that contains a shorter secret as a prefix is
	// replaced whole rather than leaving a tail behind.
	sort.Slice(s.values, func(i, j int) bool { return len(s.values[i]) > len(s.values[j]) })
}

// Scrub replaces every registered value in text with the placeholder.
func (s *Scrubber) Scrub(text string) string {
	if s == nil || len(s.values) == 0 {
		return text
	}
	for _, v := range s.values {
		text = strings.ReplaceAll(text, v, Redacted)
	}
	return text
}

// ScrubError returns an error whose message has every registered value
// removed. The original is not wrapped, because wrapping keeps the unscrubbed
// text reachable through errors.Unwrap and %+v - which is exactly the path
// that puts it back in a log.
func (s *Scrubber) ScrubError(err error) error {
	if err == nil {
		return nil
	}
	scrubbed := s.Scrub(err.Error())
	if scrubbed == err.Error() {
		return err
	}
	if IsPermanent(err) {
		return Permanent(scrubbedError(scrubbed))
	}
	return scrubbedError(scrubbed)
}

// ScrubberForConfig builds a scrubber from every secret-looking value in a
// channel config, so the dispatcher can clean an error without knowing which
// sender produced it or which fields that sender considers sensitive.
func ScrubberForConfig(cfg map[string]interface{}) *Scrubber {
	s := &Scrubber{}
	collectSecrets(cfg, s)
	return s
}

// collectSecrets walks a config, adding every value under a secret key, plus
// the path and query of every URL value.
func collectSecrets(cfg map[string]interface{}, s *Scrubber) {
	for key, value := range cfg {
		switch v := value.(type) {
		case map[string]interface{}:
			collectSecrets(v, s)
		case string:
			if IsSecretKey(key) {
				s.Add(v)
				continue
			}
			if IsURLKey(key) {
				if u, err := url.Parse(v); err == nil {
					// The credential is in the path or the query, never the
					// host, and the host is what makes the error readable.
					if p := strings.TrimPrefix(u.Path, "/"); p != "" {
						s.Add(p)
					}
					if u.RawQuery != "" {
						s.Add(u.RawQuery)
					}
				}
			}
		}
	}
}

// scrubbedError is a plain error carrying already-cleaned text.
type scrubbedError string

func (e scrubbedError) Error() string { return string(e) }
