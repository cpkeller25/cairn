package api

import "github.com/prometheus/client_golang/prometheus"

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cairn_http_requests_total",
			Help: "Total HTTP requests by route, method and status.",
		},
		[]string{"method", "route", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cairn_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds, by route.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)

	evaluationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cairn_evaluations_total",
			Help: "Scorecard evaluations by outcome.",
		},
		[]string{"outcome"},
	)

	evaluationScore = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name: "cairn_evaluation_score",
			Help: "Distribution of overall scorecard scores.",
			// Bucketed around the Bronze/Silver/Gold boundaries.
			Buckets: []float64{20, 40, 59, 60, 70, 84, 85, 90, 100},
		},
	)
)

func init() {
	prometheus.MustRegister(
		httpRequestsTotal,
		httpRequestDuration,
		evaluationsTotal,
		evaluationScore,
	)
}
