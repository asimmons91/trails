package channels

import "context"

// Message is one payload delivered to a Conn's Outbox, tagged with the
// subscribed channel name it arrived on.
type Message struct {
	Channel string
	Payload []byte
}

// Conn is one client's realtime connection, created by Hub.Connect.
type Conn struct {
	id     string
	outbox chan Message
	subs   map[string]*Subscriber // channel name -> subscriber
}

// ID uniquely identifies c. Clients present it back in HeaderConnectionID
// on every subsequent command.
func (c *Conn) ID() string { return c.id }

// Outbox delivers every Message destined for c — one per payload
// received on any topic a Subscriber on this connection is streaming
// from via StreamFrom. The handler serving c (see Backend.Routes) reads
// from it and writes each Message out, e.g. as an SSE frame.
func (c *Conn) Outbox() <-chan Message { return c.outbox }

// Subscriber is one Channel's live subscription on one Conn, created by
// Hub.Subscribe and passed to Channel.Subscribed/Receive/Unsubscribed.
type Subscriber struct {
	conn    *Conn
	channel Channel
	name    string
	params  map[string]string
	hub     *Hub

	topics map[string]struct{} // topics streamed from; mutated only by Hub, under Hub.mu
}

// Params returns the params the client passed when subscribing.
func (s *Subscriber) Params() map[string]string { return s.params }

// StreamFrom subscribes s's connection to topic on the Hub's Broadcaster:
// every payload subsequently Published to topic is delivered to s.conn's
// Outbox as a Message tagged with s's channel name. Calling it again for
// a topic s is already streaming from is a safe no-op; the subscription
// is torn down automatically when s's own subscription ends.
func (s *Subscriber) StreamFrom(ctx context.Context, topic string) error {
	return s.hub.streamFrom(ctx, s, topic)
}
