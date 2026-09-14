package trails

import "net/http"

type GroupBuilder func(g *Group)

type Group struct {
	prefix     string
	router     *Router
	middleware []MiddlewareFunc
}

func (g *Group) HandleFunc(method, pattern string, h HandlerFunc) {
	final := chain(h, g.middleware)
	g.router.HandleFunc(method, g.prefix+pattern, final)
}

func (g *Group) NewGroup(prefix string, mws ...MiddlewareFunc) *Group {
	combined := append(append([]MiddlewareFunc{}, g.middleware...), mws...)
	return &Group{g.prefix + prefix, g.router, combined}
}

func (g *Group) WithGroup(prefix string, fn GroupBuilder, mws ...MiddlewareFunc) {
	fn(g.NewGroup(prefix, mws...))
}

func (g *Group) Resource(prefix string, r Resource, mw ...MiddlewareFunc) {
	g.WithGroup(prefix, func(gg *Group) {
		gg.Get("/new", r.New)
		gg.Post("", r.Create)
		gg.Get("", r.Show)
		gg.Put("/edit", r.Edit)
		gg.Delete("", r.Destroy)
	}, mw...)
}

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

func (g *Group) Use(mws ...MiddlewareFunc) {
	g.middleware = append(g.middleware, mws...)
}

func (g *Group) Get(pattern string, h HandlerFunc)    { g.HandleFunc(http.MethodGet, pattern, h) }
func (g *Group) Post(pattern string, h HandlerFunc)   { g.HandleFunc(http.MethodPost, pattern, h) }
func (g *Group) Put(pattern string, h HandlerFunc)    { g.HandleFunc(http.MethodPut, pattern, h) }
func (g *Group) Patch(pattern string, h HandlerFunc)  { g.HandleFunc(http.MethodPatch, pattern, h) }
func (g *Group) Delete(pattern string, h HandlerFunc) { g.HandleFunc(http.MethodDelete, pattern, h) }
