package status_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/rancher/fleet/agent-simulator/pkg/status"
)

func TestStatus(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status Suite")
}

var _ = Describe("Build", func() {
	baseParams := func() status.BuildParams {
		return status.BuildParams{
			DeploymentID:   "deploy-abc123",
			ResourceCount:  5,
			ReadyCount:     5,
			AgentNamespace: "cattle-fleet-system",
			Namespace:      "default",
			BDName:         "my-bundle",
		}
	}

	It("sets AppliedDeploymentID from DeploymentID", func() {
		s := status.Build(baseParams())
		Expect(s.AppliedDeploymentID).To(Equal("deploy-abc123"))
	})

	It("is ready when ReadyCount == ResourceCount", func() {
		s := status.Build(baseParams())
		Expect(s.Ready).To(BeTrue())
		Expect(s.NonReadyStatus).To(BeEmpty())
	})

	It("is not ready when ReadyCount < ResourceCount", func() {
		p := baseParams()
		p.ReadyCount = 2
		s := status.Build(p)
		Expect(s.Ready).To(BeFalse())
		Expect(s.NonReadyStatus).To(HaveLen(3))
	})

	It("has NonModified=true", func() {
		s := status.Build(baseParams())
		Expect(s.NonModified).To(BeTrue())
	})

	It("generates resource list with correct count", func() {
		s := status.Build(baseParams())
		Expect(s.Resources).To(HaveLen(5))
	})

	It("sets ResourceCounts correctly when fully ready", func() {
		s := status.Build(baseParams())
		Expect(s.ResourceCounts.Ready).To(Equal(5))
		Expect(s.ResourceCounts.DesiredReady).To(Equal(5))
		Expect(s.ResourceCounts.NotReady).To(Equal(0))
	})

	It("sets ResourceCounts correctly when partially ready", func() {
		p := baseParams()
		p.ReadyCount = 3
		s := status.Build(p)
		Expect(s.ResourceCounts.Ready).To(Equal(3))
		Expect(s.ResourceCounts.DesiredReady).To(Equal(5))
		Expect(s.ResourceCounts.NotReady).To(Equal(2))
	})

	It("has four conditions: Deployed, Installed, Monitored, Ready", func() {
		s := status.Build(baseParams())
		Expect(s.Conditions).To(HaveLen(4))
		types := make([]string, 0, 4)
		for _, c := range s.Conditions {
			types = append(types, c.Type)
		}
		Expect(types).To(ConsistOf("Deployed", "Installed", "Monitored", "Ready"))
	})

	It("sets Ready condition to True when fully ready", func() {
		s := status.Build(baseParams())
		for _, c := range s.Conditions {
			if c.Type == "Ready" {
				Expect(c.Status).To(Equal(corev1.ConditionTrue))
				return
			}
		}
		Fail("Ready condition not found")
	})

	It("sets Ready condition to False when not fully ready", func() {
		p := baseParams()
		p.ReadyCount = 1
		s := status.Build(p)
		for _, c := range s.Conditions {
			if c.Type == "Ready" {
				Expect(c.Status).To(Equal(corev1.ConditionFalse))
				return
			}
		}
		Fail("Ready condition not found")
	})

	It("sets Display.State to Ready when fully ready", func() {
		s := status.Build(baseParams())
		Expect(s.Display.State).To(Equal("Ready"))
	})

	It("sets Display.State to WaitApplied when not fully ready", func() {
		p := baseParams()
		p.ReadyCount = 0
		s := status.Build(p)
		Expect(s.Display.State).To(Equal("WaitApplied"))
	})

	It("generates a deterministic Release name from the BD name", func() {
		p := baseParams()
		s1 := status.Build(p)
		s2 := status.Build(p)
		Expect(s1.Release).To(Equal(s2.Release))
		Expect(s1.Release).To(HavePrefix("cattle-fleet-system/s-"))
	})

	It("generates different Release names for different BD names", func() {
		p1 := baseParams()
		p1.BDName = "bundle-a"
		p2 := baseParams()
		p2.BDName = "bundle-b"
		Expect(status.Build(p1).Release).NotTo(Equal(status.Build(p2).Release))
	})

	It("non-ready resources have WaitApplied summary state", func() {
		p := baseParams()
		p.ReadyCount = 2
		s := status.Build(p)
		for _, nr := range s.NonReadyStatus {
			Expect(nr.Summary.State).To(Equal("WaitApplied"))
			Expect(nr.Summary.Transitioning).To(BeTrue())
		}
	})

	It("handles ResourceCount=0 gracefully", func() {
		p := baseParams()
		p.ResourceCount = 0
		p.ReadyCount = 0
		s := status.Build(p)
		Expect(s.Ready).To(BeTrue())
		Expect(s.Resources).To(BeEmpty())
		Expect(s.NonReadyStatus).To(BeEmpty())
	})
})
