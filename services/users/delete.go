package users

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	deleteUserCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "velociraptor_delete_user_count",
			Help: "Count of users deleted from the Velociraptor system.",
		})
)
