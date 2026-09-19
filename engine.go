package velocity

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HandlerFunc is the unit of routing and middleware. Returning an error stops
// the chain and delegates rendering to the engine's ErrorHandler.
type HandlerFunc func(*Context) error

// ErrorHandler translates errors returned from handlers into an HTTP response.
type ErrorHandler func(*Context, error)

// Engine is an http.Handler and is safe to serve while routes are added. For
// predictable configuration, register routes and middleware before serving.
type Engine struct {
	router router
	pool   sync.Pool

	middlewareMu   sync.RWMutex
	middleware     []HandlerFunc
	notFound       []HandlerFunc
	noMethod       []HandlerFunc
	frozenGlobal   []HandlerFunc
	frozenNotFound []HandlerFunc
	frozenNoMethod []HandlerFunc
	frozen         atomic.Bool

	errorMu      sync.RWMutex
	errorHandler ErrorHandler
	templateMu   sync.RWMutex
	htmlTemplate *template.Template

	proxyMu        sync.RWMutex
	trustedProxies []*net.IPNet
}

// New returns an engine with no implicit middleware. Use Default for secure
// baseline middleware, or compose exactly the middleware your service needs.
func New() *Engine {
	e := &Engine{router: newRouter(), errorHandler: defaultErrorHandler}
	e.pool.New = func() any {
		return &Context{
			handlers: make([]HandlerFunc, 0, 12),
			params:   make([]param, 0, 4),
			errors:   make([]error, 0, 2),
		}
	}
	return e
}

// Default returns an engine with panic recovery, a cryptographically random
// request ID, and conservative browser security headers enabled.
func Default() *Engine {
	e := New()
	e.Use(Recovery(), RequestID(), SecureHeaders(DefaultSecurityHeaders()))
	return e
}

func validateHandlers(handlers []HandlerFunc) {
	for _, handler := range handlers {
		if handler == nil {
			panic("velocity: nil handler")
		}
	}
}

// Use adds global middleware. Middleware should be registered before serving
// so every route has predictable behavior.
func (e *Engine) Use(handlers ...HandlerFunc) *Engine {
	validateHandlers(handlers)
	e.middlewareMu.Lock()
	if e.frozen.Load() {
		e.middlewareMu.Unlock()
		panic("velocity: engine is frozen")
	}
	e.middleware = append(e.middleware, handlers...)
	e.middlewareMu.Unlock()
	return e
}

// Freeze locks the route and middleware configuration and enables lock-free
// dispatch. Call it after application setup and before serving. Run calls it
// automatically. Attempts to add a route afterward return an error; attempts
// to change middleware panic because they are programming-configuration errors.
func (e *Engine) Freeze() {
	if e.frozen.Load() {
		return
	}
	// Hold middleware configuration while capturing immutable handler slices.
	// router.freeze rejects a registration that was already waiting on its lock.
	e.middlewareMu.Lock()
	if e.frozen.Load() {
		e.middlewareMu.Unlock()
		return
	}
	e.router.freeze()
	e.frozenGlobal = append([]HandlerFunc(nil), e.middleware...)
	e.frozenNotFound = append([]HandlerFunc(nil), e.notFound...)
	e.frozenNoMethod = append([]HandlerFunc(nil), e.noMethod...)
	e.frozen.Store(true)
	e.middlewareMu.Unlock()
}

// IsFrozen reports whether lock-free production dispatch is enabled.
func (e *Engine) IsFrozen() bool { return e.frozen.Load() }

// SetErrorHandler replaces the response renderer for returned errors.
func (e *Engine) SetErrorHandler(handler ErrorHandler) {
	if handler == nil {
		panic("velocity: nil error handler")
	}
	e.errorMu.Lock()
	e.errorHandler = handler
	e.errorMu.Unlock()
}

// SetHTMLTemplate installs parsed html/templates used by Context.HTML. Parse
// templates during startup so template syntax failures do not affect requests.
func (e *Engine) SetHTMLTemplate(templates *template.Template) {
	if templates == nil {
		panic("velocity: nil HTML template")
	}
	e.templateMu.Lock()
	e.htmlTemplate = templates
	e.templateMu.Unlock()
}

// LoadHTMLGlob parses templates from a glob and installs them atomically.
func (e *Engine) LoadHTMLGlob(pattern string) error {
	templates, err := template.ParseGlob(pattern)
	if err != nil {
		return err
	}
	e.SetHTMLTemplate(templates)
	return nil
}

func (e *Engine) htmlTemplates() *template.Template {
	e.templateMu.RLock()
	templates := e.htmlTemplate
	e.templateMu.RUnlock()
	return templates
}

// Handle registers a method and returns configuration failures instead of
// panicking. It is useful for generated or user-provided route definitions.
func (e *Engine) Handle(method, path string, handlers ...HandlerFunc) (*Route, error) {
	validateHandlers(handlers)
	return e.router.add(method, path, handlers)
}

