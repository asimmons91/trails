package channels

import (
	"context"
	"io/fs"
	"log/slog"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/jobs"
)

const (
	defaultCommandPath = "/command"
	defaultHeartbeat   = 20 * time.Second
)

// HeaderConnectionID is the request header a client must set on every
// command POST (see Backend.Routes) to identify which SSE connection the
// command applies to. Its value is the connection_id from the "connected"
// event sent when the stream opened.
const HeaderConnectionID = "X-Cable-Connection-Id"

var _ trails.Spur = (*Backend)(nil)

// Backend wraps a Hub as a trails.Spur, exposing it to clients over
// Server-Sent Events. Construct one with New.
type Backend struct {
	hub *Hub

	commandPath string
	heartbeat   time.Duration
	logger      *slog.Logger
}

// Option configures New.
type Option func(*Backend)

// WithCommandPath overrides the path, relative to the Spur's mount
// prefix, that command POSTs are routed to. The default is "/command".
func WithCommandPath(p string) Option { return func(b *Backend) { b.commandPath = p } }

// WithHeartbeat sets how often an idle SSE stream sends a keep-alive
// comment line, to stop an intermediary (proxy, load balancer) from
// timing out the connection. The default is 20 seconds.
func WithHeartbeat(d time.Duration) Option { return func(b *Backend) { b.heartbeat = d } }

// WithLogger sets the logger used to report a stream write failure or a
// failed command. The default is slog.Default().
func WithLogger(l *slog.Logger) Option { return func(b *Backend) { b.logger = l } }

// New wraps hub as a trails.Spur. It panics if hub is nil.
func New(hub *Hub, opts ...Option) *Backend {
	if hub == nil {
		panic("channels: hub is required")
	}

	b := &Backend{
		hub:         hub,
		commandPath: defaultCommandPath,
		heartbeat:   defaultHeartbeat,
		logger:      slog.Default(),
	}
	for _, opt := range opts {
		opt(b)
	}

	return b
}

// ViewFS always returns nil: channels has no views of its own.
func (b *Backend) ViewFS() fs.FS { return nil }

// AssetsFS always returns nil: channels has no assets of its own.
func (b *Backend) AssetsFS() fs.FS { return nil }

// Routes wires the SSE stream (GET, at the mount's root) and the command
// endpoint (POST, at commandPath) that together implement channels' wire
// protocol: a client GETs the root to open the stream and receive its
// connection_id, then POSTs each subscribe/unsubscribe/message command to
// commandPath with that ID in HeaderConnectionID.
func (b *Backend) Routes(g *trails.Group) {
	g.Get("", b.handleConnect)
	g.Post(b.commandPath, b.handleCommand)
}

// Jobs registers no job kinds: channels has none of its own.
func (b *Backend) Jobs(r *jobs.Registry) {}

// Run delegates to the Hub's Broadcaster's own Run loop, if it implements
// trails.Runner (both the database and memory Broadcasters do — the
// database Broadcaster's is a real polling loop, the memory Broadcaster's
// is a no-op that just waits for ctx to be cancelled) — satisfying
// trails.Runner so this Spur can be picked up by RegisterSpurRunners. A
// Broadcaster that doesn't implement it at all falls back to an immediate
// no-op.
func (b *Backend) Run(ctx context.Context) error {
	r, ok := b.hub.Broadcaster().(trails.Runner)
	if !ok {
		return nil
	}
	return r.Run(ctx)
}
