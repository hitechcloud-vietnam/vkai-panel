package notify

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTP is a real SMTP server on a loopback socket. httptest cannot help
// here - SMTP is not HTTP - so the sender is driven against an actual
// conversation: net/smtp writes the commands, this reads them, and the test
// asserts on what arrived rather than on what was intended.
type fakeSMTP struct {
	listener net.Listener
	mu       sync.Mutex
	sessions []smtpSession

	// authReply lets a test make AUTH fail with a chosen code.
	authReply string
	// dataReply lets a test make the message itself be refused.
	dataReply string
	// offerStartTLS controls whether EHLO advertises STARTTLS.
	offerStartTLS bool

	wg sync.WaitGroup
}

// smtpSession is what one client did.
type smtpSession struct {
	From string
	To   []string
	Data string
	Auth string
}

// newFakeSMTP starts a server and returns it with its host and port.
func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &fakeSMTP{listener: listener}
	server.wg.Add(1)
	go server.serve()
	t.Cleanup(func() {
		_ = listener.Close()
		server.wg.Wait()
	})
	return server
}

// addr returns the host and port the server is on.
func (s *fakeSMTP) addr() (string, int) {
	tcp := s.listener.Addr().(*net.TCPAddr)
	return "127.0.0.1", tcp.Port
}

// config builds the channel config pointing at this server.
func (s *fakeSMTP) config() Config {
	host, port := s.addr()
	return Config{
		"host":     host,
		"port":     port,
		"from":     "alerts@example.vn",
		"to":       "ops@example.vn, oncall@example.vn",
		"security": SecurityNone,
	}
}

// received returns the completed sessions.
func (s *fakeSMTP) received() []smtpSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]smtpSession, len(s.sessions))
	copy(out, s.sessions)
	return out
}

func (s *fakeSMTP) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(conn)
		}()
	}
}

// handle speaks enough SMTP for net/smtp to complete a send.
func (s *fakeSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	reader := bufio.NewReader(conn)
	write := func(format string, args ...interface{}) {
		_, _ = fmt.Fprintf(conn, format+"\r\n", args...)
	}

	write("220 fake.example.vn ESMTP ready")

	session := smtpSession{}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"):
			write("250-fake.example.vn")
			if s.offerStartTLS {
				write("250-STARTTLS")
			}
			write("250 AUTH PLAIN LOGIN")

		case strings.HasPrefix(upper, "HELO"):
			write("250 fake.example.vn")

		case strings.HasPrefix(upper, "AUTH"):
			session.Auth = line
			if s.authReply != "" {
				write("%s", s.authReply)
				continue
			}
			write("235 2.7.0 accepted")

		case strings.HasPrefix(upper, "MAIL FROM"):
			session.From = strings.TrimSpace(line[len("MAIL FROM:"):])
			write("250 2.1.0 ok")

		case strings.HasPrefix(upper, "RCPT TO"):
			session.To = append(session.To, strings.TrimSpace(line[len("RCPT TO:"):]))
			write("250 2.1.5 ok")

		case strings.HasPrefix(upper, "DATA"):
			if s.dataReply != "" {
				write("%s", s.dataReply)
				continue
			}
			write("354 end with <CRLF>.<CRLF>")
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if dataLine == ".\r\n" || dataLine == ".\n" {
					break
				}
				body.WriteString(dataLine)
			}
			session.Data = body.String()
			write("250 2.0.0 queued")

		case strings.HasPrefix(upper, "QUIT"):
			s.mu.Lock()
			s.sessions = append(s.sessions, session)
			s.mu.Unlock()
			write("221 2.0.0 bye")
			return

		case strings.HasPrefix(upper, "RSET"):
			write("250 2.0.0 ok")

		default:
			write("500 5.5.1 unrecognised")
		}
	}
}

// TestEmailSenderDeliversTheMessage drives the SMTP sender end to end and
// checks what actually arrived on the wire.
func TestEmailSenderDeliversTheMessage(t *testing.T) {
	server := newFakeSMTP(t)

	sender, err := newEmailSender(server.config(), Deps{})
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

	sessions := server.received()
	if len(sessions) != 1 {
		t.Fatalf("the server saw %d sessions, want 1", len(sessions))
	}
	session := sessions[0]

	if !strings.Contains(session.From, "alerts@example.vn") {
		t.Errorf("MAIL FROM = %q, want the configured sender", session.From)
	}
	if len(session.To) != 2 {
		t.Fatalf("RCPT TO count = %d, want 2 (the config lists two addresses)", len(session.To))
	}
	if !strings.Contains(session.To[0], "ops@example.vn") || !strings.Contains(session.To[1], "oncall@example.vn") {
		t.Errorf("recipients = %v, want both configured addresses", session.To)
	}

	// The headers have to say it is UTF-8 plain text, or a Vietnamese subject
	// arrives as mojibake.
	for _, want := range []string{
		"Subject: ",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: base64",
		"Auto-Submitted: auto-generated",
	} {
		if !strings.Contains(session.Data, want) {
			t.Errorf("the message is missing the header %q:\n%s", want, session.Data)
		}
	}

	// Decode the body and check the operator's facts survived the transport.
	parts := strings.SplitN(session.Data, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("the message has no header/body separator:\n%s", session.Data)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(parts[1]), "\r\n", ""))
	if err != nil {
		t.Fatalf("decode body: %v\n%s", err, parts[1])
	}
	for _, want := range []string{"web-01.hcm.example.vn", "92.5%", "90%", "https://panel.example.vn/monitoring/servers/"} {
		if !strings.Contains(string(decoded), want) {
			t.Errorf("the delivered body is missing %q:\n%s", want, decoded)
		}
	}
}

