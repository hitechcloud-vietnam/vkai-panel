package notify

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// SMTP transport security modes, as written in a channel's config under
// "security".
const (
	// SecurityStartTLS connects in the clear on the submission port and
	// upgrades with STARTTLS. This is the default because it is what port 587
	// does everywhere.
	SecurityStartTLS = "starttls"
	// SecurityTLS connects with TLS from the first byte, which is port 465.
	SecurityTLS = "tls"
	// SecurityNone is plaintext. It is only reachable when the operator asks
	// for it by name, and net/smtp still refuses to send a password over it to
	// anything but loopback.
	SecurityNone = "none"
)

// emailSender delivers over SMTP using only net/smtp and crypto/tls.
//
// Config keys:
//
//	host                  required, the SMTP server
//	port                  default 587
//	username              optional; when empty no AUTH is attempted
//	password              optional, held as a Secret
//	from                  required, the envelope and header sender
//	to                    required, one address or a list
//	security              starttls (default), tls, or none
//	helo                  optional EHLO name, defaults to "localhost"
//	insecure_skip_verify  optional; see the comment on Send
type emailSender struct {
	host               string
	port               int
	username           string
	password           Secret
	from               string
	to                 []string
	security           string
	helo               string
	insecureSkipVerify bool

	dial  func(ctx context.Context, network, address string) (net.Conn, error)
	now   func() time.Time
	scrub *Scrubber
}

// newEmailSender validates a channel config and builds an SMTP sender.
func newEmailSender(cfg Config, deps Deps) (Sender, error) {
	if err := cfg.Require("host", "from"); err != nil {
		return nil, err
	}
	to := cfg.StringList("to")
	if len(to) == 0 {
		return nil, fmt.Errorf("missing required setting(s): to")
	}

	security := strings.ToLower(cfg.String("security"))
	switch security {
	case "":
		security = SecurityStartTLS
	case SecurityStartTLS, SecurityTLS, SecurityNone:
	default:
		return nil, fmt.Errorf("security must be one of %q, %q or %q (got %q)",
			SecurityStartTLS, SecurityTLS, SecurityNone, security)
	}

	port := cfg.Int("port", 0)
	if port == 0 {
		if security == SecurityTLS {
			port = 465
		} else {
			port = 587
		}
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535 (got %d)", port)
	}

	helo := cfg.String("helo")
	if helo == "" {
		helo = "localhost"
	}

	dial := deps.DialContext
	if dial == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		dial = dialer.DialContext
	}

	password := cfg.Secret("password")
	return &emailSender{
		host:               cfg.String("host"),
		port:               port,
		username:           cfg.String("username"),
		password:           password,
		from:               cfg.String("from"),
		to:                 to,
		security:           security,
		helo:               helo,
		insecureSkipVerify: cfg.Bool("insecure_skip_verify", false),
		dial:               dial,
		now:                deps.clock(),
		scrub:              NewScrubber(password.Reveal()),
	}, nil
}

// Type implements Sender.
func (s *emailSender) Type() string { return ChannelEmail }

// Send delivers one message over SMTP.
//
// insecure_skip_verify exists because a self-hosted panel often talks to a
// mail server on the same box with a self-signed certificate, and the
// alternative to offering the setting is operators disabling TLS entirely. It
// defaults to false and has to be asked for.
func (s *emailSender) Send(ctx context.Context, msg Message) error {
	address := net.JoinHostPort(s.host, fmt.Sprint(s.port))

	conn, err := s.dial(ctx, "tcp", address)
	if err != nil {
		return s.scrub.ScrubError(fmt.Errorf("connect to %s: %w", address, err))
	}

	tlsConfig := &tls.Config{
		ServerName:         s.host,
		InsecureSkipVerify: s.insecureSkipVerify, //nolint:gosec // operator-set, defaults to false
		MinVersion:         tls.VersionTLS12,
	}

	if s.security == SecurityTLS {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return s.scrub.ScrubError(fmt.Errorf("TLS handshake with %s: %w", address, err))
		}
		conn = tlsConn
	}

	// The deadline covers the whole conversation. net/smtp has no context
	// support, so the context is honoured by putting its deadline on the
	// socket and by closing the connection when it is cancelled.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return s.scrub.ScrubError(fmt.Errorf("SMTP greeting from %s: %w", address, err))
	}
	defer func() { _ = client.Close() }()

	if err := client.Hello(s.helo); err != nil {
		return s.smtpError("EHLO", err)
	}

	if s.security == SecurityStartTLS {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			// Asking for STARTTLS and not getting it is a configuration
			// mismatch, not a transient fault: the operator has to choose a
			// different port or a different mode.
			return Permanent(fmt.Errorf("%s does not offer STARTTLS; use security \"tls\" on port 465 or \"none\" if this server really is plaintext", address))
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return s.smtpError("STARTTLS", err)
		}
	}

	if s.username != "" {
		// smtp.PlainAuth refuses to hand a password to an unencrypted
		// connection unless the server is loopback. That refusal is correct
		// and is surfaced rather than worked around.
		auth := smtp.PlainAuth("", s.username, s.password.Reveal(), s.host)
		if err := client.Auth(auth); err != nil {
			return s.smtpError("AUTH", err)
		}
	}

	if err := client.Mail(s.from); err != nil {
		return s.smtpError("MAIL FROM", err)
	}
	for _, recipient := range s.to {
		if err := client.Rcpt(recipient); err != nil {
			return s.smtpError("RCPT TO", err)
		}
	}

	writer, err := client.Data()
	if err != nil {
		return s.smtpError("DATA", err)
	}
	if _, err := writer.Write(s.compose(msg)); err != nil {
		_ = writer.Close()
		return s.smtpError("message body", err)
	}
	if err := writer.Close(); err != nil {
		return s.smtpError("end of message", err)
	}

	if err := client.Quit(); err != nil {
		// The message was accepted at end-of-DATA; a rude close afterwards is
		// not a delivery failure and must not cause a duplicate send.
		return nil
	}
	return nil
}

// smtpError classifies an SMTP failure and removes the password from it.
//
// A 5xx reply is the server saying the command itself is unacceptable - bad
// credentials, unknown recipient, message refused - and repeating it changes
// nothing. A 4xx reply is an explicit "try later".
func (s *emailSender) smtpError(stage string, err error) error {
	wrapped := fmt.Errorf("SMTP %s failed: %w", stage, err)
	var protoErr *textproto.Error
	if errors.As(err, &protoErr) && protoErr.Code >= 500 {
		return s.scrub.ScrubError(Permanent(wrapped))
	}
	return s.scrub.ScrubError(wrapped)
}

// compose renders the RFC 5322 message.
//
// The subject is MIME word-encoded and the body is base64 with CRLF line
// breaks, because the panel is used in Vietnam and a subject line reading
// "Đĩa cứng đầy" must arrive as that and not as mojibake or a bounce for a
// line longer than 998 octets.
func (s *emailSender) compose(msg Message) []byte {
	var b strings.Builder
	b.WriteString("From: " + s.from + "\r\n")
	b.WriteString("To: " + strings.Join(s.to, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", msg.Subject) + "\r\n")
	b.WriteString("Date: " + s.now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("X-VKAI-Alert-Kind: " + string(msg.Kind) + "\r\n")
	b.WriteString("X-VKAI-Alert-Severity: " + string(msg.Severity) + "\r\n")
	b.WriteString("\r\n")

	encoded := base64.StdEncoding.EncodeToString([]byte(msg.Body))
	for len(encoded) > 76 {
		b.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	if encoded != "" {
		b.WriteString(encoded + "\r\n")
	}
	return []byte(b.String())
}
