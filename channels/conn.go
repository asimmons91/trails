package channels

import "context"

type Message struct {
	Channel string
	Payload []byte
}

type Conn struct {
	id     string
	outbox chan Message
	subs   map[string]*Subscriber // channel name -> subscriber
}

func (c *Conn) ID() string { return c.id }

func (c *Conn) Outbox() <-chan Message { return c.outbox }

type Subscriber struct {
	conn    *Conn
	channel Channel
	name    string
	params  map[string]string
	hub     *Hub

	topics map[string]struct{} // topics streamed from; mutated only by Hub, under Hub.mu
}

func (s *Subscriber) Params() map[string]string { return s.params }

func (s *Subscriber) StreamFrom(ctx context.Context, topic string) error {
	return s.hub.streamFrom(ctx, s, topic)
}
