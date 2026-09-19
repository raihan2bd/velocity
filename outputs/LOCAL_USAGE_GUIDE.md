# Using Velocity locally

Velocity is ready to use from this folder; publishing it to GitHub is not
required. It needs Go 1.24 or newer and no framework dependencies.

## Fastest first run

In PowerShell:

```powershell
Set-Location 'C:\Users\raihan\Documents\Codex\2026-09-12\i-x20'
go test ./...
go run ./examples/hello
```

The demo listens at `http://127.0.0.1:8080`. Keep that terminal open, then use
a second PowerShell window:

```powershell
curl.exe http://127.0.0.1:8080/health
curl.exe http://127.0.0.1:8080/api/v1/users/42
curl.exe -X POST http://127.0.0.1:8080/api/v1/users -H 'Content-Type: application/json' -d '{"name":"Ada Lovelace","email":"ada@example.com"}'
```

The POST endpoint rejects malformed JSON, unknown JSON fields, trailing JSON
documents, oversized bodies, and invalid `name` or `email` values.

## Build an app inside this folder

Create `cmd/my-api/main.go` and import the local module path already declared
in `go.mod`:

```go
package main

import (
    "log"
    "net/http"

    "github.com/raihan2bd/velocity"
)

func main() {
    app := velocity.Default()

    app.GET("/health", func(c *velocity.Context) error {
        return c.JSON(http.StatusOK, velocity.H{"status": "ok"})
    })

    api := app.Group("/api/v1", velocity.MaxBodyBytes(1<<20))
    api.GET("/users/:id", func(c *velocity.Context) error {
        return c.JSON(http.StatusOK, velocity.H{"id": c.Param("id")})
    })

    if err := app.Run(":8080"); err != nil {
        log.Fatal(err)
    }
}
```

Run it from the framework folder:

```powershell
go run ./cmd/my-api
```

`Run` freezes the routing and middleware configuration before accepting
requests, enabling Velocity's lock-free dispatch path. Register all routes and
middleware before calling `Run`. If you start your own `http.Server`, call
`app.Freeze()` first, or use `app.Server(":8080")`, which freezes it for you.

## Use it from a separate local project

For a sibling application folder such as `C:\Users\raihan\Documents\Codex\my-api`,
create the app's `go.mod` like this:

```go
module local/my-api

go 1.24.0

require github.com/raihan2bd/velocity v0.0.0

replace github.com/raihan2bd/velocity => C:/Users/raihan/Documents/Codex/2026-09-12/i-x20
```

Then write your `main.go` using the same import:

```go
import "github.com/raihan2bd/velocity"
```

From the separate project directory, run:

```powershell
go mod tidy
go run .
```

The `replace` line tells Go to use the local Velocity folder instead of looking
for GitHub. Keep it while developing locally; remove it only after publishing a
real version.

## Common building blocks

### JSON input and validation

```go
type CreateUser struct {
    Name  string `json:"name" validate:"required,min=3,max=80"`
    Email string `json:"email" validate:"required,email"`
}

app.POST("/users", func(c *velocity.Context) error {
    var input CreateUser
    if err := c.BindJSON(&input); err != nil {
        return err
    }
    return c.JSON(http.StatusCreated, input)
})
```

### Middleware and groups

```go
api := app.Group("/api/v1", velocity.RequestID())
api.Use(velocity.RateLimit(velocity.RateLimitConfig{Rate: 10, Burst: 20}))
api.GET("/profile", profileHandler)
```

### File upload

Create the destination directory before starting the app. Keep it outside any
public static-file directory.

```go
app.POST("/upload", velocity.MaxBodyBytes(8<<20), func(c *velocity.Context) error {
    file, err := c.FormFile("file", 1<<20)
    if err != nil {
        return err
    }
    path, err := velocity.SaveUploadedFile(file, "./uploads", "")
    if err != nil {
        return err
    }
    return c.JSON(http.StatusCreated, velocity.H{"path": path})
})
```

### CORS behind a browser frontend

Use explicit origins and non-simple headers. Forwarded client-IP headers are
ignored unless you explicitly trust only your proxy networks.

```go
app.Use(velocity.CORS(velocity.CORSConfig{
    AllowedOrigins: []string{"https://app.example.com"},
    AllowedMethods: []string{http.MethodGet, http.MethodPost},
    AllowedHeaders: []string{"Authorization", "Content-Type"},
}))

if err := app.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
    log.Fatal(err)
}
```

## Before running a real service

```powershell
go test ./...
go vet ./...
go test -race ./...
```

Use `velocity.Default()` for the included panic recovery, request IDs, and
browser security headers. Use `velocity.New()` only when you deliberately want
to choose every middleware component yourself.
