package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// capturedRequest is what a fake endpoint saw.
type capturedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

// ---------------------------------------------------------------
// Telegram
// ---------------------------------------------------------------

const testBotToken = "8123456789:AAHtestbottokenvaluethatmustnotleak"

// TestTelegramSenderPostsTheMessage drives the sender against httptest and
// checks the request Telegram would actually have received.
func TestTelegramSenderPostsTheMessage(t *testing.T) {
	var captured capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Method = r.Method
		captured.Path = r.URL.Path
		captured.Body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	}))
	defer server.Close()

	sender, err := newTelegramSender(Config{
		"bot_token": testBotToken,
		"chat_id":   "-1001234567890",
		"api_base":  server.URL,
	}, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	msg, err := Render(DefaultTemplates(), diskAlert(), RenderOptions{PanelBaseURL: "https://panel.example.vn"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	if captured.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", captured.Method)
	}
	// The token goes in the path; that is the Bot API's shape.
	if captured.Path != "/bot"+testBotToken+"/sendMessage" {
		t.Errorf("path = %q, want /bot<token>/sendMessage", captured.Path)
	}

	var body struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(captured.Body, &body); err != nil {
		t.Fatalf("decode request body: %v\n%s", err, captured.Body)
	}
	if body.ChatID != "-1001234567890" {
		t.Errorf("chat_id = %q, want the configured chat", body.ChatID)
	}
	for _, want := range []string{"web-01.hcm.example.vn", "92.5%", "90%", "https://panel.example.vn/monitoring/servers/"} {
		if !strings.Contains(body.Text, want) {
			t.Errorf("the Telegram text is missing %q:\n%s", want, body.Text)
		}
	}
}

// TestTelegramSenderNeverLeaksTheTokenInAnError is the one that matters: the
// token is in the URL, so every transport error carries it unless it is
// scrubbed.
func TestTelegramSenderNeverLeaksTheTokenInAnError(t *testing.T) {
	// A server that is closed immediately, so the client fails at the
	// transport layer and puts the whole URL in the error.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := server.Client()
	url := server.URL
	server.Close()

	sender, err := newTelegramSender(Config{
		"bot_token": testBotToken,
		"chat_id":   "123",
		"api_base":  url,
	}, Deps{HTTPClient: client})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	err = sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatalf("sending to a closed server produced no error")
	}
	if strings.Contains(err.Error(), testBotToken) {
		t.Fatalf("the bot token leaked into the transport error: %v", err)
	}
	if !strings.Contains(err.Error(), Redacted) {
		t.Errorf("the error does not show that something was removed: %v", err)
	}
}

func TestTelegramSenderErrorClassification(t *testing.T) {
	cases := []struct {
		name          string
		status        int
		body          string
		wantPermanent bool
		wantDetail    string
	}{
		{
			name:          "bad chat id is permanent",
			status:        http.StatusBadRequest,
			body:          `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
			wantPermanent: true,
			wantDetail:    "chat not found",
		},
		{
			name:          "revoked token is permanent",
			status:        http.StatusUnauthorized,
			body:          `{"ok":false,"error_code":401,"description":"Unauthorized"}`,
			wantPermanent: true,
		},
		{
			name:          "rate limit is retried",
			status:        http.StatusTooManyRequests,
			body:          `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":30}}`,
			wantPermanent: false,
			wantDetail:    "retry after 30s",
		},
		{
			name:          "server error is retried",
			status:        http.StatusBadGateway,
			body:          `<html>bad gateway</html>`,
			wantPermanent: false,
		},
		{
			name:          "200 with ok:false is permanent",
			status:        http.StatusOK,
			body:          `{"ok":false,"description":"message text is empty"}`,
			wantPermanent: true,
			wantDetail:    "message text is empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			sender, err := newTelegramSender(Config{
				"bot_token": testBotToken, "chat_id": "1", "api_base": server.URL,
			}, Deps{HTTPClient: server.Client()})
			if err != nil {
				t.Fatalf("build sender: %v", err)
			}

			err = sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
			if err == nil {
				t.Fatalf("status %d produced no error", tc.status)
			}
			if IsPermanent(err) != tc.wantPermanent {
				t.Errorf("IsPermanent = %v, want %v (error: %v)", IsPermanent(err), tc.wantPermanent, err)
			}
			if tc.wantDetail != "" && !strings.Contains(err.Error(), tc.wantDetail) {
				t.Errorf("the error does not carry %q, which is what an operator needs: %v", tc.wantDetail, err)
			}
			if strings.Contains(err.Error(), testBotToken) {
				t.Errorf("the bot token leaked: %v", err)
			}
		})
	}
}

