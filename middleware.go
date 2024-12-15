package ginprom

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"strconv"
	"time"
)

var (
	totalRequests *prometheus.CounterVec
	responseSize  *prometheus.HistogramVec
	requestSize   *prometheus.HistogramVec
	duration      *prometheus.HistogramVec
)

func init() {
	totalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Number of requests.",
		},
		[]string{"status_code", "method", "path"},
	)
	prometheus.MustRegister(totalRequests)

	responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "Size of HTTP response in bytes.",
			Buckets: prometheus.ExponentialBuckets(100, 2, 10),
		},
		[]string{"status_code", "method", "path"},
	)
	prometheus.MustRegister(responseSize)

	requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_size_bytes",
			Help:    "Size of HTTP request in bytes.",
			Buckets: prometheus.ExponentialBuckets(100, 2, 10),
		},
		[]string{"status_code", "method", "path"},
	)
	prometheus.MustRegister(requestSize)

	duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
		},
		[]string{"status_code", "method", "path"},
	)
	prometheus.MustRegister(duration)
}

// Middleware returns a Gin middleware handler function for collecting and exporting Prometheus metrics.
// It supports optional configuration through variadic Option parameters.
func Middleware(options ...Option) gin.HandlerFunc {
	conf := applyOpt(options...)

	return func(c *gin.Context) {
		start := time.Now()

		route := c.FullPath()
		path := getPathFromContext(c)

		if conf.filterPath(route, path) {
			c.Next()
			return
		}

		defer func() {
			handleMetrics(c, conf, route, path, start)
		}()

		c.Next()
	}
}

// Extracts path from gin.Context safely
func getPathFromContext(c *gin.Context) string {
	if c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

// Handles metrics collection after request execution
func handleMetrics(c *gin.Context, conf *config, route, path string, start time.Time) {
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

	// Collect metrics based on configuration
	recordRequestMetrics(conf, c, params, start)
}

// Records request-related metrics
func recordRequestMetrics(conf *config, c *gin.Context, params []string, start time.Time) {
	// Increment total requests
	totalRequests.WithLabelValues(params...).Inc()

	// Record response size
	if conf.recordResponseSize {
		responseSize.WithLabelValues(params...).Observe(float64(computeResponseSize(c)))
	}

	// Record request size
	if conf.recordRequestSize {
		size := getRequestSize(c.Request)
		requestSize.WithLabelValues(params...).Observe(float64(size))
	}

	// Record duration
	if conf.recordDuration {
		elapsedTimeInSeconds := time.Since(start).Seconds()
		duration.WithLabelValues(params...).Observe(elapsedTimeInSeconds)
	}
}

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
