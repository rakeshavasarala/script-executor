package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ExecutionsTotal counts script executions.
	ExecutionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "script_executor_executions_total",
			Help: "Total number of script executions",
		},
		[]string{"status"},
	)

	// ExecutionDuration tracks execution duration.
	ExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "script_executor_execution_duration_seconds",
			Help:    "Script execution duration in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		},
		[]string{"status"},
	)

	// ApprovalsTotal counts approval requests.
	ApprovalsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "script_executor_approvals_total",
			Help: "Total approval requests",
		},
		[]string{"status"},
	)
)
