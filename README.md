# gin-prometheus

A Gin middleware that automatically records HTTP request metrics and exposes
them to [Prometheus](https://prometheus.io/).

[![Go Reference](https://pkg.go.dev/badge/github.com/logocomune/gin-prometheus.svg)](https://pkg.go.dev/github.com/logocomune/gin-prometheus)
[![Go Report Card](https://goreportcard.com/badge/github.com/logocomune/gin-prometheus)](https://goreportcard.com/report/github.com/logocomune/gin-prometheus)

---

## Features

| Capability | Details |
|---|---|
| Request counter | `http_requests_total` labelled by `status_code`, `method`, `path` |
| Latency histogram | `http_request_duration_seconds` |
| Request-size histogram | `http_request_size_bytes` |
| Response-size histogram | `http_response_size_bytes` |
| Metrics endpoint | Drop-in `http.Handler` compatible with any Gin route |
| Basic Auth | Optional username/password protection on the `/metrics` endpoint |
| Route filtering | Exclude specific routes (e.g. `/healthz`) from metrics |
| Status-code aggregation | Collapse codes into `2xx` / `4xx` / `5xx` classes |
| Custom buckets | Override default histogram buckets |
| Custom metric prefix | Namespace metrics per service |
| Custom registry | Isolated `prometheus.Registry` for testing or multi-tenant use |
| Unmatched route handling | Group or filter 404/unknown paths to avoid cardinality explosion |

---

## Installation

```bash
go get github.com/logocomune/gin-prometheus
```

Requires Go 1.23+ and Gin v1.10+.

---

## Quick Start

```go
package main

import (
    "github.com/gin-gonic/gin"
    ginprom "github.com/logocomune/gin-prometheus"
)

func main() {
    r := gin.Default()

    // Register the metrics middleware (records all routes by default).
    r.Use(ginprom.Middleware())

    // Expose the /metrics endpoint for Prometheus to scrape.
    r.GET("/metrics", gin.WrapH(ginprom.GetMetricHandler()))

    r.GET("/", func(c *gin.Context) {
        c.String(200, "Hello World")
    })

    r.Run(":8080")
}
```

---

## Metrics Reference

All metrics carry three labels:

| Label | Description | Example |
|---|---|---|
| `status_code` | HTTP response status code (or class) | `200`, `404`, `2xx` |
| `method` | HTTP method | `GET`, `POST` |
| `path` | Gin route pattern | `/users/:id` |

### Metric names

| Name | Type | Description |
|---|---|---|
| `http_requests_total` | Counter | Total number of handled requests |
| `http_request_duration_seconds` | Histogram | Time elapsed from first byte received to last byte sent |
| `http_request_size_bytes` | Histogram | Inbound request size (headers + body) |
| `http_response_size_bytes` | Histogram | Outbound response body size |

Default histogram buckets:

- **Duration** – exponential, 15 buckets from 1 ms to ~16 s (factor 2).
- **Size** – exponential, 10 buckets from 100 B to ~51 KB (factor 2).

---

## Configuration Options

### Middleware options (`Option`)

Pass these to `Middleware(...)` or `MiddlewareWithMetrics(mc, ...)`.

| Option | Default | Description |
|---|---|---|
| `WithRecordRequestSize(bool)` | `true` | Enable/disable request-size histogram |
| `WithRecordResponseSize(bool)` | `true` | Enable/disable response-size histogram |
| `WithRecordDuration(bool)` | `true` | Enable/disable latency histogram |
| `WithAggregateStatusCode(bool)` | `false` | Group codes into `1xx`–`5xx` classes |
| `WithFilterRoutes([]string)` | — | Skip listed route patterns entirely |
| `WithFilterPath(func)` | — | Custom per-request filter function |
| `WithPathAggregator(func)` | — | Custom path-label mapping function |
| `WithUnmatchedRouteHandling(bool)` | `true` | Count or ignore 404/unmatched routes |
| `WithUnmatchedRouteGrouping(bool)` | `true` | Collapse all unmatched under `/unmatched/*` |

### Metrics handler options (`HandlerOption`)

Pass these to `GetMetricHandler(...)`.

| Option | Description |
|---|---|
| `WithBasicAuth(username, password string)` | Require HTTP Basic Auth to access `/metrics` |

### Metrics collection options (`MetricsOption`)

Pass these to `NewMetricsCollection(...)` when you need a custom setup.

| Option | Description |
|---|---|
| `WithCustomRegistry(*prometheus.Registry)` | Use an isolated registry instead of the global one |
| `WithMetricPrefix(string)` | Prefix all metric names (e.g. `"myapp"` → `myapp_http_requests_total`) |
| `WithCustomBuckets(duration, size []float64)` | Override all histogram buckets at once |
| `WithCustomRequestCounter(*prometheus.CounterVec)` | Bring your own request counter |
| `WithCustomRequestSizeHistogram(*prometheus.HistogramVec)` | Bring your own request-size histogram |
| `WithCustomResponseSizeHistogram(*prometheus.HistogramVec)` | Bring your own response-size histogram |
| `WithCustomDurationHistogram(*prometheus.HistogramVec)` | Bring your own duration histogram |

---

## Usage Examples

### Exclude health-check routes from metrics

```go
r.Use(ginprom.Middleware(
    ginprom.WithFilterRoutes([]string{"/healthz", "/readyz", "/metrics"}),
))
```

### Aggregate status codes into classes

Reduces label cardinality: `200`, `201`, `204` all become `2xx`.

```go
r.Use(ginprom.Middleware(
    ginprom.WithAggregateStatusCode(true),
))
```

### Custom path filter

```go
r.Use(ginprom.Middleware(
    ginprom.WithFilterPath(func(route, path string) bool {
        // Skip any route starting with /internal
        return strings.HasPrefix(path, "/internal")
    }),
))
```

### Custom path aggregator

Useful when you have path segments that are not Gin parameters but still
produce high cardinality (e.g. UUIDs embedded in the path).

```go
r.Use(ginprom.Middleware(
    ginprom.WithPathAggregator(func(route, path string, statusCode int) string {
        if route != "" {
            return route
        }
        // Bucket unmatched paths by status class only
        if statusCode >= 500 {
            return "error_5xx"
        }
        return "other"
    }),
))
```

### Protect the metrics endpoint with Basic Auth

```go
r.GET("/metrics", gin.WrapH(
    ginprom.GetMetricHandler(
        ginprom.WithBasicAuth("prometheus", "s3cr3t"),
    ),
))
```

### Custom metric prefix

```go
mc := ginprom.NewMetricsCollection(
    ginprom.WithMetricPrefix("myservice"),
)
r.Use(ginprom.MiddlewareWithMetrics(mc))
// Metrics: myservice_http_requests_total, myservice_http_request_duration_seconds, …
```

### Custom histogram buckets

```go
mc := ginprom.NewMetricsCollection(
    ginprom.WithCustomBuckets(
        []float64{0.005, 0.01, 0.05, 0.1, 0.5, 1, 2, 5}, // duration (seconds)
        []float64{512, 1024, 4096, 16384, 65536},          // sizes (bytes)
    ),
)
r.Use(ginprom.MiddlewareWithMetrics(mc))
```

### Isolated registry (useful in tests)

```go
reg := prometheus.NewRegistry()
mc := ginprom.NewMetricsCollection(
    ginprom.WithCustomRegistry(reg),
)
r.Use(ginprom.MiddlewareWithMetrics(mc))
```

### Disable specific measurements

```go
r.Use(ginprom.Middleware(
    ginprom.WithRecordRequestSize(false),  // don't track request body sizes
    ginprom.WithRecordResponseSize(false), // don't track response sizes
))
```

### Unmatched route handling

By default, requests to routes that are not registered in Gin (which would
return 404) are still counted under the label `/unmatched/*` to keep
cardinality bounded.  You can turn this off:

```go
// Do not record metrics for unmatched routes at all
r.Use(ginprom.Middleware(
    ginprom.WithUnmatchedRouteHandling(false),
))

// Record each unmatched path individually (may explode cardinality!)
r.Use(ginprom.Middleware(
    ginprom.WithUnmatchedRouteHandling(true),
    ginprom.WithUnmatchedRouteGrouping(false),
))
```

---

## Prometheus Scrape Configuration

Add a scrape job to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: my-gin-service
    static_configs:
      - targets: ["localhost:8080"]
    metrics_path: /metrics
    # If you enabled Basic Auth:
    # basic_auth:
    #   username: prometheus
    #   password: s3cr3t
```

---

## Example Grafana Queries

```promql
# Request rate per route (last 5 min)
sum(rate(http_requests_total[5m])) by (path, method)

# 99th-percentile latency per route
histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, path))

# Error rate (5xx)
sum(rate(http_requests_total{status_code=~"5.."}[5m])) by (path)

# Average request size in KB
sum(rate(http_request_size_bytes_sum[5m])) by (path)
/ sum(rate(http_request_size_bytes_count[5m])) by (path) / 1024
```

---

## Contributing

Contributions, bug reports and feature requests are welcome. Please open an
issue or submit a pull request.

## License

[MIT](LICENSE)
