package trails

import "net/http"

// GroupBuilder registers routes on g. Router.WithGroup and Group.WithGroup
// call one immediately with the Group they just created.
type GroupBuilder func(g *Group)

// Group scopes a set of routes under a common prefix and middleware.
// There is no exported constructor — get one via Router.NewGroup/
// WithGroup or Group.NewGroup/WithGroup/Resource/Resources.
type Group struct {
	prefix     string
	router     *Router
	middleware []MiddlewareFunc
}

// HandleFunc registers h for method and pattern under g's prefix,
// wrapped by g's own middleware (see Group.Use) and then, via the
// underlying Router, by the Router's own middleware. Get/Post/Put/Patch/
// Delete are shorthands for this.
func (g *Group) HandleFunc(method, pattern string, h HandlerFunc) {
	final := chain(h, g.middleware)
	g.router.HandleFunc(method, g.prefix+pattern, final)
}

// NewGroup returns a nested Group under g: prefix is appended to g's own
// prefix, and mws is appended after g's own middleware (so g's
// middleware still runs first, then mws). g itself is never mutated.
func (g *Group) NewGroup(prefix string, mws ...MiddlewareFunc) *Group {
	combined := append(append([]MiddlewareFunc{}, g.middleware...), mws...)
	return &Group{g.prefix + prefix, g.router, combined}
}

// WithGroup calls fn with a fresh Group nested under g (see NewGroup) —
// a convenience for registering a batch of routes on it inline without
// naming it.
func (g *Group) WithGroup(prefix string, fn GroupBuilder, mws ...MiddlewareFunc) {
	fn(g.NewGroup(prefix, mws...))
}

// Resource registers r's methods on a nested Group scoped to prefix —
// see the Resource interface's doc comment for the exact routes.
func (g *Group) Resource(prefix string, r Resource, mw ...MiddlewareFunc) {
	g.WithGroup(prefix, func(gg *Group) {
		gg.Get("/new", r.New)
		gg.Post("", r.Create)
		gg.Get("", r.Show)
		gg.Put("/edit", r.Edit)
		gg.Delete("", r.Destroy)
	}, mw...)
}

// Resources registers r's methods on a nested Group scoped to prefix —
// see the Resources interface's doc comment for the exact routes.
func (g *Group) Resources(prefix string, r Resources, mw ...MiddlewareFunc) {
	g.WithGroup(prefix, func(gg *Group) {
		gg.Get("", r.Index)
		gg.Get("/new", r.New)
		gg.Post("", r.Create)
		gg.Get("/{id}", r.Show)
		gg.Get("/{id}/edit", r.Edit)
		gg.Put("/{id}", r.Update)
		gg.Delete("/{id}", r.Destroy)
	}, mw...)
}

// Use appends mws to run around every route registered on g from this
// call onward — see Router.Use for the same registration-snapshotting
// rule and how Router- and Group-level middleware nest.
func (g *Group) Use(mws ...MiddlewareFunc) {
	g.middleware = append(g.middleware, mws...)
}

// Get registers h for GET requests matching pattern.
func (g *Group) Get(pattern string, h HandlerFunc) { g.HandleFunc(http.MethodGet, pattern, h) }

// Post registers h for POST requests matching pattern.
func (g *Group) Post(pattern string, h HandlerFunc) { g.HandleFunc(http.MethodPost, pattern, h) }

// Put registers h for PUT requests matching pattern.
func (g *Group) Put(pattern string, h HandlerFunc) { g.HandleFunc(http.MethodPut, pattern, h) }

// Patch registers h for PATCH requests matching pattern.
func (g *Group) Patch(pattern string, h HandlerFunc) { g.HandleFunc(http.MethodPatch, pattern, h) }

// Delete registers h for DELETE requests matching pattern.
func (g *Group) Delete(pattern string, h HandlerFunc) { g.HandleFunc(http.MethodDelete, pattern, h) }
