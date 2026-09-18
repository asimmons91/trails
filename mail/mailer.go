// Package mail renders and sends application email: Mailer.Deliver renders
// a named template through a Renderer and hands the result to a Transport.
// Mailer is meant to be embedded by value in a jobs.Job struct so delivery
// can be enqueued for async sending — its fields are unexported, so they
// never appear in the job's JSON-marshaled args.
package mail

import (
	"context"
	"fmt"
)

// Mailer renders a template via Renderer and sends the result via
// Transport. It embeds cleanly by value into a jobs.Job: its fields are
// unexported, so json.Marshal of the embedding struct only serializes the
// job's own fields.
type Mailer struct {
	renderer  Renderer
	transport Transport
	from      string
}

// NewMailer returns a Mailer that renders through renderer and sends
// through transport, defaulting a Message's From to from when unset (see
// Deliver).
func NewMailer(renderer Renderer, transport Transport, from string) Mailer {
	return Mailer{renderer: renderer, transport: transport, from: from}
}

// Deliver renders template with data, sets the result as msg's HTML and
// Text bodies (overwriting any already there), defaults msg.From to the
// Mailer's from only if it's empty, and sends msg via the Transport.
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
