package simulator_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/fleet/agent-simulator/pkg/simulator"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// startManagerForNS spins up a simulator manager scoped to a specific cluster namespace.
func startManagerForNS(ns, clusterName string) context.CancelFunc {
	mgrCtx, mgrCancel := context.WithCancel(ctx)

	mgr, err := simulator.NewManager(restCfg, scheme, simulator.Options{
		ClusterNamespace:      "fleet-default",
		BDNamespace:           ns,
		ClusterName:           clusterName,
		AgentNamespace:        agentNS,
		ResourceCount:         resourceCount,
		HeartbeatInterval:     time.Hour,
		HeartbeatInitialDelay: time.Hour,
		RolloutSteps:          1,
		RolloutInterval:       50 * time.Millisecond,
		SkipNameValidation:    true,
	})
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()

	return mgrCancel
}

var _ = Describe("Multi-cluster simulation", func() {
	const (
		ns1 = "cluster-mc-test-1"
		ns2 = "cluster-mc-test-2"
	)

	var (
		stopManager1 context.CancelFunc
		stopManager2 context.CancelFunc
	)

	BeforeEach(func() {
		// Ensure the two cluster namespaces exist (idempotent).
		for _, name := range []string{ns1, ns2} {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
			_ = k8sClient.Create(ctx, ns) // ignore AlreadyExists
		}
	})

	AfterEach(func() {
		if stopManager1 != nil {
			stopManager1()
			stopManager1 = nil
		}
		if stopManager2 != nil {
			stopManager2()
			stopManager2 = nil
		}
	})

	It("processes BundleDeployments in two independent cluster namespaces", func() {
		bd1Name := uniqueName("mc-bd1")
		bd2Name := uniqueName("mc-bd2")

		bd1 := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: bd1Name, Namespace: ns1},
			Spec:       fleet.BundleDeploymentSpec{DeploymentID: "dep-1"},
		}
		bd2 := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: bd2Name, Namespace: ns2},
			Spec:       fleet.BundleDeploymentSpec{DeploymentID: "dep-2"},
		}

		Expect(k8sClient.Create(ctx, bd1)).To(Succeed())
		Expect(k8sClient.Create(ctx, bd2)).To(Succeed())

		stopManager1 = startManagerForNS(ns1, "sim-mc-1")
		stopManager2 = startManagerForNS(ns2, "sim-mc-2")

		// Both BDs should become ready independently.
		Eventually(func(g Gomega) {
			var got1 fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd1Name, Namespace: ns1}, &got1)).To(Succeed())
			g.Expect(got1.Status.Ready).To(BeTrue())
			g.Expect(got1.Status.AppliedDeploymentID).To(Equal("dep-1"))
		}, timeout, pollInterval).Should(Succeed())

		Eventually(func(g Gomega) {
			var got2 fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd2Name, Namespace: ns2}, &got2)).To(Succeed())
			g.Expect(got2.Status.Ready).To(BeTrue())
			g.Expect(got2.Status.AppliedDeploymentID).To(Equal("dep-2"))
		}, timeout, pollInterval).Should(Succeed())
	})

	It("manager 1 does not process BDs in namespace 2 and vice versa", func() {
		bd1Name := uniqueName("cross-ns-bd1")
		bd2Name := uniqueName("cross-ns-bd2")

		bd1 := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: bd1Name, Namespace: ns1},
			Spec:       fleet.BundleDeploymentSpec{DeploymentID: "cross-dep-1"},
		}
		bd2 := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: bd2Name, Namespace: ns2},
			Spec:       fleet.BundleDeploymentSpec{DeploymentID: "cross-dep-2"},
		}

		Expect(k8sClient.Create(ctx, bd1)).To(Succeed())
		Expect(k8sClient.Create(ctx, bd2)).To(Succeed())

		// Start manager 1 only — it should NOT process bd2.
		stopManager1 = startManagerForNS(ns1, "sim-cross-1")

		Eventually(func(g Gomega) {
			var got1 fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd1Name, Namespace: ns1}, &got1)).To(Succeed())
			g.Expect(got1.Status.Ready).To(BeTrue())
		}, timeout, pollInterval).Should(Succeed())

		// bd2 should remain untouched because no manager watches ns2.
		Consistently(func(g Gomega) {
			var got2 fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd2Name, Namespace: ns2}, &got2)).To(Succeed())
			g.Expect(got2.Status.Ready).To(BeFalse())
		}, 2*time.Second, pollInterval).Should(Succeed())
	})

	It("each cluster processes a DeploymentID change independently", func() {
		bd1Name := uniqueName("mc-redeploy")
		bd2Name := uniqueName("mc-redeploy")

		bd1 := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: bd1Name, Namespace: ns1},
			Spec:       fleet.BundleDeploymentSpec{DeploymentID: "v1"},
		}
		bd2 := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: bd2Name, Namespace: ns2},
			Spec:       fleet.BundleDeploymentSpec{DeploymentID: "v1"},
		}

		Expect(k8sClient.Create(ctx, bd1)).To(Succeed())
		Expect(k8sClient.Create(ctx, bd2)).To(Succeed())

		stopManager1 = startManagerForNS(ns1, "sim-redeploy-1")
		stopManager2 = startManagerForNS(ns2, "sim-redeploy-2")

		// Wait for both to reach ready on v1.
		for _, item := range []struct {
			name, ns string
		}{{bd1Name, ns1}, {bd2Name, ns2}} {
			n, ns := item.name, item.ns
			Eventually(func(g Gomega) {
				var got fleet.BundleDeployment
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: n, Namespace: ns}, &got)).To(Succeed())
				g.Expect(got.Status.Ready).To(BeTrue())
				g.Expect(got.Status.AppliedDeploymentID).To(Equal("v1"))
			}, timeout, pollInterval).Should(Succeed())
		}

		// Update bd1 to v2 — bd2 should stay at v1.
		var got1 fleet.BundleDeployment
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd1Name, Namespace: ns1}, &got1)).To(Succeed())
		got1.Spec.DeploymentID = "v2"
		Expect(k8sClient.Update(ctx, &got1)).To(Succeed())

		Eventually(func(g Gomega) {
			var updated fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd1Name, Namespace: ns1}, &updated)).To(Succeed())
			g.Expect(updated.Status.AppliedDeploymentID).To(Equal("v2"))
		}, timeout, pollInterval).Should(Succeed())

		// bd2 must still be at v1.
		Consistently(func(g Gomega) {
			var got2 fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd2Name, Namespace: ns2}, &got2)).To(Succeed())
			g.Expect(got2.Status.AppliedDeploymentID).To(Equal("v1"))
		}, 2*time.Second, pollInterval).Should(Succeed())
	})
})

// uniqueName generates a unique resource name with a given prefix.
var uniqueCounter int

func uniqueName(prefix string) string {
	uniqueCounter++
	return fmt.Sprintf("%s-%d", prefix, uniqueCounter)
}
