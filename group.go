package velocity

import (
	"net/http"
	"strings"
)

// Group scopes a path prefix and middleware chain to a set of routes.
type Group struct {
	engine     *Engine
	prefix     string
	middleware []HandlerFunc
}

func cleanGroupPrefix(prefix string) string {
	if prefix == "/" {
		return ""
	}
	return strings.TrimSuffix(prefix, "/")
}

func joinPath(prefix, path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		panic("velocity: route path must start with '/'")
	}
	if prefix == "" {
		return path
	}
	if path == "/" {
		return prefix
	}
	return prefix + path
}

func (g *Group) Group(prefix string, middleware ...HandlerFunc) *Group {
	if _, err := splitRoute(prefix); err != nil {
		panic("velocity: invalid group prefix: " + err.Error())
	}
	validateHandlers(middleware)
	return &Group{engine: g.engine, prefix: joinPath(g.prefix, cleanGroupPrefix(prefix)), middleware: append(append([]HandlerFunc(nil), g.middleware...), middleware...)}
}

func (g *Group) Use(handlers ...HandlerFunc) *Group {
	validateHandlers(handlers)
	g.middleware = append(g.middleware, handlers...)
	return g
}

func (g *Group) Handle(method, path string, handlers ...HandlerFunc) (*Route, error) {
	all := make([]HandlerFunc, 0, len(g.middleware)+len(handlers))
	all = append(all, g.middleware...)
	all = append(all, handlers...)
	return g.engine.Handle(method, joinPath(g.prefix, path), all...)
}

func (g *Group) MustHandle(method, path string, handlers ...HandlerFunc) *Route {
	route, err := g.Handle(method, path, handlers...)
	if err != nil {
		panic("velocity: " + err.Error())
	}
	return route
}
func (g *Group) GET(path string, handlers ...HandlerFunc) *Route {
	return g.MustHandle(http.MethodGet, path, handlers...)
}
func (g *Group) POST(path string, handlers ...HandlerFunc) *Route {
	return g.MustHandle(http.MethodPost, path, handlers...)
}
func (g *Group) PUT(path string, handlers ...HandlerFunc) *Route {
	return g.MustHandle(http.MethodPut, path, handlers...)
}
func (g *Group) PATCH(path string, handlers ...HandlerFunc) *Route {
	return g.MustHandle(http.MethodPatch, path, handlers...)
}
func (g *Group) DELETE(path string, handlers ...HandlerFunc) *Route {
	return g.MustHandle(http.MethodDelete, path, handlers...)
}
func (g *Group) HEAD(path string, handlers ...HandlerFunc) *Route {
	return g.MustHandle(http.MethodHead, path, handlers...)
}
func (g *Group) OPTIONS(path string, handlers ...HandlerFunc) *Route {
	return g.MustHandle(http.MethodOptions, path, handlers...)
}
