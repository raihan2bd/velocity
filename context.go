package velocity

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// H is a convenience type for JSON object responses.
type H map[string]any

type param struct {
	key   string
	value string
}

// Context is valid only for the lifetime of a request. Do not retain it or use
// it in goroutines after its handler returns.
type Context struct {
	Request *http.Request
	engine  *Engine
	writer  responseWriter

	handlers []HandlerFunc
	index    int
	params   []param
	values   map[string]any
	errors   []error
}

func (c *Context) reset(e *Engine, w http.ResponseWriter, r *http.Request) {
	c.engine = e
	c.Request = r
	c.writer.reset(w)
	c.handlers = c.handlers[:0]
	c.index = -1
	c.params = c.params[:0]
	if len(c.values) > 0 {
		clear(c.values)
	}
	if len(c.errors) > 0 {
		clear(c.errors)
	}
	c.errors = c.errors[:0]
}

func (c *Context) release() {
	c.Request = nil
	c.engine = nil
	c.handlers = c.handlers[:0]
	c.params = c.params[:0]
	if len(c.errors) > 0 {
		clear(c.errors)
	}
	c.errors = c.errors[:0]
}

// Writer returns the wrapped http response writer.
func (c *Context) Writer() http.ResponseWriter { return &c.writer }

// ResponseStatus returns the status that will be (or has been) sent.
func (c *Context) ResponseStatus() int { return c.writer.status }

// ResponseBytes returns the number of response body bytes written so far.
func (c *Context) ResponseBytes() int64 { return c.writer.bytes }

// IsWritten reports whether the response headers have been committed.
func (c *Context) IsWritten() bool { return c.writer.wroteHeader }

// Next runs the remaining handlers in the current chain.
func (c *Context) Next() error {
	c.index++
	for c.index < len(c.handlers) {
		if err := c.handlers[c.index](c); err != nil {
			c.Abort()
			return err
		}
		c.index++
	}
	return nil
}

// Abort prevents all remaining handlers from running.
func (c *Context) Abort() { c.index = len(c.handlers) }

// IsAborted reports whether execution of the handler chain was stopped.
func (c *Context) IsAborted() bool { return c.index >= len(c.handlers) }

// AbortWithStatus writes a status and stops the chain.
func (c *Context) AbortWithStatus(status int) {
	c.Status(status)
	c.Abort()
}

// AbortWithJSON writes JSON and stops the chain.
func (c *Context) AbortWithJSON(status int, value any) error {
	c.Abort()
	return c.JSON(status, value)
}

// Errors returns errors captured from handlers during this request.
func (c *Context) Errors() []error { return append([]error(nil), c.errors...) }

// Param gets a named parameter from the matching route.
func (c *Context) Param(key string) string {
	for i := len(c.params) - 1; i >= 0; i-- {
		if c.params[i].key == key {
			return c.params[i].value
		}
	}
	return ""
}

// ParamInt parses a route parameter as an integer.
func (c *Context) ParamInt(key string) (int, error) {
	value, err := strconv.Atoi(c.Param(key))
	if err != nil {
		return 0, WrapHTTPError(http.StatusBadRequest, "invalid route parameter", err)
	}
	return value, nil
}

func (c *Context) Query(key string) string { return c.Request.URL.Query().Get(key) }

func (c *Context) QueryDefault(key, fallback string) string {
	if value, ok := c.Request.URL.Query()[key]; ok && len(value) > 0 {
		return value[0]
	}
	return fallback
}

func (c *Context) QueryArray(key string) []string {
	return append([]string(nil), c.Request.URL.Query()[key]...)
}

func (c *Context) Set(key string, value any) {
	if c.values == nil {
		c.values = make(map[string]any)
	}
	c.values[key] = value
}

func (c *Context) Get(key string) (any, bool) {
	value, ok := c.values[key]
	return value, ok
}

func (c *Context) MustGet(key string) any {
	value, ok := c.Get(key)
	if !ok {
		panic("velocity: context key not found: " + key)
	}
	return value
}

// WithContext replaces the request's standard context.
func (c *Context) WithContext(ctx context.Context) { c.Request = c.Request.WithContext(ctx) }

