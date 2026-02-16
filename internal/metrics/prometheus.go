package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "essensys_backend_http_requests_total",
			Help: "Total HTTP requests received by the backend",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "essensys_backend_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		},
		[]string{"method", "path"},
	)

	HTTPRequestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "essensys_backend_http_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 2, 10),
		},
		[]string{"method", "path"},
	)

	HTTPResponseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "essensys_backend_http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: prometheus.ExponentialBuckets(100, 2, 10),
		},
		[]string{"method", "path"},
	)

	MQTTMessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "essensys_backend_mqtt_messages_total",
			Help: "Total MQTT messages published/received",
		},
		[]string{"direction", "topic"},
	)

	RedisOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "essensys_backend_redis_operations_total",
			Help: "Total Redis operations",
		},
		[]string{"operation", "status"},
	)

	ActionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "essensys_backend_actions_total",
			Help: "Total actions processed",
		},
		[]string{"type", "status"},
	)

	ActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "essensys_backend_active_connections",
			Help: "Number of active HTTP connections",
		},
	)

	LegacyClientsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "essensys_backend_legacy_clients_total",
			Help: "Total requests from legacy BP_MQX_ETH clients",
		},
	)

	VersionInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "essensys_backend_version_info",
			Help: "Backend version information",
		},
		[]string{"version"},
	)
)

func Init(version string) {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		HTTPRequestSize,
		HTTPResponseSize,
		MQTTMessagesTotal,
		RedisOperationsTotal,
		ActionsTotal,
		ActiveConnections,
		LegacyClientsTotal,
		VersionInfo,
	)
	VersionInfo.WithLabelValues(version).Set(1)
}

func Handler() http.Handler {
	return promhttp.Handler()
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

func normalizePath(path string) string {
	switch {
	case len(path) > 10 && path[:10] == "/api/done/":
		return "/api/done/{guid}"
	case path == "/api/serverinfos":
		return "/api/serverinfos"
	case path == "/api/mystatus":
		return "/api/mystatus"
	case path == "/api/myactions":
		return "/api/myactions"
	case path == "/api/admin/inject":
		return "/api/admin/inject"
	case path == "/api/web/actions":
		return "/api/web/actions"
	case path == "/api/web/history/latest":
		return "/api/web/history/latest"
	case path == "/health":
		return "/health"
	case path == "/debug":
		return "/debug"
	case path == "/table_ref":
		return "/table_ref"
	default:
		return "other"
	}
}

func InstrumentHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		ActiveConnections.Inc()
		defer ActiveConnections.Dec()

		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		status := strconv.Itoa(rw.statusCode)

		HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
		HTTPResponseSize.WithLabelValues(r.Method, path).Observe(float64(rw.size))

		if r.ContentLength > 0 {
			HTTPRequestSize.WithLabelValues(r.Method, path).Observe(float64(r.ContentLength))
		}
	})
}
