package smtp

import (
	"strings"
	"testing"

	"github.com/anthropics/mdm-server/internal/config"
)

func TestBuildMessage_EncodesChineseSubjectAndFromName(t *testing.T) {
	cfg := config.SMTPConfig{
		From:     "noreply@example.com",
		FromName: "資產管理系統",
	}

	built, err := buildMessage(cfg, "user@example.com", "設備維修申請已核准", "<p>內容</p>")
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	msg := string(built.Data)

	// The raw UTF-8 bytes must NOT appear directly in a header line — that's
	// exactly the mojibake bug being fixed.
	if strings.Contains(headerSection(msg), "設備維修申請已核准") {
		t.Error("subject header contains raw UTF-8 bytes; should be RFC 2047 encoded")
	}
	if strings.Contains(headerSection(msg), "資產管理系統") {
		t.Error("From display name contains raw UTF-8 bytes; should be RFC 2047 encoded")
	}

	if !strings.Contains(msg, "Subject: =?utf-8?q?") {
		t.Errorf("expected RFC 2047 encoded-word Subject header, got message:\n%s", msg)
	}
	if !strings.Contains(msg, "From: =?utf-8?q?") {
		t.Errorf("expected RFC 2047 encoded-word From header, got message:\n%s", msg)
	}

	// The body is the one place raw UTF-8 is fine — it's declared UTF-8/8bit
	// via Content-Type/Content-Transfer-Encoding, not subject to header ASCII
	// rules.
	if !strings.Contains(msg, "<p>內容</p>") {
		t.Error("body should contain the raw HTML unchanged")
	}
	if !strings.Contains(msg, "Content-Transfer-Encoding: 8bit") {
		t.Error("expected Content-Transfer-Encoding: 8bit header for the UTF-8 body")
	}
}

func TestBuildMessage_ASCIIOnlyPassesThroughUnencoded(t *testing.T) {
	cfg := config.SMTPConfig{From: "noreply@example.com", FromName: "MDM Console"}

	built, err := buildMessage(cfg, "user@example.com", "Rental Approved", "<p>Hello</p>")
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	msg := string(built.Data)

	// Pure-ASCII values should NOT get wrapped in encoded-words — that would
	// be needless overhead and a behavior change for the common case.
	if strings.Contains(msg, "=?utf-8?") {
		t.Errorf("ASCII-only subject/from-name should not be RFC 2047 encoded, got:\n%s", msg)
	}
	if !strings.Contains(msg, "Subject: Rental Approved") {
		t.Errorf("expected plain Subject header, got:\n%s", msg)
	}
}

func TestBuildMessage_SanitizesHeaderInjection(t *testing.T) {
	cfg := config.SMTPConfig{From: "noreply@example.com"}

	built, err := buildMessage(cfg, "user@example.com", "Subject\r\nBcc: attacker@evil.com", "body")
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	msg := string(built.Data)

	// "Bcc:" as text within the Subject value is harmless; what must NOT
	// happen is the CRLF surviving to start an actual new header line.
	if strings.Contains(headerSection(msg), "\r\nBcc:") {
		t.Errorf("CRLF in subject should be stripped, not allowed to inject a new header line:\n%s", msg)
	}
	wantHeaderLines := 6 // From, To, Subject, MIME-Version, Content-Type, Content-Transfer-Encoding
	if got := len(strings.Split(headerSection(msg), "\r\n")); got != wantHeaderLines {
		t.Errorf("expected exactly %d header lines (no injected extras), got %d:\n%s", wantHeaderLines, got, msg)
	}
}

func TestBuildMessage_RequiresToAndFrom(t *testing.T) {
	if _, err := buildMessage(config.SMTPConfig{From: "noreply@example.com"}, "", "s", "b"); err == nil {
		t.Error("expected error when to is empty")
	}
	if _, err := buildMessage(config.SMTPConfig{}, "user@example.com", "s", "b"); err == nil {
		t.Error("expected error when from is empty")
	}
}

// headerSection returns everything before the blank-line separator, i.e. the
// raw header block, so body content isn't accidentally matched by header
// assertions above.
func headerSection(msg string) string {
	if i := strings.Index(msg, "\r\n\r\n"); i >= 0 {
		return msg[:i]
	}
	return msg
}
