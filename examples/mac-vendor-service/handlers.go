package main

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	ginprom "github.com/logocomune/gin-prometheus"
)

// newRouter wires the HTTP API. metricsHandler is served at /metrics.
//
// The gin-prometheus middleware labels metrics with the route pattern
// ("/api/v1/vendors/:mac"), not the raw URL, so arbitrary MAC addresses
// cannot blow up metric cardinality.
func newRouter(svc *VendorService, metricsHandler http.Handler) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginprom.Middleware(
		ginprom.WithFilterRoutes([]string{"/healthz", "/metrics"}),
	))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(metricsHandler))

	v1 := r.Group("/api/v1")
	v1.GET("/vendors/:mac", func(c *gin.Context) {
		info, err := svc.Lookup(c.Param("mac"))
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(foundStatus(info.Found), info)
	})
	v1.GET("/vendors/:mac/name", func(c *gin.Context) {
		name, err := svc.CompanyName(c.Param("mac"))
		if err != nil {
			writeError(c, err)
			return
		}
		c.JSON(foundStatus(name.Found), name)
	})

	return r
}

func foundStatus(found bool) int {
	if found {
		return http.StatusOK
	}

	return http.StatusNotFound
}

// writeError maps service errors to HTTP responses. Upstream details are
// logged but not returned, so the client never sees configuration problems
// such as a rejected API key.
func writeError(c *gin.Context, err error) {
	var (
		rateLimited *RateLimitedError
		badRequest  *BadRequestError
	)

	switch {
	case errors.Is(err, ErrInvalidMAC):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.As(err, &badRequest):
		c.JSON(http.StatusBadRequest, gin.H{"error": badRequest.Msg})
	case errors.As(err, &rateLimited):
		if rateLimited.RetryAfter > 0 {
			secs := int(math.Ceil(rateLimited.RetryAfter.Seconds()))
			c.Header("Retry-After", strconv.Itoa(secs))
		}
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "upstream rate limit exceeded, retry later"})
	case errors.Is(err, ErrUpstreamTimeout):
		slog.Warn("upstream timeout", "err", err)
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "upstream timed out"})
	default:
		slog.Error("upstream failure", "err", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream unavailable"})
	}
}
