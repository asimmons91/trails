package mail

import (
	"context"
	"fmt"
)

type Mailer struct {
	renderer  Renderer
	transport Transport
	from      string
}

func NewMailer(renderer Renderer, transport Transport, from string) Mailer {
	return Mailer{renderer: renderer, transport: transport, from: from}
}

func (m *Mailer) Deliver(ctx context.Context, template string, msg Message, data any) error {
	html, text, err := m.renderer.Render(template, data)
	if err != nil {
		return fmt.Errorf("mail: rendering %s: %w", template, err)
	}

	msg.HTML = html
	msg.Text = text
	if msg.From == "" {
		msg.From = m.from
	}

	if err := m.transport.Send(ctx, &msg); err != nil {
		return fmt.Errorf("mail: sending %s: %w", template, err)
	}

	return nil
}
