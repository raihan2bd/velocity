# Velocity

Velocity is a compact, standard-library-only HTTP framework for Go. It is designed around a pooled request context, a method-aware route tree, explicit error returns, and safe defaults. It has no transitive 3rd party dependencies.

## Quick start

```go
package main

import (
	"net/http"

	"github.com/raihan2bd/velocity"
)

func main() {
	e := velocity.Default()
	e.GET("/users/:id", func(c *velocity.Context) error {
		return c.JSON(http.StatusOK, velocity.H{"id": c.Param("id")})
	})
	_ = e.Run(":8080")
}
```

## Routes and middleware

```go
api := e.Group("/api/v1", velocity.RequestID())
api.POST("/avatars", velocity.MaxBodyBytes(8<<20), uploadAvatar)
api.GET("/users/:id", showUser)
e.Static("/public", "./public")
```

Routes support static segments, `:parameters`, and a final `*catchAll`. Static
routes take priority over parameters. Groups can nest and add middleware. An
unmatched method returns `405` with `Allow`; `OPTIONS` is generated automatically.
Use `Handle` when route definitions are dynamic and you need configuration errors
instead of a startup panic. `NoRoute` and `NoMethod` customize fallback behavior.

Middleware returns `c.Next()` to continue the chain. A returned error stops it
and is rendered by one central, replaceable error handler.

## Binding, validation, and uploads

```go
type CreateUser struct {
    Email string   `json:"email" validate:"required,email"`
    Name  string   `json:"name" validate:"required,min=3,max=80"`
    Tags  []string `json:"tags" validate:"dive,min=2"`
}

func createUser(c *velocity.Context) error {
    var input CreateUser
    if err := c.BindJSON(&input); err != nil { return err }
    return c.JSON(http.StatusCreated, input)
}
```

`BindJSON` rejects unknown fields and multiple documents. `BindQuery`,
`BindForm`, `BindMultipart`, and `Bind` handle other standard formats.
Validation tags are `required`, `omitempty`, `min=N`, `max=N`, `len=N`, `email`,
`url`, `uuid`, `alpha`, `alphanum`, `numeric`, `oneof=a b`, and `dive`.

For files, use `MaxBodyBytes`, then `FormFile` or `MultipartFiles` and
`SaveUploadedFile`. Saving is exclusive (`O_EXCL`), restricted to a simple
filename, and uses 0600 permissions. Do not expose upload directories directly.
`Static` requires an existing directory and refuses traversal and symlink escapes
outside that directory.

## Security defaults

`Default()` installs panic recovery, a random request ID, `nosniff`, frame,
referrer, permissions, and a conservative CSP header. `New()` is intentionally
bare for applications that need total middleware control.

Other included standard-library middleware: `SecureHeaders`, `MaxBodyBytes`,
explicit-origin `CORS`, bounded in-memory token-bucket `RateLimit`, `BasicAuth`,
`JSONLogger`, and `RecoveryWithLogger`. Proxy headers are ignored by default;
call `SetTrustedProxies` with only your real proxy CIDRs before relying on
`ClientIP`. CORS preflights also require every requested non-simple header to
be listed in `AllowedHeaders`; no header reflection is performed.

## Responses and server operation

`Context` provides JSON, XML, HTML templates, literal text, byte data, files,
secure file attachments, cookies, redirects, and flushed server-sent events.
`Engine.Server` freezes routes and middleware for lock-free dispatch, then
supplies conservative HTTP timeouts; customize the returned `http.Server` for
your deployment before starting it. `Run` and `RunTLS` do the same. If you use
your own `http.Server`, call `Freeze` after startup configuration.

## Included

- Static, parameter, and catch-all routes; route groups; automatic `OPTIONS` and `405` handling
- Middleware with `Next`, abort support, panic recovery, request IDs, structured logging, secure headers, CORS, size limits, and an in-memory rate limiter
- JSON, query, form, and multipart binding with reflection-based validation tags
- Secure file-upload helpers, JSON/text/bytes responses, cookies, redirects, static files, and server-sent events
- A consistent typed `HTTPError` model with one customizable error handler
- Context pooling and a concurrency-safe router

## Verify and benchmark

```sh
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -bench 'Benchmark(Static|Parameter)Route$' -benchmem -count=3
go test -C benchmarks -run '^$' -bench . -benchmem -count=5
```

See [`BENCHMARKS.md`](BENCHMARKS.md) for the reproducible Gin, Echo, Fiber, and
Velocity comparison and the current measured medians. The framework module has
no dependencies; competitors are isolated in `benchmarks/`.

See the Go documentation for exported types and `example_test.go` for a full runnable example. Run `go test ./...` and `go vet ./...` before shipping changes.

For local-only use before publishing, see [`outputs/LOCAL_USAGE_GUIDE.md`](outputs/LOCAL_USAGE_GUIDE.md).

## Performance note

Performance is workload- and Go-version-dependent. Velocity intentionally minimizes hot-path allocations, but it does not claim to be universally faster than Gin, Echo, Fiber, or any other framework without a reproducible benchmark on the target workload.
