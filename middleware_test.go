package ginprom

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRegistry creates a fresh prometheus registry for isolated tests.
func newTestRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

// newTestMetrics returns a MetricsCollection backed by an isolated registry.
func newTestMetrics() *MetricsCollection {
	return NewMetricsCollection(WithCustomRegistry(newTestRegistry()))
}

// performRequest fires a GET request against the provided router and returns the response.
func performRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, nil)
	r.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// getPathWithFallback
// ---------------------------------------------------------------------------

func TestGetPathWithFallback_RegisteredRoute(t *testing.T) {
	r := gin.New()
	var capturedPath string
	r.GET("/ping", func(c *gin.Context) {
		capturedPath = getPathWithFallback(c)
		c.Status(http.StatusOK)
	})
	performRequest(r, "GET", "/ping")
	if capturedPath != "/ping" {
		t.Errorf("expected /ping, got %q", capturedPath)
	}
}

func TestGetPathWithFallback_UnregisteredRoute(t *testing.T) {
	r := gin.New()
	r.NoRoute(func(c *gin.Context) {
		path := getPathWithFallback(c)
		// FullPath is empty for unregistered routes; should fall back to URL path
		if !strings.HasPrefix(path, "/") {
			t.Errorf("expected a path starting with '/', got %q", path)
		}
		c.Status(http.StatusNotFound)
	})
	performRequest(r, "GET", "/does-not-exist")
}

func TestGetPathWithFallback_WithParam(t *testing.T) {
	r := gin.New()
	var capturedPath string
	r.GET("/users/:id", func(c *gin.Context) {
		capturedPath = getPathWithFallback(c)
		c.Status(http.StatusOK)
	})
	performRequest(r, "GET", "/users/42")
	if capturedPath != "/users/:id" {
		t.Errorf("expected /users/:id, got %q", capturedPath)
	}
}

// ---------------------------------------------------------------------------
// handleUnmatchedPath
// ---------------------------------------------------------------------------

func TestHandleUnmatchedPath_Disabled(t *testing.T) {
	conf := &config{handleUnmatchedRoutes: false}
	route, path := handleUnmatchedPath(conf, "", "/some/path")
	if route != "" {
		t.Errorf("expected empty route, got %q", route)
	}
	if path != "/some/path" {
		t.Errorf("expected /some/path, got %q", path)
	}
}

func TestHandleUnmatchedPath_Grouped(t *testing.T) {
	conf := &config{handleUnmatchedRoutes: true, groupUnmatchedRoutes: true}
	route, _ := handleUnmatchedPath(conf, "", "/random/url")
	if route != "/unmatched/*" {
		t.Errorf("expected /unmatched/*, got %q", route)
	}
}

func TestHandleUnmatchedPath_Ungrouped(t *testing.T) {
	conf := &config{handleUnmatchedRoutes: true, groupUnmatchedRoutes: false}
	route, _ := handleUnmatchedPath(conf, "", "/random/url")
	if route != "/unmatched/random/url" {
		t.Errorf("expected /unmatched/random/url, got %q", route)
	}
}

func TestHandleUnmatchedPath_MatchedRoute(t *testing.T) {
	// When route is already set, handleUnmatchedPath should not change it
	conf := &config{handleUnmatchedRoutes: true, groupUnmatchedRoutes: true}
	route, _ := handleUnmatchedPath(conf, "/api/v1", "/api/v1")
	if route != "/api/v1" {
		t.Errorf("expected /api/v1, got %q", route)
	}
}

// ---------------------------------------------------------------------------
// MiddlewareWithMetrics integration tests
// ---------------------------------------------------------------------------

func TestMiddlewareWithMetrics_BasicRequest(t *testing.T) {
	mc := newTestMetrics()
	r := gin.New()
	r.Use(MiddlewareWithMetrics(mc))
	r.GET("/hello", func(c *gin.Context) {
		c.String(http.StatusOK, "hello")
	})

	w := performRequest(r, "GET", "/hello")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMiddlewareWithMetrics_FilteredPath(t *testing.T) {
	mc := newTestMetrics()
	r := gin.New()
	r.Use(MiddlewareWithMetrics(mc, WithFilterRoutes([]string{"/health"})))
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/api", func(c *gin.Context) {
		c.String(http.StatusOK, "api")
	})

	// Both requests should succeed; the health one is just not measured
	w1 := performRequest(r, "GET", "/health")
	w2 := performRequest(r, "GET", "/api")
	if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
		t.Errorf("unexpected status codes: %d %d", w1.Code, w2.Code)
	}
}

func TestMiddlewareWithMetrics_AggregatedStatusCode(t *testing.T) {
	mc := newTestMetrics()
	r := gin.New()
	r.Use(MiddlewareWithMetrics(mc, WithAggregateStatusCode(true)))
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/err", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	performRequest(r, "GET", "/ok")
	performRequest(r, "GET", "/err")
}