func (c *Context) Deadline() (time.Time, bool) { return c.Request.Context().Deadline() }

// ClientIP uses the socket peer by default. Forwarded headers are honored only
// when the direct peer is within one of the engine's explicitly trusted CIDRs.
func (c *Context) ClientIP() string { return c.engine.clientIP(c.Request) }

func (c *Context) ContentType() string {
	contentType, _, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil {
		return ""
	}
	return contentType
}

func (c *Context) Header(key string) string { return c.Request.Header.Get(key) }

func (c *Context) Status(status int) { c.writer.WriteHeader(status) }

func (c *Context) String(status int, format string, values ...any) error {
	c.writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.writer.WriteHeader(status)
	_, err := fmt.Fprintf(&c.writer, format, values...)
	return err
}

func (c *Context) Data(status int, contentType string, data []byte) error {
	if contentType != "" {
		c.writer.Header().Set("Content-Type", contentType)
	}
	c.writer.WriteHeader(status)
	_, err := c.writer.Write(data)
	return err
}

// JSON serializes one document, escaping HTML-sensitive characters by default.
func (c *Context) JSON(status int, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	return c.Data(status, "", data)
}

// XML serializes one XML document.
func (c *Context) XML(status int, value any) error {
	c.writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	c.writer.WriteHeader(status)
	return xml.NewEncoder(&c.writer).Encode(value)
}

// HTML executes a previously configured html/template into a buffer before
// committing headers. A template error therefore cannot produce a partial page.
func (c *Context) HTML(status int, name string, data any) error {
	templates := c.engine.htmlTemplates()
	if templates == nil {
		return WrapHTTPError(http.StatusInternalServerError, "HTML templates are not configured", nil)
	}
	var output bytes.Buffer
	if err := templates.ExecuteTemplate(&output, name, data); err != nil {
		return WrapHTTPError(http.StatusInternalServerError, "cannot render template", err)
	}
	return c.Data(status, "text/html; charset=utf-8", output.Bytes())
}

// Text writes literal UTF-8 text. Use String when printf-style formatting is
// desired; Text avoids treating user-controlled percent characters as formats.
func (c *Context) Text(status int, value string) error {
	return c.Data(status, "text/plain; charset=utf-8", []byte(value))
}

func (c *Context) Redirect(status int, location string) {
	http.Redirect(&c.writer, c.Request, location, status)
}

func (c *Context) SetCookie(cookie *http.Cookie) { http.SetCookie(&c.writer, cookie) }

func (c *Context) Cookie(name string) (string, error) {
	cookie, err := c.Request.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// File safely serves one explicitly selected file.
func (c *Context) File(path string) { http.ServeFile(&c.writer, c.Request, path) }

// FileAttachment streams a file with a caller-controlled download name.
func (c *Context) FileAttachment(path, name string) {
	c.writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(name)}))
	http.ServeFile(&c.writer, c.Request, path)
}

// SSE sends a single server-sent event and flushes it. Event names and IDs must
// be single-line values; data is split into the SSE line format.
func (c *Context) SSE(event, id string, data []byte) error {
	if strings.ContainsAny(event, "\r\n") || strings.ContainsAny(id, "\r\n") {
		return NewHTTPError(http.StatusBadRequest, "invalid SSE metadata")
	}
	c.writer.Header().Set("Content-Type", "text/event-stream")
	c.writer.Header().Set("Cache-Control", "no-cache")
	c.writer.Header().Set("Connection", "keep-alive")
	if event != "" {
		if _, err := io.WriteString(&c.writer, "event: "+event+"\n"); err != nil {
			return err
		}
	}
	if id != "" {
		if _, err := io.WriteString(&c.writer, "id: "+id+"\n"); err != nil {
			return err
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if _, err := io.WriteString(&c.writer, "data: "+line+"\n"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(&c.writer, "\n"); err != nil {
		return err
	}
	c.writer.Flush()
	return nil
}

// RequestURI returns a copy of the current URI suitable for logs.
func (c *Context) RequestURI() *url.URL {
	copy := *c.Request.URL
	return &copy
}

func remoteIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}
