// Package chaos provides simulated chaos events for the Fleet agent simulator.
// Currently supports drift simulation (Phase 4).
package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/rancher/fleet/agent-simulator/pkg/resources"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DriftOptions configures the drift scheduler.
type DriftOptions struct {
	// Namespace is the cluster namespace to watch BundleDeployments in.
	Namespace string
	// TotalResourceCount is the total number of fake resources per BD.
	TotalResourceCount int
	// AffectedResourceCount is the number of resources to mark as drifted per event.
	AffectedResourceCount int
	// MinInterval is the minimum time between drift events per BD.
	MinInterval time.Duration
	// MaxInterval is the maximum time between drift events per BD.
	MaxInterval time.Duration
	// AffectsReady controls whether drift events set Ready=false on the BD.
	AffectsReady bool
	// AutoRecover controls whether drifted BDs are automatically recovered.
	AutoRecover bool
	// RecoveryDelay is the delay between a drift event and auto-recovery.
	RecoveryDelay time.Duration
}

// bdDriftState tracks per-BD drift timing and state.
type bdDriftState struct {
	// nextDrift is when the next drift event should fire.
	nextDrift time.Time
	// drifted is true when the BD is currently in a drifted state.
	drifted bool
	// nextRecovery is when auto-recovery should fire (zero if not scheduled).
	nextRecovery time.Time
}

// DriftScheduler periodically injects drift into ready BundleDeployments.
// It implements manager.Runnable.
type DriftScheduler struct {
	client client.Client
	opts   DriftOptions
	rng    *rand.Rand

	mu     sync.Mutex
	states map[string]*bdDriftState // key = "namespace/name"
}

// NewDriftScheduler creates a new DriftScheduler with a random seed.
func NewDriftScheduler(c client.Client, opts DriftOptions) *DriftScheduler {
	return newDriftSchedulerWithRand(c, opts, rand.New(rand.NewSource(time.Now().UnixNano())))
}

// NewDriftSchedulerWithRand creates a DriftScheduler with a caller-supplied RNG (useful in tests).
func NewDriftSchedulerWithRand(c client.Client, opts DriftOptions, rng *rand.Rand) *DriftScheduler {
	return newDriftSchedulerWithRand(c, opts, rng)
}

func newDriftSchedulerWithRand(c client.Client, opts DriftOptions, rng *rand.Rand) *DriftScheduler {
	return &DriftScheduler{
		client: c,
		opts:   opts,
		rng:    rng,
		states: make(map[string]*bdDriftState),
	}
}

// Start implements manager.Runnable. It runs the drift tick loop until ctx is cancelled.
func (d *DriftScheduler) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("drift-scheduler")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := d.tick(ctx, now); err != nil {
				logger.Error(err, "drift tick failed")
			}
		}
	}
}

