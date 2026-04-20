package smtp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"sync"

	"github.com/anthropics/mdm-server/internal/config"
)

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

func sendWith(cfg config.SMTPConfig, to, subject, htmlBody string) error {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	fromHeader := cfg.From
	if cfg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", cfg.FromName, cfg.From)
	}

	msg := "From: " + fromHeader + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		htmlBody

	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)

	var err error
	if cfg.TLS {
		err = sendWithTLS(addr, auth, cfg.Host, cfg.From, to, []byte(msg))
	} else {
		err = smtp.SendMail(addr, auth, cfg.From, []string{to}, []byte(msg))
	}
	if err != nil {
		log.Printf("[smtp] send to %s failed: %v", to, err)
		return err
	}
	log.Printf("[smtp] sent to %s: %s", to, subject)
	return nil
}

func sendWithTLS(addr string, auth smtp.Auth, host, from, to string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

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
