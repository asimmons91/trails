package channels

import (
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