// tick processes all BDs in the namespace for drift and recovery events.
func (d *DriftScheduler) tick(ctx context.Context, now time.Time) error {
	logger := log.FromContext(ctx).WithName("drift-scheduler")

	var bdList fleet.BundleDeploymentList
	if err := d.client.List(ctx, &bdList, client.InNamespace(d.opts.Namespace)); err != nil {
		return fmt.Errorf("listing BundleDeployments: %w", err)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Track which BD keys still exist for cleanup.
	current := make(map[string]struct{}, len(bdList.Items))

	for i := range bdList.Items {
		bd := &bdList.Items[i]
		key := bd.Namespace + "/" + bd.Name
		current[key] = struct{}{}

		state, exists := d.states[key]
		if !exists {
			state = &bdDriftState{
				nextDrift: now.Add(d.randomInterval()),
			}
			d.states[key] = state
		}

		// Handle pending auto-recovery.
		if state.drifted && !state.nextRecovery.IsZero() && !now.Before(state.nextRecovery) {
			logger.V(1).Info("Recovering drifted BD", "bd", bd.Name)
			if err := d.applyRecovery(ctx, bd); err != nil {
				logger.Error(err, "failed to recover BD", "bd", bd.Name)
			} else {
				state.drifted = false
				state.nextRecovery = time.Time{}
				state.nextDrift = now.Add(d.randomInterval())
			}
			continue
		}

		// Only drift ready, non-drifted BDs whose timer has fired.
		if !state.drifted && bd.Status.Ready && !now.Before(state.nextDrift) {
			logger.V(1).Info("Applying drift to BD", "bd", bd.Name)
			if err := d.applyDrift(ctx, bd); err != nil {
				logger.Error(err, "failed to apply drift", "bd", bd.Name)
			} else {
				state.drifted = true
				if d.opts.AutoRecover {
					state.nextRecovery = now.Add(d.opts.RecoveryDelay)
				}
			}
		}
	}

	// Remove state for BDs that no longer exist.
	for key := range d.states {
		if _, ok := current[key]; !ok {
			delete(d.states, key)
		}
	}

	return nil
}

// applyDrift patches a BD's status to simulate drift (NonModified=false, ModifiedStatus populated).
func (d *DriftScheduler) applyDrift(ctx context.Context, bd *fleet.BundleDeployment) error {
	now := time.Now().UTC().Format(time.RFC3339)
	modified := BuildModifiedStatus(bd.Name, bd.Namespace, d.opts.TotalResourceCount, d.opts.AffectedResourceCount)

	patch := client.MergeFrom(bd.DeepCopy())
	bd.Status.NonModified = false
	bd.Status.ModifiedStatus = modified
	bd.Status.ResourceCounts.Modified = len(modified)

	if d.opts.AffectsReady {
		bd.Status.Ready = false
		for i, cond := range bd.Status.Conditions {
			if cond.Type == "Ready" {
				bd.Status.Conditions[i].Status = corev1.ConditionFalse
				bd.Status.Conditions[i].Reason = "Drifted"
				bd.Status.Conditions[i].Message = fmt.Sprintf("%d resource(s) drifted", len(modified))
				bd.Status.Conditions[i].LastUpdateTime = now
				break
			}
		}
		bd.Status.Display.State = "Modified"
	}

	return d.client.Status().Patch(ctx, bd, patch)
}

// applyRecovery patches a BD's status to clear drift (NonModified=true, ModifiedStatus cleared).
func (d *DriftScheduler) applyRecovery(ctx context.Context, bd *fleet.BundleDeployment) error {
	now := time.Now().UTC().Format(time.RFC3339)

	patch := client.MergeFrom(bd.DeepCopy())
	bd.Status.NonModified = true
	bd.Status.ModifiedStatus = nil
	bd.Status.ResourceCounts.Modified = 0

	if d.opts.AffectsReady {
		bd.Status.Ready = true
		for i, cond := range bd.Status.Conditions {
			if cond.Type == "Ready" {
				bd.Status.Conditions[i].Status = corev1.ConditionTrue
				bd.Status.Conditions[i].Reason = "Ready"
				bd.Status.Conditions[i].Message = ""
				bd.Status.Conditions[i].LastUpdateTime = now
				break
			}
		}
		bd.Status.Display.State = "Ready"
	}

	return d.client.Status().Patch(ctx, bd, patch)
}

// randomInterval returns a random duration in [MinInterval, MaxInterval).
func (d *DriftScheduler) randomInterval() time.Duration {
	return RandomInterval(d.rng, d.opts.MinInterval, d.opts.MaxInterval)
}

// RandomInterval returns a random duration in [min, max) using the provided RNG.
// If max <= min, returns min.
func RandomInterval(rng *rand.Rand, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	return min + time.Duration(rng.Int63n(int64(max-min)))
}

// BuildModifiedStatus generates synthetic ModifiedStatus entries for drifted resources.
// It selects the first affectedCount resources from the BD's fake resource list.
func BuildModifiedStatus(bdName, namespace string, totalCount, affectedCount int) []fleet.ModifiedStatus {
	res := resources.Generate(bdName, namespace, totalCount)
	if affectedCount > len(res) {
		affectedCount = len(res)
	}
	result := make([]fleet.ModifiedStatus, 0, affectedCount)
	for i := 0; i < affectedCount; i++ {
		r := res[i]
		result = append(result, fleet.ModifiedStatus{
			Kind:       r.Kind,
			APIVersion: r.APIVersion,
			Namespace:  r.Namespace,
			Name:       r.Name,
			Patch:      `{"spec":{"replicas":3}}`,
		})
	}
	return result
}
