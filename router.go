package velocity

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type routeNode struct {
	// Most route nodes have one static child. Keep it inline and promote to a
	// map only when a node genuinely branches, avoiding string hashing on the
	// common static-prefix/parameter/static-prefix route shape.
	staticSegment string
	staticSingle  *routeNode
	static        map[string]*routeNode
	exactPath     string
	exactOne      *Route
	exact         map[string]*Route
	param         *routeNode
	paramName     string
	wildcard      *routeNode
	wildName      string
	route         *Route
}

// Route describes one registered endpoint. Its handlers must be treated as
// immutable after registration.
type Route struct {
	Method   string
	Pattern  string
	Handlers []HandlerFunc
}

type router struct {
	mu      sync.RWMutex
	methods map[string]*routeNode
	routes  []*Route
	frozen  atomic.Bool

	// Standard method roots are copied while holding mu in freeze. They remove
	// a map lookup from the common lock-free dispatch path.
	frozenGet     *routeNode
	frozenPost    *routeNode
	frozenPut     *routeNode
	frozenPatch   *routeNode
	frozenDelete  *routeNode
	frozenHead    *routeNode
	frozenOptions *routeNode
	frozenConnect *routeNode
	frozenTrace   *routeNode
}

func newRouter() router {
	return router{methods: make(map[string]*routeNode)}
}

func splitRoute(path string) ([]string, error) {
	if path == "" || path[0] != '/' {
		return nil, fmt.Errorf("route path must start with '/': %q", path)
	}
	if path == "/" {
		return nil, nil
	}
	segments := strings.Split(path[1:], "/")
	for _, segment := range segments {
		if segment == ".." || segment == "." {
			return nil, fmt.Errorf("route path contains disallowed segment: %q", path)
		}
	}
	return segments, nil
}

func validateRouteMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "", fmt.Errorf("HTTP method is required")
	}
	for _, char := range method {
		if !((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", char)) {
			return "", fmt.Errorf("invalid HTTP method: %q", method)
		}
	}
	return method, nil
}