func TestTelegramSenderTruncatesALongMessage(t *testing.T) {
	var body struct {
		Text string `json:"text"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	sender, _ := newTelegramSender(Config{
		"bot_token": testBotToken, "chat_id": "1", "api_base": server.URL,
	}, Deps{HTTPClient: server.Client()})

	if err := sender.Send(context.Background(), Message{Subject: "s", Body: strings.Repeat("đ", 9000)}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if runes := len([]rune(body.Text)); runes > telegramMaxMessage {
		t.Errorf("text is %d runes, want at most %d", runes, telegramMaxMessage)
	}
	if !strings.HasSuffix(body.Text, "[truncated]") {
		t.Errorf("a cut-off message is not marked as cut off, so it reads as complete")
	}
}

// ---------------------------------------------------------------
// Generic webhook
// ---------------------------------------------------------------

func TestWebhookSenderPostsSignedJSON(t *testing.T) {
	const secret = "webhook-shared-secret-value"
	var captured capturedRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Method = r.Method
		captured.Headers = r.Header.Clone()
		captured.Body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sender, err := newWebhookSender(Config{
		"url":     server.URL + "/incidents",
		"secret":  secret,
		"headers": map[string]interface{}{"X-Team": "platform"},
	}, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	msg, err := Render(DefaultTemplates(), diskAlert(), RenderOptions{PanelBaseURL: "https://panel.example.vn"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	if captured.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", captured.Method)
	}
	if got := captured.Headers.Get("X-Team"); got != "platform" {
		t.Errorf("configured header not sent: X-Team = %q", got)
	}
	if got := captured.Headers.Get(WebhookEventHeader); got != string(KindFiring) {
		t.Errorf("%s = %q, want %q", WebhookEventHeader, got, KindFiring)
	}

	// The structured half has to be machine-readable, not just prose.
	var payload webhookPayload
	if err := json.Unmarshal(captured.Body, &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, captured.Body)
	}
	if payload.Alert.ServerName != "web-01.hcm.example.vn" {
		t.Errorf("payload.alert.server_name = %q", payload.Alert.ServerName)
	}
	if payload.Alert.Value != 92.5 || payload.Alert.Threshold != 90 {
		t.Errorf("payload.alert value/threshold = %v/%v, want 92.5/90", payload.Alert.Value, payload.Alert.Threshold)
	}
	if payload.Link == "" || !strings.HasPrefix(payload.Link, "https://panel.example.vn") {
		t.Errorf("payload.link = %q, want the panel link", payload.Link)
	}
	if payload.DedupKey != "server:web-01:disk:/var" {
		t.Errorf("payload.dedup_key = %q", payload.DedupKey)
	}

	// The signature has to verify, or it is decoration.
	timestamp := captured.Headers.Get(WebhookTimestampHeader)
	if timestamp == "" {
		t.Fatalf("no %s header", WebhookTimestampHeader)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(captured.Body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := captured.Headers.Get(WebhookSignatureHeader); got != want {
		t.Errorf("signature = %q, want %q", got, want)
	}

	// The secret itself must not travel.
	if strings.Contains(string(captured.Body), secret) {
		t.Errorf("the shared secret was sent in the body")
	}
	for name, values := range captured.Headers {
		for _, value := range values {
			if strings.Contains(value, secret) {
				t.Errorf("the shared secret was sent in header %s", name)
			}
		}
	}
}

func TestWebhookSenderUnsignedWhenNoSecret(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender, _ := newWebhookSender(Config{"url": server.URL}, Deps{HTTPClient: server.Client()})
	if err := sender.Send(context.Background(), Message{Subject: "s", Body: "b"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if headers.Get(WebhookSignatureHeader) != "" {
		t.Errorf("a signature header was sent for a channel with no secret")
	}
}

func TestWebhookSenderErrorClassification(t *testing.T) {
	cases := []struct {
		status        int
		wantPermanent bool
	}{
		{http.StatusOK, false}, // no error at all; checked separately below
		{http.StatusBadRequest, true},
		{http.StatusUnauthorized, true},
		{http.StatusNotFound, true},
		{http.StatusRequestTimeout, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusServiceUnavailable, false},
	}

	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte("detail from the receiver"))
		}))

		sender, _ := newWebhookSender(Config{"url": server.URL}, Deps{HTTPClient: server.Client()})
		err := sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
		server.Close()

		if tc.status == http.StatusOK {
			if err != nil {
				t.Errorf("status 200 produced an error: %v", err)
			}
			continue
		}
		if err == nil {
			t.Errorf("status %d produced no error", tc.status)
			continue
		}
		if IsPermanent(err) != tc.wantPermanent {
			t.Errorf("status %d: IsPermanent = %v, want %v", tc.status, IsPermanent(err), tc.wantPermanent)
		}
		if !strings.Contains(err.Error(), "detail from the receiver") {
			t.Errorf("status %d: the receiver's own explanation was dropped: %v", tc.status, err)
		}
	}
}

// TestWebhookSenderRedactsTheURLInErrors: a Slack webhook URL is a credential
// with a hostname in front of it.
func TestWebhookSenderRedactsTheURLInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	sender, _ := newWebhookSender(Config{
		"url": server.URL + "/services/T000/B111/SUPERSECRETWEBHOOKPATH",
	}, Deps{HTTPClient: server.Client()})

	err := sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatalf("a 404 produced no error")
	}
	if strings.Contains(err.Error(), "SUPERSECRETWEBHOOKPATH") {
		t.Fatalf("the webhook URL's secret path leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("the error dropped the host, which is what makes it readable: %v", err)
	}
}

func TestWebhookSenderConfigValidation(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"no url", Config{}, "url"},
		{"bad scheme", Config{"url": "ftp://example.vn/hook"}, "http or https"},
		{"no host", Config{"url": "https:///hook"}, "host"},
		{"bad method", Config{"url": "https://example.vn/hook", "method": "DELETE"}, "method"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newWebhookSender(tc.cfg, Deps{}); err == nil {
				t.Fatalf("an invalid config built a sender")
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name %q", err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------
// Zalo
// ---------------------------------------------------------------

const testZaloToken = "zalo-oa-access-token-that-must-not-leak"

func TestZaloSenderPostsAnOAMessage(t *testing.T) {
	var captured capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Method = r.Method
		captured.Path = r.URL.Path
		captured.Headers = r.Header.Clone()
		captured.Body, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"error":0,"message":"Success","data":{"message_id":"abc"}}`))
	}))
	defer server.Close()

	sender, err := newZaloSender(Config{
		"access_token": testZaloToken,
		"user_id":      "1234567890123456789",
		"api_base":     server.URL,
	}, Deps{HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	msg, err := Render(DefaultTemplates(), diskAlert(), RenderOptions{PanelBaseURL: "https://panel.example.vn"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	if captured.Path != zaloMessagePath {
		t.Errorf("path = %q, want %q", captured.Path, zaloMessagePath)
	}
	// Zalo takes the credential in its own header, not in Authorization.
	if got := captured.Headers.Get(zaloTokenHeader); got != testZaloToken {
		t.Errorf("%s header = %q, want the access token", zaloTokenHeader, got)
	}

	var body struct {
		Recipient struct {
			UserID string `json:"user_id"`
		} `json:"recipient"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
	}
	if err := json.Unmarshal(captured.Body, &body); err != nil {
		t.Fatalf("decode body: %v\n%s", err, captured.Body)
	}
	if body.Recipient.UserID != "1234567890123456789" {
		t.Errorf("recipient.user_id = %q", body.Recipient.UserID)
	}
	for _, want := range []string{"web-01.hcm.example.vn", "92.5%", "https://panel.example.vn"} {
		if !strings.Contains(body.Message.Text, want) {
			t.Errorf("the Zalo text is missing %q:\n%s", want, body.Message.Text)
		}
	}
}

// TestZaloSenderExpiredTokenIsPermanent is the decision this sender is built
// around: an expired OA token cannot come back without a human walking through
// Zalo's consent flow, so retrying only delays the dead letter that says so.
func TestZaloSenderExpiredTokenIsPermanent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Zalo answers HTTP 200 with a non-zero error for this.
		_, _ = w.Write([]byte(`{"error":-216,"message":"Access token is invalid"}`))
	}))
	defer server.Close()

	sender, _ := newZaloSender(Config{
		"access_token": testZaloToken, "user_id": "1", "api_base": server.URL,
	}, Deps{HTTPClient: server.Client()})

	err := sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatalf("an invalid access token produced no error")
	}
	if !IsPermanent(err) {
		t.Errorf("an invalid access token was not marked permanent: %v", err)
	}
	if !strings.Contains(err.Error(), "Access token is invalid") {
		t.Errorf("Zalo's own explanation was dropped: %v", err)
	}
	// The error has to tell an operator what to do about it.
	if !strings.Contains(err.Error(), "re-issued") {
		t.Errorf("the error does not say how to fix it: %v", err)
	}
	if strings.Contains(err.Error(), testZaloToken) {
		t.Errorf("the access token leaked: %v", err)
	}
}

func TestZaloSenderServerErrorIsRetried(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("upstream unavailable"))
	}))
	defer server.Close()

	sender, _ := newZaloSender(Config{
		"access_token": testZaloToken, "user_id": "1", "api_base": server.URL,
	}, Deps{HTTPClient: server.Client()})

	err := sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatalf("a 503 produced no error")
	}
	if IsPermanent(err) {
		t.Errorf("a 503 was marked permanent, which would throw the alert away: %v", err)
	}
}

func TestZaloSenderConfigValidation(t *testing.T) {
	if _, err := newZaloSender(Config{"user_id": "1"}, Deps{}); err == nil {
		t.Errorf("a config with no access token built a sender")
	}
	if _, err := newZaloSender(Config{"access_token": "t"}, Deps{}); err == nil {
		t.Errorf("a config with no recipient built a sender")
	}
}

// ---------------------------------------------------------------
// Registry
// ---------------------------------------------------------------

func TestRegistryShipsEverySender(t *testing.T) {
	registry := NewRegistry(Deps{})

	want := []string{ChannelEmail, ChannelTelegram, ChannelWebhook, ChannelZalo}
	for _, channelType := range want {
		if !registry.Supports(channelType) {
			t.Errorf("the registry does not support %q", channelType)
		}
	}
	if got := len(registry.Types()); got != len(want) {
		t.Errorf("registry has %d types (%v), want %d", got, registry.Types(), len(want))
	}

	// An unknown type is permanent: no retry teaches the panel a protocol.
	_, err := registry.Build("carrier-pigeon", Config{})
	if err == nil {
		t.Fatalf("an unknown channel type built a sender")
	}
	if !IsPermanent(err) {
		t.Errorf("an unknown channel type was not permanent: %v", err)
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("the error does not list what is supported: %v", err)
	}

	// A config that cannot build is permanent too, for the same reason.
	_, err = registry.Build(ChannelEmail, Config{})
	if err == nil {
		t.Fatalf("an empty email config built a sender")
	}
	if !IsPermanent(err) {
		t.Errorf("a misconfigured channel was not permanent: %v", err)
	}
}
