package simulator_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/fleet/agent-simulator/pkg/simulator"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	clusterNS      = "cluster-sim-test"
	agentNS        = "cattle-fleet-system"
	resourceCount  = 5
	timeout        = 10 * time.Second
	pollInterval   = 200 * time.Millisecond
)

// startManager spins up a simulator manager in a background goroutine and
// returns a cancel func that stops it.
func startManager() context.CancelFunc {
	mgrCtx, mgrCancel := context.WithCancel(ctx)

	mgr, err := simulator.NewManager(restCfg, scheme, simulator.Options{
		ClusterNamespace:      "fleet-default", // registration ns – not used in these tests
		BDNamespace:           clusterNS,
		ClusterName:           "sim-test",
		AgentNamespace:        agentNS,
		ResourceCount:         resourceCount,
		HeartbeatInterval:     time.Hour, // large – we don't test heartbeat here
		HeartbeatInitialDelay: time.Hour,
		SkipNameValidation:    true, // allow multiple managers in the same test process
	})
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()

	return mgrCancel
}

// newBD creates a minimal BundleDeployment in the cluster namespace.
func newBD(name, deploymentID string) *fleet.BundleDeployment {
	return &fleet.BundleDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: clusterNS,
		},
		Spec: fleet.BundleDeploymentSpec{
			DeploymentID: deploymentID,
		},
	}
}

var _ = Describe("BundleDeploymentReconciler", func() {
	var stopManager context.CancelFunc

	BeforeEach(func() {
		stopManager = startManager()
	})

	AfterEach(func() {
		stopManager()
	})

	It("marks a new BundleDeployment as ready", func() {
		bd := newBD("bd-instant-ready", "deploy-001")
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeTrue())
			g.Expect(got.Status.AppliedDeploymentID).To(Equal("deploy-001"))
			g.Expect(got.Status.Resources).To(HaveLen(resourceCount))
			g.Expect(got.Status.NonReadyStatus).To(BeEmpty())
			g.Expect(got.Status.NonModified).To(BeTrue())
		}, timeout, pollInterval).Should(Succeed())
	})

	It("does not reprocess a BD whose DeploymentID is already applied", func() {
		bd := newBD("bd-already-applied", "deploy-002")
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// Wait for initial ready patch.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeTrue())
		}, timeout, pollInterval).Should(Succeed())

		// Record the release name so we can confirm it doesn't change.
		var got fleet.BundleDeployment
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
		firstRelease := got.Status.Release

		// Wait a bit and confirm status is stable (no spurious re-reconcile).
		Consistently(func(g Gomega) {
			var got2 fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got2)).To(Succeed())
			g.Expect(got2.Status.Release).To(Equal(firstRelease))
		}, 2*time.Second, pollInterval).Should(Succeed())
	})

	It("re-reconciles when DeploymentID changes", func() {
		bd := newBD("bd-rereconcile", "deploy-v1")
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		// Wait for initial ready.
		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.AppliedDeploymentID).To(Equal("deploy-v1"))
		}, timeout, pollInterval).Should(Succeed())

		// Update DeploymentID.
		var latest fleet.BundleDeployment
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &latest)).To(Succeed())
		latest.Spec.DeploymentID = "deploy-v2"
		Expect(k8sClient.Update(ctx, &latest)).To(Succeed())

		Eventually(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.AppliedDeploymentID).To(Equal("deploy-v2"))
		}, timeout, pollInterval).Should(Succeed())
	})

	It("skips Paused BundleDeployments", func() {
		bd := newBD("bd-paused", "deploy-003")
		bd.Spec.Paused = true
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, bd) })

		Consistently(func(g Gomega) {
			var got fleet.BundleDeployment
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: bd.Name, Namespace: clusterNS}, &got)).To(Succeed())
			g.Expect(got.Status.Ready).To(BeFalse())
			g.Expect(got.Status.AppliedDeploymentID).To(BeEmpty())
		}, 2*time.Second, pollInterval).Should(Succeed())
	})
})
