package ginprom

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricsCollectionRegistrationCollision(t *testing.T) {
	for _, customBuckets := range []bool{false, true} {
		name := "identical configuration"
		if customBuckets {
			name = "different buckets"
		}
		t.Run(name, func(t *testing.T) {
			registry := newTestRegistry()
			NewMetricsCollection(WithCustomRegistry(registry))
			options := []MetricsOption{WithCustomRegistry(registry)}
			if customBuckets {
				options = append(options, WithCustomBuckets([]float64{1, 2}, []float64{10, 20}))
			}
			defer func() {
				if _, ok := recover().(prometheus.AlreadyRegisteredError); !ok {
					t.Fatal("duplicate registration must fail explicitly")
				}
			}()
			NewMetricsCollection(options...)
		})
	}
}

func TestMetricsCollectionIndependentRegistrations(t *testing.T) {
	for _, sharedRegistry := range []bool{false, true} {
		name := "separate registries"
		if sharedRegistry {
			name = "distinct prefixes"
		}
		t.Run(name, func(t *testing.T) {
			firstRegistry := newTestRegistry()
			secondRegistry := firstRegistry
			firstOptions := []MetricsOption{WithCustomRegistry(firstRegistry)}
			secondOptions := []MetricsOption{}
			if sharedRegistry {
				firstOptions = append(firstOptions, WithMetricPrefix("first"))
				secondOptions = append(secondOptions, WithMetricPrefix("second"))
			} else {
				secondRegistry = newTestRegistry()
			}
			secondOptions = append(secondOptions, WithCustomRegistry(secondRegistry))
			first := NewMetricsCollection(firstOptions...)
			second := NewMetricsCollection(secondOptions...)
			first.TotalRequests.WithLabelValues("200", "GET", "/").Inc()
			metric := &dto.Metric{}
			if err := second.TotalRequests.WithLabelValues("200", "GET", "/").Write(metric); err != nil {
				t.Fatal(err)
			}
			if metric.GetCounter().GetValue() != 0 {
				t.Fatal("independent collections share observations")
			}
		})
	}
}

func TestMetricsCollectionCustomBucketBoundaries(t *testing.T) {
	metrics := NewMetricsCollection(WithCustomRegistry(newTestRegistry()), WithCustomBuckets([]float64{1, 2}, []float64{10, 20}))
	for _, test := range []struct {
		name       string
		histogram  *prometheus.HistogramVec
		boundaries []float64
	}{
		{name: "duration", histogram: metrics.Duration, boundaries: []float64{1, 2}},
		{name: "request", histogram: metrics.RequestSize, boundaries: []float64{10, 20}},
		{name: "response", histogram: metrics.ResponseSize, boundaries: []float64{10, 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			metric := &dto.Metric{}
			if err := test.histogram.WithLabelValues("200", "GET", "/").(prometheus.Metric).Write(metric); err != nil {
				t.Fatal(err)
			}
			buckets := metric.GetHistogram().Bucket
			if len(buckets) != len(test.boundaries) {
				t.Fatalf("bucket count = %d, want %d", len(buckets), len(test.boundaries))
			}
			for index, bucket := range buckets {
				if bucket.GetUpperBound() != test.boundaries[index] {
					t.Fatalf("boundary = %g, want %g", bucket.GetUpperBound(), test.boundaries[index])
				}
			}
		})
	}
}

func TestMetricsCollectionExplicitSharing(t *testing.T) {
	metrics := newTestMetrics()
	for index := 0; index < 2; index++ {
		router := gin.New()
		router.Use(MiddlewareWithMetrics(metrics))
		router.GET("/shared", func(c *gin.Context) { c.Status(http.StatusOK) })
		performRequest(router, http.MethodGet, "/shared")
	}
	metric := &dto.Metric{}
	if err := metrics.TotalRequests.WithLabelValues("200", "GET", "/shared").Write(metric); err != nil {
		t.Fatal(err)
	}
	if metric.GetCounter().GetValue() != 2 {
		t.Fatalf("shared count = %g, want 2", metric.GetCounter().GetValue())
	}
}
