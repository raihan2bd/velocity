package velocity

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func serve(engine http.Handler, method, target string, body io.Reader) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, body)
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestRoutesGroupsAndMiddleware(t *testing.T) {
	engine := New()
	var order []string
	engine.Use(func(c *Context) error {
		order = append(order, "global-before")
		err := c.Next()
		order = append(order, "global-after")
		return err
	})
	api := engine.Group("/api", func(c *Context) error {
		order = append(order, "group-before")
		err := c.Next()
		order = append(order, "group-after")
		return err
	})
	api.GET("/users/:id", func(c *Context) error {
		order = append(order, "handler")
		return c.String(http.StatusOK, "%s", c.Param("id"))
	})
	engine.GET("/users/me", func(c *Context) error { return c.String(http.StatusOK, "me") })
	engine.GET("/files/*path", func(c *Context) error { return c.String(http.StatusOK, "%s", c.Param("path")) })

	response := serve(engine, http.MethodGet, "/api/users/42", nil)
	if response.Code != http.StatusOK || response.Body.String() != "42" {
		t.Fatalf("unexpected group route response: %d %q", response.Code, response.Body.String())
	}
	if got, want := strings.Join(order, ","), "global-before,group-before,handler,group-after,global-after"; got != want {
		t.Fatalf("middleware order = %q, want %q", got, want)
	}
	if response = serve(engine, http.MethodGet, "/users/me", nil); response.Body.String() != "me" {
		t.Fatalf("static route did not win: %q", response.Body.String())
	}
	if response = serve(engine, http.MethodGet, "/files/a/b.txt", nil); response.Body.String() != "/a/b.txt" {
		t.Fatalf("catch-all = %q", response.Body.String())
	}
}

func TestMethodHandlingAndCustomNotFound(t *testing.T) {
	engine := New()
	engine.GET("/items", func(c *Context) error { return c.String(http.StatusOK, "ok") })

	response := serve(engine, http.MethodOptions, "/items", nil)
	if response.Code != http.StatusNoContent || response.Header().Get("Allow") != "GET, HEAD, OPTIONS" {
		t.Fatalf("automatic options = %d, Allow=%q", response.Code, response.Header().Get("Allow"))
	}
	response = serve(engine, http.MethodPost, "/items", nil)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD, OPTIONS" {
		t.Fatalf("method response = %d, Allow=%q", response.Code, response.Header().Get("Allow"))
	}
	response = serve(engine, http.MethodHead, "/items", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET fallback for HEAD returned %d", response.Code)
	}
	engine.NoRoute(func(c *Context) error { return c.String(http.StatusGone, "gone") })
	response = serve(engine, http.MethodGet, "/missing", nil)
	if response.Code != http.StatusGone || response.Body.String() != "gone" {
		t.Fatalf("custom 404 = %d %q", response.Code, response.Body.String())
	}
}

