// Copyright (c) 2024-2026 SUSE LLC

package reconciler

// EventTypeFilters controls which event types produce detailed logs
type EventTypeFilters struct {
	GenerationChange      bool // generation-change events
	StatusChange          bool // status-change events
	AnnotationChange      bool // annotation-change events
	LabelChange           bool // label-change events
	ResourceVersionChange bool // resourceversion-change events
	Deletion              bool // deletion events
	NotFound              bool // not-found events
	Create                bool // create events
	TriggeredBy           bool // triggered-by events
}

// IsEmpty returns true if no filters were explicitly set (use all events)
func (f EventTypeFilters) IsEmpty() bool {
	return !f.GenerationChange &&
		!f.StatusChange &&
		!f.AnnotationChange &&
		!f.LabelChange &&
		!f.ResourceVersionChange &&
		!f.Deletion &&
		!f.NotFound &&
		!f.Create &&
		!f.TriggeredBy
}

// ShouldLog returns true if the given event type should produce detailed logs
func (f EventTypeFilters) ShouldLog(eventType EventType) bool {
	// If no filters set, log everything (backwards compatible)
	if f.IsEmpty() {
		return true
	}

	switch eventType {
	case EventTypeGenerationChange:
		return f.GenerationChange
	case EventTypeStatusChange:
		return f.StatusChange
	case EventTypeAnnotationChange:
		return f.AnnotationChange
	case EventTypeLabelChange:
		return f.LabelChange
	case EventTypeResourceVersionChange:
		return f.ResourceVersionChange
	case EventTypeDeletion:
		return f.Deletion
	case EventTypeNotFound:
		return f.NotFound
	case EventTypeCreate:
		return f.Create
	default:
		return true // Unknown event types always logged
	}
}

// ShouldLogTrigger returns true if triggered-by events should produce detailed logs
func (f EventTypeFilters) ShouldLogTrigger() bool {
	if f.IsEmpty() {
		return true
	}
	return f.TriggeredBy
}
