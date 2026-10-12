// Command mac-vendor-service is an example HTTP service that combines
// gin-prometheus with maclookup-go: it resolves MAC addresses to their
// IEEE-registered vendor and exposes Prometheus metrics about both its own
// HTTP traffic and its use of the maclookup.app API.
//
// Configuration (environment variables, all optional):
//
//	ADDR               listen address (default ":8080")
//	MACLOOKUP_API_KEY  legacy maclookup.app API key
//	MACLOOKUP_BASE_URL alternative API base URL (e.g. a proxy or a mock)
//	MACLOOKUP_TIMEOUT  per-request upstream timeout (default "5s")
//	CACHE_TTL          how long lookups are cached (default "24h", "0" disables)
//	CACHE_MAX_ENTRIES  maximum number of cached lookups (default 10000)
//	METRICS_USER       together with METRICS_PASSWORD, protects /metrics
//	METRICS_PASSWORD   with HTTP Basic Auth
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	ginprom "github.com/logocomune/gin-prometheus"
	"github.com/logocomune/maclookup-go"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	timeout, err := durationEnv("MACLOOKUP_TIMEOUT", 5*time.Second)
	if err != nil {
		return err
	}
	cacheTTL, err := durationEnv("CACHE_TTL", 24*time.Hour)
	if err != nil {
		return err
	}
	cacheMax, err := intEnv("CACHE_MAX_ENTRIES", 10000)
	if err != nil {
		return err
	}

	client := maclookup.New()
	client.WithTimeout(timeout)
	if key := os.Getenv("MACLOOKUP_API_KEY"); key != "" {
		client.WithAPIKey(key)
	}
	if base := os.Getenv("MACLOOKUP_BASE_URL"); base != "" {
		client.WithPrefixURI(base)
	}

	// gin-prometheus registers its HTTP metrics on the default registry, and
	// GetMetricHandler serves that registry, so the service metrics go there too.
	svc := NewVendorService(client, prometheus.DefaultRegisterer, cacheTTL, cacheMax)

	var handlerOpts []ginprom.HandlerOption
	user, pass := os.Getenv("METRICS_USER"), os.Getenv("METRICS_PASSWORD")
	if user != "" && pass != "" {
		handlerOpts = append(handlerOpts, ginprom.WithBasicAuth(user, pass))
	} else {
		slog.Warn("/metrics is not protected; set METRICS_USER and METRICS_PASSWORD to require Basic Auth")
	}

	srv := &http.Server{
		Addr:              envOr("ADDR", ":8080"),
		Handler:           newRouter(svc, ginprom.GetMetricHandler(handlerOpts...)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// Must exceed the upstream timeout, or slow lookups get cut off.
		WriteTimeout: timeout + 10*time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func durationEnv(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s: invalid duration %q", key, v)
	}

	return d, nil
}

func intEnv(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: invalid non-negative integer %q", key, v)
	}

	return n, nil
}