// MustHandle is Handle for statically configured route tables.
func (e *Engine) MustHandle(method, path string, handlers ...HandlerFunc) *Route {
	route, err := e.Handle(method, path, handlers...)
	if err != nil {
		panic("velocity: " + err.Error())
	}
	return route
}

func (e *Engine) GET(path string, handlers ...HandlerFunc) *Route {
	return e.MustHandle(http.MethodGet, path, handlers...)
}
func (e *Engine) POST(path string, handlers ...HandlerFunc) *Route {
	return e.MustHandle(http.MethodPost, path, handlers...)
}
func (e *Engine) PUT(path string, handlers ...HandlerFunc) *Route {
	return e.MustHandle(http.MethodPut, path, handlers...)
}
func (e *Engine) PATCH(path string, handlers ...HandlerFunc) *Route {
	return e.MustHandle(http.MethodPatch, path, handlers...)
}
func (e *Engine) DELETE(path string, handlers ...HandlerFunc) *Route {
	return e.MustHandle(http.MethodDelete, path, handlers...)
}
func (e *Engine) HEAD(path string, handlers ...HandlerFunc) *Route {
	return e.MustHandle(http.MethodHead, path, handlers...)
}
func (e *Engine) OPTIONS(path string, handlers ...HandlerFunc) *Route {
	return e.MustHandle(http.MethodOptions, path, handlers...)
}

// Group creates a path-scoped collection of routes and middleware.
func (e *Engine) Group(prefix string, middleware ...HandlerFunc) *Group {
	if _, err := splitRoute(prefix); err != nil {
		panic("velocity: invalid group prefix: " + err.Error())
	}
	validateHandlers(middleware)
	return &Group{engine: e, prefix: cleanGroupPrefix(prefix), middleware: append([]HandlerFunc(nil), middleware...)}
}

// NoRoute sets handlers used for unmatched paths.
func (e *Engine) NoRoute(handlers ...HandlerFunc) *Engine {
	validateHandlers(handlers)
	e.middlewareMu.Lock()
	if e.frozen.Load() {
		e.middlewareMu.Unlock()
		panic("velocity: engine is frozen")
	}
	e.notFound = append([]HandlerFunc(nil), handlers...)
	e.middlewareMu.Unlock()
	return e
}

// NoMethod sets handlers used when a path exists but the method does not.
func (e *Engine) NoMethod(handlers ...HandlerFunc) *Engine {
	validateHandlers(handlers)
	e.middlewareMu.Lock()
	if e.frozen.Load() {
		e.middlewareMu.Unlock()
		panic("velocity: engine is frozen")
	}
	e.noMethod = append([]HandlerFunc(nil), handlers...)
	e.middlewareMu.Unlock()
	return e
}

// Routes returns a copy of the registered route metadata.
func (e *Engine) Routes() []Route { return e.router.list() }

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := e.pool.Get().(*Context)
	c.reset(e, w, r)
	defer func() { c.release(); e.pool.Put(c) }()

	method := r.Method
	route, params := e.router.find(method, r.URL.Path, c.params)
	if route == nil && method == http.MethodHead {
		route, params = e.router.find(http.MethodGet, r.URL.Path, c.params)
	}
	c.params = params
	if e.frozen.Load() {
		c.handlers = append(c.handlers, e.frozenGlobal...)
	} else {
		// Copy function pointers into the pooled context while protected by the
		// configuration lock. This avoids a per-request middleware snapshot
		// allocation and lets configuration changes remain race-free.
		e.middlewareMu.RLock()
		c.handlers = append(c.handlers, e.middleware...)
	}
	if route != nil {
		c.handlers = append(c.handlers, route.Handlers...)
	} else {
		allowed := e.router.allowed(r.URL.Path)
		switch {
		case method == http.MethodOptions && len(allowed) > 0:
			c.handlers = append(c.handlers, func(c *Context) error {
				c.writer.Header().Set("Allow", strings.Join(allowed, ", "))
				c.Status(http.StatusNoContent)
				return nil
			})
		case len(allowed) > 0:
			c.writer.Header().Set("Allow", strings.Join(allowed, ", "))
			noMethod := e.noMethod
			if e.frozen.Load() {
				noMethod = e.frozenNoMethod
			}
			if len(noMethod) > 0 {
				c.handlers = append(c.handlers, noMethod...)
			} else {
				c.handlers = append(c.handlers, func(*Context) error { return ErrMethodNotAllowed })
			}
		default:
			notFound := e.notFound
			if e.frozen.Load() {
				notFound = e.frozenNotFound
			}
			if len(notFound) > 0 {
				c.handlers = append(c.handlers, notFound...)
			} else {
				c.handlers = append(c.handlers, func(*Context) error { return ErrNotFound })
			}
		}
	}
	if !e.frozen.Load() {
		e.middlewareMu.RUnlock()
	}
	if err := c.Next(); err != nil {
		c.errors = append(c.errors, err)
		e.handleError(c, err)
	}
}

