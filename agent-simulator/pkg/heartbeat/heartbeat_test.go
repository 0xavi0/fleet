package heartbeat_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"github.com/rancher/fleet/agent-simulator/pkg/heartbeat"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	clusterNS   = "cluster-fleet-sim-test"
	clusterName = "sim-cluster"
	agentNS     = "cattle-fleet-system"
)

var _ = Describe("Heartbeat", func() {
	var clusterKey types.NamespacedName

	BeforeEach(func() {
		clusterKey = types.NamespacedName{Namespace: clusterNS, Name: clusterName}

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterNS}}
		_ = k8sClient.Create(ctx, ns) // idempotent; ignore already-exists

		cluster := &fleet.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: clusterNS,
			},
		}
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		// Initialize the status subresource so that the JSONPatch op:add
		// on /status/agent has a valid /status parent to work with.
		Expect(k8sClient.Status().Update(ctx, cluster)).To(Succeed())
	})

	AfterEach(func() {
		cluster := &fleet.Cluster{}
		if err := k8sClient.Get(ctx, clusterKey, cluster); err == nil {
			Expect(k8sClient.Delete(ctx, cluster)).To(Succeed())
		}
	})

	Describe("Patch", func() {
		It("updates Cluster.Status.Agent.LastSeen", func() {
			before := time.Now().UTC().Truncate(time.Second)

			err := heartbeat.Patch(ctx, k8sClient, clusterNS, clusterName, agentNS)
			Expect(err).NotTo(HaveOccurred())

			cluster := &fleet.Cluster{}
			Expect(k8sClient.Get(ctx, clusterKey, cluster)).To(Succeed())
			Expect(cluster.Status.Agent.LastSeen.Time).To(BeTemporally(">=", before))
			Expect(cluster.Status.Agent.Namespace).To(Equal(agentNS))
		})

		It("updates LastSeen on successive calls", func() {
			Expect(heartbeat.Patch(ctx, k8sClient, clusterNS, clusterName, agentNS)).To(Succeed())
			cluster := &fleet.Cluster{}
			Expect(k8sClient.Get(ctx, clusterKey, cluster)).To(Succeed())
			first := cluster.Status.Agent.LastSeen.Time

			// Ensure at least one second passes so RFC3339 timestamps differ.
			time.Sleep(1100 * time.Millisecond)

			Expect(heartbeat.Patch(ctx, k8sClient, clusterNS, clusterName, agentNS)).To(Succeed())
			Expect(k8sClient.Get(ctx, clusterKey, cluster)).To(Succeed())
			second := cluster.Status.Agent.LastSeen.Time

			Expect(second).To(BeTemporally(">", first))
		})
	})

	Describe("Ticker", func() {
		It("sends the initial heartbeat after initialDelay and then periodically", func() {
			tickerCtx, tickerCancel := context.WithCancel(ctx)
			defer tickerCancel()

			initialDelay := 50 * time.Millisecond
			interval := 200 * time.Millisecond

			heartbeat.Ticker(tickerCtx, k8sClient, clusterNS, clusterName, agentNS, initialDelay, interval)

			// Wait for the initial heartbeat.
			cluster := &fleet.Cluster{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, clusterKey, cluster)).To(Succeed())
				g.Expect(cluster.Status.Agent.LastSeen.IsZero()).To(BeFalse())
			}, 2*time.Second, 50*time.Millisecond).Should(Succeed())

			first := cluster.Status.Agent.LastSeen.Time

			// Wait for a periodic update (at least one full second to ensure
			// the RFC3339 timestamp advances).
			time.Sleep(1200 * time.Millisecond)

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, clusterKey, cluster)).To(Succeed())
				g.Expect(cluster.Status.Agent.LastSeen.Time).To(BeTemporally(">", first))
			}, 3*time.Second, 50*time.Millisecond).Should(Succeed())
		})
	})
})
