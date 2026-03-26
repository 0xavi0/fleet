// Package config provides configuration loading for the Fleet agent simulator.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the simulator configuration.
// In multi-cluster mode the top-level fields (except Clusters) act as global
// defaults; each ClusterConfig in Clusters can override any of them.
type Config struct {
	Kubeconfig        string        `yaml:"kubeconfig"`
	ClusterNamespace  string        `yaml:"clusterNamespace"`
	ClusterName       string        `yaml:"clusterName"`
	AgentNamespace    string        `yaml:"agentNamespace"`
	// BDNamespace is the cluster namespace where BundleDeployments live (e.g. cluster-fleet-default-sim-xyz).
	BDNamespace       string        `yaml:"bdNamespace"`
	HeartbeatInterval time.Duration `yaml:"heartbeatInterval"`
	InitialDelay      time.Duration `yaml:"initialDelay"`
	// ResourceCount is the number of fake resources reported per BundleDeployment.
	ResourceCount int `yaml:"resourceCount"`
	// RolloutSteps is the number of incremental status updates before a BD is fully ready.
	// 1 means instant-ready (Phase 2 behaviour). Default 1.
	RolloutSteps int `yaml:"rolloutSteps"`
	// RolloutInterval is the delay between rollout steps. Default 5s.
	RolloutInterval time.Duration `yaml:"rolloutInterval"`
	// Drift configures optional drift simulation (Phase 4).
	Drift DriftConfig `yaml:"drift"`
	// Failure configures optional random failure simulation (Phase 5).
	Failure FailureConfig `yaml:"failure"`
	// Clusters is the list of simulated clusters for multi-cluster mode (Phase 6).
	// When non-empty, the top-level fields above serve as global defaults that
	// each cluster entry can override. When empty, single-cluster mode is used
	// with ClusterNamespace/ClusterName/AgentNamespace/BDNamespace directly.
	Clusters []ClusterConfig `yaml:"clusters"`
}

// ClusterConfig holds per-cluster settings for multi-cluster mode.
// Any zero/nil value means "inherit from the top-level Config defaults".
type ClusterConfig struct {
	// ClusterNamespace is required for each cluster entry.
	ClusterNamespace string `yaml:"clusterNamespace"`
	// ClusterName is required for each cluster entry.
	ClusterName string `yaml:"clusterName"`
	// Kubeconfig overrides the global kubeconfig path when non-empty.
	// Use this when each simulated cluster has its own ServiceAccount and kubeconfig
	// (the typical output of create-cluster), so that each manager authenticates
	// with the correct credentials for its cluster namespace.
	Kubeconfig string `yaml:"kubeconfig"`
	// AgentNamespace overrides the global default when non-empty.
	AgentNamespace string `yaml:"agentNamespace"`
	// BDNamespace overrides the global default when non-empty.
	BDNamespace string `yaml:"bdNamespace"`
	// HeartbeatInterval overrides the global default when non-zero.
	HeartbeatInterval time.Duration `yaml:"heartbeatInterval"`
	// InitialDelay overrides the global default when non-zero.
	InitialDelay time.Duration `yaml:"initialDelay"`
	// ResourceCount overrides the global default when non-zero.
	ResourceCount int `yaml:"resourceCount"`
	// RolloutSteps overrides the global default when non-zero.
	RolloutSteps int `yaml:"rolloutSteps"`
	// RolloutInterval overrides the global default when non-zero.
	RolloutInterval time.Duration `yaml:"rolloutInterval"`
	// Drift overrides the global default when non-nil.
	Drift *DriftConfig `yaml:"drift"`
	// Failure overrides the global default when non-nil.
	Failure *FailureConfig `yaml:"failure"`
}

// Resolve merges this ClusterConfig with the global defaults in base and returns
// a fully-populated single-cluster Config for that cluster. The Clusters field
// of the result is always nil to prevent accidental recursion.
func (cc ClusterConfig) Resolve(base Config) Config {
	result := base
	result.Clusters = nil

	result.ClusterNamespace = cc.ClusterNamespace
	result.ClusterName = cc.ClusterName

	if cc.Kubeconfig != "" {
		result.Kubeconfig = cc.Kubeconfig
	}
	if cc.AgentNamespace != "" {
		result.AgentNamespace = cc.AgentNamespace
	}
	if cc.BDNamespace != "" {
		result.BDNamespace = cc.BDNamespace
	}
	if cc.HeartbeatInterval != 0 {
		result.HeartbeatInterval = cc.HeartbeatInterval
	}
	if cc.InitialDelay != 0 {
		result.InitialDelay = cc.InitialDelay
	}
	if cc.ResourceCount != 0 {
		result.ResourceCount = cc.ResourceCount
	}
	if cc.RolloutSteps != 0 {
		result.RolloutSteps = cc.RolloutSteps
	}
	if cc.RolloutInterval != 0 {
		result.RolloutInterval = cc.RolloutInterval
	}
	if cc.Drift != nil {
		result.Drift = *cc.Drift
	}
	if cc.Failure != nil {
		result.Failure = *cc.Failure
	}
	return result
}

