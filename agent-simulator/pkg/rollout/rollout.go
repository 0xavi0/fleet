// Package rollout manages per-BundleDeployment rollout state for the simulator.
// It tracks incremental progress toward a fully-ready state across N steps.
package rollout

import "sync"

// State tracks the rollout progress for a single BundleDeployment.
type State struct {
	DeploymentID string
	CurrentStep  int
	TotalSteps   int
}

// Tracker manages per-BD rollout state in memory.
// Each key is typically "namespace/name" of the BundleDeployment.
type Tracker struct {
	mu     sync.Mutex
	states map[string]*State
}

// NewTracker creates a new Tracker.
func NewTracker() *Tracker {
	return &Tracker{states: make(map[string]*State)}
}

// IsInProgress returns true if a rollout is actively in progress for the given
// key and deploymentID (i.e., the rollout has started but not yet completed).
func (t *Tracker) IsInProgress(key, deploymentID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.states[key]
	return ok && s.DeploymentID == deploymentID && s.CurrentStep < s.TotalSteps
}

// Next advances the rollout by one step and returns (currentStep, isDone).
// If the deploymentID has changed, the state is reset and starts from step 1.
// Once done (currentStep >= totalSteps), further calls return (totalSteps, true)
// without modifying the state.
func (t *Tracker) Next(key, deploymentID string, totalSteps int) (step int, done bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, exists := t.states[key]
	if !exists || s.DeploymentID != deploymentID {
		s = &State{
			DeploymentID: deploymentID,
			CurrentStep:  0,
			TotalSteps:   totalSteps,
		}
		t.states[key] = s
	}

	if s.CurrentStep < s.TotalSteps {
		s.CurrentStep++
	}
	return s.CurrentStep, s.CurrentStep >= s.TotalSteps
}

// Delete removes the rollout state for the given key.
func (t *Tracker) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.states, key)
}

// ReadyCount computes the number of ready resources for the given step.
// The final step always returns resourceCount.
func ReadyCount(resourceCount, currentStep, totalSteps int) int {
	if currentStep >= totalSteps {
		return resourceCount
	}
	return resourceCount * currentStep / totalSteps
}
