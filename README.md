# Velocity

**A compact, standard-library-only HTTP framework for Go.**

Velocity is a lightweight web framework for building APIs, web applications, and HTTP services with Go's standard library.

It focuses on:

- Simple routing
- Middleware
- Request/response handling
- JSON, XML, HTML, text, and file responses
- Request binding
- Validation
- File uploads
- Static files
- Cookies
- Redirects
- Server-Sent Events
- CORS
- Rate limiting
- Security headers
- Request IDs
- Basic authentication
- Structured JSON logging
- Panic recovery
- Typed HTTP errors
- Route groups
- Context pooling
- Concurrency-safe routing
- Conservative HTTP server defaults

Velocity has **no third-party runtime dependencies**.

> **Status:** Velocity is currently in early development. The API may change before `v1.0.0`.

---

# Table of Contents

1. [Requirements](#requirements)
2. [Installation](#installation)
3. [Your First Velocity Application](#your-first-velocity-application)
4. [Understanding the Engine](#understanding-the-engine)
5. [New vs Default](#new-vs-default)
6. [Starting the Server](#starting-the-server)
7. [Routes](#routes)
8. [Route Parameters](#route-parameters)
9. [Catch-All Routes](#catch-all-routes)
10. [HTTP Methods](#http-methods)
11. [Handle vs GET/POST/etc.](#handle-vs-getpostetc)
12. [Route Groups](#route-groups)
13. [Middleware](#middleware)
14. [Context](#context)
15. [Reading Request Data](#reading-request-data)
16. [Route Parameters as Integers](#route-parameters-as-integers)
17. [Sending Responses](#sending-responses)
18. [JSON Responses](#json-responses)
19. [XML Responses](#xml-responses)
20. [Text Responses](#text-responses)
21. [Raw Data Responses](#raw-data-responses)
22. [HTML Templates](#html-templates)
23. [Redirects](#redirects)
24. [Cookies](#cookies)
25. [Serving Files](#serving-files)
26. [File Downloads](#file-downloads)
27. [Server-Sent Events](#server-sent-events)
28. [Request Binding](#request-binding)
29. [JSON Binding](#json-binding)
30. [Query Binding](#query-binding)
31. [Form Binding](#form-binding)
32. [Multipart Binding](#multipart-binding)
33. [Validation](#validation)
34. [File Uploads](#file-uploads)
35. [Static Files](#static-files)
36. [HTTP Errors](#http-errors)
37. [Custom Error Handling](#custom-error-handling)
38. [Panic Recovery](#panic-recovery)
39. [Request IDs](#request-ids)
40. [Security Headers](#security-headers)
41. [Body Size Limits](#body-size-limits)
42. [CORS](#cors)
43. [Rate Limiting](#rate-limiting)
44. [Basic Authentication](#basic-authentication)
45. [JSON Logging](#json-logging)
46. [Trusted Proxies and Client IP](#trusted-proxies-and-client-ip)
47. [NoRoute and NoMethod](#noroute-and-nomethod)
48. [Automatic OPTIONS and 405](#automatic-options-and-405)
49. [Freeze and Request-Time Performance](#freeze-and-request-time-performance)
50. [Context Pooling](#context-pooling)
51. [Building a Real API](#building-a-real-api)
52. [Recommended Project Structure](#recommended-project-structure)
53. [Testing](#testing)
54. [Race Detection](#race-detection)
55. [go vet](#go-vet)
56. [Benchmarks](#benchmarks)
57. [Common Mistakes](#common-mistakes)
58. [Production Checklist](#production-checklist)
59. [Complete Example Application](#complete-example-application)
60. [Versioning](#versioning)

---

# Requirements

Velocity currently targets:

- Go 1.24 or newer
- Any operating system supported by Go
- No external runtime dependencies

Check your Go installation:

```bash
go version
```

---

# Installation

Create a Go project:

```bash
mkdir myapp
cd myapp
go mod init myapp
```

Install Velocity:

```bash
go get github.com/raihan2bd/velocity
```

Your `go.mod` will contain:

```go
module myapp

go 1.24

require github.com/raihan2bd/velocity ...
```

You can also use a specific Velocity version:

```bash
go get github.com/raihan2bd/velocity@v0.0.1
```

---

# Your First Velocity Application

Create `main.go`:

```go
package main

import (
	"net/http"

	"github.com/raihan2bd/velocity"
)

func main() {
	app := velocity.Default()

	app.GET("/", func(c *velocity.Context) error {
		return c.String(http.StatusOK, "Hello, Velocity!")
	})

	_ = app.Run(":8080")
}
```

Start it:

```bash
go run .
```

Open:

```text
http://localhost:8080
```

You should see:

```text
Hello, Velocity!
```

---

# Understanding the Engine

The `Engine` is the main object of a Velocity application.

It manages:

- Routes
- Middleware
- The router
- Error handling
- Request contexts
- HTTP server configuration
- Static file handling
- Templates
- Application lifecycle

Most applications begin with:

```go
app := velocity.New()
```

or:

```go
app := velocity.Default()
```

---

# New vs Default

Velocity provides two starting points.

## `velocity.New()`

`New()` creates an engine without implicit middleware.

```go
app := velocity.New()
```

This is useful when you want complete control.

For example:

```go
app := velocity.New()

app.Use(
	velocity.Recovery(),
	velocity.RequestID(),
)
```

## `velocity.Default()`

`Default()` creates an engine with useful baseline middleware:

- Panic recovery
- Request IDs
- Security headers

```go
app := velocity.Default()
```

This is usually the easiest starting point for beginners.

Conceptually:

```text
New()
  ↓
Empty application
  ↓
You choose middleware

Default()
  ↓
New()
  ↓
Recovery
Request ID
Security headers
```

---

# Starting the Server

The simplest option is:

```go
app.Run(":8080")
```

Example:

```go
package main

import "github.com/raihan2bd/velocity"

func main() {
	app := velocity.Default()

	app.GET("/", func(c *velocity.Context) error {
		return c.Text(200, "Hello")
	})

	_ = app.Run(":8080")
}
```

---

# Custom HTTP Server

Velocity also allows you to obtain an HTTP server and customize it.

Conceptually:

```go
server := app.Server(":8080")

server.ReadTimeout = 10 * time.Second
server.WriteTimeout = 10 * time.Second
server.IdleTimeout = 60 * time.Second

log.Fatal(server.ListenAndServe())
```

This is useful when your deployment requires custom server settings.

Velocity supplies conservative server timeout defaults, but you can customize the returned `http.Server` for your environment.

---

# HTTPS

Velocity also supports TLS through:

```go
app.RunTLS(":8443", "server.crt", "server.key")
```

Example:

```go
package main

import (
	"github.com/raihan2bd/velocity"
)

func main() {
	app := velocity.Default()

	app.GET("/", func(c *velocity.Context) error {
		return c.Text(200, "HTTPS works!")
	})

	_ = app.RunTLS(
		":8443",
		"server.crt",
		"server.key",
	)
}
```

---

# Routes

Routes connect an HTTP method and URL to a handler.

The basic pattern is:

```go
app.GET("/hello", handler)
```

Example:

```go
app.GET("/hello", func(c *velocity.Context) error {
	return c.Text(200, "Hello!")
})
```

---

# HTTP Methods

Velocity provides helpers for the common HTTP methods:

```go
app.GET("/users", listUsers)
app.POST("/users", createUser)
app.PUT("/users/:id", updateUser)
app.PATCH("/users/:id", patchUser)
app.DELETE("/users/:id", deleteUser)
app.HEAD("/users", headUsers)
app.OPTIONS("/users", optionsUsers)
```

Handlers have this general shape:

```go
func handler(c *velocity.Context) error {
	return c.Text(200, "OK")
}
```

Because handlers return errors, application errors can flow into Velocity's central error handling system.

---

# Route Parameters

Use `:name` for a parameter.

```go
app.GET("/users/:id", func(c *velocity.Context) error {
	id := c.Param("id")

	return c.JSON(200, velocity.H{
		"id": id,
	})
})
```

Request:

```text
GET /users/42
```

Result:

```json
{
  "id": "42"
}
```

You can have multiple parameters:

```go
app.GET("/users/:userID/posts/:postID", func(c *velocity.Context) error {
	return c.JSON(200, velocity.H{
		"user_id": c.Param("userID"),
		"post_id": c.Param("postID"),
	})
})
```

---

# Catch-All Routes

A final `*catchAll` parameter can capture the remaining path.

Example:

```go
app.GET("/files/*path", func(c *velocity.Context) error {
	return c.JSON(200, velocity.H{
		"path": c.Param("path"),
	})
})
```

A request such as:

```text
/files/images/avatar.png
```

can capture the remaining path.

Catch-all parameters must be the final route segment.

---

# Route Matching Priority

Velocity gives static routes priority over parameter routes.

For example:

```go
app.GET("/users/new", newUser)

app.GET("/users/:id", showUser)
```

When the request is:

```text
/users/new
```

Velocity chooses:

```text
/users/new
```

instead of interpreting `new` as an `id`.

This allows you to combine specific routes with parameterized routes naturally.

---

# Handle vs GET/POST/etc.

The convenience methods:

```go
app.GET(...)
app.POST(...)
app.PUT(...)
```

use the equivalent of `MustHandle`.

That means invalid static route configuration causes a panic during application configuration.

For dynamic route definitions, use:

```go
route, err := app.Handle(
	http.MethodGet,
	"/users/:id",
	handler,
)

if err != nil {
	// handle configuration error
}
```

Use `Handle` when you need configuration errors returned as normal Go errors.

Use:

```go
app.GET(...)
```

when your routes are statically defined in application code.

---

# Route Groups

Groups allow you to share a URL prefix and middleware.

Example:

```go
api := app.Group("/api/v1")

api.GET("/users", listUsers)
api.GET("/users/:id", showUser)
api.POST("/users", createUser)
```

The resulting routes are:

```text
GET  /api/v1/users
GET  /api/v1/users/:id
POST /api/v1/users
```

---

# Groups with Middleware

You can attach middleware to a group:

```go
api := app.Group(
	"/api/v1",
	authMiddleware,
)

api.GET("/profile", profile)
api.GET("/settings", settings)
```

The middleware applies to routes registered through that group.

---

# Nested Groups

Groups can be nested.

```go
api := app.Group("/api")

	v1 := api.Group("/v1")

	v1.GET("/users", listUsers)
```

The route becomes:

```text
/api/v1/users
```

This is useful for organizing APIs.

---

# Middleware

Middleware runs around your request handlers.

A middleware looks like:

```go
func Logger() velocity.HandlerFunc {
	return func(c *velocity.Context) error {
		fmt.Println("request started")

		err := c.Next()

		fmt.Println("request finished")

		return err
	}
}
```

Register it:

```go
app.Use(Logger())
```

---

# Middleware Execution

Think of middleware as a chain:

```text
Request
   ↓
Middleware A
   ↓
Middleware B
   ↓
Handler
   ↓
Middleware B
   ↓
Middleware A
   ↓
Response
```

Calling:

```go
return c.Next()
```

continues execution.

If middleware returns an error instead:

```go
return errors.New("not allowed")
```

the chain stops and the error is handled by Velocity's error handler.

---

# Middleware That Stops a Request

Example authentication middleware:

```go
func RequireAuth() velocity.HandlerFunc {
	return func(c *velocity.Context) error {
		if c.Header("Authorization") == "" {
			return velocity.NewHTTPError(
				401,
				"authentication required",
			)
		}

		return c.Next()
	}
}
```

Register it:

```go
app.Use(RequireAuth())
```

---

# Context

Every handler receives:

```go
*c.Context
```

The context gives you access to:

- Request information
- Route parameters
- Query parameters
- Headers
- Cookies
- Response methods
- Binding
- File uploads
- Request-scoped data
- Middleware execution

Example:

```go
app.GET("/hello", func(c *velocity.Context) error {
	name := c.QueryDefault("name", "World")

	return c.String(
		200,
		"Hello, %s!",
		name,
	)
})
```

Request:

```text
/hello?name=Raihan
```

Response:

```text
Hello, Raihan!
```

---

# Reading Request Data

## Query Parameters

```go
name := c.Query("name")
```

For a default:

```go
name := c.QueryDefault("name", "Guest")
```

Example:

```go
app.GET("/search", func(c *velocity.Context) error {
	query := c.Query("q")

	return c.JSON(200, velocity.H{
		"query": query,
	})
})
```

Request:

```text
/search?q=golang
```

---

# Headers

Read a header:

```go
token := c.Header("Authorization")
```

Example:

```go
app.GET("/headers", func(c *velocity.Context) error {
	return c.JSON(200, velocity.H{
		"user_agent": c.Header("User-Agent"),
	})
})
```

---

# Route Parameters as Integers

Instead of manually converting:

```go
id, err := strconv.Atoi(c.Param("id"))
```

Velocity provides:

```go
id, err := c.ParamInt("id")
```

Example:

```go
app.GET("/users/:id", func(c *velocity.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return err
	}

	return c.JSON(200, velocity.H{
		"id": id,
	})
})
```

Invalid values become a bad-request error.

---

# Sending Responses

Velocity supports several response styles.

---

# JSON Responses

The most common API response:

```go
return c.JSON(200, velocity.H{
	"message": "Hello",
})
```

`velocity.H` is convenient for JSON objects.

You can also use structs:

```go
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

app.GET("/user", func(c *velocity.Context) error {
	user := User{
		ID:   1,
		Name: "Raihan",
	}

	return c.JSON(200, user)
})
```

---

# JSON Status Codes

Use standard Go HTTP constants:

```go
return c.JSON(
	http.StatusCreated,
	user,
)
```

For example:

```go
return c.JSON(http.StatusCreated, velocity.H{
	"message": "user created",
})
```

---

# Text Responses

Use:

```go
return c.Text(200, "Hello World")
```

For formatted text:

```go
return c.String(
	200,
	"Hello %s",
	name,
)
```

Use `Text` when you already have the final string.

Use `String` when you need `fmt.Fprintf`-style formatting.

---

# Raw Data Responses

For byte data:

```go
return c.Data(
	200,
	"application/octet-stream",
	data,
)
```

This is useful for:

- Generated files
- Binary data
- Images
- Custom content types

---

# XML Responses

Velocity can serialize XML:

```go
type User struct {
	ID   int    `xml:"id"`
	Name string `xml:"name"`
}

app.GET("/user.xml", func(c *velocity.Context) error {
	user := User{
		ID:   1,
		Name: "Raihan",
	}

	return c.XML(200, user)
})
```

---

# HTML Templates

Velocity can render templates created with Go's standard `html/template` package.

Configure your templates before serving the application.

Then:

```go
return c.HTML(
	http.StatusOK,
	"home.html",
	data,
)
```

Velocity renders the template into a buffer before committing the response. This means template execution errors can be returned without partially writing the HTML response.

---

# Redirects

Redirect a client:

```go
c.Redirect(
	http.StatusFound,
	"/login",
)
```

For permanent redirects:

```go
c.Redirect(
	http.StatusPermanentRedirect,
	"/new-location",
)
```

---

# Cookies

## Reading Cookies

```go
value, err := c.Cookie("session")
if err != nil {
	return err
}
```

---

# Setting Cookies

Use Go's standard `http.Cookie`:

```go
c.SetCookie(&http.Cookie{
	Name:     "session",
	Value:    "abc123",
	Path:     "/",
	HttpOnly: true,
	Secure:   true,
	SameSite: http.SameSiteLaxMode,
})
```

For authentication/session cookies, configure security attributes intentionally.

---

# Serving Files

Velocity can serve an explicitly selected file:

```go
app.GET("/logo", func(c *velocity.Context) error {
	c.File("./public/logo.png")
	return nil
})
```

`File` is useful when the application itself decides which file should be returned.

Do not pass untrusted user-controlled paths directly into file-serving functions.

---

# File Downloads

Use:

```go
c.FileAttachment(
	"./reports/report.pdf",
	"monthly-report.pdf",
)
```

This sends the file with an attachment disposition.

The download name is sanitized through `filepath.Base` before being used in the `Content-Disposition` header.

---

# Server-Sent Events

Velocity supports Server-Sent Events through:

```go
c.SSE(event, id, data)
```

Example:

```go
app.GET("/events", func(c *velocity.Context) error {
	return c.SSE(
		"message",
		"1",
		[]byte("Hello from Velocity"),
	)
})
```

A browser can consume the endpoint with:

```javascript
const events = new EventSource("/events");

events.addEventListener("message", (event) => {
  console.log(event.data);
});
```

The event name and ID are single-line values, and the data is formatted according to SSE rules.

---

# Request Binding

Binding converts HTTP request data into Go structs.

Instead of manually reading every field:

```go
name := c.Query("name")
email := c.Query("email")
```

you can define a struct:

```go
type CreateUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
```

and bind the request into it.

Velocity supports:

- JSON
- Query parameters
- Forms
- Multipart forms
- Automatic/general binding

---

# JSON Binding

Example:

```go
type CreateUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

app.POST("/users", func(c *velocity.Context) error {
	var input CreateUser

	if err := c.BindJSON(&input); err != nil {
		return err
	}

	return c.JSON(201, input)
})
```

Request:

```http
POST /users
Content-Type: application/json
```

Body:

```json
{
  "name": "Raihan",
  "email": "raihan@example.com"
}
```

Velocity's JSON binding is intentionally strict.

It rejects:

- Unknown fields
- Multiple JSON documents in one request

This helps catch client mistakes instead of silently ignoring them.

---

# Query Binding

Example struct:

```go
type SearchQuery struct {
	Query string `query:"q"`
	Page  int    `query:"page"`
}
```

Then:

```go
app.GET("/search", func(c *velocity.Context) error {
	var input SearchQuery

	if err := c.BindQuery(&input); err != nil {
		return err
	}

	return c.JSON(200, input)
})
```

Request:

```text
/search?q=golang&page=2
```

---

# Form Binding

For normal form submissions:

```go
type LoginForm struct {
	Email    string `form:"email"`
	Password string `form:"password"`
}

app.POST("/login", func(c *velocity.Context) error {
	var input LoginForm

	if err := c.BindForm(&input); err != nil {
		return err
	}

	return c.JSON(200, velocity.H{
		"email": input.Email,
	})
})
```

---

# Multipart Binding

Multipart requests are commonly used for forms containing files.

Velocity provides:

```go
c.BindMultipart(...)
```

along with dedicated file helpers.

For uploads, always combine multipart handling with a body-size limit.

---

# General Binding

Velocity also provides:

```go
c.Bind(...)
```

for general request binding.

Use the specific binder when you know exactly what input format your endpoint expects:

```go
c.BindJSON(...)
c.BindQuery(...)
c.BindForm(...)
c.BindMultipart(...)
```

This makes your endpoint behavior clearer.

---

# Validation

Velocity supports reflection-based validation tags.

Example:

```go
type CreateUser struct {
	Email string `json:"email" validate:"required,email"`
	Name  string `json:"name" validate:"required,min=3,max=80"`
}
```

Then:

```go
var input CreateUser

if err := c.BindJSON(&input); err != nil {
	return err
}
```

The validation tags can be used with the framework's validation/binding flow.

---

# Available Validation Tags

Velocity currently supports:

```text
required
omitempty
min=N
max=N
len=N
email
url
uuid
alpha
alphanum
numeric
oneof=a b
dive
```

---

# `required`

```go
type User struct {
	Name string `validate:"required"`
}
```

The value must be present/non-empty according to the validation rules.

---

# `min` and `max`

```go
type User struct {
	Name string `validate:"min=3,max=80"`
}
```

Useful for:

- String lengths
- Numeric constraints
- Collection sizes

---

# `len`

Require an exact length:

```go
type User struct {
	Code string `validate:"len=6"`
}
```

---

# `email`

```go
type User struct {
	Email string `validate:"required,email"`
}
```

---

# `url`

```go
type Website struct {
	URL string `validate:"required,url"`
}
```

---

# `uuid`

```go
type Request struct {
	ID string `validate:"required,uuid"`
}
```

---

# `alpha`

Only alphabetic characters:

```go
type Input struct {
	Name string `validate:"alpha"`
}
```

---

# `alphanum`

Alphabetic and numeric characters:

```go
type Input struct {
	Code string `validate:"alphanum"`
}
```

---

# `numeric`

Numeric validation:

```go
type Input struct {
	Code string `validate:"numeric"`
}
```

---

# `oneof`

Restrict a value to a list:

```go
type User struct {
	Role string `validate:"oneof=admin user guest"`
}
```

---

# `dive`

Validate individual collection elements.

```go
type Request struct {
	Tags []string `json:"tags" validate:"dive,min=2"`
}
```

Each tag is validated individually.

---

# `omitempty`

Skip validation when the value is empty:

```go
type User struct {
	Website string `validate:"omitempty,url"`
}
```

An empty website is allowed.

If provided, it must be a valid URL.

---

# File Uploads

For uploads, first limit the request size.

```go
app.POST(
	"/upload",
	velocity.MaxBodyBytes(8<<20),
	uploadAvatar,
)
```

`8 << 20` means approximately 8 MiB.

Then access the uploaded file.

The framework provides:

```go
FormFile(...)
MultipartFiles(...)
SaveUploadedFile(...)
```

A typical upload flow is:

```go
func uploadAvatar(c *velocity.Context) error {
	file, err := c.FormFile("avatar")
	if err != nil {
		return velocity.NewHTTPError(400, "avatar is required")
	}

	if err := c.SaveUploadedFile(file, "./uploads"); err != nil {
		return err
	}

	return c.JSON(201, velocity.H{
		"message": "uploaded",
	})
}
```

Use an upload directory that is not directly exposed as public static content unless that is explicitly intended.

Velocity's upload-saving behavior uses exclusive file creation and restrictive permissions, and upload filenames are restricted to simple filenames.

---

# Static Files

Serve a directory:

```go
app.Static("/public", "./public")
```

Now:

```text
public/style.css
```

can be available under:

```text
/public/style.css
```

Velocity requires the static directory to exist.

Static serving also protects against traversal and symlink escapes outside the configured directory.

---

# Example Static Website

Project:

```text
myapp/
├── main.go
└── public/
    ├── index.html
    ├── style.css
    └── app.js
```

Velocity:

```go
package main

import "github.com/raihan2bd/velocity"

func main() {
	app := velocity.Default()

	app.Static("/", "./public")

	_ = app.Run(":8080")
}
```

---

# HTTP Errors

Velocity uses a typed `HTTPError`.

An HTTP error contains:

```go
type HTTPError struct {
	Status  int
	Code    string
	Message string
	Err     error
}
```

The important distinction is:

- `Message` is safe to expose to the client.
- `Err` is intended for internal logging/debugging.

This allows your application to keep internal errors private.

---

# Returning an HTTP Error

For example:

```go
return velocity.NewHTTPError(
	http.StatusNotFound,
	"user not found",
)
```

The framework's error handling system can convert that into the appropriate HTTP response.

---

# Internal Errors vs Public Errors

Avoid this:

```go
return fmt.Errorf(
	"database password connection failed: %s",
	err,
)
```

if the resulting message could be exposed to the client.

Instead, create a safe public error while preserving the internal cause:

```go
return velocity.WrapHTTPError(
	http.StatusInternalServerError,
	"could not load user",
	err,
)
```

The client receives:

```text
could not load user
```

while the underlying error remains available for logging.

---

# Custom Error Codes

You can use the `Code` field to provide machine-readable application error codes.

For example:

```go
err := &velocity.HTTPError{
	Status:  http.StatusBadRequest,
	Code:    "INVALID_USER",
	Message: "The user data is invalid",
}

return err
```

Your frontend can use:

```json
{
  "error": {
    "code": "INVALID_USER",
    "message": "The user data is invalid"
  }
}
```

---

# Custom Error Handling

Velocity uses one central error handler.

This means handlers can simply return errors:

```go
func handler(c *velocity.Context) error {
	return someOperation()
}
```

Instead of every handler needing to manually convert every error into an HTTP response.

You can replace/customize the framework's error handler when your application needs a different response format.

---

# Panic Recovery

`Default()` includes recovery middleware.

That means an unexpected panic in a handler does not have to bring down the HTTP server.

Example:

```go
app.GET("/panic", func(c *velocity.Context) error {
	panic("something went wrong")
})
```

With recovery enabled, Velocity converts the panic into an internal server error response.

For logging, use:

```go
velocity.RecoveryWithLogger(logger)
```

when you want panic details sent to a logger.

---

# Request IDs

Velocity provides:

```go
velocity.RequestID()
```

A request ID allows you to connect a user's request with log entries.

Example:

```go
app.Use(velocity.RequestID())
```

Velocity accepts a valid client-provided request ID or generates a random 128-bit ID.

The ID is attached to the request context and response.

---

# Security Headers

Velocity provides:

```go
velocity.SecureHeaders(...)
```

and:

```go
velocity.DefaultSecurityHeaders()
```

The default configuration includes conservative browser security headers such as:

```text
X-Content-Type-Options
X-Frame-Options
Referrer-Policy
Permissions-Policy
Content-Security-Policy
```

HSTS is only emitted when serving through TLS.

Example:

```go
app.Use(
	velocity.SecureHeaders(
		velocity.DefaultSecurityHeaders(),
	),
)
```

---

# `Default()` Security Behavior

When you use:

```go
app := velocity.Default()
```

Velocity installs:

```text
Recovery
RequestID
SecureHeaders
```

automatically.

If you use:

```go
app := velocity.New()
```

you choose the middleware yourself.

This distinction is intentional.

---

# Body Size Limits

Protect endpoints from unexpectedly large request bodies:

```go
app.POST(
	"/upload",
	velocity.MaxBodyBytes(8<<20),
	upload,
)
```

The limit is approximately 8 MiB.

This is especially important for:

- File uploads
- JSON APIs
- Public endpoints
- Authentication endpoints

---

# CORS

Velocity provides explicit-origin CORS.

Example:

```go
app.Use(
	velocity.CORS(velocity.CORSConfig{
		AllowedOrigins: []string{
			"https://example.com",
		},
		AllowedMethods: []string{
			"GET",
			"POST",
			"PUT",
			"DELETE",
		},
		AllowedHeaders: []string{
			"Content-Type",
			"Authorization",
		},
	}),
)
```

Do not use wildcard origins together with credentials.

---

# CORS Preflight Requests

Browsers may send:

```text
OPTIONS
```

before the actual request.

Velocity's CORS handling validates requested headers rather than blindly reflecting arbitrary headers.

For example, if your browser sends:

```text
Access-Control-Request-Headers: Authorization
```

then:

```go
AllowedHeaders: []string{
	"Authorization",
}
```

should explicitly permit it.

---

# Rate Limiting

Velocity includes an in-memory token-bucket rate limiter.

Example:

```go
app.Use(
	velocity.RateLimit(velocity.RateLimitConfig{
		Rate:       10,
		Burst:      20,
		MaxEntries: 10000,
	}),
)
```

The limiter returns HTTP `429 Too Many Requests` when a bucket is empty and provides `Retry-After`.

By default, the key is based on:

```text
ClientIP
```

You can provide your own key function when the application needs a different rate-limit identity.

---

# Rate Limiting by API Key

For example:

```go
limiter := velocity.RateLimit(
	velocity.RateLimitConfig{
		Rate:  5,
		Burst: 10,
		Key: func(c *velocity.Context) string {
			return c.Header("X-API-Key")
		},
	},
)

app.Use(limiter)
```

This can limit clients based on an API key rather than IP address.

For distributed applications, remember that this limiter is in-memory and local to a process.

---

# Basic Authentication

Velocity includes Basic Authentication.

Example:

```go
auth := velocity.BasicAuth(
	map[string]string{
		"admin": "secret",
	},
	"admin",
)

app.GET(
	"/admin",
	auth,
	adminPage,
)
```

Use Basic Authentication for simple protected endpoints.

For a large user system, use a proper identity/authentication system rather than storing a large mutable credential map in middleware.

Always use HTTPS when transmitting Basic Authentication credentials.

---

# JSON Logging

Velocity includes JSON logging middleware.

Example:

```go
app.Use(
	velocity.JSONLogger(),
)
```

Structured logs are useful for systems such as:

- Production logging
- Log aggregation
- Cloud platforms
- Monitoring systems

JSON makes logs easier for machines to parse.

---

# Trusted Proxies

Velocity does not automatically trust proxy headers.

This is intentional.

If your application runs behind a reverse proxy such as:

```text
Browser
   ↓
Nginx
   ↓
Velocity
```

you may need to configure trusted proxies before relying on forwarded client IP information.

Use:

```go
app.SetTrustedProxies(...)
```

with only the CIDRs of proxies you actually control.

Do not blindly trust:

```text
X-Forwarded-For
X-Real-IP
```

from arbitrary clients.

---

# Client IP

Velocity provides client-IP functionality through the context.

The important rule is:

> Proxy information should only be trusted when the proxy is explicitly trusted.

Otherwise, an attacker may send a forged forwarding header and make your application believe the request came from another IP.

This matters especially for:

- Rate limiting
- Audit logging
- IP-based authorization
- Abuse detection

---

# NoRoute

Customize the response when no route matches.

Conceptually:

```go
app.NoRoute(func(c *velocity.Context) error {
	return c.JSON(404, velocity.H{
		"error": "route not found",
	})
})
```

This is useful for API applications that want consistent JSON errors.

---

# NoMethod

A path can exist while the requested HTTP method does not.

For example:

```text
GET /users
```

exists, but:

```text
POST /users
```

does not.

Velocity can return:

```text
405 Method Not Allowed
```

and provide the appropriate `Allow` header.

You can customize this behavior using `NoMethod`.

---

# Automatic OPTIONS

Velocity can automatically handle `OPTIONS` for routes.

This is particularly useful for browser CORS preflight behavior.

You can still explicitly register an `OPTIONS` route when your application needs custom behavior.

---

# Automatic 405 Handling

If a path exists but the HTTP method does not match:

```text
405 Method Not Allowed
```

is returned.

The response includes:

```text
Allow: GET, POST, ...
```

where appropriate.

This is standard HTTP behavior and makes API behavior more predictable.

---

# Freeze and Request-Time Performance

Velocity separates configuration from serving.

During application setup you can:

```text
Register routes
Register middleware
Configure groups
Configure handlers
Configure templates
```

Then the engine can freeze its configuration before serving.

The `Server`, `Run`, and `RunTLS` paths freeze the application before starting request processing.

Conceptually:

```text
Application startup
        ↓
Register routes
        ↓
Register middleware
        ↓
Configure application
        ↓
Freeze
        ↓
Serve requests
```

Freezing allows Velocity to prepare routing and middleware state for efficient request dispatch.

If you create your own `http.Server`, call:

```go
app.Freeze()
```

after application configuration.

---

# Context Pooling

Velocity uses a pool for request contexts.

This reduces repeated allocations in the request path.

However, there is an important rule:

> **Never retain a `*velocity.Context` after the handler finishes.**

Do not do this:

```go
var saved *velocity.Context

app.GET("/", func(c *velocity.Context) error {
	saved = c
	return nil
})
```

Do not do this either:

```go
app.GET("/", func(c *velocity.Context) error {
	go func() {
		c.JSON(200, velocity.H{
			"hello": "world",
		})
	}()

	return nil
})
```

The context belongs to the current request and can be reused by Velocity after the request finishes.

If a goroutine needs data, copy the data it needs before starting the goroutine.

For example:

```go
app.GET("/", func(c *velocity.Context) error {
	userID := c.Param("id")

	go func(id string) {
		doSomething(id)
	}(userID)

	return c.Text(200, "started")
})
```

Here the goroutine receives its own copy of the string.

---

# Building a Real API

A simple API might look like this:

```go
package main

import (
	"net/http"

	"github.com/raihan2bd/velocity"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	app := velocity.Default()

	api := app.Group("/api/v1")

	api.GET("/users/:id", getUser)
	api.POST("/users", createUser)
	api.DELETE("/users/:id", deleteUser)

	_ = app.Run(":8080")
}

func getUser(c *velocity.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return err
	}

	user := User{
		ID:    id,
		Name:  "Raihan",
		Email: "raihan@example.com",
	}

	return c.JSON(http.StatusOK, user)
}

func createUser(c *velocity.Context) error {
	var user User

	if err := c.BindJSON(&user); err != nil {
		return err
	}

	user.ID = 1

	return c.JSON(http.StatusCreated, user)
}

func deleteUser(c *velocity.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, velocity.H{
		"deleted": id,
	})
}
```

---

# Recommended Project Structure

Velocity itself does not force a project structure.

A beginner-friendly application could use:

```text
myapp/
│
├── main.go
│
├── handlers/
│   ├── users.go
│   └── auth.go
│
├── middleware/
│   └── auth.go
│
├── models/
│   └── user.go
│
├── routes/
│   └── routes.go
│
├── templates/
│   └── home.html
│
├── public/
│   ├── css/
│   └── js/
│
└── uploads/
```

As the project grows, separate responsibilities.

---

# Testing

Velocity is designed to work with Go's standard testing tools.

Create:

```text
users_test.go
```

Example:

```go
func TestGetUser(t *testing.T) {
	app := velocity.New()

	app.GET("/users/:id", func(c *velocity.Context) error {
		return c.JSON(200, velocity.H{
			"id": c.Param("id"),
		})
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/users/42",
		nil,
	)

	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d",
			rec.Code,
		)
	}
}
```

This tests the framework as an `http.Handler`.

---

# Testing JSON Responses

```go
func TestJSON(t *testing.T) {
	app := velocity.New()

	app.GET("/", func(c *velocity.Context) error {
		return c.JSON(200, velocity.H{
			"message": "hello",
		})
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/",
		nil,
	)

	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if got := rec.Body.String(); got == "" {
		t.Fatal("expected JSON body")
	}
}
```

---

# Race Detection

Because Velocity is designed for concurrent HTTP requests, race detection is especially important.

Run:

```bash
go test -race ./...
```

If the race detector reports a problem, investigate it before releasing.

This is particularly important for:

- Router state
- Middleware
- Context pooling
- Rate limiting
- Shared application state
- Custom middleware

---

# go vet

Run:

```bash
go vet ./...
```

This catches many common Go mistakes.

Before releasing a version, use:

```bash
go test ./...
go test -race ./...
go vet ./...
```

---

# Benchmarks

Velocity includes benchmarks for routing and comparisons with other Go frameworks.

Run the framework benchmarks:

```bash
go test -run '^$' -bench 'Benchmark(Static|Parameter)Route$' -benchmem -count=3
```

Run the comparison benchmarks:

```bash
go test -C benchmarks -run '^$' -bench . -benchmem -count=5
```

Benchmark results depend on:

- Go version
- CPU
- operating system
- workload
- compiler
- benchmark implementation

Do not assume a framework is universally faster based on one benchmark.

Velocity intentionally avoids claiming universal superiority over other frameworks.

---

# Common Mistakes

## Mistake 1: Starting two servers on the same port

This will fail:

```text
listen tcp :8080:
bind: Only one usage of each socket address...
```

This means another process is already using port 8080.

On Windows:

```cmd
netstat -ano | findstr :8080
```

Then identify the PID:

```cmd
tasklist /FI "PID eq 1234"
```

Stop it if appropriate:

```cmd
taskkill /PID 1234 /F
```

---

# Mistake 2: Keeping the Context

Don't store:

```go
var context *velocity.Context
```

Contexts are request-scoped and pooled.

---

# Mistake 3: Trusting Proxy Headers Automatically

Don't assume:

```text
X-Forwarded-For
```

is trustworthy.

Configure trusted proxy CIDRs explicitly.

---

# Mistake 4: Accepting Unlimited Uploads

Always combine file upload endpoints with:

```go
velocity.MaxBodyBytes(...)
```

---

# Mistake 5: Exposing Upload Directories

Don't automatically serve:

```text
./uploads
```

as public static files.

Uploaded files may contain content that you don't want browsers to execute or expose.

---

# Mistake 6: Using wildcard CORS casually

Don't configure permissive CORS without understanding the security consequences.

Prefer explicit origins:

```go
AllowedOrigins: []string{
	"https://example.com",
}
```

---

# Mistake 7: Exposing internal errors

Don't send database or filesystem errors directly to clients.

Prefer:

```go
return velocity.WrapHTTPError(
	500,
	"something went wrong",
	err,
)
```

---

# Mistake 8: Adding routes after serving

Configure the application first:

```text
New
 ↓
Routes
 ↓
Middleware
 ↓
Freeze
 ↓
Serve
```

Don't treat the running router as your normal configuration API.

---

# Production Checklist

Before deploying a Velocity application:

## Application

- [ ] Routes are registered during startup.
- [ ] Middleware is registered before serving.
- [ ] Errors are handled consistently.
- [ ] Context values aren't retained.
- [ ] Background goroutines don't use request contexts after handlers return.

## Security

- [ ] HTTPS is enabled.
- [ ] Security headers are configured.
- [ ] CORS origins are explicit.
- [ ] Upload limits are configured.
- [ ] Upload directories are protected.
- [ ] Proxy CIDRs are explicitly trusted.
- [ ] Basic Auth is only used over HTTPS.
- [ ] Sensitive errors are not exposed.

## Server

- [ ] Read timeout configured.
- [ ] Write timeout configured.
- [ ] Idle timeout configured.
- [ ] Header size limits considered.
- [ ] Graceful shutdown implemented.

## Testing

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] Integration tests
- [ ] Router edge-case tests
- [ ] Upload tests
- [ ] CORS tests
- [ ] Middleware tests

## Performance

- [ ] Benchmark representative workloads.
- [ ] Test with realistic payload sizes.
- [ ] Test concurrency.
- [ ] Profile before optimizing.

---

# Complete Example Application

The following example combines many Velocity features into one small application.

```go
package main

import (
	"net/http"
	"time"

	"github.com/raihan2bd/velocity"
)

type CreateUserRequest struct {
	Name  string `json:"name" validate:"required,min=3,max=80"`
	Email string `json:"email" validate:"required,email"`
}

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	app := velocity.Default()

	// Global request protection.
	app.Use(
		velocity.MaxBodyBytes(8 << 20),
	)

	// Public route.
	app.GET("/", func(c *velocity.Context) error {
		return c.JSON(http.StatusOK, velocity.H{
			"name":    "Velocity API",
			"version": "0.0.1",
		})
	})

	// API group.
	api := app.Group("/api/v1")

	// List users.
	api.GET("/users", func(c *velocity.Context) error {
		return c.JSON(http.StatusOK, []User{
			{
				ID:    1,
				Name:  "Raihan",
				Email: "raihan@example.com",
			},
		})
	})

	// Get a user.
	api.GET("/users/:id", func(c *velocity.Context) error {
		id, err := c.ParamInt("id")
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, User{
			ID:    id,
			Name:  "Raihan",
			Email: "raihan@example.com",
		})
	})

	// Create a user.
	api.POST("/users", func(c *velocity.Context) error {
		var input CreateUserRequest

		if err := c.BindJSON(&input); err != nil {
			return err
		}

		user := User{
			ID:    2,
			Name:  input.Name,
			Email: input.Email,
		}

		return c.JSON(
			http.StatusCreated,
			user,
		)
	})

	// Cookie example.
	api.GET("/login", func(c *velocity.Context) error {
		c.SetCookie(&http.Cookie{
			Name:     "session",
			Value:    "example-session",
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int((24 * time.Hour).Seconds()),
		})

		return c.JSON(http.StatusOK, velocity.H{
			"message": "logged in",
		})
	})

	// Protected route.
	api.GET("/profile", func(c *velocity.Context) error {
		session, err := c.Cookie("session")
		if err != nil {
			return velocity.NewHTTPError(
				http.StatusUnauthorized,
				"not authenticated",
			)
		}

		return c.JSON(http.StatusOK, velocity.H{
			"session": session,
		})
	})

	// Static files.
	app.Static("/public", "./public")

	// Custom not-found response.
	app.NoRoute(func(c *velocity.Context) error {
		return c.JSON(http.StatusNotFound, velocity.H{
			"error": "route not found",
		})
	})

	// Start.
	_ = app.Run(":8080")
}
```

---

# Recommended Beginner Learning Path

If you are new to Go and Velocity, don't try to learn every feature at once.

Follow this order:

## Step 1 — Basic server

Learn:

```go
velocity.New()
velocity.Default()
app.Run()
```

## Step 2 — Routes

Learn:

```go
GET
POST
PUT
PATCH
DELETE
```

## Step 3 — Parameters

Learn:

```go
:id
*path
c.Param()
c.ParamInt()
```

## Step 4 — Responses

Learn:

```go
JSON
Text
String
Data
XML
```

## Step 5 — Request data

Learn:

```go
Query
QueryDefault
Header
Cookie
```

## Step 6 — Binding

Learn:

```go
BindJSON
BindQuery
BindForm
BindMultipart
Bind
```

## Step 7 — Validation

Learn:

```go
required
email
min
max
oneof
dive
```

## Step 8 — Middleware

Learn:

```go
Use()
Group()
Next()
```

## Step 9 — Errors

Learn:

```go
HTTPError
NewHTTPError
WrapHTTPError
```

## Step 10 — Security

Learn:

```go
Recovery
RequestID
SecureHeaders
CORS
RateLimit
MaxBodyBytes
BasicAuth
TrustedProxies
```

## Step 11 — Files

Learn:

```go
Static
File
FileAttachment
FormFile
MultipartFiles
SaveUploadedFile
```

## Step 12 — Advanced features

Finally learn:

```text
HTML templates
SSE
custom HTTP server
Freeze
context pooling
benchmarks
```

---

# Velocity's Mental Model

If you're completely new to web frameworks, remember this:

```text
                 HTTP Request
                       │
                       ▼
                 ┌───────────┐
                 │  Router   │
                 └─────┬─────┘
                       │
                       ▼
                 ┌───────────┐
                 │Middleware│
                 └─────┬─────┘
                       │
                       ▼
                 ┌───────────┐
                 │  Context  │
                 └─────┬─────┘
                       │
              ┌────────┼────────┐
              │        │        │
              ▼        ▼        ▼
           Params    Binding   Headers
              │        │        │
              └────────┼────────┘
                       │
                       ▼
                    Handler
                       │
                       ▼
                  Response
                       │
                       ▼
                    Client
```

The important idea is:

> **A route receives a request, middleware prepares or protects it, the handler performs the application's work, and the handler returns a response or an error.**

Once you understand that flow, most of Velocity becomes much easier.

---

# Philosophy

Velocity tries to stay close to Go's standard library.

Instead of hiding HTTP concepts, it gives you a convenient layer around them.

For example:

```go
func(c *velocity.Context) error
```

is your application handler.

Underneath it, Velocity still works with Go's:

```go
net/http
```

types and behavior.

This means learning Velocity should also help you learn Go HTTP development.

---

# Dependency Philosophy

Velocity intentionally has no third-party runtime dependencies.

The framework uses Go's standard library for core functionality such as:

- HTTP
- JSON
- XML
- HTML templates
- cookies
- files
- cryptography/randomness
- synchronization
- logging
- networking

This keeps the dependency tree small and makes the framework easier to inspect.

---

# Performance Philosophy

Velocity is designed to minimize unnecessary work in the request path.

Some of the design choices include:

- Pooled request contexts
- Method-aware routing
- Prepared route state
- Middleware preparation
- Freeze-time configuration
- Avoiding unnecessary hot-path allocations

However:

> **Velocity does not claim to be universally faster than every other Go framework.**

Always benchmark your real workload.

---

# Versioning

Velocity is currently pre-1.0.

The first public development release can be:

```text
v0.0.1
```

Install it with:

```bash
go get github.com/raihan2bd/velocity@v0.0.1
```

Before `v1.0.0`, APIs may change.

Once Velocity reaches a stable API, semantic versioning should be followed:

```text
v1.0.0
v1.0.1
v1.1.0
v2.0.0
```

Generally:

```text
PATCH
```

is for compatible fixes.

```text
MINOR
```

is for compatible features.

```text
MAJOR
```

is for breaking changes.

---

# Getting Help

When something doesn't work, first check:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Then verify:

1. The route is registered.
2. The HTTP method is correct.
3. Middleware isn't stopping the request.
4. The request body matches the expected format.
5. Validation tags are correct.
6. The server is listening on the expected port.
7. Another process isn't already using the port.

For the latest API and source code:

```text
https://github.com/raihan2bd/velocity
```

---

# Final Example: A Small but Proper Velocity API

```go
package main

import (
	"net/http"

	"github.com/raihan2bd/velocity"
)

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUser struct {
	Name  string `json:"name" validate:"required,min=3,max=80"`
	Email string `json:"email" validate:"required,email"`
}

func main() {
	app := velocity.Default()

	api := app.Group("/api/v1")

	api.GET("/users/:id", getUser)
	api.POST("/users", createUser)

	app.NoRoute(func(c *velocity.Context) error {
		return c.JSON(404, velocity.H{
			"error": "not found",
		})
	})

	_ = app.Run(":8080")
}

func getUser(c *velocity.Context) error {
	id, err := c.ParamInt("id")
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, User{
		ID:    id,
		Name:  "Raihan",
		Email: "raihan@example.com",
	})
}

func createUser(c *velocity.Context) error {
	var input CreateUser

	if err := c.BindJSON(&input); err != nil {
		return err
	}

	return c.JSON(http.StatusCreated, velocity.H{
		"message": "user created",
		"user": User{
			ID:    1,
			Name:  input.Name,
			Email: input.Email,
		},
	})
}
```

Run:

```bash
go run .
```

Then test:

```bash
curl http://localhost:8080/api/v1/users/1
```

and:

```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Raihan\",\"email\":\"raihan@example.com\"}"
```

---

# Summary

Velocity gives you a relatively small set of concepts that combine to build complete HTTP applications:

```text
Engine
  │
  ├── Routes
  │     ├── Static
  │     ├── Parameters
  │     └── Catch-all
  │
  ├── Groups
  │
  ├── Middleware
  │     ├── Recovery
  │     ├── Request ID
  │     ├── Security Headers
  │     ├── CORS
  │     ├── Rate Limit
  │     ├── Body Limits
  │     ├── Basic Auth
  │     └── JSON Logging
  │
  ├── Context
  │     ├── Request data
  │     ├── Responses
  │     ├── Cookies
  │     ├── Files
  │     └── SSE
  │
  ├── Binding
  │     ├── JSON
  │     ├── Query
  │     ├── Form
  │     └── Multipart
  │
  ├── Validation
  │
  ├── HTTP Errors
  │
  ├── Static Files
  │
  └── HTTP Server
```

If you understand those pieces, you understand the core of Velocity.

**Build first. Keep the API understandable. Return errors explicitly. Configure security consciously. Test before optimizing.**
