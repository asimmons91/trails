package trails

import (
	"context"
	"io/fs"
	"log/slog"
)

type AssetsStrategy string

const (
	AssetsStrategyImportMap AssetsStrategy = "importmap"
	AssetsStrategyBundler   AssetsStrategy = "bundler"
	AssetsStrategyNone      AssetsStrategy = "none"
)

type TrailOptions struct {
	Context        context.Context
	Logger         *slog.Logger
	ViewFS         fs.FS
	AssetsFS       fs.FS
	ConfigFS       fs.FS
	ErrorHandler   ErrorHandlerFunc
	RouteBuilder   RouteBuilder
	Renderer       Renderer
	Binder         Binder
	LayoutName     string
	Host           string
	Port           int
	AssetsStrategy AssetsStrategy
	// Runners are started as background goroutines alongside the HTTP
	// server by Trail.Run, and stopped on the same shutdown signal —
	// typically built via RegisterSpurRunners(mounts...).
	Runners []Runner
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
		Host:           opts.Host,
		Port:           opts.Port,
		AssetsStrategy: opts.AssetsStrategy,
		Runners:        opts.Runners,
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

	return o
}
