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
	// If the route pattern is empty (not matched), and handling of unmatched routes is enabled
	if routePattern == "" && conf.handleUnmatchedRoutes {
		// If we should use a placeholder for all unmatched routes (to prevent cardinality explosion)
		if conf.groupUnmatchedRoutes {
			routePattern = "/unmatched/*"
		} else {
			// Otherwise, mark it as unmatched but keep the original path
			routePattern = "/unmatched" + path
		}
	}

	return routePattern, path
}

// Replace the existing getPathFromContext with our improved implementation
func getPathFromContext(c *gin.Context) string {
	return getPathWithFallback(c)
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
		path := getPathFromContext(c)

		// Process unmatched routes according to configuration
		route, path = handleUnmatchedPath(conf, route, path)

		if conf.filterPath(route, path) {
			c.Next()
			return
		}

		defer func() {
			handleMetricsWithCollection(c, conf, route, path, start, metrics)
		}()

		c.Next()
	}
}

// Handles metrics collection after request execution with custom metrics collection
func handleMetricsWithCollection(c *gin.Context, conf *config, route, path string, start time.Time, metrics *MetricsCollection) {
	statusCode := strconv.Itoa(c.Writer.Status())
	if conf.aggregateStatusCode {
		statusCode = strconv.Itoa(c.Writer.Status()/100) + "xx"
	}

	aggregatePath := conf.pathAggregator(route, path, c.Writer.Status())
	params := []string{
		statusCode,
		c.Request.Method,
		aggregatePath,
	}

	// Collect metrics based on configuration with custom metrics collection
	recordRequestMetricsWithCollection(conf, c, params, start, metrics)
}

// Records request-related metrics with custom metrics collection
func recordRequestMetricsWithCollection(conf *config, c *gin.Context, params []string, start time.Time, metrics *MetricsCollection) {
	// Increment total requests
	metrics.TotalRequests.WithLabelValues(params...).Inc()

	// Record response size
	if conf.recordResponseSize {
		metrics.ResponseSize.WithLabelValues(params...).Observe(float64(computeResponseSize(c)))
	}

	// Record request size
	if conf.recordRequestSize {
		size := getRequestSize(c.Request)
		metrics.RequestSize.WithLabelValues(params...).Observe(float64(size))
	}

	// Record duration
	if conf.recordDuration {
		elapsedTimeInSeconds := time.Since(start).Seconds()
		metrics.Duration.WithLabelValues(params...).Observe(elapsedTimeInSeconds)
	}
}

// Keep the old functions for backward compatibility
func handleMetrics(c *gin.Context, conf *config, route, path string, start time.Time) {
	handleMetricsWithCollection(c, conf, route, path, start, defaultMetrics)
}

func recordRequestMetrics(conf *config, c *gin.Context, params []string, start time.Time) {
	recordRequestMetricsWithCollection(conf, c, params, start, defaultMetrics)
}

var (
	totalRequests *prometheus.CounterVec
	responseSize  *prometheus.HistogramVec
	requestSize   *prometheus.HistogramVec
	duration      *prometheus.HistogramVec
)

// Safely retrieves request size, falling back if Content-Length is unavailable
func getRequestSize(r *http.Request) int64 {
	if r.ContentLength != -1 {
		return r.ContentLength
	}

	size, err := calculateRequestSize(r)
	if err != nil {
		return 0 // Fallback to 0 if calculation fails
	}
	return size
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
