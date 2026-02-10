// Copyright (c) 2021-2026 SUSE LLC

package reconciler

import (
	"reflect"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// TypedResourceVersionUnchangedPredicate implements a update predicate to
// allow syncPeriod to trigger the reconciler
type TypedResourceVersionUnchangedPredicate[T metav1.Object] struct {
	predicate.TypedFuncs[T]
}

func isNil(arg any) bool {
	if v := reflect.ValueOf(arg); !v.IsValid() || ((v.Kind() == reflect.Ptr ||
		v.Kind() == reflect.Interface ||
		v.Kind() == reflect.Slice ||
		v.Kind() == reflect.Map ||
		v.Kind() == reflect.Chan ||
		v.Kind() == reflect.Func) && v.IsNil()) {
		return true
	}
	return false
}

func (TypedResourceVersionUnchangedPredicate[T]) Create(e event.CreateEvent) bool {
	return false
}

func (TypedResourceVersionUnchangedPredicate[T]) Delete(e event.DeleteEvent) bool {
	return false
}

// Update implements default UpdateEvent filter for validating resource version change.
func (TypedResourceVersionUnchangedPredicate[T]) Update(e event.TypedUpdateEvent[T]) bool {
	if isNil(e.ObjectOld) {
		return false
	}
	if isNil(e.ObjectNew) {
		return false
	}

	return e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion()
}

func (TypedResourceVersionUnchangedPredicate[T]) Generic(e event.GenericEvent) bool {
	return false
}

// bundleDeploymentStatusChangedPredicate returns true if the bundledeployment
// status has changed, or the bundledeployment was created
func bundleDeploymentStatusChangedPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			n := e.ObjectNew.(*fleet.BundleDeployment)
			o := e.ObjectOld.(*fleet.BundleDeployment)
			if n == nil || o == nil {
				return false
			}
			return !n.DeletionTimestamp.IsZero() || !reflect.DeepEqual(n.Status, o.Status)
		},
	}
}

// jobUpdatedPredicate returns true if the job status has changed
func jobUpdatedPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			n, isJob := e.ObjectNew.(metav1.Object)
			if !isJob {
				return false
			}
			o := e.ObjectOld.(metav1.Object)
			if n == nil || o == nil {
				return false
			}
			// For the monitor, we only care about status changes
			// We can't directly access .Status without type assertion,
			// but we can check if the resource version changed
			return n.GetResourceVersion() != o.GetResourceVersion()
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

// webhookCommitChangedPredicate returns true if the webhook commit has changed
func webhookCommitChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldGitRepo, ok := e.ObjectOld.(*fleet.GitRepo)
			if !ok {
				return true
			}
			newGitRepo, ok := e.ObjectNew.(*fleet.GitRepo)
			if !ok {
				return true
			}
			return oldGitRepo.Status.WebhookCommit != newGitRepo.Status.WebhookCommit
		},
	}
}
