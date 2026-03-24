// Package simulator runs a controller-runtime manager that simulates a Fleet agent.
// It watches BundleDeployments in the cluster namespace and responds with status
// updates – either instantly ready (Phase 2) or via a gradual N-step rollout (Phase 3).
package simulator

import (
	"context"
	"fmt"
	"time"

	"github.com/rancher/fleet/agent-simulator/pkg/heartbeat"
	"github.com/rancher/fleet/agent-simulator/pkg/rollout"
	"github.com/rancher/fleet/agent-simulator/pkg/status"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Options configures the simulator manager.
type Options struct {
	// ClusterNamespace is the registration namespace where the Cluster CR lives
	// (e.g. "fleet-default"). Used by the heartbeat to patch Cluster.Status.
	ClusterNamespace string
	// BDNamespace is the cluster namespace where BundleDeployments live.
	BDNamespace string
	// ClusterName is the name of the simulated cluster.
	ClusterName string
	// AgentNamespace is the simulated agent namespace, placed in heartbeats
	// and used to derive Helm release names.
	AgentNamespace string
	// ResourceCount is the number of fake resources to report per BundleDeployment.
	ResourceCount int
	// HeartbeatInterval is the period between heartbeats.
	HeartbeatInterval time.Duration
	// HeartbeatInitialDelay is the delay before the first heartbeat.
	HeartbeatInitialDelay time.Duration
	// RolloutSteps is the number of incremental status updates before a BD is
	// fully ready. 1 means instant-ready (Phase 2 behaviour).
	RolloutSteps int
	// RolloutInterval is the delay between rollout steps.
	RolloutInterval time.Duration
	// SkipNameValidation skips the controller name uniqueness check.
	// Set to true in tests where multiple managers are created in the same process.
	SkipNameValidation bool
}

// NewManager creates a controller-runtime manager scoped to clusterNamespace,
// registers the BundleDeployment reconciler, and adds the heartbeat runnable.
func NewManager(restCfg *rest.Config, scheme *runtime.Scheme, opts Options) (ctrl.Manager, error) {
	skipNameValidation := opts.SkipNameValidation
	mgrOpts := ctrl.Options{
		Scheme: scheme,
		// Disable the metrics and health-probe endpoints – not needed for the simulator.
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		LeaderElection:         false,
		// Scope the cache to the cluster namespace only, same as the real agent.
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				opts.BDNamespace: {},
			},
		},
		Controller: ctrlconfig.Controller{
			SkipNameValidation: &skipNameValidation,
		},
	}
	mgr, err := ctrl.NewManager(restCfg, mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	rolloutSteps := opts.RolloutSteps
	if rolloutSteps < 1 {
		rolloutSteps = 1
	}

	r := &BundleDeploymentReconciler{
		Client:          mgr.GetClient(),
		ClusterName:     opts.ClusterName,
		AgentNamespace:  opts.AgentNamespace,
		ResourceCount:   opts.ResourceCount,
		RolloutSteps:    rolloutSteps,
		RolloutInterval: opts.RolloutInterval,
		tracker:         rollout.NewTracker(),
	}
	if err := r.SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("setting up BD reconciler: %w", err)
	}

	hb := &heartbeatRunnable{
		c:                mgr.GetClient(),
		clusterNamespace: opts.ClusterNamespace,
		clusterName:      opts.ClusterName,
		agentNamespace:   opts.AgentNamespace,
		initialDelay:     opts.HeartbeatInitialDelay,
		interval:         opts.HeartbeatInterval,
	}
	if err := mgr.Add(hb); err != nil {
		return nil, fmt.Errorf("adding heartbeat runnable: %w", err)
	}

	return mgr, nil
}

// heartbeatRunnable adapts heartbeat.Ticker to the ctrl.Runnable interface.
type heartbeatRunnable struct {
	c                client.Client
	clusterNamespace string
	clusterName      string
	agentNamespace   string
	initialDelay     time.Duration
	interval         time.Duration
}

