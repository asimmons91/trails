package channels

import "context"

type Broadcaster interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic string) (Subscription, error)
}

type Subscription interface {
	Messages() <-chan []byte
	Unsubscribe() error
}