func TestDefaultSecurityAndErrors(t *testing.T) {
	type payload struct {
		Email string `json:"email" validate:"required,email"`
		Name  string `json:"name" validate:"min=3"`
	}
	engine := Default()
	engine.POST("/accounts", func(c *Context) error {
		var input payload
		if err := c.BindJSON(&input); err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, input)
	})

	// Content-Type belongs on the request rather than the response.
	request := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{"email":"not-an-email","name":"ab"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("validation status = %d, body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Security-Policy") == "" || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("missing default security headers: %#v", response.Header())
	}

	request = httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(`{"email":"a@b.com","unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown JSON field status = %d", response.Code)
	}
}

func TestRecoveryAndSingleCapturedError(t *testing.T) {
	engine := New()
	engine.Use(RecoveryWithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	var captured int
	engine.SetErrorHandler(func(c *Context, err error) {
		captured = len(c.Errors())
		_ = c.String(http.StatusInternalServerError, "safe")
	})
	engine.GET("/panic", func(*Context) error { panic("boom") })
	response := serve(engine, http.MethodGet, "/panic", nil)
	if response.Code != http.StatusInternalServerError || response.Body.String() != "safe" || captured != 1 {
		t.Fatalf("recovery result = %d %q errors=%d", response.Code, response.Body.String(), captured)
	}
}

func TestBindQueryAndValidationDive(t *testing.T) {
	type filters struct {
		Name string   `query:"name" validate:"required,min=3"`
		Tags []string `query:"tag" validate:"dive,min=2"`
	}
	engine := Default()
	engine.GET("/search", func(c *Context) error {
		var filter filters
		if err := c.BindQuery(&filter); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, filter)
	})
	response := serve(engine, http.MethodGet, "/search?name=valid&tag=ok&tag=x", nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("dive validation status = %d: %s", response.Code, response.Body.String())
	}
	response = serve(engine, http.MethodGet, "/search?name=valid&tag=ok&tag=good", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("query bind status = %d: %s", response.Code, response.Body.String())
	}
	type nestedChild struct {
		Token string `validate:"required"`
	}
	type nestedRequest struct {
		Child nestedChild `json:"child"`
	}
	if err := Validate(nestedRequest{}); err == nil {
		t.Fatal("nested validation tag was skipped")
	}
}

func TestCORSRateLimitAndTrustedProxy(t *testing.T) {
	engine := New()
	engine.Use(CORS(CORSConfig{AllowedOrigins: []string{"https://app.example"}, AllowedMethods: []string{http.MethodGet}}))
	engine.Use(RateLimit(RateLimitConfig{Rate: 1, Burst: 1, Key: func(*Context) string { return "same" }}))
	if err := engine.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	engine.GET("/whoami", func(c *Context) error { return c.String(http.StatusOK, "%s", c.ClientIP()) })
	preflight := httptest.NewRequest(http.MethodOptions, "/whoami", nil)
	preflight.Header.Set("Origin", "https://app.example")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightResponse := httptest.NewRecorder()
	engine.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent || preflightResponse.Header().Get("Access-Control-Allow-Methods") != http.MethodGet {
		t.Fatalf("CORS preflight = %d %#v", preflightResponse.Code, preflightResponse.Header())
	}
	blockedPreflight := httptest.NewRequest(http.MethodOptions, "/whoami", nil)
	blockedPreflight.Header.Set("Origin", "https://app.example")
	blockedPreflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	blockedPreflight.Header.Set("Access-Control-Request-Headers", "X-Not-Allowed")
	blockedResponse := httptest.NewRecorder()
	engine.ServeHTTP(blockedResponse, blockedPreflight)
	if blockedResponse.Code != http.StatusForbidden {
		t.Fatalf("unsafe CORS request headers were accepted: %d", blockedResponse.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	request.Header.Set("Origin", "https://app.example")
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.2")
	request.RemoteAddr = "10.0.0.1:1234"
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "203.0.113.9" || response.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("proxy/CORS result = %d %q %#v", response.Code, response.Body.String(), response.Header())
	}
	response = serve(engine, http.MethodGet, "/whoami", nil)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit result = %d", response.Code)
	}
}

func TestUploadAndStaticFiles(t *testing.T) {
	directory := t.TempDir()
	engine := Default()
	engine.POST("/upload", func(c *Context) error {
		file, err := c.FormFile("file", 1<<20)
		if err != nil {
			return err
		}
		path, err := SaveUploadedFile(file, directory, "saved.txt")
		if err != nil {
			return err
		}
		return c.String(http.StatusCreated, "%s", filepath.Base(path))
	})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "../../untrusted.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write([]byte("hello upload")); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Body.String() != "saved.txt" {
		t.Fatalf("upload response = %d %q", response.Code, response.Body.String())
	}
	contents, err := os.ReadFile(filepath.Join(directory, "saved.txt"))
	if err != nil || string(contents) != "hello upload" {
		t.Fatalf("saved content = %q, err=%v", contents, err)
	}
	engine.Static("/assets", directory)
	response = serve(engine, http.MethodGet, "/assets/saved.txt", nil)
	if response.Code != http.StatusOK || response.Body.String() != "hello upload" {
		t.Fatalf("static response = %d %q", response.Code, response.Body.String())
	}
	if !isWithinDirectory(directory, filepath.Join(directory, "saved.txt")) || isWithinDirectory(directory, filepath.Join(directory, "..", "outside.txt")) {
		t.Fatal("static path containment check failed")
	}
}

func TestConfigurationErrorsAndHead(t *testing.T) {
	engine := New()
	if _, err := engine.Handle(http.MethodGet, "no-leading-slash", func(*Context) error { return nil }); err == nil {
		t.Fatal("expected bad path error")
	}
	if _, err := engine.Handle(http.MethodGet, "/x/:a", func(*Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Handle(http.MethodGet, "/x/:b", func(*Context) error { return nil }); err == nil {
		t.Fatal("expected parameter conflict")
	}
	if _, err := engine.Handle(http.MethodGet, "/x/*rest/more", func(*Context) error { return nil }); err == nil {
		t.Fatal("expected wildcard error")
	}

	server := engine.Server(":0")
	if server.ReadHeaderTimeout != 5*time.Second || server.MaxHeaderBytes != 1<<20 {
		t.Fatal("server safe defaults missing")
	}
}

func TestFreezeEnablesImmutableConfiguration(t *testing.T) {
	engine := New()
	engine.GET("/health", func(c *Context) error { return c.Text(http.StatusOK, "ok") })
	engine.Freeze()
	if !engine.IsFrozen() {
		t.Fatal("engine was not frozen")
	}
	if response := serve(engine, http.MethodGet, "/health", nil); response.Code != http.StatusOK || response.Body.String() != "ok" {
		t.Fatalf("frozen engine response = %d %q", response.Code, response.Body.String())
	}
	if _, err := engine.Handle(http.MethodGet, "/later", func(*Context) error { return nil }); err == nil {
		t.Fatal("expected frozen route error")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected frozen middleware panic")
		}
	}()
	engine.Use(func(c *Context) error { return c.Next() })
}

func TestHTMLXMLAndTextResponses(t *testing.T) {
	engine := New()
	engine.SetHTMLTemplate(template.Must(template.New("welcome").Parse(`Hello {{.Name}}`)))
	engine.GET("/page", func(c *Context) error { return c.HTML(http.StatusOK, "welcome", H{"Name": "Ada"}) })
	engine.GET("/xml", func(c *Context) error {
		return c.XML(http.StatusOK, struct {
			XMLName string `xml:"message"`
			Text    string `xml:",chardata"`
		}{Text: "hello"})
	})
	engine.GET("/text", func(c *Context) error { return c.Text(http.StatusOK, "100% safe") })

	response := serve(engine, http.MethodGet, "/page", nil)
	if response.Code != http.StatusOK || response.Body.String() != "Hello Ada" || response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("HTML response = %d %q %#v", response.Code, response.Body.String(), response.Header())
	}
	response = serve(engine, http.MethodGet, "/xml", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<message>hello</message>") {
		t.Fatalf("XML response = %d %q", response.Code, response.Body.String())
	}
	response = serve(engine, http.MethodGet, "/text", nil)
	if response.Body.String() != "100% safe" {
		t.Fatalf("text response = %q", response.Body.String())
	}
}
