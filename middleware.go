// Package ginprom provides a Gin middleware that automatically collects and
// exposes HTTP request metrics to Prometheus.
//
// It records request counts, latency, request size, and response size, all
// labelled by HTTP method, status code, and route pattern.  Metrics can be
// exposed through the helper [GetMetricHandler] and optionally protected with
// HTTP Basic Authentication via [WithBasicAuth].
//
// Basic usage:
//
//	r := gin.Default()
//	r.Use(ginprom.Middleware())
//	r.GET("/metrics", gin.WrapH(ginprom.GetMetricHandler()))
package ginprom

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"strconv"
	"time"
)

// MetricsCollection groups the four Prometheus collectors used by the
// middleware: a counter for total requests and three histograms for duration,
// request size, and response size.  An optional custom registry may be set so
// that metrics are not registered with the default global Prometheus registry.
type MetricsCollection struct {
	TotalRequests *prometheus.CounterVec
	ResponseSize  *prometheus.HistogramVec
	RequestSize   *prometheus.HistogramVec
	Duration      *prometheus.HistogramVec
	Registry      *prometheus.Registry // Optional custom registry
}

// Default histogram bucket sets used when no custom buckets are provided.
//
// DefaultDurationBuckets covers request latency from 1 ms up to ~16 s in 15
// exponential steps (base 2, start 0.001 s).
//
// DefaultSizeBuckets covers payload sizes from 100 bytes up to ~51 KB in 10
// exponential steps (base 2, start 100 bytes).
var (
	DefaultDurationBuckets = prometheus.ExponentialBuckets(0.001, 2, 15)
	DefaultSizeBuckets     = prometheus.ExponentialBuckets(100, 2, 10)
)

var statusAddr = [1000]string{}

func init() {
	for i := 0; i < 1000; i++ {
		statusAddr[i] = strconv.Itoa(i)
	}
}

// defaultMetricsCollection creates and returns a new MetricsCollection with default settings
func defaultMetricsCollection() *MetricsCollection {
	mc := &MetricsCollection{
		TotalRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Number of requests.",
			},
			[]string{"status_code", "method", "path"},
		),
		ResponseSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "Size of HTTP response in bytes.",
				Buckets: DefaultSizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		),
		RequestSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_size_bytes",
				Help:    "Size of HTTP request in bytes.",
				Buckets: DefaultSizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		),
		Duration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds.",
				Buckets: DefaultDurationBuckets,
			},
			[]string{"status_code", "method", "path"},
		),
	}

	// Use default prometheus registry
	prometheus.MustRegister(mc.TotalRequests)
	prometheus.MustRegister(mc.ResponseSize)
	prometheus.MustRegister(mc.RequestSize)
	prometheus.MustRegister(mc.Duration)

	return mc
}

// NewMetricsCollection creates a [MetricsCollection] and registers all four
// collectors with Prometheus.  Pass [MetricsOption] functions to customise
// metric names, buckets, or the target registry.
//
// Example – use a custom registry and a metric name prefix:
//
//	reg := prometheus.NewRegistry()
//	mc := ginprom.NewMetricsCollection(
//	    ginprom.WithCustomRegistry(reg),
//	    ginprom.WithMetricPrefix("myservice"),
//	)
func NewMetricsCollection(opts ...MetricsOption) *MetricsCollection {
	mc := &MetricsCollection{
		// Create with nil values that will be set by options
	}

	// Apply all options
	for _, opt := range opts {
		opt(mc)
	}

	// If any metrics are still nil after options, create them with defaults
	if mc.TotalRequests == nil {
		mc.TotalRequests = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Number of requests.",
			},
			[]string{"status_code", "method", "path"},
		)
	}

	if mc.ResponseSize == nil {
		mc.ResponseSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "Size of HTTP response in bytes.",
				Buckets: DefaultSizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		)
	}

	if mc.RequestSize == nil {
		mc.RequestSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_size_bytes",
				Help:    "Size of HTTP request in bytes.",
				Buckets: DefaultSizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		)
	}

	if mc.Duration == nil {
		mc.Duration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds.",
				Buckets: DefaultDurationBuckets,
			},
			[]string{"status_code", "method", "path"},
		)
	}

	// Register with the appropriate registry
	registry := prometheus.DefaultRegisterer
	if mc.Registry != nil {
		registry = mc.Registry
	}

	registry.MustRegister(mc.TotalRequests)
	registry.MustRegister(mc.ResponseSize)
	registry.MustRegister(mc.RequestSize)
	registry.MustRegister(mc.Duration)

	return mc
}

// MetricsOption is a functional option that configures a [MetricsCollection].
// Options are applied in order by [NewMetricsCollection].
type MetricsOption func(*MetricsCollection)

