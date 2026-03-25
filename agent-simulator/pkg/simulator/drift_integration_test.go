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

// startManagerWithDrift starts a simulator manager with drift enabled and the provided
// drift options. The drift Namespace and TotalResourceCount fields are set automatically
// from the standard test options.
func startManagerWithDrift(driftOpts chaos.DriftOptions) context.CancelFunc {
	mgrCtx, mgrCancel := context.WithCancel(ctx)

	opts := simulator.Options{
		ClusterNamespace:      "fleet-default",
		BDNamespace:           clusterNS,
		ClusterName:           "sim-drift-test",
		AgentNamespace:        agentNS,
		ResourceCount:         resourceCount,
		HeartbeatInterval:     time.Hour,
		HeartbeatInitialDelay: time.Hour,
		RolloutSteps:          1,
		RolloutInterval:       50 * time.Millisecond,
		SkipNameValidation:    true,
		DriftEnabled:          true,
		Drift:                 driftOpts,
	}

	mgr, err := simulator.NewManager(restCfg, scheme, opts)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()

	return mgrCancel
}

var _ = Describe("DriftScheduler", func() {
	var stopManager context.CancelFunc

	AfterEach(func() {
		stopManager()
	})

	It("transitions a ready BD to drifted state", func() {
		stopManager = startManagerWithDrift(chaos.DriftOptions{
			AffectedResourceCount: 2,
			MinInterval:           50 * time.Millisecond,
			MaxInterval:           100 * time.Millisecond,
			AffectsReady:          false,
			AutoRecover:           false,
		})

		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bd-drift-basic",
				Namespace: clusterNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "deploy-drift-001",
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

		// The drift scheduler should set NonModified=false with ModifiedStatus populated.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.NonModified).To(BeFalse())
			g.Expect(got.Status.ModifiedStatus).To(HaveLen(2))
			g.Expect(got.Status.ResourceCounts.Modified).To(Equal(2))
		}, timeout, pollInterval).Should(Succeed())
	})

	It("sets Ready=false when AffectsReady is true", func() {
		stopManager = startManagerWithDrift(chaos.DriftOptions{
			AffectedResourceCount: 1,
			MinInterval:           50 * time.Millisecond,
			MaxInterval:           100 * time.Millisecond,
			AffectsReady:          true,
			AutoRecover:           false,
		})

		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bd-drift-affects-ready",
				Namespace: clusterNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "deploy-drift-002",
			},
		}
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// Wait for ready.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeTrue())
		}, timeout, pollInterval).Should(Succeed())

		// Drift should set Ready=false.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeFalse())
			g.Expect(got.Status.NonModified).To(BeFalse())
			g.Expect(got.Status.ModifiedStatus).NotTo(BeEmpty())
		}, timeout, pollInterval).Should(Succeed())
	})

	It("auto-recovers a drifted BD after the recovery delay", func() {
		stopManager = startManagerWithDrift(chaos.DriftOptions{
			AffectedResourceCount: 1,
			MinInterval:           50 * time.Millisecond,
			MaxInterval:           100 * time.Millisecond,
			AffectsReady:          false,
			AutoRecover:           true,
			RecoveryDelay:         200 * time.Millisecond,
		})

		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bd-drift-recover",
				Namespace: clusterNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "deploy-drift-003",
			},
		}
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// Wait for ready.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeTrue())
		}, timeout, pollInterval).Should(Succeed())

		// Wait for drift.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.NonModified).To(BeFalse())
		}, timeout, pollInterval).Should(Succeed())

		// Auto-recovery should restore NonModified=true and clear ModifiedStatus.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.NonModified).To(BeTrue())
			g.Expect(got.Status.ModifiedStatus).To(BeEmpty())
			g.Expect(got.Status.ResourceCounts.Modified).To(Equal(0))
		}, timeout, pollInterval).Should(Succeed())
	})
})
