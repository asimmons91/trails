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

// Runner is a background loop a host app needs kept alive alongside the HTTP
// server — e.g. a polling Broadcaster's delivery loop, or a jobs.Backend's
// worker loop. Run must return once ctx is cancelled; see Trail.Run and
// RegisterSpurRunners.
type Runner interface {
	Run(ctx context.Context) error
}

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

	Host string
	Port int
}

func New(o *TrailOptions) (*Trail, error) {
	t := &Trail{
		context:      o.Context,
		logger:       o.Logger,
		errorHandler: o.ErrorHandler,
		routeBuilder: o.RouteBuilder,
		binder:       o.Binder,
		runners:      o.Runners,
		Host:         o.Host,
		Port:         o.Port,
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

func (t *Trail) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.router.ServeHTTP(w, r)
}

func (t *Trail) Use(mws ...MiddlewareFunc) {
	t.router.Use(mws...)
}

func (t *Trail) Run() error {
	addr := fmt.Sprintf("%s:%d", t.Host, t.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: t,
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
