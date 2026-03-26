package config_test

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/fleet/agent-simulator/pkg/config"
)

var _ = Describe("ClusterConfig.Resolve", func() {
	var base config.Config

	BeforeEach(func() {
		base = config.Config{
			Kubeconfig:        "/etc/kube/config",
			ClusterNamespace:  "fleet-default",
			ClusterName:       "global-cluster",
			AgentNamespace:    "cattle-fleet-system",
			BDNamespace:       "cluster-fleet-default-global-abc",
			HeartbeatInterval: 30 * time.Second,
			InitialDelay:      10 * time.Second,
			ResourceCount:     20,
			RolloutSteps:      3,
			RolloutInterval:   15 * time.Second,
			Drift: config.DriftConfig{
				Enabled:       true,
				MinInterval:   60 * time.Second,
				MaxInterval:   300 * time.Second,
				ResourceCount: 2,
				AffectsReady:  true,
				AutoRecover:   true,
				RecoveryDelay: 30 * time.Second,
			},
			Failure: config.FailureConfig{
				Enabled:       true,
				MinInterval:   120 * time.Second,
				MaxInterval:   600 * time.Second,
				ResourceCount: 3,
				Probability:   0.2,
				AutoRecover:   true,
				RecoveryDelay: 60 * time.Second,
			},
			Clusters: []config.ClusterConfig{
				{ClusterName: "should-be-cleared"},
			},
		}
	})

	It("inherits all global defaults when all cluster fields are zero", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
		}
		resolved := cc.Resolve(base)

		Expect(resolved.ClusterNamespace).To(Equal("fleet-default"))
		Expect(resolved.ClusterName).To(Equal("my-cluster"))
		Expect(resolved.AgentNamespace).To(Equal(base.AgentNamespace))
		Expect(resolved.BDNamespace).To(Equal(base.BDNamespace))
		Expect(resolved.HeartbeatInterval).To(Equal(base.HeartbeatInterval))
		Expect(resolved.InitialDelay).To(Equal(base.InitialDelay))
		Expect(resolved.ResourceCount).To(Equal(base.ResourceCount))
		Expect(resolved.RolloutSteps).To(Equal(base.RolloutSteps))
		Expect(resolved.RolloutInterval).To(Equal(base.RolloutInterval))
		Expect(resolved.Drift).To(Equal(base.Drift))
		Expect(resolved.Failure).To(Equal(base.Failure))
	})

	It("overrides AgentNamespace when set", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			AgentNamespace:   "custom-agent-ns",
		}
		resolved := cc.Resolve(base)
		Expect(resolved.AgentNamespace).To(Equal("custom-agent-ns"))
	})

	It("overrides BDNamespace when set", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			BDNamespace:      "cluster-fleet-default-my-cluster-xyz",
		}
		resolved := cc.Resolve(base)
		Expect(resolved.BDNamespace).To(Equal("cluster-fleet-default-my-cluster-xyz"))
	})

	It("overrides HeartbeatInterval when non-zero", func() {
		cc := config.ClusterConfig{
			ClusterNamespace:  "fleet-default",
			ClusterName:       "my-cluster",
			HeartbeatInterval: 5 * time.Second,
		}
		resolved := cc.Resolve(base)
		Expect(resolved.HeartbeatInterval).To(Equal(5 * time.Second))
	})

	It("overrides InitialDelay when non-zero", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			InitialDelay:     2 * time.Second,
		}
		resolved := cc.Resolve(base)
		Expect(resolved.InitialDelay).To(Equal(2 * time.Second))
	})

	It("overrides ResourceCount when non-zero", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			ResourceCount:    50,
		}
		resolved := cc.Resolve(base)
		Expect(resolved.ResourceCount).To(Equal(50))
	})

	It("overrides RolloutSteps when non-zero", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			RolloutSteps:     7,
		}
		resolved := cc.Resolve(base)
		Expect(resolved.RolloutSteps).To(Equal(7))
	})

	It("overrides RolloutInterval when non-zero", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			RolloutInterval:  3 * time.Second,
		}
		resolved := cc.Resolve(base)
		Expect(resolved.RolloutInterval).To(Equal(3 * time.Second))
	})

	It("overrides Drift when non-nil", func() {
		customDrift := &config.DriftConfig{
			Enabled:       false,
			MinInterval:   10 * time.Second,
			MaxInterval:   20 * time.Second,
			ResourceCount: 5,
			AffectsReady:  false,
			AutoRecover:   false,
			RecoveryDelay: 5 * time.Second,
		}
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			Drift:            customDrift,
		}
		resolved := cc.Resolve(base)
		Expect(resolved.Drift).To(Equal(*customDrift))
	})

	It("overrides Failure when non-nil", func() {
		customFailure := &config.FailureConfig{
			Enabled:       false,
			MinInterval:   10 * time.Second,
			MaxInterval:   30 * time.Second,
			ResourceCount: 1,
			Probability:   0.05,
			AutoRecover:   false,
			RecoveryDelay: 15 * time.Second,
		}
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
			Failure:          customFailure,
		}
		resolved := cc.Resolve(base)
		Expect(resolved.Failure).To(Equal(*customFailure))
	})

	It("clears Clusters in the result to prevent recursion", func() {
		cc := config.ClusterConfig{
			ClusterNamespace: "fleet-default",
			ClusterName:      "my-cluster",
		}
		resolved := cc.Resolve(base)
		Expect(resolved.Clusters).To(BeNil())
	})
})

