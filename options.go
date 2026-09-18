package trails

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"time"
)

type AssetsStrategy string

const (
	AssetsStrategyImportMap AssetsStrategy = "importmap"
	AssetsStrategyBundler   AssetsStrategy = "bundler"
	AssetsStrategyNone      AssetsStrategy = "none"
)

type TrailOptions struct {
	Context      context.Context
	Logger       *slog.Logger
	ViewFS       fs.FS
	AssetsFS     fs.FS
	ConfigFS     fs.FS
	ErrorHandler ErrorHandlerFunc
	RouteBuilder RouteBuilder
	Renderer     Renderer
	Binder       Binder
	LayoutName   string
	// FuncMap is merged over the framework's built-in template functions
	// (asset_path, etc.), letting apps register their own view helpers —
	// e.g. a "cached" helper closing over a cache.Store for fragment
	// caching (see cache.FetchFragment).
	FuncMap        template.FuncMap
	Host           string
	Port           int
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

	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	// WriteTimeout caps how long a response may take to write. It defaults
	// to 0 (no limit) because channels/sse.go holds connections open to
	// stream Server-Sent Events for as long as a client stays connected —
	// Go's WriteTimeout doesn't reset while a handler is actively writing,
	// so any fixed default here would silently kill long-lived SSE streams.
	// Set it explicitly if your app doesn't serve long-lived responses.
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

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
