# Gin Prometheus Metrics Middleware

This repository provides a Gin middleware to collect Prometheus metrics for your Gin-based applications. The middleware captures HTTP request data, including response time, request counts, and more, and exposes them in a format that Prometheus can scrape.

## Features

- Collects HTTP request count by method, status, and endpoint
- Tracks response time (latency) for each endpoint
- Collects request size and response size
- Provides a `/metrics` endpoint for Prometheus to scrape
- Easy integration with existing Gin applications

## Installation

Install the middleware using `go get`:

```bash
go get github.com/logocomune/gin-prometheus
```

## Usage

Here’s how to integrate the middleware into your Gin application:

```go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/logocomune/gin-prometheus"
)

func main() {
	r := gin.Default()

	r.Use(ginprom.Middleware())

	r.GET("/metrics", gin.WrapH(ginprom.GetMetricHandler()))

	r.GET("/", func(c *gin.Context) {
		c.String(200, "Hello World")
	})
	r.GET("/test/:id", func(c *gin.Context) {
		c.String(200, "Hello World")
	})
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})
	r.Run() // listen and serve on 0.0.0.0:8080
}

```

## Middleware Options

The `Middleware` function allows you to customize the behavior of the middleware by passing configuration options:

```go
metricsMiddleware := ginprometheus.Middleware(
ginprometheus.WithRecordRequests(false),
)
```

## Metrics Collected

The middleware collects the following metrics:

1. **HTTP Request Count**
    - Metric name: `http_requests_total`
    - Labels: ` `method`, `status`, `path`

2. **HTTP Request Duration**
    - Metric name: `http_request_duration_seconds`
    - Labels: `method`, `status`, `path`

4. **Request Size**
    - Metric name: `http_request_size_bytes`
    - Labels: `method`, `status`, `path`

5. **Response Size**
    - Metric name: `http_response_size_bytes`
    - Labels:`method`, `status`, `path`


## Contributing

Contributions are welcome! Please open an issue or submit a pull request if you’d like to improve this middleware.

## License

This project is licensed under the [MIT License](LICENSE).
