package mail

import (
	"context"
	"log/slog"
)

type logTransport struct {
	logger *slog.Logger
}

// NewLogTransport returns a Transport for development and testing: it
// logs each message via logger at Info level instead of sending it, and
// never returns an error.
func NewLogTransport(logger *slog.Logger) Transport {
	return &logTransport{logger: logger}
}

func (t *logTransport) Send(ctx context.Context, msg *Message) error {
	t.logger.InfoContext(ctx, "mail: delivered",
		"from", msg.From,
		"to", msg.To,
		"cc", msg.Cc,
		"bcc", msg.Bcc,
		"subject", msg.Subject,
		"html", msg.HTML,
		"text", msg.Text,
	)
	return nil
}