// WithCustomRegistry configures the [MetricsCollection] to register all
// collectors with the provided registry instead of the default global one.
// This is especially useful in tests or when running multiple independent
// metric namespaces inside the same process.
func WithCustomRegistry(registry *prometheus.Registry) MetricsOption {
	return func(mc *MetricsCollection) {
		mc.Registry = registry
	}
}

// WithCustomRequestCounter replaces the default request-count counter with the
// provided one.  The counter must use the same label set as the middleware
// (status_code, method, path).
func WithCustomRequestCounter(counter *prometheus.CounterVec) MetricsOption {
	return func(mc *MetricsCollection) {
		mc.TotalRequests = counter
	}
}

// WithCustomResponseSizeHistogram replaces the default response-size histogram
// with the provided one.  The histogram must carry the same labels as the
// middleware (status_code, method, path).
func WithCustomResponseSizeHistogram(histogram *prometheus.HistogramVec) MetricsOption {
	return func(mc *MetricsCollection) {
		mc.ResponseSize = histogram
	}
}

// WithCustomRequestSizeHistogram replaces the default request-size histogram
// with the provided one.  The histogram must carry the same labels as the
// middleware (status_code, method, path).
func WithCustomRequestSizeHistogram(histogram *prometheus.HistogramVec) MetricsOption {
	return func(mc *MetricsCollection) {
		mc.RequestSize = histogram
	}
}

// WithCustomDurationHistogram replaces the default request-duration histogram
// with the provided one.  The histogram must carry the same labels as the
// middleware (status_code, method, path).
func WithCustomDurationHistogram(histogram *prometheus.HistogramVec) MetricsOption {
	return func(mc *MetricsCollection) {
		mc.Duration = histogram
	}
}

// WithMetricPrefix prepends prefix to all four default metric names.  For
// example, passing "myapp" will produce metrics named
// "myapp_http_requests_total", "myapp_http_request_duration_seconds", etc.
// This option recreates the four collectors, so it must appear before any
// individual collector option that should apply to the prefixed names.
func WithMetricPrefix(prefix string) MetricsOption {
	return func(mc *MetricsCollection) {
		mc.TotalRequests = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "_http_requests_total",
				Help: "Number of requests.",
			},
			[]string{"status_code", "method", "path"},
		)

		mc.ResponseSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    prefix + "_http_response_size_bytes",
				Help:    "Size of HTTP response in bytes.",
				Buckets: DefaultSizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		)

		mc.RequestSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    prefix + "_http_request_size_bytes",
				Help:    "Size of HTTP request in bytes.",
				Buckets: DefaultSizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		)

		mc.Duration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    prefix + "_http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds.",
				Buckets: DefaultDurationBuckets,
			},
			[]string{"status_code", "method", "path"},
		)
	}
}

// WithCustomBuckets replaces the bucket definitions for all three histogram
// collectors.  durationBuckets configures the request-duration histogram;
// sizeBuckets configures both the request-size and response-size histograms.
func WithCustomBuckets(durationBuckets, sizeBuckets []float64) MetricsOption {
	return func(mc *MetricsCollection) {
		mc.ResponseSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "Size of HTTP response in bytes.",
				Buckets: sizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		)

		mc.RequestSize = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_size_bytes",
				Help:    "Size of HTTP request in bytes.",
				Buckets: sizeBuckets,
			},
			[]string{"status_code", "method", "path"},
		)

		mc.Duration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds.",
				Buckets: durationBuckets,
			},
			[]string{"status_code", "method", "path"},
		)
	}
}

// Global default metrics collection for backward compatibility
var defaultMetrics *MetricsCollection

// init initializes the default metrics collection for backward compatibility
func init() {
	defaultMetrics = defaultMetricsCollection()
}

// getPathWithFallback returns the most appropriate path representation for a request
// using a cascade of fallback options to handle various routing scenarios
func getPathWithFallback(c *gin.Context) string {
	// Start with FullPath which returns the registered route pattern (like "/user/:id")
	path := c.FullPath()

	// If FullPath is empty (route not found/registered), try the request URL path
	if path == "" {
		// Use the raw request path as fallback
		if c.Request != nil && c.Request.URL != nil {
			path = c.Request.URL.Path
		}

		// If still empty (very unlikely), use a placeholder
		if path == "" {
			path = "/unknown"
		}
	}

	return path
}

// handleUnmatchedPath processes paths for routes that weren't matched by Gin router
func handleUnmatchedPath(conf *config, routePattern, path string) (string, string) {
	if routePattern != "" {
		return routePattern, path
	}

	if conf.handleUnmatchedRoutes {
		if conf.groupUnmatchedRoutes {
			return "/unmatched/*", path
		}
		return "/unmatched" + path, path
	}

	return "", path
}

