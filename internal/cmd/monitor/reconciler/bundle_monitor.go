// Copyright (c) 2024-2026 SUSE LLC

package reconciler

import (
	"context"

	"github.com/rancher/fleet/internal/cmd/controller/target"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/pkg/sharding"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// BundleMonitorReconciler monitors Bundle reconciliations
type BundleMonitorReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	ShardID string
	Workers int

	// Cache to store previous state
	cache *ObjectCache

	// Per-controller logging mode
	DetailedLogs bool
	EventFilters EventTypeFilters
}

// SetupWithManager sets up the controller - IDENTICAL to BundleReconciler.SetupWithManager
func (r *BundleMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.cache = NewObjectCache()

	return ctrl.NewControllerManagedBy(mgr).
		For(&fleet.Bundle{},
			builder.WithPredicates(
				// do not trigger for bundle status changes (except for cache sync)
				predicate.Or(
					TypedResourceVersionUnchangedPredicate[client.Object]{},
					predicate.GenerationChangedPredicate{},
					predicate.AnnotationChangedPredicate{},
					predicate.LabelChangedPredicate{},
				),
			),
		).
		// Note: Maybe improve with WatchesMetadata, does it have access to labels?
		Watches(
			// Fan out from bundledeployment to bundle
			&fleet.BundleDeployment{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []ctrl.Request {
				bd := a.(*fleet.BundleDeployment)
				labels := bd.GetLabels()
				if labels == nil {
					return nil
				}

				ns, name := target.BundleFromDeployment(labels)
				if ns != "" && name != "" {
					// Log trigger source
					logger := log.FromContext(ctx)
					logRelatedResourceTrigger(logger, r.DetailedLogs, r.EventFilters, "Bundle", ns, name, "BundleDeployment", a.GetName(), a.GetNamespace())

					return []ctrl.Request{{
						NamespacedName: types.NamespacedName{
							Namespace: ns,
							Name:      name,
						},
					}}
				}

				return nil
			}),
			builder.WithPredicates(bundleDeploymentStatusChangedPredicate()),
		).
		Watches(
			// Fan out from cluster to bundle
			&fleet.Cluster{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []ctrl.Request {
				// Note: In monitor, we can't query bundles like the real controller
				// Log the trigger but can't enqueue specific bundles without BundleQuery interface
				logger := log.FromContext(ctx)
				logRelatedResourceTrigger(logger, r.DetailedLogs, r.EventFilters, "Bundle", "", "", "Cluster", a.GetName(), a.GetNamespace())
				return nil // Monitor only - can't query without BundleQuery interface
			}),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithEventFilter(sharding.FilterByShardID(r.ShardID)).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.Workers}).
		Complete(r)
}

// Reconcile monitors bundle reconciliation events
func (r *BundleMonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("bundle-monitor")
	logger = logger.WithValues(
		"bundle", req.NamespacedName.String(),
		"mode", logMode(r.DetailedLogs),
	)
	ctx = log.IntoContext(ctx, logger)

	bundle := &fleet.Bundle{}
	if err := r.Get(ctx, req.NamespacedName, bundle); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logNotFound(logger, r.DetailedLogs, r.EventFilters, "Bundle", req.Namespace, req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add gitrepo context if available
	if bundle.Labels[fleet.RepoLabel] != "" {
		logger = logger.WithValues(
			"gitrepo", bundle.Labels[fleet.RepoLabel],
			"commit", bundle.Labels[fleet.CommitLabel],
		)
	}

	// Check for deletion
	if !bundle.DeletionTimestamp.IsZero() {
		logDeletion(logger, r.DetailedLogs, r.EventFilters, "Bundle", bundle.Namespace, bundle.Name, bundle.DeletionTimestamp.String())
		r.cache.Delete(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// Retrieve old object from cache
	oldBundle, exists := r.cache.Get(req.NamespacedName)
	if !exists {
		logCreate(logger, r.DetailedLogs, r.EventFilters, "Bundle", bundle.Namespace, bundle.Name, bundle.Generation, bundle.ResourceVersion)
		r.cache.Set(req.NamespacedName, bundle.DeepCopy())
		return ctrl.Result{}, nil
	}

	oldBundleTyped := oldBundle.(*fleet.Bundle)

	// Detect what changed - pass DetailedLogs flag
	logSpecChange(logger, r.DetailedLogs, r.EventFilters, "Bundle", bundle.Namespace, bundle.Name, oldBundleTyped.Spec, bundle.Spec, oldBundleTyped.Generation, bundle.Generation)
	logStatusChange(logger, r.DetailedLogs, r.EventFilters, "Bundle", bundle.Namespace, bundle.Name, oldBundleTyped.Status, bundle.Status)
	logResourceVersionChangeWithMetadata(logger, r.DetailedLogs, r.EventFilters, "Bundle", bundle.Namespace, bundle.Name, oldBundleTyped, bundle)
	logAnnotationChange(logger, r.DetailedLogs, r.EventFilters, "Bundle", bundle.Namespace, bundle.Name, oldBundleTyped.Annotations, bundle.Annotations)
	logLabelChange(logger, r.DetailedLogs, r.EventFilters, "Bundle", bundle.Namespace, bundle.Name, oldBundleTyped.Labels, bundle.Labels)

	// Update cache with new state
	r.cache.Set(req.NamespacedName, bundle.DeepCopy())

	return ctrl.Result{}, nil
}

func logMode(detailed bool) string {
	if detailed {
		return "detailed"
	}
	return "summary"
}