var _ = Describe("Config.Validate multi-cluster mode", func() {
	var base config.Config

	BeforeEach(func() {
		base = config.Config{
			AgentNamespace: "cattle-fleet-system",
			BDNamespace:    "cluster-fleet-default-global-abc",
		}
	})

	It("passes when all clusters have valid resolved configs using global agentNamespace", func() {
		cfg := base
		cfg.Clusters = []config.ClusterConfig{
			{
				ClusterNamespace: "fleet-default",
				ClusterName:      "cluster-a",
			},
			{
				ClusterNamespace: "fleet-default",
				ClusterName:      "cluster-b",
				AgentNamespace:   "custom-agent-ns",
			},
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	It("fails when a cluster is missing clusterNamespace", func() {
		cfg := base
		cfg.Clusters = []config.ClusterConfig{
			{
				ClusterName: "cluster-a",
			},
		}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("clusterNamespace is required")))
	})

	It("fails when a cluster is missing clusterName", func() {
		cfg := base
		cfg.Clusters = []config.ClusterConfig{
			{
				ClusterNamespace: "fleet-default",
			},
		}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("clusterName is required")))
	})

	It("fails when a cluster is missing agentNamespace and no global default is set", func() {
		cfg := config.Config{
			BDNamespace: "cluster-fleet-default-global-abc",
			Clusters: []config.ClusterConfig{
				{
					ClusterNamespace: "fleet-default",
					ClusterName:      "cluster-a",
				},
			},
		}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("agentNamespace is required")))
	})

	It("fails when a cluster is missing bdNamespace and no global default is set", func() {
		cfg := config.Config{
			AgentNamespace: "cattle-fleet-system",
			Clusters: []config.ClusterConfig{
				{
					ClusterNamespace: "fleet-default",
					ClusterName:      "cluster-a",
				},
			},
		}
		Expect(cfg.Validate()).To(MatchError(ContainSubstring("bdNamespace is required")))
	})

	It("includes the cluster name in the error message", func() {
		cfg := base
		cfg.Clusters = []config.ClusterConfig{
			{
				ClusterNamespace: "fleet-default",
				ClusterName:      "bad-cluster",
				// AgentNamespace intentionally left empty, override global
			},
		}
		// Remove global agentNamespace so validation fails for this cluster
		cfg.AgentNamespace = ""
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("bad-cluster"))
	})

	It("includes index in the error message when cluster has no name", func() {
		cfg := config.Config{
			Clusters: []config.ClusterConfig{
				{
					ClusterNamespace: "fleet-default",
					// no ClusterName, so validateSingle will fail on clusterName
				},
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("index %d", 0)))
	})
})

var _ = Describe("Config.Load multi-cluster mode", func() {
	It("loads multi-cluster config with global defaults and per-cluster overrides", func() {
		f := writeTemp(`
agentNamespace: cattle-fleet-system
bdNamespace: cluster-fleet-default-global-abc
heartbeatInterval: 30s
initialDelay: 10s
resourceCount: 15
rolloutSteps: 2
rolloutInterval: 8s
clusters:
  - clusterNamespace: fleet-default
    clusterName: cluster-a
  - clusterNamespace: fleet-local
    clusterName: cluster-b
    agentNamespace: custom-ns
    heartbeatInterval: 5s
    resourceCount: 50
`)
		defer os.Remove(f)

		cfg, err := config.Load(f)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.Clusters).To(HaveLen(2))

		clusterA := cfg.Clusters[0]
		Expect(clusterA.ClusterNamespace).To(Equal("fleet-default"))
		Expect(clusterA.ClusterName).To(Equal("cluster-a"))
		// cluster-a does not override agentNamespace — inherits global via Resolve
		resolvedA := clusterA.Resolve(*cfg)
		Expect(resolvedA.AgentNamespace).To(Equal("cattle-fleet-system"))
		Expect(resolvedA.HeartbeatInterval).To(Equal(30 * time.Second))
		Expect(resolvedA.ResourceCount).To(Equal(15))
		Expect(resolvedA.RolloutSteps).To(Equal(2))
		Expect(resolvedA.RolloutInterval).To(Equal(8 * time.Second))

		clusterB := cfg.Clusters[1]
		Expect(clusterB.ClusterNamespace).To(Equal("fleet-local"))
		Expect(clusterB.ClusterName).To(Equal("cluster-b"))
		resolvedB := clusterB.Resolve(*cfg)
		Expect(resolvedB.AgentNamespace).To(Equal("custom-ns"))
		Expect(resolvedB.HeartbeatInterval).To(Equal(5 * time.Second))
		Expect(resolvedB.ResourceCount).To(Equal(50))
		// Fields not overridden in cluster-b fall back to global
		Expect(resolvedB.RolloutSteps).To(Equal(2))
		Expect(resolvedB.RolloutInterval).To(Equal(8 * time.Second))
	})

	It("applies default heartbeatInterval (20s) when not set in a cluster's resolved config", func() {
		f := writeTemp(`
agentNamespace: cattle-fleet-system
bdNamespace: cluster-fleet-default-global-abc
clusters:
  - clusterNamespace: fleet-default
    clusterName: cluster-a
`)
		defer os.Remove(f)

		cfg, err := config.Load(f)
		Expect(err).NotTo(HaveOccurred())

		// The global heartbeatInterval is not set in YAML, so applyDefaults sets it to 20s.
		// Resolve inherits it.
		Expect(cfg.HeartbeatInterval).To(Equal(20 * time.Second))

		resolved := cfg.Clusters[0].Resolve(*cfg)
		Expect(resolved.HeartbeatInterval).To(Equal(20 * time.Second))
	})
})
