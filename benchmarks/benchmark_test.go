package benchmarks

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gofiber/fiber/v3"
	"github.com/labstack/echo/v4"
	velocity "github.com/raihan2bd/velocity"
)

type discardWriter struct{ header http.Header }

func newDiscardWriter() *discardWriter                { return &discardWriter{header: make(http.Header)} }
func (w *discardWriter) Header() http.Header          { return w.header }
func (*discardWriter) Write(data []byte) (int, error) { return len(data), nil }
func (*discardWriter) WriteHeader(int)                {}

type payload struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

var jsonPayload = []byte(`{"name":"Ada Lovelace","email":"ada@example.test"}`)

func resetJSONRequest(request *http.Request) {
	request.Body = io.NopCloser(bytes.NewReader(jsonPayload))
	request.ContentLength = int64(len(jsonPayload))
}

func decodeStrictJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func BenchmarkStaticRoute(b *testing.B) {
	b.Run("Velocity", benchmarkVelocityStatic)
	b.Run("Gin", benchmarkGinStatic)
	b.Run("Echo", benchmarkEchoStatic)
	b.Run("Fiber_AppTest", benchmarkFiberStatic)
}

func BenchmarkParameterRoute(b *testing.B) {
	b.Run("Velocity", benchmarkVelocityParameter)
	b.Run("Gin", benchmarkGinParameter)
	b.Run("Echo", benchmarkEchoParameter)
	b.Run("Fiber_AppTest", benchmarkFiberParameter)
}

func BenchmarkMiddlewareChain(b *testing.B) {
	b.Run("Velocity", benchmarkVelocityMiddleware)
	b.Run("Gin", benchmarkGinMiddleware)
	b.Run("Echo", benchmarkEchoMiddleware)
	b.Run("Fiber_AppTest", benchmarkFiberMiddleware)
}

func BenchmarkJSONRoundTrip(b *testing.B) {
	b.Run("Velocity", benchmarkVelocityJSON)
	b.Run("Gin", benchmarkGinJSON)
	b.Run("Echo", benchmarkEchoJSON)
	b.Run("Fiber_AppTest", benchmarkFiberJSON)
}

