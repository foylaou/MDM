package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"

	"github.com/anthropics/mdm-server/internal/config"
)

// Sender implements port.EmailSender via SMTP.
type Sender struct {
	cfg config.SMTPConfig
}

// NewSender returns an EmailSender. Returns nil if SMTP is not configured.
func NewSender(cfg config.SMTPConfig) *Sender {
	if cfg.Host == "" || cfg.From == "" {
		return nil
	}
	return &Sender{cfg: cfg}
}

func (s *Sender) Send(_ context.Context, to string, subject string, htmlBody string) error {
	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)

	fromHeader := s.cfg.From
	if s.cfg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.cfg.FromName, s.cfg.From)
	}

	msg := "From: " + fromHeader + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		htmlBody

	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	var err error
	if s.cfg.TLS {
		err = sendWithTLS(addr, auth, s.cfg.Host, s.cfg.From, to, []byte(msg))
	} else {
		err = smtp.SendMail(addr, auth, s.cfg.From, []string{to}, []byte(msg))
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
