// Package metrics defines and registers Prometheus metrics for the Fleet agent simulator.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// HeartbeatsTotal counts Cluster/status heartbeats sent, partitioned by cluster.
	HeartbeatsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "simulator_heartbeats_total",
			Help: "Total number of Cluster/status heartbeats sent, partitioned by cluster.",
		},
		[]string{"cluster"},
	)

	// StatusPatchesTotal counts BundleDeployment status patches sent,
	// partitioned by cluster and BD state ("rolling" or "ready").
	StatusPatchesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "simulator_status_patches_total",
			Help: "Total number of BundleDeployment status patches sent, partitioned by cluster and state.",
		},
		[]string{"cluster", "state"},
	)

	// BDState tracks the current count of BundleDeployments in each state per cluster.
	// States: "rolling" (mid-rollout), "ready" (fully applied).
	BDState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "simulator_bd_state",
			Help: "Current count of BundleDeployments in each state, partitioned by cluster and state.",
		},
		[]string{"cluster", "state"},
	)

	// PatchDurationSeconds tracks the latency of BundleDeployment status patch API calls.
	PatchDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "simulator_patch_duration_seconds",
			Help:    "Duration of BundleDeployment status patch API calls in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"cluster"},
	)

	// PatchErrorsTotal counts failed BundleDeployment status patch attempts per cluster.
	PatchErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "simulator_patch_errors_total",
			Help: "Total number of failed BundleDeployment status patch attempts, partitioned by cluster.",
		},
		[]string{"cluster"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		HeartbeatsTotal,
		StatusPatchesTotal,
		BDState,
		PatchDurationSeconds,
		PatchErrorsTotal,
	)
}
