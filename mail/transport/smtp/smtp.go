package smtp

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/asimmons91/trails/mail"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
}

type Transport struct {
	cfg Config
}

func New(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

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
