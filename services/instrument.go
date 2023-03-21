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
)

func Instrument(operation string) func() time.Duration {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		OpensearchHistorgram.WithLabelValues(operation).Observe(v)
	}))

	return timer.ObserveDuration
}