func TestMiddlewareWithMetrics_NoSizeNoDuration(t *testing.T) {
	mc := newTestMetrics()
	r := gin.New()
	r.Use(MiddlewareWithMetrics(mc,
		WithRecordRequestSize(false),
		WithRecordResponseSize(false),
		WithRecordDuration(false),
	))
	r.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := performRequest(r, "GET", "/test")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMiddlewareWithMetrics_UnmatchedRoute(t *testing.T) {
	mc := newTestMetrics()
	r := gin.New()
	r.Use(MiddlewareWithMetrics(mc, WithUnmatchedRouteHandling(true)))
	// No routes registered; every request will be unmatched
	w := performRequest(r, "GET", "/ghost")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestMiddlewareWithMetrics_CustomPathAggregator(t *testing.T) {
	mc := newTestMetrics()
	r := gin.New()
	r.Use(MiddlewareWithMetrics(mc, WithPathAggregator(func(route, path string, status int) string {
		return "/aggregated"
	})))
	r.GET("/foo", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := performRequest(r, "GET", "/foo")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMiddleware_DefaultUsesGlobalMetrics(t *testing.T) {
	// Middleware() uses the package-level defaultMetrics; just ensure no panic.
	r := gin.New()
	r.Use(Middleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := performRequest(r, "GET", "/ping")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// NewMetricsCollection with options
// ---------------------------------------------------------------------------

func TestNewMetricsCollection_Defaults(t *testing.T) {
	reg := newTestRegistry()
	mc := NewMetricsCollection(WithCustomRegistry(reg))
	if mc.TotalRequests == nil {
		t.Error("TotalRequests should not be nil")
	}
	if mc.ResponseSize == nil {
		t.Error("ResponseSize should not be nil")
	}
	if mc.RequestSize == nil {
		t.Error("RequestSize should not be nil")
	}
	if mc.Duration == nil {
		t.Error("Duration should not be nil")
	}
}

func TestNewMetricsCollection_WithPrefix(t *testing.T) {
	reg := newTestRegistry()
	mc := NewMetricsCollection(
		WithCustomRegistry(reg),
		WithMetricPrefix("myapp"),
	)
	if mc.TotalRequests == nil {
		t.Error("TotalRequests should not be nil after prefix option")
	}
}

func TestNewMetricsCollection_WithCustomBuckets(t *testing.T) {
	reg := newTestRegistry()
	durationBuckets := []float64{0.01, 0.1, 1.0}
	sizeBuckets := []float64{512, 1024, 4096}
	mc := NewMetricsCollection(
		WithCustomRegistry(reg),
		WithCustomBuckets(durationBuckets, sizeBuckets),
	)
	if mc.Duration == nil {
		t.Error("Duration should not be nil")
	}
}

func TestNewMetricsCollection_WithCustomCounter(t *testing.T) {
	reg := newTestRegistry()
	customCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "custom_requests_total", Help: "custom"},
		[]string{"status_code", "method", "path"},
	)
	mc := NewMetricsCollection(
		WithCustomRegistry(reg),
		WithCustomRequestCounter(customCounter),
	)
	if mc.TotalRequests != customCounter {
		t.Error("TotalRequests should be the custom counter")
	}
}

func TestNewMetricsCollection_WithCustomHistograms(t *testing.T) {
	reg := newTestRegistry()

	respHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "custom_response_size", Help: "h"},
		[]string{"status_code", "method", "path"},
	)
	reqHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "custom_request_size", Help: "h"},
		[]string{"status_code", "method", "path"},
	)
	durHist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "custom_duration", Help: "h"},
		[]string{"status_code", "method", "path"},
	)

	mc := NewMetricsCollection(
		WithCustomRegistry(reg),
		WithCustomResponseSizeHistogram(respHist),
		WithCustomRequestSizeHistogram(reqHist),
		WithCustomDurationHistogram(durHist),
	)

	if mc.ResponseSize != respHist {
		t.Error("ResponseSize mismatch")
	}
	if mc.RequestSize != reqHist {
		t.Error("RequestSize mismatch")
	}
	if mc.Duration != durHist {
		t.Error("Duration mismatch")
	}
}

// ---------------------------------------------------------------------------
// WithUnmatchedRouteMarking / WithUnmatchedRouteGrouping options
// ---------------------------------------------------------------------------

func TestWithUnmatchedRouteMarking(t *testing.T) {
	conf := applyOpt(WithUnmatchedRouteMarking(true))
	if !conf.markUnmatchedRoutes {
		t.Error("expected markUnmatchedRoutes to be true")
	}
}

func TestWithUnmatchedRouteGrouping(t *testing.T) {
	conf := applyOpt(WithUnmatchedRouteGrouping(true))
	if !conf.unmatchedRoutesGrouping {
		t.Error("expected unmatchedRoutesGrouping to be true")
	}
}

// ---------------------------------------------------------------------------
// getRequestSize
// ---------------------------------------------------------------------------

