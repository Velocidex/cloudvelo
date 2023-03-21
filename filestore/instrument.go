package filestore

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	S3Historgram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "s3_filestore_latency",
			Help:    "Latency to access datastore.",
			Buckets: prometheus.LinearBuckets(0.01, 0.05, 10),
		},
		[]string{"operation"},
	)

	s3_counter_upload = promauto.NewCounter(prometheus.CounterOpts{
		Name: "s3_bytes_uploaded",
		Help: "Total number of bytes send to S3.",
	})

	s3_counter_download = promauto.NewCounter(prometheus.CounterOpts{
		Name: "s3_bytes_downloaded",
		Help: "Total number of bytes read from S3.",
	})
)

func Instrument(operation string) func() time.Duration {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		S3Historgram.WithLabelValues(operation).Observe(v)
	}))

	return timer.ObserveDuration
}
