// Package chaos provides simulated chaos events for the Fleet agent simulator.
// failure.go implements the random failure scheduler (Phase 5).
package chaos

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/rancher/fleet/agent-simulator/pkg/resources"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1/summary"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// errorType describes a simulated pod failure mode.
type errorType struct {
	state   string
	message string
}

// errorPool is the set of realistic pod failure states the scheduler cycles through.
var errorPool = []errorType{
	{"CrashLoopBackOff", "back-off 5m0s restarting failed container=sim"},
	{"ImagePullBackOff", "Back-off pulling image \"registry.example.com/app:latest\""},
	{"OOMKilled", "OOMKilled: container exceeded memory limit"},
	{"CreateContainerError", "container create failed: cannot create container for pod"},
	{"ErrImageNeverPull", "ErrImageNeverPull: container image \"app:latest\" is not present with pull policy of Never"},
}

// FailureOptions configures the failure scheduler.
type FailureOptions struct {
	// Namespace is the cluster namespace to watch BundleDeployments in.
	Namespace string
	// TotalResourceCount is the total number of fake resources per BD.
	TotalResourceCount int
	// AffectedResourceCount is the number of resources to mark as failed per event.
	AffectedResourceCount int
	// MinInterval is the minimum time between global failure ticks.
	MinInterval time.Duration
	// MaxInterval is the maximum time between global failure ticks.
	MaxInterval time.Duration
	// Probability is the per-BD probability of being selected on each tick (0.0–1.0).
	Probability float64
	// AutoRecover controls whether failed BDs are automatically recovered.
	AutoRecover bool
	// RecoveryDelay is the delay between a failure event and auto-recovery.
	RecoveryDelay time.Duration
}

// bdFailureState tracks per-BD failure timing and state.
type bdFailureState struct {
	// failed is true when the BD is currently in a failed state.
	failed bool
	// nextRecovery is when auto-recovery should fire (zero if not scheduled).
	nextRecovery time.Time
}

// FailureScheduler periodically injects random failures into ready BundleDeployments.
// It implements manager.Runnable.
type FailureScheduler struct {
	client client.Client
	opts   FailureOptions
	rng    *rand.Rand

	mu       sync.Mutex
	nextTick time.Time
	states   map[string]*bdFailureState // key = "namespace/name"
}

// NewFailureScheduler creates a new FailureScheduler with a random seed.
func NewFailureScheduler(c client.Client, opts FailureOptions) *FailureScheduler {
	return newFailureSchedulerWithRand(c, opts, rand.New(rand.NewSource(time.Now().UnixNano())))
}

// NewFailureSchedulerWithRand creates a FailureScheduler with a caller-supplied RNG (useful in tests).
func NewFailureSchedulerWithRand(c client.Client, opts FailureOptions, rng *rand.Rand) *FailureScheduler {
	return newFailureSchedulerWithRand(c, opts, rng)
}

func newFailureSchedulerWithRand(c client.Client, opts FailureOptions, rng *rand.Rand) *FailureScheduler {
	return &FailureScheduler{
		client: c,
		opts:   opts,
		rng:    rng,
		states: make(map[string]*bdFailureState),
	}
}

// Start implements manager.Runnable. It runs the failure tick loop until ctx is cancelled.
func (f *FailureScheduler) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("failure-scheduler")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := f.tick(ctx, now); err != nil {
				logger.Error(err, "failure tick failed")
			}
		}
	}
}

