package notify

// Zalo: what was decided, and why it is shaped like this.
//
// The question asked was whether Zalo can be reached with the standard library
// and no new dependency. The transport answer is yes, and this file is the
// proof: the Zalo Official Account API is HTTPS with a JSON body and the
// credential in an "access_token" header, so net/http and encoding/json are
// the whole of it. No SDK is needed and none is added.
//
// The transport was never the hard part. The credential lifecycle is, and it
// is the reason this sender does one thing and refuses to pretend about the
// rest:
//
//   - An OA access token is short-lived. Refreshing it is a second call, to a
//     different host, with a refresh token - and Zalo rotates the refresh
//     token on every use. A panel that refreshes has to durably store the new
//     refresh token in the same instant it is issued, because losing that one
//     write locks the Official Account out until a human walks back through
//     Zalo's OAuth consent screen. That is a credential store with a rotation
//     protocol, not a sender.
//   - Getting the first token at all requires an interactive OAuth consent
//     flow against an approved Official Account, which a background alerting
//     path cannot perform and this task does not own.
//   - Zalo also restricts which users an OA may message and when, so a
//     delivery can fail for policy reasons that no retry will change.
//
// So the seam is: this sender sends with the token it is given, and never
// tries to refresh. When Zalo rejects the credential the delivery is
// dead-lettered immediately with a message that names re-authorisation as the
// fix, rather than retried five times against a token that cannot come back on
// its own. When the OAuth flow and the rotating-token store are built, they
// hang off the "access_token" config key and nothing here changes.
//
// What is verified and what is not, stated plainly: the request this file
// builds and the responses it classifies are exercised against an httptest
// server in zalo_test.go. Nothing here has been run against Zalo's production
// endpoint, because that needs an approved Official Account. The wire format
// is documented in the constants below so the next person can check it against
// Zalo's documentation without reading the code.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// zaloAPIBase is the Official Account open API host.
	zaloAPIBase = "https://openapi.zalo.me"
	// zaloMessagePath is the "customer service" message endpoint: a reply to a
	// user who has interacted with the OA, which is the category an
	// operational alert to your own staff falls into.
	zaloMessagePath = "/v3.0/oa/message/cs"
	// zaloTokenHeader is where the credential goes. Zalo does not use
	// Authorization: Bearer.
	zaloTokenHeader = "access_token"
	// zaloMaxMessage is a conservative cap on the text field.
	zaloMaxMessage = 2000
)

// zaloSender posts an OA message with net/http and nothing else.
//
// Config keys:
//
//	access_token  required, held as a Secret; see the note above about its
//	              lifetime and why this sender does not refresh it
//	user_id       required, the Zalo user id of the recipient
//	api_base      optional, defaults to https://openapi.zalo.me
type zaloSender struct {
	token   Secret
	userID  string
	apiBase string

	client *http.Client
	scrub  *Scrubber
}

// newZaloSender validates a channel config and builds a Zalo sender.
func newZaloSender(cfg Config, deps Deps) (Sender, error) {
	if err := cfg.Require("access_token", "user_id"); err != nil {
		return nil, err
	}
	apiBase := strings.TrimRight(cfg.String("api_base"), "/")
	if apiBase == "" {
		apiBase = zaloAPIBase
	}
	token := cfg.Secret("access_token")
	return &zaloSender{
		token:   token,
		userID:  cfg.String("user_id"),
		apiBase: apiBase,
		client:  deps.httpClient(),
		scrub:   NewScrubber(token.Reveal()),
	}, nil
}

// Type implements Sender.
func (s *zaloSender) Type() string { return ChannelZalo }

// zaloResponse is the envelope every OA API method answers with. Note that
// Zalo answers HTTP 200 with error != 0 for application-level failures, so the
// status code alone says nothing.
type zaloResponse struct {
	Error   int    `json:"error"`
	Message string `json:"message"`
}

// Send posts one message to the configured Zalo user.
func (s *zaloSender) Send(ctx context.Context, msg Message) error {
	text := truncateRunes(msg.Subject+"\n\n"+msg.Body, zaloMaxMessage)

	body := map[string]interface{}{
		"recipient": map[string]interface{}{"user_id": s.userID},
		"message":   map[string]interface{}{"text": text},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Permanent(fmt.Errorf("encode Zalo request: %w", err))
	}

	endpoint := s.apiBase + zaloMessagePath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return s.scrub.ScrubError(Permanent(fmt.Errorf("build Zalo request: %w", err)))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(zaloTokenHeader, s.token.Reveal())

	resp, err := s.client.Do(req)
	if err != nil {
		return s.scrub.ScrubError(fmt.Errorf("call Zalo: %w", err))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if err := classifyHTTPStatus(resp.StatusCode, strings.TrimSpace(string(payload))); err != nil {
		return s.scrub.ScrubError(fmt.Errorf("Zalo message: %w", err))
	}

	var parsed zaloResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		// A 2xx that is not the documented envelope means the request did not
		// reach the OA API - a captive portal or a proxy answering for it.
		// That can clear on its own, so it is retried.
		return s.scrub.ScrubError(fmt.Errorf("Zalo returned a body that is not an OA API response: %q", truncateRunes(string(payload), 200)))
	}
	if parsed.Error == 0 {
		return nil
	}

	// Every non-zero error from this endpoint is about the credential, the
	// Official Account's approval state, or Zalo's rules on who may be
	// messaged. None of those change by trying again in thirty seconds, and
	// retrying an expired token five times only delays the dead letter that
	// tells the operator to re-authorise.
	return s.scrub.ScrubError(Permanent(fmt.Errorf(
		"Zalo refused the message (error %d: %s). If this is a credential error, the Official Account access token has to be re-issued through Zalo's OAuth consent flow: this panel deliberately does not refresh it automatically",
		parsed.Error, parsed.Message)))
}
