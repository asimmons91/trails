// Package trails is a batteries-included Go web framework in the
// style of Rails/Django: Trail is the application (built via New from a
// TrailOptions, and run via Trail.Run), Router/Group register handlers
// and middleware (Get/Post/etc., Use, NewGroup/WithGroup, Resource(s)),
// Context is the per-request handle every HandlerFunc receives, and
// html/template-based views under a ViewFS are rendered via
// Context.Render/RenderBlock. Cross-cutting concerns each live in their
// own subpackage (allowedhosts, cors, session, csrf, secureheaders,
// trustedproxy, ratelimit, ...) as Router.Use middleware, and larger
// optional features (assets, cache, channels, jobs, auth) as separate
// subpackages composed in rather than built into this one; Spur lets a
// subpackage bundle its own views/assets/routes/jobs/background work into
// one mountable unit (see Mount, RegisterSpurRoutes/Jobs/Runners).
//
//	t, err := trails.New(trails.WithDefaultOptions(&trails.TrailOptions{
//	    ViewFS:       viewFS,
//	    AssetsFS:     assetsFS,
//	    RouteBuilder: routes.Build,
//	}))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	log.Fatal(t.Run())
package trails

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/asimmons91/trails/assets"
)

// Runner is a background loop a host app needs kept alive alongside the
// HTTP server — e.g. channels/backend/database.Backend's polling
// delivery loop, or jobs/backend/dbqueue.Backend's worker loop. Run must
// return once ctx is cancelled; see Trail.Run and RegisterSpurRunners.
type Runner interface {
	Run(ctx context.Context) error
}

// Trail is a running trails application: the HTTP handler (via
// ServeHTTP), Router, and any background Runners, all built from a
// TrailOptions by New. Host and Port are read directly by Run to build
// the listen address.
type Trail struct {
	router       *Router
	context      context.Context
	contextPool  *sync.Pool
	errorHandler ErrorHandlerFunc
	routeBuilder RouteBuilder
	renderer     Renderer
	binder       Binder
	logger       *slog.Logger
	runners      []Runner

	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration

	// Host is the address Run listens on.
	Host string
	// Port is the port Run listens on.
	Port int
}

// New builds a Trail from o: it constructs the Router (calling
// o.RouteBuilder once to register routes) and the view renderer (loading
// o.AssetsFS's manifest and, for AssetsStrategyImportMap, o.ConfigFS's
// importmap.toml). Callers should pass o through WithDefaultOptions
// first — New itself applies no defaults, so a zero-value field (e.g. a
// nil Logger) reaches Trail as-is and can panic later (e.g. in Run).
func New(o *TrailOptions) (*Trail, error) {
	t := &Trail{
		context:      o.Context,
		logger:       o.Logger,
		errorHandler: o.ErrorHandler,
		routeBuilder: o.RouteBuilder,
		binder:       o.Binder,
		runners:      o.Runners,

		readHeaderTimeout: o.ReadHeaderTimeout,
		readTimeout:       o.ReadTimeout,
		writeTimeout:      o.WriteTimeout,
		idleTimeout:       o.IdleTimeout,

		Host: o.Host,
		Port: o.Port,
	}
	t.setupPool()
	t.setupRouter()

	err := t.setupViews(o)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (t *Trail) setupViews(o *TrailOptions) error {
	manifest, err := assets.LoadManifest(o.AssetsFS, "manifest.json")
	if err != nil {
		return fmt.Errorf("trails: loading asset manifest: %w", err)
	}

	importMapTag := template.HTML("")
	if o.AssetsStrategy == AssetsStrategyImportMap {
		importMap, err := assets.LoadImportMapConfig(o.ConfigFS, "importmap.toml")
		if err != nil {
			return fmt.Errorf("trails: loading importmap config: %w", err)
		}

		entries, err := importMap.Resolve(manifest, "/assets")
		if err != nil {
			return fmt.Errorf("trails: parsing importmap entries: %w", err)
		}

		importMapTag, err = assets.RenderImportMapTag(entries)
		if err != nil {
			return fmt.Errorf("trails: rendering importmap tags: %w", err)
		}
	}

	funcMap := assets.FuncMap(manifest, "/assets", importMapTag)
	for name, fn := range o.FuncMap {
		funcMap[name] = fn
	}

	renderer, err := newTemplateRenderer(o.ViewFS, o.LayoutName, funcMap, o.RequestFuncMap)
	if err != nil {
		return fmt.Errorf("trails: loading templates: %w", err)
	}

	t.renderer = renderer

	return nil
}

func (t *Trail) setupRouter() {
	t.router = &Router{
		mux:          http.NewServeMux(),
		errorHandler: t.errorHandler,
		middleware:   []MiddlewareFunc{},
		pool:         t.contextPool,
	}

	t.routeBuilder(t.router)
}

func (t *Trail) setupPool() {
	t.contextPool = &sync.Pool{}
	t.contextPool.New = func() any {
		return newContext(nil, nil, t)
	}
}

// ServeHTTP implements http.Handler by delegating to the Router.
func (t *Trail) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.router.ServeHTTP(w, r)
}

// Use appends mws to the Trail's Router — see Router.Use.
func (t *Trail) Use(mws ...MiddlewareFunc) {
	t.router.Use(mws...)
}

// Run starts the HTTP server on Host:Port and every configured Runner
// (TrailOptions.Runners), then blocks until os.Interrupt or SIGTERM, a
// server error, or a Runner error — whichever comes first — and shuts
// everything down: it cancels the Runners' shared context (so they all
// start exiting immediately, even if a Runner error rather than the
// signal is what triggered shutdown), gives the HTTP server 5 seconds to
// finish in-flight requests via Shutdown before force-closing it, then
// waits for every Runner to return. It returns the first server or
// Runner error encountered, or nil on a clean signal-triggered shutdown.
func (t *Trail) Run() error {
	addr := fmt.Sprintf("%s:%d", t.Host, t.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           t,
		ReadHeaderTimeout: t.readHeaderTimeout,
		ReadTimeout:       t.readTimeout,
		WriteTimeout:      t.writeTimeout,
		IdleTimeout:       t.idleTimeout,
	}

	ctx, stop := signal.NotifyContext(t.context, os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 1+len(t.runners))

	go func() {
		t.logger.Info("starting server")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("server error: %w", err)
		}
	}()

	var wg sync.WaitGroup
	for _, r := range t.runners {
		wg.Add(1)
		go func(r Runner) {
			defer wg.Done()
			if err := r.Run(ctx); err != nil {
				errs <- fmt.Errorf("runner error: %w", err)
			}
		}(r)
	}

	var runErr error
	select {
	case err := <-errs:
		runErr = err
	case <-ctx.Done():
	}

	// Cancel explicitly (rather than relying on the deferred stop()) so
	// every runner observes ctx.Done() and starts exiting immediately,
	// even when we got here via the errs branch — otherwise wg.Wait()
	// below would block until Run itself returns and unwinds defers,
	// which can't happen until wg.Wait() unblocks.
	stop()

	shutdownCtx, cancel := context.WithTimeout(t.context, 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && runErr == nil {
		_ = server.Close()
		runErr = fmt.Errorf("server shutdown error: %w", err)
	}

	// Runners observe ctx.Done() (cancelled above by stop(), or already
	// Done if a runner/server error triggered this shutdown) and exit on
	// their own; wait for them so Run doesn't return while they're still
	// mid-flight.
	wg.Wait()

	return runErr
}
