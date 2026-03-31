package simulator_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"github.com/rancher/fleet/agent-simulator/pkg/simulator"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("DryRun", func() {
	const (
		dryRunNS      = "cluster-sim-dryrun"
		dryRunCluster = "dry-run-cluster"
	)

	BeforeEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: dryRunNS}}
		_ = k8sClient.Create(ctx, ns) // idempotent
	})

	It("does not patch BundleDeployment status when DryRun=true", func() {
		opts := simulator.Options{
			ClusterNamespace:      "fleet-default",
			BDNamespace:           dryRunNS,
			ClusterName:           dryRunCluster,
			AgentNamespace:        "cattle-fleet-system",
			ResourceCount:         3,
			HeartbeatInterval:     100 * time.Millisecond,
			HeartbeatInitialDelay: 10 * time.Millisecond,
			RolloutSteps:          1,
			SkipNameValidation:    true,
			DryRun:                true,
		}

		mgr, err := simulator.NewManager(restCfg, scheme, opts)
		Expect(err).NotTo(HaveOccurred())

		mgrCtx, mgrCancel := context.WithCancel(ctx)
		defer mgrCancel()

		go func() { _ = mgr.Start(mgrCtx) }()

		// Wait for the manager's cache to sync before creating the BD.
		// A short sleep is sufficient since envtest is local.
		time.Sleep(100 * time.Millisecond)

		bd := &fleet.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dry-run-bd",
				Namespace: dryRunNS,
			},
			Spec: fleet.BundleDeploymentSpec{
				DeploymentID: "dry-run-deploy-1",
			},
		}
		Expect(k8sClient.Create(ctx, bd)).To(Succeed())

		// With DryRun=true the reconciler must NOT patch the status.
		// Verify with Consistently over 600 ms.
		Consistently(func(g Gomega) {
			current := &fleet.BundleDeployment{}
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(bd), current)).To(Succeed())
			g.Expect(current.Status.AppliedDeploymentID).To(BeEmpty(),
				"dry-run reconciler must not write AppliedDeploymentID")
		}, 600*time.Millisecond, 50*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Delete(ctx, bd)).To(Succeed())
	})
})
