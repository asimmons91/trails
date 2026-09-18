package trails

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"time"
)

// AssetsStrategy selects how the asset_path/asset_url template helpers
// are backed and whether an import map is rendered. See the
// AssetsStrategy* constants.
type AssetsStrategy string

const (
	// AssetsStrategyImportMap renders a <script type="importmap"> tag
	// from ConfigFS's importmap.toml, resolved against AssetsFS's
	// manifest. This is the default (see WithDefaultOptions).
	AssetsStrategyImportMap AssetsStrategy = "importmap"
	// AssetsStrategyBundler skips loading an import map; assets are
	// referenced via asset_path/asset_url against a bundler-produced
	// manifest instead.
	AssetsStrategyBundler AssetsStrategy = "bundler"
	// AssetsStrategyNone also skips loading an import map — currently
	// identical to AssetsStrategyBundler at runtime, kept as a separate
	// value only to document the app's own intent.
	AssetsStrategyNone AssetsStrategy = "none"
)

// TrailOptions configures a Trail. Pass it through WithDefaultOptions
// before New to fill in unset fields with usable defaults — New itself
// applies none.
type TrailOptions struct {
	// Context is the Trail's own long-lived context, returned by
	// Context.Context (not a per-request context — see Context.Context).
	// It's also the base for Run's shutdown-signal context and the 5
	// second timeout Run gives the server to shut down. Defaults to
	// context.Background().
	Context context.Context
	// Logger is returned by Context.Logger and used by Run's own
	// startup logging. Defaults to slog.Default().
	Logger *slog.Logger
	// ViewFS holds the app's view templates: layouts/*.gohtml plus one
	// directory per controller of action/partial .gohtml files.
	ViewFS fs.FS
	// AssetsFS holds the compiled asset output — a manifest.json at its
	// root (see assets.Compile) — that New loads to resolve
	// asset_path/asset_url and, for AssetsStrategyImportMap, the import
	// map.
	AssetsFS fs.FS
	// ConfigFS holds importmap.toml, read when AssetsStrategy is
	// AssetsStrategyImportMap.
	ConfigFS fs.FS
	// ErrorHandler handles an error a HandlerFunc/MiddlewareFunc
	// returns. Defaults to one that writes err's message and status via
	// http.Error.
	ErrorHandler ErrorHandlerFunc
	// RouteBuilder registers the app's routes; called once, by New,
	// with the Trail's own Router. Defaults to a no-op.
	RouteBuilder RouteBuilder
	// Binder populates a target struct from a request in Context.Bind.
	// Defaults to &DefaultBinder{}.
	Binder Binder
	// LayoutName selects which non-underscore-prefixed file under
	// layouts/ wraps every rendered view; a mismatch is an error at New
	// time. Defaults to "application".
	LayoutName string
	// FuncMap is merged over the framework's built-in template functions
	// (asset_path, etc.), letting apps register their own view helpers —
	// e.g. a "cached" helper closing over a cache.Store for fragment
	// caching (see cache.FetchFragment).
	FuncMap template.FuncMap
	// Host is the address Run listens on. Defaults to "127.0.0.1".
	Host string
	// Port is the port Run listens on. Defaults to 3000.
	Port int
	// AssetsStrategy selects the asset/import-map strategy — see the
	// AssetsStrategy* constants. Defaults to AssetsStrategyImportMap.
	AssetsStrategy AssetsStrategy
	// Runners are started as background goroutines alongside the HTTP
	// server by Trail.Run, and stopped on the same shutdown signal —
	// typically built via RegisterSpurRunners(mounts...).
	Runners []Runner
	// RequestFuncMap, when set, is called once per Render to build a set of
	// template functions scoped to the current request (e.g. a CSRF helper
	// that needs the current session's token) and merged over FuncMap for
	// that render only. Leave nil to skip the extra per-render template
	// clone entirely — see csrf.RequestFuncMap for the built-in use case.
	RequestFuncMap func(c *Context) template.FuncMap

	// ReadHeaderTimeout caps how long reading a request's headers may
	// take. Defaults to 5 seconds.
	ReadHeaderTimeout time.Duration
	// ReadTimeout caps how long reading an entire request (headers and
	// body) may take. Defaults to 30 seconds.
	ReadTimeout time.Duration
	// WriteTimeout caps how long a response may take to write. It defaults
	// to 0 (no limit) because channels/sse.go holds connections open to
	// stream Server-Sent Events for as long as a client stays connected —
	// Go's WriteTimeout doesn't reset while a handler is actively writing,
	// so any fixed default here would silently kill long-lived SSE streams.
	// Set it explicitly if your app doesn't serve long-lived responses.
	WriteTimeout time.Duration
	// IdleTimeout caps how long an idle keep-alive connection is kept
	// open. Defaults to 120 seconds.
	IdleTimeout time.Duration
}

// WithDefaultOptions returns a new TrailOptions with every unset field
// (Go zero value) in opts filled in with a usable default — see each
// field's own doc comment for its default. opts itself is never
// mutated. ViewFS, AssetsFS, ConfigFS, FuncMap, Runners, RequestFuncMap,
// and WriteTimeout have no default and are copied through as-is.
func WithDefaultOptions(opts *TrailOptions) *TrailOptions {
	o := &TrailOptions{
		Context:        opts.Context,
		Logger:         opts.Logger,
		ViewFS:         opts.ViewFS,
		AssetsFS:       opts.AssetsFS,
		ConfigFS:       opts.ConfigFS,
		ErrorHandler:   opts.ErrorHandler,
		RouteBuilder:   opts.RouteBuilder,
		Binder:         opts.Binder,
		LayoutName:     opts.LayoutName,
		FuncMap:        opts.FuncMap,
		Host:           opts.Host,
		Port:           opts.Port,
		AssetsStrategy: opts.AssetsStrategy,
		Runners:        opts.Runners,
		RequestFuncMap: opts.RequestFuncMap,

		ReadHeaderTimeout: opts.ReadHeaderTimeout,
		ReadTimeout:       opts.ReadTimeout,
		WriteTimeout:      opts.WriteTimeout,
		IdleTimeout:       opts.IdleTimeout,
	}

	if o.Context == nil {
		o.Context = context.Background()
	}

	if o.Logger == nil {
		o.Logger = slog.Default()
	}

	if o.ErrorHandler == nil {
		o.ErrorHandler = defaultErrorHandler
	}

	if o.RouteBuilder == nil {
		o.RouteBuilder = func(r *Router) {}
	}

	if o.Binder == nil {
		o.Binder = &DefaultBinder{}
	}

	if o.LayoutName == "" {
		o.LayoutName = "application"
	}

	if o.Host == "" {
		o.Host = "127.0.0.1"
	}

	if o.Port == 0 {
		o.Port = 3000
	}

	if o.AssetsStrategy == "" {
		o.AssetsStrategy = AssetsStrategyImportMap
	}

	if o.ReadHeaderTimeout == 0 {
		o.ReadHeaderTimeout = 5 * time.Second
	}

	if o.ReadTimeout == 0 {
		o.ReadTimeout = 30 * time.Second
	}

	if o.IdleTimeout == 0 {
		o.IdleTimeout = 120 * time.Second
	}

	return o
}