// TestEmailSenderEncodesAVietnameseSubject: the panel is used in Vietnam, and
// a subject line with diacritics has to survive both the SMTP transport and
// the reader's mail client.
func TestEmailSenderEncodesAVietnameseSubject(t *testing.T) {
	server := newFakeSMTP(t)
	sender, err := newEmailSender(server.config(), Deps{})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	const subject = "[CRITICAL] máy chủ web-01: đĩa cứng đầy 92.5%"
	const body = "Dung lượng ổ đĩa đã vượt ngưỡng cho phép."

	if err := sender.Send(context.Background(), Message{Kind: KindFiring, Subject: subject, Body: body}); err != nil {
		t.Fatalf("send: %v", err)
	}

	sessions := server.received()
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}

	// Raw UTF-8 must not appear unencoded in the header: that is what produces
	// a bounce or mojibake.
	headerEnd := strings.Index(sessions[0].Data, "\r\n\r\n")
	headers := sessions[0].Data[:headerEnd]
	if strings.Contains(headers, "đĩa") {
		t.Errorf("the subject was written as raw UTF-8 rather than MIME-encoded:\n%s", headers)
	}
	if !strings.Contains(headers, "=?utf-8?") {
		t.Errorf("the subject is not MIME word-encoded:\n%s", headers)
	}

	decoded := decodeBase64Body(t, sessions[0].Data)
	if !strings.Contains(decoded, body) {
		t.Errorf("the Vietnamese body did not survive: %q", decoded)
	}
}

// decodeBase64Body pulls the body out of a delivered message.
func decodeBase64Body(t *testing.T, message string) string {
	t.Helper()
	parts := strings.SplitN(message, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("no header/body separator:\n%s", message)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(parts[1]), "\r\n", ""))
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return string(decoded)
}

// TestEmailSenderAuthFailureIsPermanent: a rejected password does not become
// correct by being sent four more times.
func TestEmailSenderAuthFailureIsPermanent(t *testing.T) {
	server := newFakeSMTP(t)
	server.authReply = "535 5.7.8 Authentication credentials invalid"

	cfg := server.config()
	cfg["username"] = "alerts@example.vn"
	cfg["password"] = "the-smtp-password"

	sender, err := newEmailSender(cfg, Deps{})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	err = sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatalf("a rejected password produced no error")
	}
	if !IsPermanent(err) {
		t.Errorf("a 535 auth failure was not marked permanent: %v", err)
	}
	if !strings.Contains(err.Error(), "535") {
		t.Errorf("the error does not carry the server's reply, which is what an operator needs: %v", err)
	}
	// And the password is not in it.
	if strings.Contains(err.Error(), "the-smtp-password") {
		t.Errorf("the SMTP password leaked into the error: %v", err)
	}
}

// TestEmailSenderTemporaryFailureIsRetried: a mail server saying "later" is
// not a reason to throw an alert away.
func TestEmailSenderTemporaryFailureIsRetried(t *testing.T) {
	server := newFakeSMTP(t)
	server.dataReply = "451 4.3.0 Temporary local problem, try again"

	sender, err := newEmailSender(server.config(), Deps{})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	err = sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatalf("a 451 produced no error")
	}
	if IsPermanent(err) {
		t.Errorf("a 451 temporary failure was marked permanent, which would throw the alert away: %v", err)
	}
}

// TestEmailSenderRefusesToPretendAboutSTARTTLS: asking for STARTTLS against a
// server that does not offer it must fail loudly rather than sending the
// message, and the password, in the clear.
func TestEmailSenderRefusesToPretendAboutSTARTTLS(t *testing.T) {
	server := newFakeSMTP(t)
	server.offerStartTLS = false

	cfg := server.config()
	cfg["security"] = SecurityStartTLS

	sender, err := newEmailSender(cfg, Deps{})
	if err != nil {
		t.Fatalf("build sender: %v", err)
	}

	err = sender.Send(context.Background(), Message{Subject: "s", Body: "b"})
	if err == nil {
		t.Fatalf("STARTTLS was requested, not offered, and the send succeeded anyway")
	}
	if !IsPermanent(err) {
		t.Errorf("a STARTTLS mismatch is a configuration error and should be permanent: %v", err)
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("the error does not name the cause: %v", err)
	}
	if len(server.received()) != 0 {
		t.Errorf("the message was delivered despite the STARTTLS requirement not being met")
	}
}

func TestEmailSenderConfigValidation(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"no host", Config{"from": "a@b.vn", "to": "c@d.vn"}, "host"},
		{"no from", Config{"host": "smtp.vn", "to": "c@d.vn"}, "from"},
		{"no recipients", Config{"host": "smtp.vn", "from": "a@b.vn"}, "to"},
		{"bad security", Config{"host": "smtp.vn", "from": "a@b.vn", "to": "c@d.vn", "security": "ssl"}, "security"},
		{"bad port", Config{"host": "smtp.vn", "from": "a@b.vn", "to": "c@d.vn", "port": 99999}, "port"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newEmailSender(tc.cfg, Deps{})
			if err == nil {
				t.Fatalf("an invalid config built a sender")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name %q", err, tc.wantErr)
			}
		})
	}

	// The port defaults follow the mode, so an operator writing only
	// security: tls gets 465 rather than a connection refused on 587.
	sender, err := newEmailSender(Config{"host": "smtp.vn", "from": "a@b.vn", "to": "c@d.vn", "security": SecurityTLS}, Deps{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := sender.(*emailSender).port; got != 465 {
		t.Errorf("default port for TLS = %d, want 465", got)
	}
}