// FailureConfig controls the random failure simulation.
type FailureConfig struct {
	// Enabled activates the failure scheduler. Default false.
	Enabled bool `yaml:"enabled"`
	// MinInterval is the minimum time between global failure ticks. Default 120s.
	MinInterval time.Duration `yaml:"minInterval"`
	// MaxInterval is the maximum time between global failure ticks. Default 600s.
	MaxInterval time.Duration `yaml:"maxInterval"`
	// ResourceCount is the number of resources to report as failed per event. Default 1.
	ResourceCount int `yaml:"resourceCount"`
	// Probability is the per-BD probability of being selected on each tick (0.0–1.0). Default 0.1.
	Probability float64 `yaml:"probability"`
	// AutoRecover controls whether failed BDs are automatically recovered. Default true.
	AutoRecover bool `yaml:"autoRecover"`
	// RecoveryDelay is the delay between a failure event and auto-recovery. Default 60s.
	RecoveryDelay time.Duration `yaml:"recoveryDelay"`
}

// DriftConfig controls the periodic drift simulation.
type DriftConfig struct {
	// Enabled activates the drift scheduler. Default false.
	Enabled bool `yaml:"enabled"`
	// MinInterval is the minimum time between drift events per BD. Default 60s.
	MinInterval time.Duration `yaml:"minInterval"`
	// MaxInterval is the maximum time between drift events per BD. Default 300s.
	MaxInterval time.Duration `yaml:"maxInterval"`
	// ResourceCount is the number of resources to report as drifted per event. Default 1.
	ResourceCount int `yaml:"resourceCount"`
	// AffectsReady controls whether a drift event also sets Ready=false. Default false.
	AffectsReady bool `yaml:"affectsReady"`
	// AutoRecover controls whether drifted BDs are automatically recovered. Default false.
	AutoRecover bool `yaml:"autoRecover"`
	// RecoveryDelay is the delay between a drift event and auto-recovery. Default 30s.
	RecoveryDelay time.Duration `yaml:"recoveryDelay"`
}

// Load reads a YAML config file from path and applies defaults.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.applyDefaults()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 20 * time.Second
	}
	if c.InitialDelay == 0 {
		c.InitialDelay = 5 * time.Second
	}
	if c.ResourceCount == 0 {
		c.ResourceCount = 10
	}
	if c.RolloutSteps == 0 {
		c.RolloutSteps = 1
	}
	if c.RolloutInterval == 0 {
		c.RolloutInterval = 5 * time.Second
	}
	if c.Drift.MinInterval == 0 {
		c.Drift.MinInterval = 60 * time.Second
	}
	if c.Drift.MaxInterval == 0 {
		c.Drift.MaxInterval = 300 * time.Second
	}
	if c.Drift.ResourceCount == 0 {
		c.Drift.ResourceCount = 1
	}
	if c.Drift.RecoveryDelay == 0 {
		c.Drift.RecoveryDelay = 30 * time.Second
	}
	if c.Failure.MinInterval == 0 {
		c.Failure.MinInterval = 120 * time.Second
	}
	if c.Failure.MaxInterval == 0 {
		c.Failure.MaxInterval = 600 * time.Second
	}
	if c.Failure.ResourceCount == 0 {
		c.Failure.ResourceCount = 1
	}
	if c.Failure.Probability == 0 {
		c.Failure.Probability = 0.1
	}
	if c.Failure.RecoveryDelay == 0 {
		c.Failure.RecoveryDelay = 60 * time.Second
	}
}

// Validate returns an error if any required field is missing.
// In multi-cluster mode each cluster entry is validated against the resolved
// (merged) configuration, so global defaults can satisfy per-cluster requirements.
func (c *Config) Validate() error {
	if len(c.Clusters) > 0 {
		for i, cc := range c.Clusters {
			resolved := cc.Resolve(*c)
			if err := resolved.validateSingle(); err != nil {
				name := cc.ClusterName
				if name == "" {
					name = fmt.Sprintf("index %d", i)
				}
				return fmt.Errorf("cluster %s: %w", name, err)
			}
		}
		return nil
	}
	return c.validateSingle()
}

// validateSingle checks required fields for a single-cluster config.
func (c *Config) validateSingle() error {
	if c.ClusterNamespace == "" {
		return fmt.Errorf("clusterNamespace is required")
	}
	if c.ClusterName == "" {
		return fmt.Errorf("clusterName is required")
	}
	if c.AgentNamespace == "" {
		return fmt.Errorf("agentNamespace is required")
	}
	if c.BDNamespace == "" {
		return fmt.Errorf("bdNamespace is required")
	}
	return nil
}
