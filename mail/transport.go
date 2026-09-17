package mail

import "context"

type Transport interface {
	Send(ctx context.Context, msg *Message) error
}
