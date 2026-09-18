package mail

import "context"

// Transport delivers an already-rendered Message. Deliver calls it after
// filling in msg's HTML/Text bodies.
type Transport interface {
	Send(ctx context.Context, msg *Message) error
}
