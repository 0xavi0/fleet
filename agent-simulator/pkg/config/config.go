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
