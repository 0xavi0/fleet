package config_test

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/fleet/agent-simulator/pkg/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config", func() {
	Describe("Load", func() {
		It("loads a valid config file", func() {
			f := writeTemp(`
clusterNamespace: fleet-default
clusterName: sim-cluster
agentNamespace: cattle-fleet-system
bdNamespace: cluster-fleet-default-sim-abc123
heartbeatInterval: 30s
initialDelay: 10s
`)
			defer os.Remove(f)

			cfg, err := config.Load(f)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.ClusterNamespace).To(Equal("fleet-default"))
			Expect(cfg.ClusterName).To(Equal("sim-cluster"))
			Expect(cfg.AgentNamespace).To(Equal("cattle-fleet-system"))
			Expect(cfg.BDNamespace).To(Equal("cluster-fleet-default-sim-abc123"))
			Expect(cfg.HeartbeatInterval).To(Equal(30 * time.Second))
			Expect(cfg.InitialDelay).To(Equal(10 * time.Second))
		})

		It("applies defaults for heartbeatInterval, initialDelay, and resourceCount", func() {
			f := writeTemp(`
clusterNamespace: ns
clusterName: cl
agentNamespace: sys
`)
			defer os.Remove(f)

			cfg, err := config.Load(f)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.HeartbeatInterval).To(Equal(20 * time.Second))
			Expect(cfg.InitialDelay).To(Equal(5 * time.Second))
			Expect(cfg.ResourceCount).To(Equal(10))
		})

		It("returns an error for a missing file", func() {
			_, err := config.Load("/does/not/exist.yaml")
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for invalid YAML", func() {
			f := writeTemp("not: valid: yaml: [")
			defer os.Remove(f)

			_, err := config.Load(f)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Validate", func() {
		It("passes for a fully populated config", func() {
			cfg := &config.Config{
				ClusterNamespace: "fleet-default",
				ClusterName:      "cl",
				AgentNamespace:   "sys",
				BDNamespace:      "cluster-fleet-default-cl-xyz",
			}
			Expect(cfg.Validate()).To(Succeed())
		})

		DescribeTable("fails when a required field is missing",
			func(mutate func(*config.Config)) {
				cfg := &config.Config{
					ClusterNamespace: "fleet-default",
					ClusterName:      "cl",
					AgentNamespace:   "sys",
					BDNamespace:      "cluster-fleet-default-cl-xyz",
				}
				mutate(cfg)
				Expect(cfg.Validate()).To(HaveOccurred())
			},
			Entry("missing clusterNamespace", func(c *config.Config) { c.ClusterNamespace = "" }),
			Entry("missing clusterName", func(c *config.Config) { c.ClusterName = "" }),
			Entry("missing agentNamespace", func(c *config.Config) { c.AgentNamespace = "" }),
			Entry("missing bdNamespace", func(c *config.Config) { c.BDNamespace = "" }),
		)
	})
})

func writeTemp(content string) string {
	f, err := os.CreateTemp("", "sim-config-*.yaml")
	Expect(err).NotTo(HaveOccurred())
	_, err = f.WriteString(content)
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Close()).To(Succeed())
	return f.Name()
}
