package resources_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/fleet/agent-simulator/pkg/resources"
)

func TestResources(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resources Suite")
}

var _ = Describe("Generate", func() {
	It("returns the requested count", func() {
		res := resources.Generate("my-bd", "default", 10)
		Expect(res).To(HaveLen(10))
	})

	It("returns empty slice for count=0", func() {
		res := resources.Generate("my-bd", "default", 0)
		Expect(res).To(BeEmpty())
	})

	It("is deterministic: same inputs produce same output", func() {
		a := resources.Generate("bd-alpha", "ns-a", 7)
		b := resources.Generate("bd-alpha", "ns-a", 7)
		Expect(len(a)).To(Equal(len(b)))
		for i := range a {
			Expect(a[i].Name).To(Equal(b[i].Name))
			Expect(a[i].Kind).To(Equal(b[i].Kind))
			Expect(a[i].APIVersion).To(Equal(b[i].APIVersion))
		}
	})

	It("cycles through Deployment, Service, ConfigMap, ServiceAccount", func() {
		res := resources.Generate("bd", "ns", 4)
		Expect(res[0].Kind).To(Equal("Deployment"))
		Expect(res[1].Kind).To(Equal("Service"))
		Expect(res[2].Kind).To(Equal("ConfigMap"))
		Expect(res[3].Kind).To(Equal("ServiceAccount"))
	})

	It("includes more Deployments when count > 4", func() {
		res := resources.Generate("bd", "ns", 8)
		deployCount := 0
		for _, r := range res {
			if r.Kind == "Deployment" {
				deployCount++
			}
		}
		Expect(deployCount).To(Equal(2))
	})

	It("sets the target namespace on every resource", func() {
		res := resources.Generate("bd", "my-ns", 5)
		for _, r := range res {
			Expect(r.Namespace).To(Equal("my-ns"))
		}
	})

	It("generates unique names within the same BD", func() {
		res := resources.Generate("bd", "ns", 8)
		seen := map[string]bool{}
		for _, r := range res {
			Expect(seen[r.Name]).To(BeFalse(), "duplicate name: %s", r.Name)
			seen[r.Name] = true
		}
	})

	It("uses the bd name as a prefix in resource names", func() {
		res := resources.Generate("my-bundle", "ns", 4)
		for _, r := range res {
			Expect(r.Name).To(HavePrefix("my-bundle-"))
		}
	})
})
