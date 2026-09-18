package trails

import (
	"errors"
	"io/fs"
	"net/http"
	"slices"
	"sync"
)

// HandlerFunc handles one request. Route registration methods
// (Get/Post/HandleFunc/etc.) take one per route.
type HandlerFunc func(c *Context) error

// MiddlewareFunc wraps a HandlerFunc to run code before and/or after it.
// Router.Use and Group.Use register these — see Router.Use for how
// Router- and Group-level middleware nest.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// RouteBuilder registers routes on r. TrailOptions.RouteBuilder is called
// once, when Trail is constructed, with the Trail's own Router.
type RouteBuilder func(r *Router)

// Resource is a singular REST-style resource — one with no collection/
// index, e.g. a "current user profile" mounted at one fixed prefix.
// Router.Resource and Group.Resource wire its methods up as: GET
// prefix/new -> New, POST prefix -> Create, GET prefix -> Show, PUT
// prefix/edit -> Edit, DELETE prefix -> Destroy. That wiring never routes
// to Update, and there is no GET prefix/edit route either — Edit itself
// receives the PUT that Resources routes to Update.
type Resource interface {
	New(c *Context) error
	Create(c *Context) error
	Show(c *Context) error
	Edit(c *Context) error
	Update(c *Context) error
	Destroy(c *Context) error
}

// Resources is Resource plus an index action, for a collection of
// resources identified by {id}. Router.Resources and Group.Resources wire
// it up as: GET prefix -> Index, GET prefix/new -> New, POST prefix ->
// Create, GET prefix/{id} -> Show, GET prefix/{id}/edit -> Edit, PUT
// prefix/{id} -> Update, DELETE prefix/{id} -> Destroy — the Edit/Update
// split Resource's own wiring doesn't use.
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

// Router dispatches requests to registered handlers through Go's
// net/http.ServeMux, applying middleware and translating a returned
// error into an HTTP response. There is no exported constructor — an app
// only ever receives one as the RouteBuilder callback's parameter (see
// TrailOptions.RouteBuilder), or a *Group derived from one via NewGroup/
// WithGroup/Resource/Resources.
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

// HandleFunc registers h for method and pattern (Go 1.22+ ServeMux
// syntax, e.g. "/users/{id}"), wrapped by every MiddlewareFunc rt.Use has
// been given so far — see Use for how that interacts with Group
// middleware. Get/Post/Put/Patch/Delete are shorthands for this.
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

// ServeHTTP implements http.Handler: it lets a "_method" form value or
// X-Http-Method-Override header override r.Method to GET/POST/PUT/
// PATCH/DELETE (for HTML forms, which can only submit GET/POST) before
// dispatching to the registered routes.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	overrideReq := rt.methodOverride(r)
	rt.mux.ServeHTTP(w, overrideReq)
}

// NewGroup returns a Group scoped to prefix, with mws as its own
// middleware (see Use for how Group middleware nests under the
// Router's).
func (rt *Router) NewGroup(prefix string, mws ...MiddlewareFunc) *Group {
	return &Group{prefix, rt, mws}
}

// WithGroup calls fn with a fresh Group scoped to prefix and mws — a
// convenience for registering a batch of routes on that Group inline
// without naming it.
func (rt *Router) WithGroup(prefix string, fn GroupBuilder, mws ...MiddlewareFunc) {
	fn(&Group{prefix, rt, mws})
}

// Resource registers r's methods on a Group scoped to prefix — see the
// Resource interface's doc comment for the exact routes.
func (rt *Router) Resource(prefix string, r Resource, mw ...MiddlewareFunc) {
	rt.WithGroup(prefix, func(g *Group) {
		g.Get("/new", r.New)
		g.Post("", r.Create)
		g.Get("", r.Show)
		g.Put("/edit", r.Edit)
		g.Delete("", r.Destroy)
	}, mw...)
}

// Resources registers r's methods on a Group scoped to prefix — see the
// Resources interface's doc comment for the exact routes.
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

// Use appends mws to run around every route registered on rt from this
// call onward — routes already registered before Use is called don't get
// mws (registration snapshots the middleware chain immediately; see
// HandleFunc). Middleware added via Use runs in the order added, and
// wraps outside any Group-level middleware: for a route registered
// through a Group, rt's own Use middleware always runs before that
// Group's, regardless of the order Use and the Group were set up in.
//
//	rt.Use(A)
//	g := rt.NewGroup("/api", B)
//	g.Get("/x", h) // order: A, B, h
func (rt *Router) Use(mws ...MiddlewareFunc) {
	rt.middleware = append(rt.middleware, mws...)
}

// Get registers h for GET requests matching pattern.
func (rt *Router) Get(pattern string, h HandlerFunc) { rt.HandleFunc(http.MethodGet, pattern, h) }

// Post registers h for POST requests matching pattern.
func (rt *Router) Post(pattern string, h HandlerFunc) { rt.HandleFunc(http.MethodPost, pattern, h) }

// Put registers h for PUT requests matching pattern.
func (rt *Router) Put(pattern string, h HandlerFunc) { rt.HandleFunc(http.MethodPut, pattern, h) }

// Patch registers h for PATCH requests matching pattern.
func (rt *Router) Patch(pattern string, h HandlerFunc) { rt.HandleFunc(http.MethodPatch, pattern, h) }

// Delete registers h for DELETE requests matching pattern.
func (rt *Router) Delete(pattern string, h HandlerFunc) {
	rt.HandleFunc(http.MethodDelete, pattern, h)
}
