package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"sync"

	"github.com/anthropics/mdm-server/internal/config"
)

// sanitizeHeader strips CR, LF, and NUL from a header value. Without this an
// attacker who controls `to` or `subject` can inject extra SMTP headers
// (Bcc, Reply-To, even a full second body) by embedding "\r\nBcc: ..." in
// their input — a textbook SMTP header injection.
func sanitizeHeader(v string) string {
	r := strings.NewReplacer("\r", "", "\n", "", "\x00", "")
	return r.Replace(v)
}

// Sender implements port.EmailSender via SMTP. It holds a mutable config so
// the settings controller can hot-reload credentials without a restart.
type Sender struct {
	mu      sync.RWMutex
	cfg     config.SMTPConfig
	enabled bool
}

// NewSender always returns a sender — it may be disabled until SetConfig is
// called with a valid configuration. Callers can safely treat a nil return
// from NewSender as "no SMTP configured"; callers that want hot-reload should
// keep the instance and call SetConfig.
func NewSender(cfg config.SMTPConfig) *Sender {
	s := &Sender{cfg: cfg}
	s.enabled = cfg.Host != "" && cfg.From != ""
	return s
}

// SetConfig replaces the active SMTP config. Passing a zero-value config with
// empty host/from disables the sender.
func (s *Sender) SetConfig(cfg config.SMTPConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	s.enabled = cfg.Host != "" && cfg.From != ""
}

// Config returns a copy of the current config.
func (s *Sender) Config() config.SMTPConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Enabled reports whether the sender has a usable config.
func (s *Sender) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *Sender) Send(_ context.Context, to string, subject string, htmlBody string) error {
	s.mu.RLock()
	cfg := s.cfg
	enabled := s.enabled
	s.mu.RUnlock()

	if !enabled {
		return errors.New("smtp: not configured")
	}
	return sendWith(cfg, to, subject, htmlBody)
}

// SendWith allows callers (e.g. settings test endpoint) to send with an ad-hoc
// config without mutating the registered sender.
func SendWith(cfg config.SMTPConfig, to, subject, htmlBody string) error {
	if cfg.Host == "" || cfg.From == "" {
		return errors.New("smtp: host and from are required")
	}
	return sendWith(cfg, to, subject, htmlBody)
}

// builtMessage is the result of buildMessage: the raw RFC 5322 message bytes
// plus the envelope from/to actually used to send it (post-sanitization).
type builtMessage struct {
	From string
	To   string
	Data []byte
}

// buildMessage assembles the raw SMTP message, including CRLF-injection
// sanitization and RFC 2047 header encoding for non-ASCII subject/display
// names. Pulled out of sendWith so it can be unit tested without a network
// round-trip.
func buildMessage(cfg config.SMTPConfig, to, subject, htmlBody string) (builtMessage, error) {
	// Sanitize every value that ends up on a header line to block CRLF
	// injection. The body itself is the only place CRLF is allowed.
	cleanTo := sanitizeHeader(to)
	cleanSubject := sanitizeHeader(subject)
	cleanFrom := sanitizeHeader(cfg.From)
	cleanFromName := sanitizeHeader(cfg.FromName)
	if cleanTo == "" || cleanFrom == "" {
		return builtMessage{}, errors.New("smtp: to/from required (became empty after sanitization)")
	}

	// RFC 5322 header lines are ASCII-only; a raw UTF-8 subject/display name
	// (e.g. Chinese) renders as mojibake in most mail clients unless wrapped
	// in an RFC 2047 encoded-word. mime.QEncoding.Encode is a no-op for
	// already-ASCII input, so this is safe for English-only values too.
	encodedSubject := mime.QEncoding.Encode("utf-8", cleanSubject)
	encodedFromName := mime.QEncoding.Encode("utf-8", cleanFromName)

	fromHeader := cleanFrom
	if encodedFromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", encodedFromName, cleanFrom)
	}

	msg := "From: " + fromHeader + "\r\n" +
		"To: " + cleanTo + "\r\n" +
		"Subject: " + encodedSubject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n" +
		"\r\n" +
		htmlBody

	return builtMessage{From: cleanFrom, To: cleanTo, Data: []byte(msg)}, nil
}

func sendWith(cfg config.SMTPConfig, to, subject, htmlBody string) error {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	built, err := buildMessage(cfg, to, subject, htmlBody)
	if err != nil {
		return err
	}
	cleanFrom, cleanTo, msg := built.From, built.To, built.Data

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)

	if cfg.TLS {
		err = sendWithTLS(addr, auth, cfg.Host, cleanFrom, cleanTo, msg)
	} else {
		err = smtp.SendMail(addr, auth, cleanFrom, []string{cleanTo}, msg)
	}
	if err != nil {
		log.Printf("[smtp] send to %s failed: %v", cleanTo, err)
		return err
	}
	log.Printf("[smtp] sent to %s: %s", cleanTo, subject)
	return nil
}

// sendWithTLS speaks STARTTLS: connect in plaintext (as the SMTP submission
// port, almost always 587, expects) and then upgrade the connection before
// authenticating. This is NOT implicit/direct TLS (that's port 465, where the
// TLS handshake is the very first bytes on the wire) — dialing straight into
// TLS on a STARTTLS-only port fails immediately with "first record does not
// look like a TLS handshake" because the server's plaintext greeting isn't a
// valid TLS record.
func sendWithTLS(addr string, auth smtp.Auth, host, from, to string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return client.Quit()
}
