package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrUnknownChannel = errors.New("channels: unknown channel")

type Channel interface {
	Name() string
	Subscribed(ctx context.Context, sub *Subscriber, params map[string]string) error
	Unsubscribed(ctx context.Context, sub *Subscriber)
	Receive(ctx context.Context, sub *Subscriber, data json.RawMessage) error
}

type Registry struct {
	factories map[string]func() Channel
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]func() Channel)}
}

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
