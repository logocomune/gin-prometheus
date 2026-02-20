package ginprom

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkMiddlewareWithMetrics_Basic(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	mc := newTestMetrics()
	router := gin.New()
	router.Use(MiddlewareWithMetrics(mc))
	router.GET("/hello", func(c *gin.Context) {
		c.String(http.StatusOK, "hello")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/hello", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkMiddlewareWithMetrics_DisabledAll(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	mc := newTestMetrics()
	router := gin.New()
	router.Use(MiddlewareWithMetrics(mc,
		WithRecordRequestSize(false),
		WithRecordResponseSize(false),
		WithRecordDuration(false),
	))
	router.GET("/hello", func(c *gin.Context) {
		c.String(http.StatusOK, "hello")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/hello", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkMiddlewareWithMetrics_Filtered(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	mc := newTestMetrics()
	router := gin.New()
	router.Use(MiddlewareWithMetrics(mc, WithFilterRoutes([]string{"/health"})))
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}

func BenchmarkGetPathWithFallback_Registered(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	var capturedPath string
	router.GET("/users/:id", func(c *gin.Context) {
		capturedPath = getPathWithFallback(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users/42", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
	_ = capturedPath
}

func BenchmarkGetPathWithFallback_Unregistered(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.NoRoute(func(c *gin.Context) {
		_ = getPathWithFallback(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/does-not-exist", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.ServeHTTP(w, req)
	}
}
