// Package metrics exposes Prometheus collectors shared by all drizzy services.
//
// Each service registers HTTP request counters/histograms via the chi/standard
// net/http middleware in this package, plus a few domain-specific counters
// (interactions, matches, ranking recalculations). All counters live in the
// default global registry so the /metrics endpoint exposes them automatically.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "drizzy_http_requests_total",
		Help: "Total HTTP requests served, labelled by service, route, method, status.",
	}, []string{"service", "method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "drizzy_http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"service", "method", "path"})

	// InteractionsTotal — incremented by ranking-service when it persists a
	// like/skip event coming off RabbitMQ.
	InteractionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "drizzy_interactions_total",
		Help: "Interactions processed by ranking-service.",
	}, []string{"action"})

	// MatchesTotal — bumped on mutual-like detection.
	MatchesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "drizzy_matches_total",
		Help: "Total matches created.",
	})

	// RankingRecalcDuration — observed each time the asynq worker finishes a
	// full score recalculation pass.
	RankingRecalcDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "drizzy_ranking_recalc_duration_seconds",
		Help:    "Duration of a full ranking recalculation pass.",
		Buckets: []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
	})

	// DiscoveryQueueRefills — counts how many users had their Redis discovery
	// queue refilled by a worker pass.
	DiscoveryQueueRefills = promauto.NewCounter(prometheus.CounterOpts{
		Name: "drizzy_discovery_queue_refills_total",
		Help: "Total discovery-queue refills issued by ranking workers.",
	})
)

// Handler returns the /metrics HTTP handler for a service.
func Handler() http.Handler { return promhttp.Handler() }

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware records request count + latency. `service` is the logical
// service name ("profile-service" / "ranking-service") used as a label.
func Middleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			path := r.URL.Path
			// Skip metrics self-scrape from histograms to avoid cardinality bloat.
			if path == "/metrics" {
				return
			}
			httpRequestsTotal.WithLabelValues(service, r.Method, path, strconv.Itoa(rec.status)).Inc()
			httpRequestDuration.WithLabelValues(service, r.Method, path).Observe(time.Since(start).Seconds())
		})
	}
}
