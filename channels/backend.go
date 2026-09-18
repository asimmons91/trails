package channels

import "context"

// Broadcaster fans a published payload out to every current subscriber of
// a topic, across connections and, depending on the implementation,
// across processes. channels/backend/memory and channels/backend/database
// are the two implementations trails ships.
type Broadcaster interface {
	// Publish delivers payload to every current Subscription on topic.
	Publish(ctx context.Context, topic string, payload []byte) error
	// Subscribe returns a Subscription that receives every payload
	// subsequently Published to topic.
	Subscribe(ctx context.Context, topic string) (Subscription, error)
}

// Subscription is one subscriber's view of a Broadcaster topic, returned
// by Broadcaster.Subscribe.
type Subscription interface {
	// Messages delivers each payload Published to the subscribed topic.
	// It is closed when Unsubscribe is called.
	Messages() <-chan []byte
	// Unsubscribe stops delivery and closes the channel returned by
	// Messages. It is safe to call more than once.
	Unsubscribe() error
}
