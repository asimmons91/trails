package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrUnknownChannel is returned, wrapped, by Hub.Subscribe when the named
// channel was never registered in the Hub's Registry.
var ErrUnknownChannel = errors.New("channels: unknown channel")

// Channel is an app-defined realtime endpoint: apps implement it and
// register a factory for it in a Registry. Hub calls Subscribed once when
// a connection subscribes, Receive for each client-sent message on that
// subscription, and Unsubscribed once when it ends (explicit unsubscribe
// or the connection disconnecting). A new Channel value is created per
// subscription — Register's factory, not Channel itself, is what's
// shared.
type Channel interface {
	// Name identifies the channel. Nothing in this package calls it —
	// it's for the implementation's own use (e.g. logging).
	Name() string
	// Subscribed is called once a client's subscribe command has created
	// sub. A non-nil error rejects the subscription; StreamFrom is
	// typically called here to start receiving Broadcaster messages.
	Subscribed(ctx context.Context, sub *Subscriber, params map[string]string) error
	// Unsubscribed is called once, when sub's subscription ends, whether
	// by an explicit unsubscribe or the owning connection disconnecting.
	Unsubscribed(ctx context.Context, sub *Subscriber)
	// Receive handles one client-sent message on sub.
	Receive(ctx context.Context, sub *Subscriber, data json.RawMessage) error
}

// Registry maps channel names to factories that construct a fresh Channel
// per subscription. Construct one with NewRegistry.
type Registry struct {
	factories map[string]func() Channel
}

// NewRegistry returns an empty, ready-to-use Registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]func() Channel)}
}

// Register associates name with factory, so a client's "subscribe"
// command naming name creates a fresh Channel via factory for that
// subscription. It panics if name is already registered.
func (r *Registry) Register(name string, factory func() Channel) {
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("channels: channel %q already registered", name))
	}
	r.factories[name] = factory
}

func (r *Registry) new(name string) (Channel, error) {
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownChannel, name)
	}
	return factory(), nil
}
