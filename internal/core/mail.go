package core

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dcms/internal/store"
)

// MailToSend — письмо в очереди отправки.
type MailToSend struct {
	store.Base
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Time    int64  `json:"time"`
	Tries   int64  `json:"tries"`
}

var mailQ *store.Table[MailToSend, *MailToSend]

func init() {
	Defaults["smtp_host"] = ""
	Defaults["smtp_port"] = "587"
	Defaults["smtp_user"] = ""
	Defaults["smtp_pass"] = ""
	Defaults["smtp_from"] = ""
}

// InitMail регистрирует таблицу очереди писем и фоновую отправку; вызывается из account.Register.
func (a *App) InitMail() {
	if mailQ != nil {
		return
	}
	mailQ = store.NewTable[MailToSend](a.DB, "mail_to_send")
	a.Cron("mail.flush", 20*time.Second, func(a *App) { a.flushMail() })
}

// SendMail ставит письмо (HTML) в очередь. Без SMTP-настроек пишет его в data/mail.log.
func (a *App) SendMail(to, subject, body string) {
	if mailQ == nil || strings.TrimSpace(to) == "" {
		return
	}
	mailQ.Insert(&MailToSend{To: to, Subject: subject, Body: body, Time: Now()})
}

func (a *App) flushMail() {
	for _, m := range mailQ.All() {
		host := a.Cfg.Str("smtp_host")
		if host == "" {
			f, err := os.OpenFile(filepath.Join(a.DataDir, "mail.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err == nil {
				fmt.Fprintf(f, "=== %s → %s\nSubject: %s\n%s\n\n", time.Unix(m.Time, 0).Format(time.RFC3339), m.To, m.Subject, m.Body)
				f.Close()
			}
			mailQ.Delete(m.ID)
			continue
		}
		from := a.Cfg.Str("smtp_from")
		if from == "" {
			from = "noreply@" + host
		}
		msg := "From: " + from + "\r\nTo: " + m.To + "\r\nSubject: =?utf-8?B?" + b64(m.Subject) + "?=\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" + m.Body
		var auth smtp.Auth
		if u := a.Cfg.Str("smtp_user"); u != "" {
			auth = smtp.PlainAuth("", u, a.Cfg.Str("smtp_pass"), host)
		}
		if err := smtp.SendMail(host+":"+a.Cfg.Str("smtp_port"), auth, from, []string{m.To}, []byte(msg)); err != nil {
			log.Printf("mail to %s: %v", m.To, err)
			if m.Tries >= 5 {
				mailQ.Delete(m.ID)
			} else {
				mailQ.Update(m.ID, func(x *MailToSend) { x.Tries++ })
			}
			continue
		}
		mailQ.Delete(m.ID)
	}
}
