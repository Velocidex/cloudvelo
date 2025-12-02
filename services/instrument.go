package services

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OpensearchHistorgram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opensearch_latency",
			Help:    "Latency to access datastore.",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		},
		[]string{"operation"},
	)

	OpensearchSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "opensearch_operations",
			Help: "Latency to access datastore.",
		},
		[]string{"operation"},
	)

	// Watch operations in real time using:
	// watch 'curl -s http://localhost:8003/metrics | grep -E "operations{|opensearch_latency_bucket.+Inf"'
	OperationCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "operations",
			Help: "Count of operations.",
		},
		[]string{"operation"},
	)
)

func Count(operation string) {
	OperationCounter.WithLabelValues(operation).Inc()
}

func Instrument(operation string) func() time.Duration {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		OpensearchHistorgram.WithLabelValues(operation).Observe(v)
	}))

	return timer.ObserveDuration
}

func Summarize(operation string) func() time.Duration {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		OpensearchSummary.WithLabelValues(operation).Observe(v)
	}))

	return timer.ObserveDuration
}
