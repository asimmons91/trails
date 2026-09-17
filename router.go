package trails

import (
	"errors"
	"io/fs"
	"net/http"
	"slices"
	"sync"
)

type HandlerFunc func(c *Context) error

type MiddlewareFunc func(next HandlerFunc) HandlerFunc

type RouteBuilder func(r *Router)

type Resource interface {
	New(c *Context) error
	Create(c *Context) error
	Show(c *Context) error
	Edit(c *Context) error
	Update(c *Context) error
	Destroy(c *Context) error
}

type Resources interface {
	Resource
	Index(c *Context) error
}

func chain(h HandlerFunc, mws []MiddlewareFunc) HandlerFunc {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}

	return h
}

type Router struct {
	mux          *http.ServeMux
	errorHandler ErrorHandlerFunc
	middleware   []MiddlewareFunc
	pool         *sync.Pool
}

func (rt *Router) shim(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := rt.pool.Get().(*Context)
		defer rt.pool.Put(c)

		c.Reset(w, r)

		err := h(c)
		if err != nil {
			code := http.StatusInternalServerError
			var httpErr HTTPError
			if !errors.As(err, &httpErr) {
				httpErr = NewHTTPError(code, err)
			}

			rt.errorHandler(c, httpErr)
		}
	}
}

func (rt *Router) methodOverride(r *http.Request) *http.Request {
	method := r.FormValue("_method")
	if method == "" {
		method = r.Header.Get("X-Http-Method-Override")
	}

	switch method {
	case http.MethodGet:
		r.Method = http.MethodGet
		return r
	case http.MethodPost:
		r.Method = http.MethodPost
		return r
	case http.MethodPut:
		r.Method = http.MethodPut
		return r
	case http.MethodPatch:
		r.Method = http.MethodPatch
		return r
	case http.MethodDelete:
		r.Method = http.MethodDelete
		return r
	default:
		return r
	}
}

func (rt *Router) HandleFunc(method, pattern string, h HandlerFunc) {
	final := chain(h, rt.middleware)
	rt.mux.HandleFunc(method+" "+pattern, rt.shim(final))
}

func (rt *Router) httpHandle(pattern string, h http.Handler) {
	rt.mux.Handle(pattern, h)
}

// Static mounts fsys at prefix, serving files directly without going through
// the HandlerFunc/Context shim.
func (rt *Router) Static(prefix string, fsys fs.FS) {
	rt.httpHandle(prefix, http.StripPrefix(prefix, http.FileServerFS(fsys)))
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	overrideReq := rt.methodOverride(r)
	rt.mux.ServeHTTP(w, overrideReq)
}

func (rt *Router) NewGroup(prefix string, mws ...MiddlewareFunc) *Group {
	return &Group{prefix, rt, mws}
}

func (rt *Router) WithGroup(prefix string, fn GroupBuilder, mws ...MiddlewareFunc) {
	fn(&Group{prefix, rt, mws})
}

func (rt *Router) Resource(prefix string, r Resource, mw ...MiddlewareFunc) {
	rt.WithGroup(prefix, func(g *Group) {
		g.Get("/new", r.New)
		g.Post("", r.Create)
		g.Get("", r.Show)
		g.Put("/edit", r.Edit)
		g.Delete("", r.Destroy)
	}, mw...)
}

func (rt *Router) Resources(prefix string, r Resources, mw ...MiddlewareFunc) {
	rt.WithGroup(prefix, func(g *Group) {
		g.Get("", r.Index)
		g.Get("/new", r.New)
		g.Post("", r.Create)
		g.Get("/{id}", r.Show)
		g.Get("/{id}/edit", r.Edit)
		g.Put("/{id}", r.Update)
		g.Delete("/{id}", r.Destroy)
	}, mw...)
}

func (rt *Router) Use(mws ...MiddlewareFunc) {
	rt.middleware = append(rt.middleware, mws...)
}

func (rt *Router) Get(pattern string, h HandlerFunc)    { rt.HandleFunc(http.MethodGet, pattern, h) }
func (rt *Router) Post(pattern string, h HandlerFunc)   { rt.HandleFunc(http.MethodPost, pattern, h) }
func (rt *Router) Put(pattern string, h HandlerFunc)    { rt.HandleFunc(http.MethodPut, pattern, h) }
func (rt *Router) Patch(pattern string, h HandlerFunc)  { rt.HandleFunc(http.MethodPatch, pattern, h) }
func (rt *Router) Delete(pattern string, h HandlerFunc) { rt.HandleFunc(http.MethodDelete, pattern, h) }