// tick processes all BDs in the namespace for failure injection and auto-recovery.
func (f *FailureScheduler) tick(ctx context.Context, now time.Time) error {
	logger := log.FromContext(ctx).WithName("failure-scheduler")

	var bdList fleet.BundleDeploymentList
	if err := f.client.List(ctx, &bdList, client.InNamespace(f.opts.Namespace)); err != nil {
		return fmt.Errorf("listing BundleDeployments: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Initialize the first failure tick.
	if f.nextTick.IsZero() {
		f.nextTick = now.Add(f.randomInterval())
	}

	// Check whether this polling cycle coincides with a failure injection tick.
	shouldInject := !now.Before(f.nextTick)
	if shouldInject {
		f.nextTick = now.Add(f.randomInterval())
	}

	// Track which BD keys still exist for cleanup.
	current := make(map[string]struct{}, len(bdList.Items))

	for i := range bdList.Items {
		bd := &bdList.Items[i]
		key := bd.Namespace + "/" + bd.Name
		current[key] = struct{}{}

		state, exists := f.states[key]
		if !exists {
			state = &bdFailureState{}
			f.states[key] = state
		}

		// Handle pending auto-recovery.
		if state.failed && !state.nextRecovery.IsZero() && !now.Before(state.nextRecovery) {
			logger.V(1).Info("Recovering failed BD", "bd", bd.Name)
			if err := f.applyRecovery(ctx, bd); err != nil {
				logger.Error(err, "failed to recover BD", "bd", bd.Name)
			} else {
				state.failed = false
				state.nextRecovery = time.Time{}
			}
			continue
		}

		// On a failure tick: maybe fail ready BDs that are not already failed.
		if shouldInject && !state.failed && bd.Status.Ready {
			if f.rng.Float64() < f.opts.Probability {
				logger.V(1).Info("Injecting failure into BD", "bd", bd.Name)
				if err := f.applyFailure(ctx, bd); err != nil {
					logger.Error(err, "failed to inject failure", "bd", bd.Name)
				} else {
					state.failed = true
					if f.opts.AutoRecover {
						state.nextRecovery = now.Add(f.opts.RecoveryDelay)
					}
				}
			}
		}
	}

	// Remove state for BDs that no longer exist.
	for key := range f.states {
		if _, ok := current[key]; !ok {
			delete(f.states, key)
		}
	}

	return nil
}

// applyFailure patches a BD's status to simulate a failure (Ready=false, NonReadyStatus populated).
func (f *FailureScheduler) applyFailure(ctx context.Context, bd *fleet.BundleDeployment) error {
	now := time.Now().UTC().Format(time.RFC3339)
	nonReady := BuildNonReadyStatus(bd.Name, bd.Namespace, f.opts.TotalResourceCount, f.opts.AffectedResourceCount, f.rng)

	patch := client.MergeFrom(bd.DeepCopy())
	bd.Status.Ready = false
	bd.Status.NonReadyStatus = nonReady
	bd.Status.ResourceCounts.Ready = f.opts.TotalResourceCount - len(nonReady)
	bd.Status.ResourceCounts.NotReady = len(nonReady)

	for i, cond := range bd.Status.Conditions {
		if cond.Type == "Ready" {
			bd.Status.Conditions[i].Status = corev1.ConditionFalse
			bd.Status.Conditions[i].Reason = "SimulatedFailure"
			bd.Status.Conditions[i].Message = fmt.Sprintf("%d resource(s) failed", len(nonReady))
			bd.Status.Conditions[i].LastUpdateTime = now
			break
		}
	}
	bd.Status.Display.State = "NotReady"

	return f.client.Status().Patch(ctx, bd, patch)
}

// applyRecovery patches a BD's status to restore full readiness after a failure.
func (f *FailureScheduler) applyRecovery(ctx context.Context, bd *fleet.BundleDeployment) error {
	now := time.Now().UTC().Format(time.RFC3339)

	patch := client.MergeFrom(bd.DeepCopy())
	bd.Status.Ready = true
	bd.Status.NonReadyStatus = nil
	bd.Status.ResourceCounts.Ready = f.opts.TotalResourceCount
	bd.Status.ResourceCounts.NotReady = 0

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

	return f.client.Status().Patch(ctx, bd, patch)
}

// randomInterval returns a random duration in [MinInterval, MaxInterval).
func (f *FailureScheduler) randomInterval() time.Duration {
	return RandomInterval(f.rng, f.opts.MinInterval, f.opts.MaxInterval)
}

// BuildNonReadyStatus generates synthetic NonReadyStatus entries for failed resources.
// It selects the first affectedCount resources from the BD's fake resource list and
// assigns each a randomly-chosen error state from the pool.
func BuildNonReadyStatus(bdName, namespace string, totalCount, affectedCount int, rng *rand.Rand) []fleet.NonReadyStatus {
	res := resources.Generate(bdName, namespace, totalCount)
	if affectedCount > len(res) {
		affectedCount = len(res)
	}
	result := make([]fleet.NonReadyStatus, 0, affectedCount)
	for i := 0; i < affectedCount; i++ {
		r := res[i]
		et := errorPool[rng.Intn(len(errorPool))]
		// Simulate a pod name derived from the owning resource name.
		podName := fmt.Sprintf("%s-pod-%06x", r.Name, rng.Int31()&0xffffff)
		result = append(result, fleet.NonReadyStatus{
			Kind:       "Pod",
			APIVersion: "v1",
			Namespace:  namespace,
			Name:       podName,
			Summary: summary.Summary{
				State:   et.state,
				Error:   true,
				Message: []string{et.message},
			},
		})
	}
	return result
}
