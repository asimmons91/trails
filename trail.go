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

type Trail struct {
	router       *Router
	context      context.Context
	contextPool  *sync.Pool
	errorHandler ErrorHandlerFunc
	routeBuilder RouteBuilder
	renderer     Renderer
	binder       Binder
	logger       *slog.Logger

	Host string
	Port int
}

func New(o *TrailOptions) (*Trail, error) {
	t := &Trail{
		context:      o.Context,
		logger:       o.Logger,
		errorHandler: o.ErrorHandler,
		routeBuilder: o.RouteBuilder,
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

	renderer, err := newTemplateRenderer(o.ViewFS, o.LayoutName, assets.FuncMap(manifest, "/assets", importMapTag))
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
	serverErrs := make(chan error, 1)

	go func() {
		t.logger.Info("starting server")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrs <- err
		}
	}()

	select {
	case err := <-serverErrs:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(t.context, 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("server shutdown error: %w", err)
		}
	}

	return nil
}