func (r *router) add(method, path string, handlers []HandlerFunc) (*Route, error) {
	method, err := validateRouteMethod(method)
	if err != nil {
		return nil, err
	}
	segments, err := splitRoute(path)
	if err != nil {
		return nil, err
	}
	if len(handlers) == 0 {
		return nil, fmt.Errorf("route %s %s has no handler", method, path)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen.Load() {
		return nil, fmt.Errorf("router is frozen")
	}
	root := r.methods[method]
	if root == nil {
		root = &routeNode{}
		r.methods[method] = root
	}
	node := root
	isStatic := true
	for index, segment := range segments {
		switch {
		case strings.HasPrefix(segment, ":"):
			isStatic = false
			name := strings.TrimPrefix(segment, ":")
			if name == "" || strings.ContainsAny(name, ":*?") {
				return nil, fmt.Errorf("invalid parameter %q in route %s", segment, path)
			}
			if node.param == nil {
				node.param = &routeNode{}
				node.paramName = name
			} else if node.paramName != name {
				return nil, fmt.Errorf("parameter name conflict at %s", path)
			}
			node = node.param
		case strings.HasPrefix(segment, "*"):
			isStatic = false
			name := strings.TrimPrefix(segment, "*")
			if name == "" || index != len(segments)-1 || strings.ContainsAny(name, ":*?") {
				return nil, fmt.Errorf("catch-all must be the final named segment in %s", path)
			}
			if node.wildcard == nil {
				node.wildcard = &routeNode{}
				node.wildName = name
			} else if node.wildName != name {
				return nil, fmt.Errorf("catch-all name conflict at %s", path)
			}
			node = node.wildcard
		default:
			if strings.ContainsAny(segment, ":*") {
				return nil, fmt.Errorf("parameters must occupy an entire segment in %s", path)
			}
			node = node.addStaticChild(segment)
		}
	}
	if node.route != nil {
		return nil, fmt.Errorf("duplicate route %s %s", method, path)
	}
	route := &Route{Method: method, Pattern: path, Handlers: append([]HandlerFunc(nil), handlers...)}
	node.route = route
	if isStatic {
		root.addExactRoute(path, route)
	}
	r.routes = append(r.routes, route)
	return route, nil
}

func (n *routeNode) addExactRoute(path string, route *Route) {
	if n.exact != nil {
		n.exact[path] = route
		return
	}
	if n.exactOne == nil {
		n.exactPath, n.exactOne = path, route
		return
	}
	n.exact = map[string]*Route{n.exactPath: n.exactOne, path: route}
	n.exactPath, n.exactOne = "", nil
}

func (n *routeNode) exactRoute(path string) *Route {
	if n.exact != nil {
		return n.exact[path]
	}
	if n.exactOne != nil && n.exactPath == path {
		return n.exactOne
	}
	return nil
}

func (n *routeNode) addStaticChild(segment string) *routeNode {
	if n.static != nil {
		if child := n.static[segment]; child != nil {
			return child
		}
		child := &routeNode{}
		n.static[segment] = child
		return child
	}
	if n.staticSingle == nil {
		n.staticSegment = segment
		n.staticSingle = &routeNode{}
		return n.staticSingle
	}
	if n.staticSegment == segment {
		return n.staticSingle
	}
	n.static = map[string]*routeNode{n.staticSegment: n.staticSingle}
	n.staticSegment = ""
	n.staticSingle = nil
	child := &routeNode{}
	n.static[segment] = child
	return child
}

func (n *routeNode) staticChild(segment string) *routeNode {
	if n.static != nil {
		return n.static[segment]
	}
	if n.staticSingle != nil && n.staticSegment == segment {
		return n.staticSingle
	}
	return nil
}

func (r *router) find(method, path string, dst []param) (*Route, []param) {
	if r.frozen.Load() {
		return r.findFrozen(method, path, dst)
	}
	return r.findLocked(method, path, dst)
}

// findFrozen is lock-free after Freeze has established that no route maps can
// mutate. Its full-path static table avoids segment scanning for the common
// static-endpoint case.
func (r *router) findFrozen(method, path string, dst []param) (*Route, []param) {
	if path == "" || path[0] != '/' {
		return nil, dst[:0]
	}
	root := r.frozenRoot(method)
	if root == nil {
		return nil, dst[:0]
	}
	if route := root.exactRoute(path); route != nil {
		return route, dst[:0]
	}
	return matchPath(root, path[1:], path != "/", dst[:0])
}

func (r *router) findLocked(method, path string, dst []param) (*Route, []param) {
	if path == "" || path[0] != '/' {
		return nil, dst[:0]
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	root := r.methods[method]
	if root == nil {
		return nil, dst[:0]
	}
	if route := root.exactRoute(path); route != nil {
		return route, dst[:0]
	}
	return matchPath(root, path[1:], path != "/", dst[:0])
}

func (r *router) freeze() {
	r.mu.Lock()
	r.frozenGet = r.methods[http.MethodGet]
	r.frozenPost = r.methods[http.MethodPost]
	r.frozenPut = r.methods[http.MethodPut]
	r.frozenPatch = r.methods[http.MethodPatch]
	r.frozenDelete = r.methods[http.MethodDelete]
	r.frozenHead = r.methods[http.MethodHead]
	r.frozenOptions = r.methods[http.MethodOptions]
	r.frozenConnect = r.methods[http.MethodConnect]
	r.frozenTrace = r.methods[http.MethodTrace]
	r.frozen.Store(true)
	r.mu.Unlock()
}

func (r *router) frozenRoot(method string) *routeNode {
	switch method {
	case http.MethodGet:
		return r.frozenGet
	case http.MethodPost:
		return r.frozenPost
	case http.MethodPut:
		return r.frozenPut
	case http.MethodPatch:
		return r.frozenPatch
	case http.MethodDelete:
		return r.frozenDelete
	case http.MethodHead:
		return r.frozenHead
	case http.MethodOptions:
		return r.frozenOptions
	case http.MethodConnect:
		return r.frozenConnect
	case http.MethodTrace:
		return r.frozenTrace
	default:
		return r.methods[method]
	}
}

// matchPath walks path without constructing a []string. String slices share the
// request path backing store and therefore keep static-route matching allocation
// free after Context pool warm-up.
func matchPath(node *routeNode, rest string, hasSegment bool, params []param) (*Route, []param) {
	startParams := len(params)
	if route, result, ok := matchPathFast(node, rest, hasSegment, params); ok {
		return route, result
	}
	return matchPathBacktrack(node, rest, hasSegment, params[:startParams])
}

// matchPathFast follows the usual static > parameter > wildcard priority using
// a loop. If a static branch later dead-ends, the slower backtracking matcher
// below retries it correctly through a parameter sibling.
func matchPathFast(node *routeNode, rest string, hasSegment bool, params []param) (*Route, []param, bool) {
	for {
		if !hasSegment {
			if node.route != nil {
				return node.route, params, true
			}
			if node.wildcard != nil && node.wildcard.route != nil {
				return node.wildcard.route, append(params, param{key: node.wildName, value: "/"}), true
			}
			return nil, params, false
		}
		if node.staticSingle != nil && node.param == nil && node.wildcard == nil {
			if remaining, more, ok := consumeStaticSegment(rest, node.staticSegment); ok {
				node, rest, hasSegment = node.staticSingle, remaining, more
				continue
			}
			return nil, params, false
		}
		segment, remaining, more := nextPathSegment(rest)
		if child := node.staticChild(segment); child != nil {
			node, rest, hasSegment = child, remaining, more
			continue
		}
		if node.param != nil && segment != "" {
			params = append(params, param{key: node.paramName, value: segment})
			node, rest, hasSegment = node.param, remaining, more
			continue
		}
		if node.wildcard != nil && node.wildcard.route != nil {
			return node.wildcard.route, append(params, param{key: node.wildName, value: "/" + rest}), true
		}
		return nil, params, false
	}
}

func consumeStaticSegment(rest, segment string) (remaining string, more bool, ok bool) {
	if !strings.HasPrefix(rest, segment) {
		return "", false, false
	}
	if len(rest) == len(segment) {
		return "", false, true
	}
	if rest[len(segment)] != '/' {
		return "", false, false
	}
	return rest[len(segment)+1:], true, true
}

func nextPathSegment(rest string) (segment, remaining string, more bool) {
	segment = rest
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		return rest[:slash], rest[slash+1:], true
	}
	return segment, "", false
}

func matchPathBacktrack(node *routeNode, rest string, hasSegment bool, params []param) (*Route, []param) {
	if !hasSegment {
		if node.route != nil {
			return node.route, params
		}
		if node.wildcard != nil && node.wildcard.route != nil {
			return node.wildcard.route, append(params, param{key: node.wildName, value: "/"})
		}
		return nil, params
	}
	segment, remaining, more := nextPathSegment(rest)
	if child := node.staticChild(segment); child != nil {
		if route, result := matchPath(child, remaining, more, params); route != nil {
			return route, result
		}
	}
	if node.param != nil && segment != "" {
		params = append(params, param{key: node.paramName, value: segment})
		if route, result := matchPath(node.param, remaining, more, params); route != nil {
			return route, result
		}
		params = params[:len(params)-1]
	}
	if node.wildcard != nil && node.wildcard.route != nil {
		return node.wildcard.route, append(params, param{key: node.wildName, value: "/" + rest})
	}
	return nil, params
}

func (r *router) allowed(path string) []string {
	if path == "" || path[0] != '/' {
		return nil
	}
	r.mu.RLock()
	methods := make([]string, 0, len(r.methods))
	for method, root := range r.methods {
		if root.exactRoute(path) != nil {
			methods = append(methods, method)
			continue
		}
		if route, _ := matchPath(root, path[1:], path != "/", nil); route != nil {
			methods = append(methods, method)
		}
	}
	r.mu.RUnlock()
	if len(methods) == 0 {
		return nil
	}
	// A GET endpoint can also serve HEAD according to HTTP semantics.
	foundHead := false
	for _, method := range methods {
		if method == http.MethodHead {
			foundHead = true
			break
		}
	}
	if !foundHead {
		for _, method := range methods {
			if method == http.MethodGet {
				methods = append(methods, http.MethodHead)
				break
			}
		}
	}
	methods = append(methods, http.MethodOptions)
	sort.Strings(methods)
	return methods
}

func (r *router) list() []Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	routes := make([]Route, len(r.routes))
	for index, route := range r.routes {
		routes[index] = Route{Method: route.Method, Pattern: route.Pattern, Handlers: append([]HandlerFunc(nil), route.Handlers...)}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Pattern == routes[j].Pattern {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Pattern < routes[j].Pattern
	})
	return routes
}