func TestGetRequestSize_KnownContentLength(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", strings.NewReader("hello"))
	req.ContentLength = 5
	size := getRequestSize(req)
	if size != 5 {
		t.Errorf("expected 5, got %d", size)
	}
}

func TestGetRequestSize_ZeroContentLength(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.ContentLength = 0
	size := getRequestSize(req)
	if size != 0 {
		t.Errorf("expected 0, got %d", size)
	}
}

func TestGetRequestSize_UnknownContentLength(t *testing.T) {
	req, _ := http.NewRequest("POST", "/", strings.NewReader("body data"))
	req.ContentLength = -1
	size := getRequestSize(req)
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
}

// ---------------------------------------------------------------------------
// Metrics handler (basic auth)
// ---------------------------------------------------------------------------

func TestGetMetricHandler_NoAuth(t *testing.T) {
	handler := GetMetricHandler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetMetricHandler_WithBasicAuth_Success(t *testing.T) {
	handler := GetMetricHandler(WithBasicAuth("admin", "secret"))
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:secret")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetMetricHandler_WithBasicAuth_WrongPassword(t *testing.T) {
	handler := GetMetricHandler(WithBasicAuth("admin", "secret"))
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:wrong")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetMetricHandler_WithBasicAuth_NoCredentials(t *testing.T) {
	handler := GetMetricHandler(WithBasicAuth("admin", "secret"))
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetMetricHandler_WithBasicAuth_WrongUsername(t *testing.T) {
	handler := GetMetricHandler(WithBasicAuth("admin", "secret"))
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("other:secret")))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestDefaultConf_Defaults(t *testing.T) {
	conf := defaultConf()
	if !conf.recordRequestSize {
		t.Error("expected recordRequestSize true")
	}
	if !conf.recordResponseSize {
		t.Error("expected recordResponseSize true")
	}
	if !conf.recordDuration {
		t.Error("expected recordDuration true")
	}
	if conf.aggregateStatusCode {
		t.Error("expected aggregateStatusCode false")
	}
	if !conf.handleUnmatchedRoutes {
		t.Error("expected handleUnmatchedRoutes true")
	}
	if !conf.groupUnmatchedRoutes {
		t.Error("expected groupUnmatchedRoutes true")
	}
}

func TestApplyOpt_OverridesDefaults(t *testing.T) {
	conf := applyOpt(
		WithRecordRequestSize(false),
		WithRecordResponseSize(false),
		WithRecordDuration(false),
		WithAggregateStatusCode(true),
	)
	if conf.recordRequestSize {
		t.Error("expected recordRequestSize false")
	}
	if conf.recordResponseSize {
		t.Error("expected recordResponseSize false")
	}
	if conf.recordDuration {
		t.Error("expected recordDuration false")
	}
	if !conf.aggregateStatusCode {
		t.Error("expected aggregateStatusCode true")
	}
}

func TestWithFilterPath_Custom(t *testing.T) {
	conf := applyOpt(WithFilterPath(func(route, path string) bool {
		return route == "/skip"
	}))
	if !conf.filterPath("/skip", "/skip") {
		t.Error("expected filter to return true for /skip")
	}
	if conf.filterPath("/keep", "/keep") {
		t.Error("expected filter to return false for /keep")
	}
}

func TestWithFilterRoutes(t *testing.T) {
	conf := applyOpt(WithFilterRoutes([]string{"/health", "/ready"}))
	if !conf.filterPath("/health", "/health") {
		t.Error("expected /health to be filtered")
	}
	if !conf.filterPath("/ready", "/ready") {
		t.Error("expected /ready to be filtered")
	}
	if conf.filterPath("/api", "/api") {
		t.Error("expected /api to pass through")
	}
}

func TestWithPathAggregator_Custom(t *testing.T) {
	called := false
	conf := applyOpt(WithPathAggregator(func(route, path string, status int) string {
		called = true
		return fmt.Sprintf("%s_%d", route, status)
	}))
	result := conf.pathAggregator("/api", "/api", 200)
	if !called {
		t.Error("custom aggregator was not called")
	}
	if result != "/api_200" {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestDefaultPathAggregator_MissingRoute(t *testing.T) {
	conf := defaultConf()
	// 4xx without route
	r4xx := conf.pathAggregator("", "/whatever", 404)
	if r4xx != "path_4xx" {
		t.Errorf("expected path_4xx, got %q", r4xx)
	}
	// 5xx without route
	r5xx := conf.pathAggregator("", "/whatever", 500)
	if r5xx != "path_5xx" {
		t.Errorf("expected path_5xx, got %q", r5xx)
	}
	// other without route
	r2xx := conf.pathAggregator("", "/whatever", 200)
	if r2xx != "missing_route" {
		t.Errorf("expected missing_route, got %q", r2xx)
	}
}

func TestWithUnmatchedRouteHandling(t *testing.T) {
	conf := applyOpt(WithUnmatchedRouteHandling(false))
	if conf.handleUnmatchedRoutes {
		t.Error("expected handleUnmatchedRoutes false")
	}
}
