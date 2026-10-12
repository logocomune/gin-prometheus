package main

import "github.com/prometheus/client_golang/prometheus"

// serviceMetrics are the business metrics of this service. They complement
// the generic HTTP metrics recorded by the gin-prometheus middleware.
type serviceMetrics struct {
	upstreamRequests   *prometheus.CounterVec
	upstreamDuration   *prometheus.HistogramVec
	cacheRequests      *prometheus.CounterVec
	rateLimitRemaining prometheus.Gauge
}

func newServiceMetrics(reg prometheus.Registerer, cacheLen func() float64) *serviceMetrics {
	m := &serviceMetrics{
		upstreamRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "maclookup_upstream_requests_total",
				Help: "Calls made to the maclookup.app API, by endpoint and result.",
			},
			[]string{"endpoint", "result"},
		),
		upstreamDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "maclookup_upstream_duration_seconds",
				Help:    "Round-trip time of maclookup.app API calls.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"endpoint"},
		),
		cacheRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "maclookup_cache_requests_total",
				Help: "Vendor cache lookups, by endpoint and result (hit or miss).",
			},
			[]string{"endpoint", "result"},
		),
		rateLimitRemaining: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "maclookup_rate_limit_remaining",
			Help: "Requests left in the current maclookup.app rate-limit window, as last reported by the API (-1 until known).",
		}),
	}
	m.rateLimitRemaining.Set(-1)

	reg.MustRegister(
		m.upstreamRequests,
		m.upstreamDuration,
		m.cacheRequests,
		m.rateLimitRemaining,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "maclookup_cache_entries",
			Help: "Number of entries currently held in the vendor cache.",
		}, cacheLen),
	)

	return m
}
