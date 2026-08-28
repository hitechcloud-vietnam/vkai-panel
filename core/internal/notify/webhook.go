package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Signature headers sent when a webhook channel is configured with a secret.
// The timestamp is signed alongside the body so a captured request cannot be
// replayed indefinitely by a receiver that checks it.
const (
	WebhookSignatureHeader = "X-VKAI-Signature"
	WebhookTimestampHeader = "X-VKAI-Timestamp"
	WebhookEventHeader     = "X-VKAI-Event"
)

// webhookSender posts the alert as JSON to an arbitrary endpoint.
//
// Config keys:
//
//	url            required; https is expected and http is allowed for a
//	               loopback or private receiver
//	method         optional, defaults to POST
//	headers        optional map of extra headers
//	secret         optional; when set, requests are signed (see Send)
//	content_type   optional, defaults to application/json
//
// This is the escape hatch: Slack, Discord, Microsoft Teams, an in-house
// incident bus and a script behind nginx are all "a URL that accepts JSON".
type webhookSender struct {
	endpoint    string
	method      string
	headers     map[string]string
	secret      Secret
	contentType string

	client *http.Client
	now    func() time.Time
	scrub  *Scrubber
}

// newWebhookSender validates a channel config and builds a webhook sender.
func newWebhookSender(cfg Config, deps Deps) (Sender, error) {
	if err := cfg.Require("url"); err != nil {
		return nil, err
	}
	endpoint := cfg.String("url")
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("url is not a valid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("url must be http or https (got %q)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("url has no host")
	}

	method := strings.ToUpper(cfg.String("method"))
	if method == "" {
		method = http.MethodPost
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return nil, fmt.Errorf("method must be POST, PUT or PATCH (got %q)", method)
	}

	contentType := cfg.String("content_type")
	if contentType == "" {
		contentType = "application/json"
	}

	secret := cfg.Secret("secret")
	scrub := NewScrubber(secret.Reveal())
	// The URL's own path and query are frequently the credential - a Slack
	// webhook URL is nothing but a token - so they are scrubbed too.
	if p := strings.TrimPrefix(parsed.Path, "/"); p != "" {
		scrub.Add(p)
	}
	if parsed.RawQuery != "" {
		scrub.Add(parsed.RawQuery)
	}

	return &webhookSender{
		endpoint:    endpoint,
		method:      method,
		headers:     cfg.StringMap("headers"),
		secret:      secret,
		contentType: contentType,
		client:      deps.httpClient(),
		now:         deps.clock(),
		scrub:       scrub,
	}, nil
}

// Type implements Sender.
func (s *webhookSender) Type() string { return ChannelWebhook }

// webhookPayload is the body posted to the endpoint. It carries both the
// rendered text and the structured alert, so a receiver can either forward the
// prose to a chat room or route on the numbers without parsing English.
type webhookPayload struct {
	Event     EventKind `json:"event"`
	Severity  Severity  `json:"severity"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Link      string    `json:"link"`
	Alert     Alert     `json:"alert"`
	SentAt    time.Time `json:"sent_at"`
	Source    string    `json:"source"`
	DedupKey  string    `json:"dedup_key"`
	PanelName string    `json:"panel"`
}

// Send posts one message.
//
// When a secret is configured the request carries
// X-VKAI-Signature: sha256=<hex>, computed over "<timestamp>.<body>" with
// HMAC-SHA256. A receiver that verifies it knows the request came from this
// panel and not from anything else that learned the URL.
func (s *webhookSender) Send(ctx context.Context, msg Message) error {
	payload := webhookPayload{
		Event:     msg.Kind,
		Severity:  msg.Severity,
		Subject:   msg.Subject,
		Body:      msg.Body,
		Link:      msg.Link,
		Alert:     msg.Alert,
		SentAt:    s.now().UTC(),
		Source:    "vkai-panel",
		DedupKey:  msg.Alert.DedupKey,
		PanelName: "vKAI Panel",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Permanent(fmt.Errorf("encode webhook payload: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, s.method, s.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return s.scrub.ScrubError(Permanent(fmt.Errorf("build webhook request: %w", err)))
	}
	req.Header.Set("Content-Type", s.contentType)
	req.Header.Set("User-Agent", "vkai-panel-notify/1")
	req.Header.Set(WebhookEventHeader, string(msg.Kind))
	for name, value := range s.headers {
		req.Header.Set(name, value)
	}

	if !s.secret.Empty() {
		timestamp := strconv.FormatInt(s.now().UTC().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(s.secret.Reveal()))
		mac.Write([]byte(timestamp))
		mac.Write([]byte("."))
		mac.Write(encoded)
		req.Header.Set(WebhookTimestampHeader, timestamp)
		req.Header.Set(WebhookSignatureHeader, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return s.scrub.ScrubError(fmt.Errorf("call webhook %s: %w", RedactURL(s.endpoint), err))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err := classifyHTTPStatus(resp.StatusCode, strings.TrimSpace(string(detail))); err != nil {
		return s.scrub.ScrubError(fmt.Errorf("webhook %s: %w", RedactURL(s.endpoint), err))
	}
	return nil
}
