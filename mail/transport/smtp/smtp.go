// Package smtp is a mail.Transport that sends via net/smtp.
package smtp

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/asimmons91/trails/mail"
)

// Config holds the SMTP server settings New connects with.
type Config struct {
	Host string
	Port int
	// Username and Password authenticate via PLAIN auth. Leaving Username
	// empty sends without authenticating.
	Username string
	Password string
}

// Transport is a mail.Transport that sends via net/smtp.SendMail.
// Construct one with New.
type Transport struct {
	cfg Config
}

// New returns a Transport that sends through the server described by cfg.
func New(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

// Send builds msg into a raw RFC822 message (mail.BuildRFC822) and sends
// it via net/smtp.SendMail to every address in msg's To, Cc, and Bcc. ctx
// is only checked for cancellation before sending — net/smtp itself isn't
// context-aware, so an already-started send can't be cancelled.
func (t *Transport) Send(ctx context.Context, msg *mail.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	raw, err := mail.BuildRFC822(msg)
	if err != nil {
		return fmt.Errorf("smtp: building message: %w", err)
	}

	var auth smtp.Auth
	if t.cfg.Username != "" {
		auth = smtp.PlainAuth("", t.cfg.Username, t.cfg.Password, t.cfg.Host)
	}

	recipients := make([]string, 0, len(msg.To)+len(msg.Cc)+len(msg.Bcc))
	recipients = append(recipients, msg.To...)
	recipients = append(recipients, msg.Cc...)
	recipients = append(recipients, msg.Bcc...)

	addr := fmt.Sprintf("%s:%d", t.cfg.Host, t.cfg.Port)
	if err := smtp.SendMail(addr, auth, msg.From, recipients, raw); err != nil {
		return fmt.Errorf("smtp: sending: %w", err)
	}

	return nil
}
