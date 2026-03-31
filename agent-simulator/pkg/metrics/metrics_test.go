package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/rancher/fleet/agent-simulator/pkg/metrics"
)

func TestHeartbeatsTotal(t *testing.T) {
	metrics.HeartbeatsTotal.Reset()

	metrics.HeartbeatsTotal.WithLabelValues("cluster-a").Inc()
	metrics.HeartbeatsTotal.WithLabelValues("cluster-a").Inc()
	metrics.HeartbeatsTotal.WithLabelValues("cluster-b").Inc()

	got := testutil.ToFloat64(metrics.HeartbeatsTotal.With(prometheus.Labels{"cluster": "cluster-a"}))
	if got != 2 {
		t.Errorf("HeartbeatsTotal cluster-a: want 2, got %v", got)
	}

	got = testutil.ToFloat64(metrics.HeartbeatsTotal.With(prometheus.Labels{"cluster": "cluster-b"}))
	if got != 1 {
		t.Errorf("HeartbeatsTotal cluster-b: want 1, got %v", got)
	}
}

func TestStatusPatchesTotal(t *testing.T) {
	metrics.StatusPatchesTotal.Reset()

	metrics.StatusPatchesTotal.WithLabelValues("cluster-a", "rolling").Inc()
	metrics.StatusPatchesTotal.WithLabelValues("cluster-a", "ready").Inc()
	metrics.StatusPatchesTotal.WithLabelValues("cluster-a", "ready").Inc()

	rolling := testutil.ToFloat64(metrics.StatusPatchesTotal.With(prometheus.Labels{"cluster": "cluster-a", "state": "rolling"}))
	if rolling != 1 {
		t.Errorf("StatusPatchesTotal rolling: want 1, got %v", rolling)
	}

	ready := testutil.ToFloat64(metrics.StatusPatchesTotal.With(prometheus.Labels{"cluster": "cluster-a", "state": "ready"}))
	if ready != 2 {
		t.Errorf("StatusPatchesTotal ready: want 2, got %v", ready)
	}
}

func TestBDState(t *testing.T) {
	metrics.BDState.Reset()

	metrics.BDState.WithLabelValues("cluster-a", "rolling").Set(3)
	metrics.BDState.WithLabelValues("cluster-a", "ready").Set(5)

	rolling := testutil.ToFloat64(metrics.BDState.With(prometheus.Labels{"cluster": "cluster-a", "state": "rolling"}))
	if rolling != 3 {
		t.Errorf("BDState rolling: want 3, got %v", rolling)
	}

	ready := testutil.ToFloat64(metrics.BDState.With(prometheus.Labels{"cluster": "cluster-a", "state": "ready"}))
	if ready != 5 {
		t.Errorf("BDState ready: want 5, got %v", ready)
	}
}

func TestBDStateIncrementDecrement(t *testing.T) {
	metrics.BDState.Reset()

	// Simulate a BD starting to roll out then becoming ready.
	metrics.BDState.WithLabelValues("cluster-a", "rolling").Inc()

	rolling := testutil.ToFloat64(metrics.BDState.With(prometheus.Labels{"cluster": "cluster-a", "state": "rolling"}))
	if rolling != 1 {
		t.Fatalf("BDState rolling after Inc: want 1, got %v", rolling)
	}

	metrics.BDState.WithLabelValues("cluster-a", "rolling").Dec()
	metrics.BDState.WithLabelValues("cluster-a", "ready").Inc()

	rolling = testutil.ToFloat64(metrics.BDState.With(prometheus.Labels{"cluster": "cluster-a", "state": "rolling"}))
	if rolling != 0 {
		t.Errorf("BDState rolling after transition: want 0, got %v", rolling)
	}

	ready := testutil.ToFloat64(metrics.BDState.With(prometheus.Labels{"cluster": "cluster-a", "state": "ready"}))
	if ready != 1 {
		t.Errorf("BDState ready after transition: want 1, got %v", ready)
	}
}

func TestPatchErrorsTotal(t *testing.T) {
	metrics.PatchErrorsTotal.Reset()

	metrics.PatchErrorsTotal.WithLabelValues("cluster-a").Inc()
	metrics.PatchErrorsTotal.WithLabelValues("cluster-a").Inc()

	got := testutil.ToFloat64(metrics.PatchErrorsTotal.With(prometheus.Labels{"cluster": "cluster-a"}))
	if got != 2 {
		t.Errorf("PatchErrorsTotal: want 2, got %v", got)
	}
}

func TestPatchDurationSecondsRegistered(t *testing.T) {
	metrics.PatchDurationSeconds.Reset()

	// Verify Observe does not panic and the histogram is registered.
	metrics.PatchDurationSeconds.WithLabelValues("cluster-a").Observe(0.01)
	metrics.PatchDurationSeconds.WithLabelValues("cluster-a").Observe(0.1)
	metrics.PatchDurationSeconds.WithLabelValues("cluster-a").Observe(1.5)

	// CollectAndCount returns the number of time series written; histogram with
	// one label set produces 1 histogram (sum + count + buckets as one family).
	n := testutil.CollectAndCount(metrics.PatchDurationSeconds)
	if n == 0 {
		t.Error("PatchDurationSeconds: expected at least one series after Observe calls")
	}
}