// Start implements manager.Runnable.
func (h *heartbeatRunnable) Start(ctx context.Context) error {
	heartbeat.Ticker(ctx, h.c, h.clusterNamespace, h.clusterName, h.agentNamespace,
		h.initialDelay, h.interval)
	<-ctx.Done()
	return nil
}

// BundleDeploymentReconciler watches BundleDeployments and marks them ready,
// either instantly (RolloutSteps=1) or via an N-step gradual rollout.
type BundleDeploymentReconciler struct {
	client.Client
	// ClusterName is the simulated cluster, used for log context only.
	ClusterName string
	// AgentNamespace is the simulated agent namespace.
	AgentNamespace string
	// ResourceCount is the number of fake resources to report.
	ResourceCount int
	// RolloutSteps is the number of incremental steps to reach full readiness.
	RolloutSteps int
	// RolloutInterval is the delay between rollout steps.
	RolloutInterval time.Duration
	// tracker maintains per-BD rollout state.
	tracker *rollout.Tracker
}

// Reconcile implements reconcile.Reconciler.
func (r *BundleDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("cluster", r.ClusterName)

	var bd fleet.BundleDeployment
	if err := r.Get(ctx, req.NamespacedName, &bd); err != nil {
		if apierrors.IsNotFound(err) {
			r.tracker.Delete(req.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting BundleDeployment: %w", err)
	}

	// Skip BDs that should not be processed by the agent.
	if bd.Spec.Paused {
		logger.V(1).Info("Skipping paused BundleDeployment", "bd", bd.Name)
		return ctrl.Result{}, nil
	}
	if bd.Spec.OffSchedule {
		logger.V(1).Info("Skipping off-schedule BundleDeployment", "bd", bd.Name)
		return ctrl.Result{}, nil
	}
	if bd.Spec.WaitingForValues {
		logger.V(1).Info("Skipping BundleDeployment waiting for values", "bd", bd.Name)
		return ctrl.Result{}, nil
	}

	key := req.NamespacedName.String()

	// If no in-progress rollout state exists for this deploymentID, check whether
	// the BD has already been fully applied (e.g. pre-existing state after restart).
	if !r.tracker.IsInProgress(key, bd.Spec.DeploymentID) {
		if bd.Spec.DeploymentID != "" && bd.Spec.DeploymentID == bd.Status.AppliedDeploymentID {
			return ctrl.Result{}, nil
		}
	}

	// Advance the rollout by one step.
	step, done := r.tracker.Next(key, bd.Spec.DeploymentID, r.RolloutSteps)
	readyCount := rollout.ReadyCount(r.ResourceCount, step, r.RolloutSteps)

	logger.V(1).Info("Updating BundleDeployment status",
		"bd", bd.Name,
		"deploymentID", bd.Spec.DeploymentID,
		"step", step,
		"totalSteps", r.RolloutSteps,
		"readyCount", readyCount,
	)

	newStatus := status.Build(status.BuildParams{
		DeploymentID:   bd.Spec.DeploymentID,
		ResourceCount:  r.ResourceCount,
		ReadyCount:     readyCount,
		AgentNamespace: r.AgentNamespace,
		Namespace:      bd.Namespace,
		BDName:         bd.Name,
	})

	patch := client.MergeFrom(bd.DeepCopy())
	bd.Status = newStatus
	if err := r.Status().Patch(ctx, &bd, patch); err != nil {
		return ctrl.Result{}, fmt.Errorf("patching BundleDeployment status: %w", err)
	}

	if !done {
		return ctrl.Result{RequeueAfter: r.RolloutInterval}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler with the controller-runtime manager.
func (r *BundleDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleet.BundleDeployment{}).
		WithEventFilter(
			// Only trigger on spec changes (generation changes), not status patches.
			// This prevents an infinite reconcile loop when we update status.
			predicate.GenerationChangedPredicate{},
		).
		Complete(r)
}
