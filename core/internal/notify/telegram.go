package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// telegramAPIBase is Telegram's Bot API. It is overridable through the
// channel's config so a test can point at an httptest server, and so an
// operator running a self-hosted Bot API server can use it.
const telegramAPIBase = "https://api.telegram.org"

// telegramMaxMessage is Telegram's hard limit on a message, in UTF-16 code
// units. Counting runes is close enough and always errs on the short side.
const telegramMaxMessage = 4096

// telegramSender posts to the Bot API with net/http.
//
// Config keys:
//
//	bot_token   required, held as a Secret
//	chat_id     required; a numeric id or an @channelname
//	api_base    optional, defaults to https://api.telegram.org
//	parse_mode  optional; when set it is passed through (MarkdownV2, HTML)
//
// The bot token is in the request path, which is the whole reason this file is
// careful: a URL is what http.Client puts in its errors, so every error
// leaving this sender goes through the scrubber.
type telegramSender struct {
	token     Secret
	chatID    string
	apiBase   string
	parseMode string

	client *http.Client
	scrub  *Scrubber
}

// newTelegramSender validates a channel config and builds a Telegram sender.
func newTelegramSender(cfg Config, deps Deps) (Sender, error) {
	if err := cfg.Require("bot_token", "chat_id"); err != nil {
		return nil, err
	}
	apiBase := strings.TrimRight(cfg.String("api_base"), "/")
	if apiBase == "" {
		apiBase = telegramAPIBase
	}
	token := cfg.Secret("bot_token")
	return &telegramSender{
		token:     token,
		chatID:    cfg.String("chat_id"),
		apiBase:   apiBase,
		parseMode: cfg.String("parse_mode"),
		client:    deps.httpClient(),
		scrub:     NewScrubber(token.Reveal()),
	}, nil
}

// Type implements Sender.
func (s *telegramSender) Type() string { return ChannelTelegram }

// telegramResponse is the envelope every Bot API method answers with.
type telegramResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// Send posts one message to the configured chat.
func (s *telegramSender) Send(ctx context.Context, msg Message) error {
	text := truncateRunes(msg.Subject+"\n\n"+msg.Body, telegramMaxMessage)

	body := map[string]interface{}{
		"chat_id":                  s.chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	}
	if s.parseMode != "" {
		body["parse_mode"] = s.parseMode
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Permanent(fmt.Errorf("encode Telegram request: %w", err))
	}

	endpoint := s.apiBase + "/bot" + s.token.Reveal() + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return s.scrub.ScrubError(Permanent(fmt.Errorf("build Telegram request: %w", err)))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		// The URL, and therefore the bot token, is inside this error.
		return s.scrub.ScrubError(fmt.Errorf("call Telegram: %w", err))
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	var parsed telegramResponse
	_ = json.Unmarshal(payload, &parsed)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 && parsed.OK {
		return nil
	}

	detail := parsed.Description
	if detail == "" {
		detail = strings.TrimSpace(string(payload))
	}
	if detail == "" {
		detail = "no detail returned"
	}

	// Telegram answers 429 with the seconds to wait. The dispatcher's backoff
	// is what actually waits, so the number is carried into the message an
	// operator reads rather than slept on here, where it would hold a worker.
	if parsed.Parameters.RetryAfter > 0 {
		detail = fmt.Sprintf("%s (Telegram asked to retry after %ds)", detail, parsed.Parameters.RetryAfter)
	}

	if err := classifyHTTPStatus(resp.StatusCode, detail); err != nil {
		return s.scrub.ScrubError(fmt.Errorf("Telegram sendMessage: %w", err))
	}

	// A 2xx with ok:false is Telegram rejecting the request itself - a bad
	// chat_id is the usual cause, and it will be just as bad next time.
	return s.scrub.ScrubError(Permanent(fmt.Errorf("Telegram sendMessage refused the request: %s", detail)))
}

// truncateRunes shortens text to at most limit runes, leaving a visible marker
// so nobody reads a cut-off alert as a complete one.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	const marker = "\n[truncated]"
	keep := limit - len([]rune(marker))
	if keep < 0 {
		keep = 0
	}
	return string(runes[:keep]) + marker
}
