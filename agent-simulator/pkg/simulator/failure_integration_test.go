package simulator_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/fleet/agent-simulator/pkg/chaos"
	"github.com/rancher/fleet/agent-simulator/pkg/simulator"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// startManagerWithFailure starts a simulator manager with failure enabled and the provided
// failure options. The failure Namespace and TotalResourceCount fields are set automatically
// from the standard test options.
func startManagerWithFailure(failureOpts chaos.FailureOptions) context.CancelFunc {
	mgrCtx, mgrCancel := context.WithCancel(ctx)

	opts := simulator.Options{
		ClusterNamespace:      "fleet-default",
		BDNamespace:           clusterNS,
		ClusterName:           "sim-failure-test",
		AgentNamespace:        agentNS,
		ResourceCount:         resourceCount,
		HeartbeatInterval:     time.Hour,
		HeartbeatInitialDelay: time.Hour,
		RolloutSteps:          1,
		RolloutInterval:       50 * time.Millisecond,
		SkipNameValidation:    true,
		FailureEnabled:        true,
		Failure:               failureOpts,
	}

	mgr, err := simulator.NewManager(restCfg, scheme, opts)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()

	return mgrCancel
}

var _ = Describe("FailureScheduler", func() {
	var stopManager context.CancelFunc

	AfterEach(func() {
		stopManager()
	})

	It("transitions a ready BD to failed state", func() {
		stopManager = startManagerWithFailure(chaos.FailureOptions{
			AffectedResourceCount: 2,
			MinInterval:           50 * time.Millisecond,
			MaxInterval:           100 * time.Millisecond,
			Probability:           1.0,
			AutoRecover:           false,
		})

		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bd-failure-basic",
				Namespace: clusterNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "deploy-fail-001",
			},
		}
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// Wait for the BD to become ready via the reconciler.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeTrue())
		}, timeout, pollInterval).Should(Succeed())

		// The failure scheduler should set Ready=false with NonReadyStatus populated.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeFalse())
			g.Expect(got.Status.NonReadyStatus).To(HaveLen(2))
			g.Expect(got.Status.ResourceCounts.NotReady).To(Equal(2))
			g.Expect(got.Status.ResourceCounts.Ready).To(Equal(resourceCount - 2))
		}, timeout, pollInterval).Should(Succeed())
	})

	It("populates NonReadyStatus with Pod kind and error state", func() {
		stopManager = startManagerWithFailure(chaos.FailureOptions{
			AffectedResourceCount: 1,
			MinInterval:           50 * time.Millisecond,
			MaxInterval:           100 * time.Millisecond,
			Probability:           1.0,
			AutoRecover:           false,
		})

		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bd-failure-pod-kind",
				Namespace: clusterNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "deploy-fail-002",
			},
		}
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// Wait for ready then failure.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeFalse())
			g.Expect(got.Status.NonReadyStatus).To(HaveLen(1))
			g.Expect(got.Status.NonReadyStatus[0].Kind).To(Equal("Pod"))
			g.Expect(got.Status.NonReadyStatus[0].APIVersion).To(Equal("v1"))
			g.Expect(got.Status.NonReadyStatus[0].Summary.Error).To(BeTrue())
			g.Expect(got.Status.NonReadyStatus[0].Summary.State).NotTo(BeEmpty())
		}, timeout, pollInterval).Should(Succeed())
	})

	It("does not fail non-ready BDs", func() {
		stopManager = startManagerWithFailure(chaos.FailureOptions{
			AffectedResourceCount: 1,
			MinInterval:           50 * time.Millisecond,
			MaxInterval:           100 * time.Millisecond,
			Probability:           1.0,
			AutoRecover:           false,
		})

		// Create a BD and keep it non-ready by using a DeploymentID that won't be processed
		// (Paused=true means the reconciler won't mark it ready).
		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bd-failure-skips-nonready",
				Namespace: clusterNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "deploy-fail-003",
				Paused:       true,
			},
		}
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// The failure scheduler should never set Ready=false on a BD that was never ready.
		// (Ready starts as false and should stay false – but NonReadyStatus from the failure
		// scheduler should NOT appear since the BD was never ready.)
		Consistently(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			// The scheduler only targets Ready=true BDs, so NonReadyStatus should remain nil.
			g.Expect(got.Status.NonReadyStatus).To(BeEmpty())
			// Display.State should not be set to "NotReady" by the failure scheduler.
			g.Expect(got.Status.Display.State).NotTo(Equal("NotReady"))
		}, 500*time.Millisecond, pollInterval).Should(Succeed())
	})

	It("auto-recovers a failed BD after the recovery delay", func() {
		stopManager = startManagerWithFailure(chaos.FailureOptions{
			AffectedResourceCount: 1,
			MinInterval:           50 * time.Millisecond,
			MaxInterval:           100 * time.Millisecond,
			Probability:           1.0,
			AutoRecover:           true,
			RecoveryDelay:         200 * time.Millisecond,
		})

		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bd-failure-recover",
				Namespace: clusterNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "deploy-fail-004",
			},
		}
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// Wait for the BD to become ready.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeTrue())
		}, timeout, pollInterval).Should(Succeed())

		// Wait for failure injection.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeFalse())
			g.Expect(got.Status.NonReadyStatus).NotTo(BeEmpty())
		}, timeout, pollInterval).Should(Succeed())

		// Auto-recovery should restore Ready=true and clear NonReadyStatus.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeTrue())
			g.Expect(got.Status.NonReadyStatus).To(BeEmpty())
			g.Expect(got.Status.ResourceCounts.Ready).To(Equal(resourceCount))
			g.Expect(got.Status.ResourceCounts.NotReady).To(Equal(0))
		}, timeout, pollInterval).Should(Succeed())
	})
})
