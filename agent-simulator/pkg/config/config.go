// Package config provides configuration loading for the Fleet agent simulator.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the simulator configuration.
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
func (c *Config) Validate() error {
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
