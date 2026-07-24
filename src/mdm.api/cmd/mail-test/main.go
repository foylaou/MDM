// mail-test is a one-off tool to manually verify that outgoing mail (see
// internal/adapter/smtp) sends correctly, in particular that Chinese text in
// the subject/body no longer renders as mojibake after the RFC 2047 header
// encoding fix.
//
// Reads SMTP_* from .env in the current directory (same as the server).
// Usage (run from src/mdm.api so .env is picked up):
//
//	go run ./cmd/mail-test [recipient-email]
//
// Defaults the recipient to s225002731@gmail.com if not given.
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	smtpAdapter "github.com/anthropics/mdm-server/internal/adapter/smtp"
	"github.com/anthropics/mdm-server/internal/config"
)

func main() {
	cfg := config.Load()

	to := "s225002731@gmail.com"
	if len(os.Args) > 1 {
		to = os.Args[1]
	}

	if cfg.SMTP.Host == "" || cfg.SMTP.From == "" {
		log.Fatal("SMTP not configured — fill in SMTP_HOST/SMTP_FROM (and friends) in .env first")
	}

	fmt.Printf("Sending test mail via %s:%s (TLS=%v) as %q <%s> to %s ...\n",
		cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.TLS, cfg.SMTP.FromName, cfg.SMTP.From, to)

	subject := "MDM 測試信 - 中文亂碼修正驗證"
	body := fmt.Sprintf(`<div style="font-family:sans-serif">
  <h2>中文編碼測試</h2>
  <p>這是一封測試信，用來驗證 email 標題與內文的中文顯示是否正常，不再出現亂碼。</p>
  <ul>
    <li>寄件人顯示名稱：%s</li>
    <li>寄送時間：%s</li>
    <li>驗證項目：Subject / From 名稱應正確顯示中文（RFC 2047 編碼修正）</li>
  </ul>
  <p>如果你看到的是正常中文而不是亂碼，代表修正成功。</p>
</div>`, cfg.SMTP.FromName, time.Now().Format("2006-01-02 15:04:05"))

	if err := smtpAdapter.SendWith(cfg.SMTP, to, subject, body); err != nil {
		log.Fatalf("send failed: %v", err)
	}
	fmt.Println("sent OK — check the inbox (and spam folder) for", to)
}