// Middleware returns a Gin handler that records Prometheus metrics for every
// request using the package-level default metric collectors registered with
// the global Prometheus registry.
//
// Accept zero or more [Option] values to tune what is measured:
//
//	r.Use(ginprom.Middleware(
//	    ginprom.WithFilterRoutes([]string{"/healthz", "/readyz"}),
//	    ginprom.WithAggregateStatusCode(true),
//	))
func Middleware(options ...Option) gin.HandlerFunc {
	return MiddlewareWithMetrics(defaultMetrics, options...)
}

// MiddlewareWithMetrics is like [Middleware] but records metrics into the
// provided [MetricsCollection] instead of the package-level default.  Use
// this when you need multiple independent metric namespaces, custom
// registries, or fine-grained control over the collectors.
func MiddlewareWithMetrics(metrics *MetricsCollection, options ...Option) gin.HandlerFunc {
	conf := applyOpt(options...)

	return func(c *gin.Context) {
		start := time.Now()

		route := c.FullPath()
		path := route
		if path == "" {
			if c.Request != nil && c.Request.URL != nil {
				path = c.Request.URL.Path
			}
			if path == "" {
				path = "/unknown"
			}
		}

		// Process unmatched routes according to configuration
		route, path = handleUnmatchedPath(conf, route, path)

		if conf.filterPath(route, path) {
			c.Next()
			return
		}

		var bodyCounter *countingReadCloser
		if conf.recordRequestSize && c.Request.Body != nil && c.Request.ContentLength < 0 {
			bodyCounter = &countingReadCloser{ReadCloser: c.Request.Body}
			c.Request.Body = bodyCounter
		}

		c.Next()

		handleMetricsWithCollection(c, conf, route, path, start, metrics, bodyCounter)
	}
}

// Handles metrics collection after request execution with custom metrics collection
func handleMetricsWithCollection(c *gin.Context, conf *config, route, path string, start time.Time, metrics *MetricsCollection, bodyCounter *countingReadCloser) {
	status := c.Writer.Status()
	var statusCode string
	if conf.aggregateStatusCode {
		statusCode = statusAddr[status/100] + "xx"
	} else if status < 1000 {
		statusCode = statusAddr[status]
	} else {
		statusCode = strconv.Itoa(status)
	}

	aggregatePath := conf.pathAggregator(route, path, status)
	method := c.Request.Method

	// Collect metrics based on configuration with custom metrics collection
	recordRequestMetricsWithCollection(conf, c, statusCode, method, aggregatePath, start, metrics, bodyCounter)
}

// Records request-related metrics with custom metrics collection
func recordRequestMetricsWithCollection(conf *config, c *gin.Context, statusCode, method, path string, start time.Time, metrics *MetricsCollection, bodyCounter *countingReadCloser) {
	// Increment total requests
	metrics.TotalRequests.WithLabelValues(statusCode, method, path).Inc()

	// Record response size
	if conf.recordResponseSize {
		metrics.ResponseSize.WithLabelValues(statusCode, method, path).Observe(float64(c.Writer.Size()))
	}

	// Record request size
	if conf.recordRequestSize {
		metrics.RequestSize.WithLabelValues(statusCode, method, path).Observe(float64(getRequestSize(c.Request, bodyCounter)))
	}

	// Record duration
	if conf.recordDuration {
		metrics.Duration.WithLabelValues(statusCode, method, path).Observe(time.Since(start).Seconds())
	}
}

// Unknown-length bodies are measured only through handler reads; draining them
// after the handler returns would buffer unbounded input and measure the remainder.
func getRequestSize(r *http.Request, bodyCounter *countingReadCloser) int64 {
	if r.ContentLength >= 0 {
		return r.ContentLength
	}
	if bodyCounter == nil {
		return 0
	}
	return bodyCounter.bytesRead
}

// WithUnmatchedRouteMarking enables or disables the special "/unmatched" prefix
// added to route patterns that the Gin router did not match.  Deprecated in
// favour of [WithUnmatchedRouteHandling], kept for backwards compatibility.
func WithUnmatchedRouteMarking(enabled bool) Option {
	return func(c *config) {
		c.markUnmatchedRoutes = enabled
	}
}

// WithUnmatchedRouteGrouping controls whether all unmatched routes are
// collapsed into a single "/unmatched/*" label value (enabled = true) or
// recorded individually under "/unmatched<original-path>" (enabled = false).
// Grouping is the safer default because it prevents metric cardinality
// explosion caused by random or attacker-supplied URL paths.
func WithUnmatchedRouteGrouping(enabled bool) Option {
	return func(c *config) {
		c.unmatchedRoutesGrouping = enabled
	}
}
