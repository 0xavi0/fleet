// Copyright (c) 2024-2026 SUSE LLC

package reconciler

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/pkg/sharding"
)

// HelmAppMonitorReconciler monitors HelmApp reconciliations
type HelmAppMonitorReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	ShardID string
	Workers int

	// Cache to store previous state
	cache *ObjectCache

	// Per-controller logging mode
	DetailedLogs   bool
	EventFilters   EventTypeFilters
	ResourceFilter *ResourceFilter
}

// SetupWithManager sets up the controller - IDENTICAL to HelmAppReconciler.SetupWithManager
func (r *HelmAppMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.cache = NewObjectCache()

	return ctrl.NewControllerManagedBy(mgr).
		For(&fleet.HelmApp{},
			builder.WithPredicates(
				predicate.Or(
					// Note: These predicates prevent cache
					// syncPeriod from triggering reconcile, since
					// cache sync is an Update event.
					predicate.GenerationChangedPredicate{},
					predicate.AnnotationChangedPredicate{},
					predicate.LabelChangedPredicate{},
				),
			),
		).
		WithEventFilter(sharding.FilterByShardID(r.ShardID)).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.Workers}).
		Complete(r)
}

// Reconcile monitors HelmApp reconciliation events
func (r *HelmAppMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Check resource filter - skip if resource doesn't match
	if !r.ResourceFilter.Matches(req.Namespace, req.Name) {
		return ctrl.Result{}, nil
	}

	logger := log.FromContext(ctx).WithName("helmapp-monitor")
	logger = logger.WithValues(
		"helmapp", req.NamespacedName.String(),
	)
	ctx = log.IntoContext(ctx, logger)

	helmapp := &fleet.HelmApp{}
	if err := r.Get(ctx, req.NamespacedName, helmapp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logNotFound(logger, r.DetailedLogs, r.EventFilters, "HelmApp", req.Namespace, req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add chart context if available
	if helmapp.Spec.Helm.Chart != "" {
		logger = logger.WithValues(
			"chart", helmapp.Spec.Helm.Chart,
			"version", helmapp.Spec.Helm.Version,
		)
	}

	// Check for deletion
	if !helmapp.DeletionTimestamp.IsZero() {
		logDeletion(logger, r.DetailedLogs, r.EventFilters, "HelmApp", helmapp.Namespace, helmapp.Name, helmapp.DeletionTimestamp.String())
		r.cache.Delete(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Retrieve old object from cache
	oldHelmApp, exists := r.cache.Get(req.NamespacedName)
	if !exists {
		logCreate(logger, r.DetailedLogs, r.EventFilters, "HelmApp", helmapp.Namespace, helmapp.Name, helmapp.Generation, helmapp.ResourceVersion)
		r.cache.Set(req.NamespacedName, helmapp.DeepCopy())
		return ctrl.Result{}, nil
	}

	oldHelmAppTyped := oldHelmApp.(*fleet.HelmApp)

	// Detect what changed
	logSpecChange(logger, r.DetailedLogs, r.EventFilters, "HelmApp", helmapp.Namespace, helmapp.Name, oldHelmAppTyped.Spec, helmapp.Spec, oldHelmAppTyped.Generation, helmapp.Generation)
	logStatusChange(logger, r.DetailedLogs, r.EventFilters, "HelmApp", helmapp.Namespace, helmapp.Name, oldHelmAppTyped.Status, helmapp.Status)
	logResourceVersionChangeWithMetadata(logger, r.DetailedLogs, r.EventFilters, "HelmApp", helmapp.Namespace, helmapp.Name, oldHelmAppTyped, helmapp)
	logAnnotationChange(logger, r.DetailedLogs, r.EventFilters, "HelmApp", helmapp.Namespace, helmapp.Name, oldHelmAppTyped.Annotations, helmapp.Annotations)
	logLabelChange(logger, r.DetailedLogs, r.EventFilters, "HelmApp", helmapp.Namespace, helmapp.Name, oldHelmAppTyped.Labels, helmapp.Labels)

	// Update cache with new state
	r.cache.Set(req.NamespacedName, helmapp.DeepCopy())

	return ctrl.Result{}, nil
}
