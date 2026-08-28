package notify

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sender delivers one message over one channel.
//
// A Sender is built per delivery from the channel's stored config and thrown
// away, so it holds no shared state and needs no locking. The cost of building
// one is a struct literal; the cost of holding SMTP passwords in a long-lived
// registry is a lifetime of them being in memory and in every heap dump.
type Sender interface {
	// Type is the channel type this sender was built for, matching
	// notification_channels.type.
	Type() string

	// Send delivers the message or returns why it could not. An error wrapped
	// with Permanent will not be retried; anything else will be, until the
	// delivery's attempt budget runs out.
	//
	// A Sender is responsible for keeping its own credentials out of the
	// errors it returns. The dispatcher scrubs again from the channel config,
	// but a sender that knows its secret should not rely on that.
	Send(ctx context.Context, msg Message) error
}

// Factory builds a sender from a channel's config. It validates: a factory
// that returns an error means the channel is misconfigured, and the delivery
// is dead-lettered immediately rather than retried five times against a
// missing hostname.
type Factory func(cfg Config, deps Deps) (Sender, error)

// Deps are the process-wide things a sender may need. They are passed in
// rather than reached for, so a test can substitute an httptest client and a
// fixed clock.
type Deps struct {
	// HTTPClient is used by every sender that speaks HTTP. It must have a
	// timeout: a webhook that never answers would otherwise hold a dispatcher
	// slot forever.
	HTTPClient *http.Client

	// Now is the clock. Nil means time.Now.
	Now func() time.Time

	// DialContext is used by the SMTP sender. Nil means a plain net.Dialer.
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
}

// clock returns the dependency's clock or the real one.
func (d Deps) clock() func() time.Time {
	if d.Now != nil {
		return d.Now
	}
	return time.Now
}

// httpClient returns the dependency's client or a sane default.
func (d Deps) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return DefaultHTTPClient()
}

// DefaultHTTPClient is the client used when a caller supplies none. The
// timeout is the whole request, not just the connect: an alerting path must
// not be able to block on a hung endpoint.
func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

// ErrUnknownChannelType is returned when a channel's type has no sender. It is
// a permanent condition: no amount of retrying teaches the panel a new
// protocol.
var ErrUnknownChannelType = errors.New("unknown notification channel type")

// Registry maps a channel type to the factory that builds its sender.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	deps      Deps
}

// NewRegistry returns a registry with every sender this panel ships:
// email over SMTP, Telegram, a generic webhook, and Zalo. See zalo.go for what
// the Zalo sender does and does not do.
func NewRegistry(deps Deps) *Registry {
	r := &Registry{factories: make(map[string]Factory), deps: deps}
	r.Register(ChannelEmail, newEmailSender)
	r.Register(ChannelTelegram, newTelegramSender)
	r.Register(ChannelWebhook, newWebhookSender)
	r.Register(ChannelZalo, newZaloSender)
	return r
}

// Channel type identifiers, matching notification_channels.type.
const (
	ChannelEmail    = "email"
	ChannelTelegram = "telegram"
	ChannelWebhook  = "webhook"
	ChannelZalo     = "zalo"
)

// Register adds or replaces a factory. It is exported so a test can install a
// sender that records what it was asked to send.
func (r *Registry) Register(channelType string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[strings.ToLower(strings.TrimSpace(channelType))] = factory
}

// Build constructs a sender for a channel type and config.
func (r *Registry) Build(channelType string, cfg Config) (Sender, error) {
	r.mu.RLock()
	factory, ok := r.factories[strings.ToLower(strings.TrimSpace(channelType))]
	deps := r.deps
	r.mu.RUnlock()

	if !ok {
		return nil, Permanent(fmt.Errorf("%w: %q (supported: %s)",
			ErrUnknownChannelType, channelType, strings.Join(r.Types(), ", ")))
	}
	sender, err := factory(cfg, deps)
	if err != nil {
		// A config that cannot build a sender will not build one on the next
		// attempt either.
		return nil, Permanent(err)
	}
	return sender, nil
}

// Types lists the registered channel types, sorted, for error messages and for
// the API to advertise.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for t := range r.factories {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Supports reports whether a channel type has a sender.
func (r *Registry) Supports(channelType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[strings.ToLower(strings.TrimSpace(channelType))]
	return ok
}

// Config is a channel's config JSONB with accessors that do not panic on a
// value of the wrong type. Everything in it came out of a database column that
// an operator can put anything into, so every read has to survive that.
type Config map[string]interface{}

// String reads a string value, accepting a number or bool written where a
// string was expected - a port typed as 587 rather than "587" is the common
// case and is not worth a configuration error.
func (c Config) String(key string) string {
	v, ok := c[key]
	if !ok || v == nil {
		return ""
	}
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

// Secret reads a credential into a type that will not print itself.
func (c Config) Secret(key string) Secret { return Secret(c.String(key)) }

// Int reads an integer value, falling back to def.
func (c Config) Int(key string, def int) int {
	s := c.String(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// Bool reads a boolean value, falling back to def. "true", "yes", "1" and "on"
// all mean true, because operators write all four.
func (c Config) Bool(key string, def bool) bool {
	v, ok := c[key]
	if !ok || v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	switch strings.ToLower(c.String(key)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	default:
		return def
	}
}

// StringList reads a list of strings, accepting either a JSON array or a
// single string with commas, semicolons or newlines between the entries.
func (c Config) StringList(key string) []string {
	v, ok := c[key]
	if !ok || v == nil {
		return nil
	}
	var out []string
	switch value := v.(type) {
	case []interface{}:
		for _, item := range value {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
	case []string:
		for _, item := range value {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
	default:
		for _, item := range strings.FieldsFunc(c.String(key), func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r'
		}) {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// StringMap reads a map of strings, used for webhook headers.
func (c Config) StringMap(key string) map[string]string {
	v, ok := c[key]
	if !ok || v == nil {
		return nil
	}
	nested, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(nested))
	for k, item := range nested {
		if item == nil {
			continue
		}
		out[k] = fmt.Sprint(item)
	}
	return out
}

// Require reports the first named key that is missing or empty, as a
// configuration error naming every missing key at once - an operator fixing a
// channel should not have to submit four times to learn about four fields.
func (c Config) Require(keys ...string) error {
	var missing []string
	for _, key := range keys {
		if c.String(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required setting(s): %s", strings.Join(missing, ", "))
}

// Redacted returns a copy safe to log or return over the API.
func (c Config) Redacted() map[string]interface{} { return RedactConfig(c) }