func (e *Engine) handleError(c *Context, err error) {
	if c.IsWritten() {
		return
	}
	e.errorMu.RLock()
	handler := e.errorHandler
	e.errorMu.RUnlock()
	handler(c, err)
}

func defaultErrorHandler(c *Context, err error) {
	httpErr := AsHTTPError(err)
	if c.IsWritten() {
		return
	}
	body := H{"error": H{"message": httpErr.Message}}
	if httpErr.Code != "" {
		body["error"].(H)["code"] = httpErr.Code
	}
	var validation ValidationErrors
	if errors.As(err, &validation) {
		body["error"].(H)["details"] = validation
	}
	_ = c.JSON(httpErr.Status, body)
}

// Server freezes routing and middleware, then returns a safely configured
// http.Server. Adjust its connection-limit fields before starting it.
func (e *Engine) Server(addr string) *http.Server {
	e.Freeze()
	return &http.Server{
		Addr:              addr,
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func (e *Engine) Run(addr string) error {
	err := e.Server(addr).ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (e *Engine) RunTLS(addr, certFile, keyFile string) error {
	err := e.Server(addr).ListenAndServeTLS(certFile, keyFile)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown is a convenience wrapper for a server returned by Server.
func Shutdown(server *http.Server, ctx context.Context) error { return server.Shutdown(ctx) }

// Static serves files below root through a catch-all route. Traversal paths and
// directory listings are rejected. Mount prefix must not end in '/'.
func (e *Engine) Static(prefix, root string) *Route {
	if prefix == "/" || strings.HasSuffix(prefix, "/") {
		panic("velocity: static prefix must not be '/' or end in '/'")
	}
	if _, err := splitRoute(prefix); err != nil {
		panic("velocity: invalid static prefix: " + err.Error())
	}
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		panic("velocity: invalid static root: " + err.Error())
	}
	resolvedRoot, err = filepath.EvalSymlinks(resolvedRoot)
	if err != nil {
		panic("velocity: static root must exist and resolve: " + err.Error())
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil || !rootInfo.IsDir() {
		panic("velocity: static root must be a directory")
	}
	return e.GET(prefix+"/*filepath", func(c *Context) error {
		name := strings.TrimPrefix(c.Param("filepath"), "/")
		if name == "" || !filepath.IsLocal(filepath.FromSlash(name)) {
			return ErrNotFound
		}
		fullPath := filepath.Join(resolvedRoot, filepath.FromSlash(name))
		resolvedPath, err := filepath.EvalSymlinks(fullPath)
		if err != nil || !isWithinDirectory(resolvedRoot, resolvedPath) {
			return ErrNotFound
		}
		info, err := os.Stat(resolvedPath)
		if err != nil || info.IsDir() {
			return ErrNotFound
		}
		http.ServeFile(c.Writer(), c.Request, resolvedPath)
		return nil
	})
}

func isWithinDirectory(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// SetTrustedProxies enables forwarded-client-IP processing only for the listed
// direct peer networks. Pass nil or an empty slice to trust no proxies.
func (e *Engine) SetTrustedProxies(proxies []string) error {
	networks := make([]*net.IPNet, 0, len(proxies))
	for _, proxy := range proxies {
		if ip := net.ParseIP(proxy); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
				ip = ip.To4()
			}
			networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		_, network, err := net.ParseCIDR(proxy)
		if err != nil {
			return fmt.Errorf("invalid trusted proxy %q: %w", proxy, err)
		}
		networks = append(networks, network)
	}
	e.proxyMu.Lock()
	e.trustedProxies = networks
	e.proxyMu.Unlock()
	return nil
}

func (e *Engine) isTrustedProxy(ip net.IP) bool {
	e.proxyMu.RLock()
	defer e.proxyMu.RUnlock()
	for _, network := range e.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (e *Engine) clientIP(r *http.Request) string {
	peer := remoteIP(r.RemoteAddr)
	peerIP := net.ParseIP(peer)
	if peerIP == nil || !e.isTrustedProxy(peerIP) {
		return peer
	}

	// Work right-to-left through a proxy chain and return the first address not
	// in a trusted range. This avoids treating a trusted intermediary as client.
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for index := len(forwarded) - 1; index >= 0; index-- {
		candidate := net.ParseIP(strings.TrimSpace(forwarded[index]))
		if candidate != nil && !e.isTrustedProxy(candidate) {
			return candidate.String()
		}
	}
	if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); realIP != nil {
		return realIP.String()
	}
	return peer
}

// JSONLogger is a small useful default logger for services that do not already
// have structured request logging.
func JSONLogger(logger *slog.Logger) HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *Context) error {
		started := time.Now()
		err := c.Next()
		requestID, _ := c.Get(RequestIDKey)
		logger.Info("http request", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.ResponseStatus(), "bytes", c.ResponseBytes(), "duration_ms", time.Since(started).Milliseconds(), "request_id", requestID, "client_ip", c.ClientIP())
		return err
	}
}
