package velocity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type benchmarkWriter struct{ header http.Header }

func (w *benchmarkWriter) Header() http.Header      { return w.header }
func (*benchmarkWriter) Write([]byte) (int, error)  { return 0, nil }
func (*benchmarkWriter) WriteHeader(statusCode int) {}

func BenchmarkStaticRoute(b *testing.B) {
	engine := New()
	engine.GET("/health", func(c *Context) error { c.Status(http.StatusNoContent); return nil })
	engine.Freeze()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	writer := &benchmarkWriter{header: make(http.Header)}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}

func BenchmarkParameterRoute(b *testing.B) {
	engine := New()
	engine.GET("/accounts/:accountID/invoices/:invoiceID", func(c *Context) error {
		if c.Param("accountID") == "" || c.Param("invoiceID") == "" {
			b.Fatal("missing route parameter")
		}
		c.Status(http.StatusNoContent)
		return nil
	})
	engine.Freeze()
	request := httptest.NewRequest(http.MethodGet, "/accounts/42/invoices/99", nil)
	writer := &benchmarkWriter{header: make(http.Header)}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		engine.ServeHTTP(writer, request)
	}
}