func benchmarkVelocityStatic(b *testing.B) {
	engine := velocity.New()
	engine.GET("/health", func(c *velocity.Context) error { c.Status(http.StatusNoContent); return nil })
	engine.Freeze()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkGinStatic(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.GET("/health", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkEchoStatic(b *testing.B) {
	engine := echo.New()
	engine.GET("/health", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkFiberStatic(b *testing.B) {
	app := fiber.New()
	app.Get("/health", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response, err := app.Test(request)
		if err != nil {
			b.Fatal(err)
		}
		_ = response.Body.Close()
	}
}

func benchmarkVelocityParameter(b *testing.B) {
	engine := velocity.New()
	engine.GET("/accounts/:accountID/invoices/:invoiceID", func(c *velocity.Context) error {
		if c.Param("accountID") != "42" || c.Param("invoiceID") != "99" {
			b.Fatal("route parameter mismatch")
		}
		c.Status(http.StatusNoContent)
		return nil
	})
	engine.Freeze()
	request := httptest.NewRequest(http.MethodGet, "/accounts/42/invoices/99", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkGinParameter(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.GET("/accounts/:accountID/invoices/:invoiceID", func(c *gin.Context) {
		if c.Param("accountID") != "42" || c.Param("invoiceID") != "99" {
			b.Fatal("route parameter mismatch")
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/accounts/42/invoices/99", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkEchoParameter(b *testing.B) {
	engine := echo.New()
	engine.GET("/accounts/:accountID/invoices/:invoiceID", func(c echo.Context) error {
		if c.Param("accountID") != "42" || c.Param("invoiceID") != "99" {
			b.Fatal("route parameter mismatch")
		}
		return c.NoContent(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/accounts/42/invoices/99", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkFiberParameter(b *testing.B) {
	app := fiber.New()
	app.Get("/accounts/:accountID/invoices/:invoiceID", func(c fiber.Ctx) error {
		if c.Params("accountID") != "42" || c.Params("invoiceID") != "99" {
			b.Fatal("route parameter mismatch")
		}
		return c.SendStatus(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/accounts/42/invoices/99", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response, err := app.Test(request)
		if err != nil {
			b.Fatal(err)
		}
		_ = response.Body.Close()
	}
}

func benchmarkVelocityMiddleware(b *testing.B) {
	engine := velocity.New()
	for index := 0; index < 3; index++ {
		engine.Use(func(c *velocity.Context) error { return c.Next() })
	}
	engine.GET("/health", func(c *velocity.Context) error { c.Status(http.StatusNoContent); return nil })
	engine.Freeze()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkGinMiddleware(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	for index := 0; index < 3; index++ {
		engine.Use(func(c *gin.Context) { c.Next() })
	}
	engine.GET("/health", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkEchoMiddleware(b *testing.B) {
	engine := echo.New()
	for index := 0; index < 3; index++ {
		engine.Use(func(next echo.HandlerFunc) echo.HandlerFunc { return func(c echo.Context) error { return next(c) } })
	}
	engine.GET("/health", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkFiberMiddleware(b *testing.B) {
	app := fiber.New()
	for index := 0; index < 3; index++ {
		app.Use(func(c fiber.Ctx) error { return c.Next() })
	}
	app.Get("/health", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		response, err := app.Test(request)
		if err != nil {
			b.Fatal(err)
		}
		_ = response.Body.Close()
	}
}

func benchmarkVelocityJSON(b *testing.B) {
	engine := velocity.New()
	engine.POST("/payload", func(c *velocity.Context) error {
		var value payload
		if err := c.BindJSON(&value); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, value)
	})
	engine.Freeze()
	request := httptest.NewRequest(http.MethodPost, "/payload", nil)
	request.Header.Set("Content-Type", "application/json")
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		resetJSONRequest(request)
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkGinJSON(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.POST("/payload", func(c *gin.Context) {
		var value payload
		if err := decodeStrictJSON(c.Request.Body, &value); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		data, err := json.Marshal(value)
		if err != nil { c.Status(http.StatusInternalServerError); return }
		c.Data(http.StatusOK, "application/json; charset=utf-8", data)
	})
	request := httptest.NewRequest(http.MethodPost, "/payload", nil)
	request.Header.Set("Content-Type", "application/json")
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		resetJSONRequest(request)
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkEchoJSON(b *testing.B) {
	engine := echo.New()
	engine.POST("/payload", func(c echo.Context) error {
		var value payload
		if err := decodeStrictJSON(c.Request().Body, &value); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest)
		}
		data, err := json.Marshal(value)
		if err != nil { return echo.NewHTTPError(http.StatusInternalServerError) }
		return c.Blob(http.StatusOK, "application/json; charset=utf-8", data)
	})
	request := httptest.NewRequest(http.MethodPost, "/payload", nil)
	request.Header.Set("Content-Type", "application/json")
	writer := newDiscardWriter()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		resetJSONRequest(request)
		engine.ServeHTTP(writer, request)
	}
}

func benchmarkFiberJSON(b *testing.B) {
	app := fiber.New()
	app.Post("/payload", func(c fiber.Ctx) error {
		var value payload
		if err := decodeStrictJSON(bytes.NewReader(c.Body()), &value); err != nil {
			return fiber.ErrBadRequest
		}
		data, err := json.Marshal(value)
		if err != nil { return err }
		c.Set("Content-Type", "application/json; charset=utf-8")
		return c.Status(http.StatusOK).Send(data)
	})
	request := httptest.NewRequest(http.MethodPost, "/payload", nil)
	request.Header.Set("Content-Type", "application/json")
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		resetJSONRequest(request)
		response, err := app.Test(request)
		if err != nil {
			b.Fatal(err)
		}
		_ = response.Body.Close()
	}
}
