package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// CacheHits is a counter for cache hits, exposed for use in cache package.
	CacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total number of cache hits",
	})

	// CacheMisses is a counter for cache misses, exposed for use in cache package.
	CacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total number of cache misses",
	})

	// SyncDuration is a histogram for sync duration.
	SyncDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "sync_duration_seconds",
		Help:    "Duration of sync operations in seconds",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
	})

	// SyncTotal counts sync operations by result.
	SyncTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sync_total",
		Help: "Total sync operations by result",
	}, []string{"result"})
)

// MetricsMiddleware records HTTP request count and duration per route and status code.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path, status).Observe(time.Since(start).Seconds())
	}
}
