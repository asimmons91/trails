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

const HeaderConnectionID = "X-Cable-Connection-Id"

var _ trails.Spur = (*Backend)(nil)

type Backend struct {
	hub *Hub

	commandPath string
	heartbeat   time.Duration
	logger      *slog.Logger
}

type Option func(*Backend)

func WithCommandPath(p string) Option { return func(b *Backend) { b.commandPath = p } }

func WithHeartbeat(d time.Duration) Option { return func(b *Backend) { b.heartbeat = d } }

func WithLogger(l *slog.Logger) Option { return func(b *Backend) { b.logger = l } }

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

func (b *Backend) ViewFS() fs.FS   { return nil }
func (b *Backend) AssetsFS() fs.FS { return nil }

func (b *Backend) Routes(g *trails.Group) {
	g.Get("", b.handleConnect)
	g.Post(b.commandPath, b.handleCommand)
}

func (b *Backend) Jobs(r *jobs.Registry) {}

// Run delegates to the Hub's Broadcaster's own Run loop, if it has one (e.g.
// the database Broadcaster's polling loop) — satisfying trails.Runner so
// this Spur can be picked up by RegisterSpurRunners. Broadcasters with
// nothing to run (e.g. the memory Broadcaster) make this a harmless
// immediate no-op.
func (b *Backend) Run(ctx context.Context) error {
	r, ok := b.hub.Broadcaster().(trails.Runner)
	if !ok {
		return nil
	}
	return r.Run(ctx)
}
