// Copyright (c) 2024-2026 SUSE LLC

package reconciler

import (
	"encoding/json"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var globalStatsTracker = NewStatsTracker()

// GetStatsTracker returns the global stats tracker
func GetStatsTracker() *StatsTracker {
	return globalStatsTracker
}

// recordEvent records an event in statistics (always, regardless of mode)
func recordEvent(resourceType, namespace, name string, eventType EventType) {
	globalStatsTracker.RecordEvent(resourceType, namespace, name, eventType)
}

// logSpecChange logs the differences in spec between old and new objects
// detailedLogs parameter controls whether to emit detailed log lines
// eventFilters parameter controls which event types to show
func logSpecChange(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string, oldSpec, newSpec interface{}, oldGen, newGen int64) {
	if oldGen == newGen {
		return
	}

	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeGenerationChange)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeGenerationChange) {
		diff := cmp.Diff(oldSpec, newSpec)
		if diff != "" {
			logger.Info("Spec changed - Generation update detected",
				"event", "generation-change",
				"oldGeneration", oldGen,
				"newGeneration", newGen,
				"specDiff", diff,
			)
		} else {
			logger.Info("Generation changed but spec appears identical",
				"event", "generation-change",
				"oldGeneration", oldGen,
				"newGeneration", newGen,
			)
		}
	}
}

// logStatusChange logs differences in status between old and new objects
func logStatusChange(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string, oldStatus, newStatus interface{}) {
	if equality.Semantic.DeepEqual(oldStatus, newStatus) {
		return
	}

	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeStatusChange)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeStatusChange) {
		oldJSON, err := json.MarshalIndent(oldStatus, "", "  ")
		if err != nil {
			logger.Error(err, "Failed to marshal old status")
			oldJSON = []byte("{}")
		}

		newJSON, err := json.MarshalIndent(newStatus, "", "  ")
		if err != nil {
			logger.Error(err, "Failed to marshal new status")
			newJSON = []byte("{}")
		}

		diff := cmp.Diff(oldStatus, newStatus)

		logger.Info("Status changed",
			"event", "status-change",
			"oldStatus", string(oldJSON),
			"newStatus", string(newJSON),
			"diff", diff,
		)
	}
}

// logResourceVersionChangeWithMetadata logs resource version changes and checks for metadata differences
func logResourceVersionChangeWithMetadata(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string, oldObj, newObj client.Object) {
	oldRV := oldObj.GetResourceVersion()
	newRV := newObj.GetResourceVersion()

	if oldRV == newRV {
		return
	}

	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeResourceVersionChange)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeResourceVersionChange) {
		// Check for specific metadata changes
		var metadataChanges []string
		var diffs []string

		// Check finalizers
		oldFinalizers := oldObj.GetFinalizers()
		newFinalizers := newObj.GetFinalizers()
		if !equality.Semantic.DeepEqual(oldFinalizers, newFinalizers) {
			metadataChanges = append(metadataChanges, "finalizers")
			diff := cmp.Diff(oldFinalizers, newFinalizers)
			diffs = append(diffs, "Finalizers:\n"+diff)
		}

		// Check owner references
		oldOwners := oldObj.GetOwnerReferences()
		newOwners := newObj.GetOwnerReferences()
		if !equality.Semantic.DeepEqual(oldOwners, newOwners) {
			metadataChanges = append(metadataChanges, "ownerReferences")
			diff := cmp.Diff(oldOwners, newOwners)
			diffs = append(diffs, "OwnerReferences:\n"+diff)
		}

		// Check managed fields (common with Server-Side Apply)
		oldManaged := oldObj.GetManagedFields()
		newManaged := newObj.GetManagedFields()
		if !equality.Semantic.DeepEqual(oldManaged, newManaged) {
			metadataChanges = append(metadataChanges, "managedFields")
			// Don't include full diff for managedFields as it's very verbose
		}

		reason := "cache sync or unknown metadata update"
		if len(metadataChanges) > 0 {
			// Format metadataChanges as comma-separated list
			var changeList string
			for i, change := range metadataChanges {
				if i > 0 {
					changeList += ", "
				}
				changeList += change
			}
			reason = "metadata update: " + changeList
		}

		logFields := []interface{}{
			"event", "resourceversion-change",
			"oldResourceVersion", oldRV,
			"newResourceVersion", newRV,
			"reason", reason,
		}

		if len(metadataChanges) > 0 {
			logFields = append(logFields, "metadataChanges", metadataChanges)
			if len(diffs) > 0 {
				for _, d := range diffs {
					logFields = append(logFields, "diff", d)
				}
			}
		}

		logger.Info("Resource version changed", logFields...)
	}
}

// logAnnotationChange logs annotation changes
func logAnnotationChange(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string, oldAnnotations, newAnnotations map[string]string) {
	if equality.Semantic.DeepEqual(oldAnnotations, newAnnotations) {
		return
	}

	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeAnnotationChange)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeAnnotationChange) {
		diff := cmp.Diff(oldAnnotations, newAnnotations)
		logger.Info("Annotations changed",
			"event", "annotation-change",
			"diff", diff,
		)
	}
}

// logLabelChange logs label changes
func logLabelChange(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string, oldLabels, newLabels map[string]string) {
	if equality.Semantic.DeepEqual(oldLabels, newLabels) {
		return
	}

	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeLabelChange)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeLabelChange) {
		diff := cmp.Diff(oldLabels, newLabels)
		logger.Info("Labels changed",
			"event", "label-change",
			"diff", diff,
		)
	}
}

// logRelatedResourceTrigger logs when a reconciliation is triggered by a related resource
func logRelatedResourceTrigger(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLogTrigger() bool }, resourceType, namespace, name string, triggerType, triggerName, triggerNamespace string) {
	// Always record in stats with breakdown by trigger type
	globalStatsTracker.RecordTrigger(resourceType, namespace, name, triggerType)

	// Only log details if detailed mode enabled AND triggered-by events are enabled
	if detailedLogs && eventFilters.ShouldLogTrigger() {
		logger.Info("Triggered by related resource change",
			"event", "related-resource-trigger",
			"triggerResourceType", triggerType,
			"triggerResourceName", triggerName,
			"triggerResourceNamespace", triggerNamespace,
		)
	}
}

// logDeletion logs when a resource is being deleted
func logDeletion(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string, deletionTimestamp string) {
	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeDeletion)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeDeletion) {
		logger.Info("Resource deletion detected",
			"event", "deletion",
			"deletionTimestamp", deletionTimestamp,
		)
	}
}

// logNotFound logs when a resource is not found (deleted)
func logNotFound(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string) {
	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeNotFound)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeNotFound) {
		logger.Info("Resource not found - likely deleted",
			"event", "not-found",
		)
	}
}

// logCreate logs first observation of a resource
func logCreate(logger logr.Logger, detailedLogs bool, eventFilters interface{ ShouldLog(EventType) bool }, resourceType, namespace, name string, generation int64, resourceVersion string) {
	// Always record in stats
	recordEvent(resourceType, namespace, name, EventTypeCreate)

	// Only log details if detailed mode enabled AND event type is enabled
	if detailedLogs && eventFilters.ShouldLog(EventTypeCreate) {
		logger.Info("First observation of resource",
			"event", "create",
			"generation", generation,
			"resourceVersion", resourceVersion,
		)
	}
}
