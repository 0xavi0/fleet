// Command simulator runs the Fleet agent simulator against an upstream management cluster.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"github.com/rancher/fleet/agent-simulator/pkg/chaos"
	"github.com/rancher/fleet/agent-simulator/pkg/config"
	"github.com/rancher/fleet/agent-simulator/pkg/simulator"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(fleet.AddToScheme(scheme))
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath string
		kubeconfig string
	)

	flag.StringVar(&configPath, "config", "config.yaml", "Path to simulator config YAML")
	// --kubeconfig may already be registered by controller-runtime's init; guard against the panic.
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig (overrides KUBECONFIG env and config file)")
	}
	flag.Parse()

	// If we didn't register the flag ourselves, read whatever value was set.
	if kubeconfig == "" {
		if f := flag.Lookup("kubeconfig"); f != nil {
			kubeconfig = f.Value.String()
		}
	}
	// Fall back to KUBECONFIG env.
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	logf.SetLogger(zap.New())
	logger := logf.Log.WithName("simulator")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if kubeconfig != "" {
		cfg.Kubeconfig = kubeconfig
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("Starting simulator",
		"cluster", cfg.ClusterName,
		"clusterNamespace", cfg.ClusterNamespace,
		"heartbeatInterval", cfg.HeartbeatInterval,
		"resourceCount", cfg.ResourceCount,
	)

	mgr, err := simulator.NewManager(restCfg, scheme, simulator.Options{
		ClusterNamespace:      cfg.ClusterNamespace,
		BDNamespace:           cfg.BDNamespace,
		ClusterName:           cfg.ClusterName,
		AgentNamespace:        cfg.AgentNamespace,
		ResourceCount:         cfg.ResourceCount,
		HeartbeatInterval:     cfg.HeartbeatInterval,
		HeartbeatInitialDelay: cfg.InitialDelay,
		RolloutSteps:          cfg.RolloutSteps,
		RolloutInterval:       cfg.RolloutInterval,
		DriftEnabled: cfg.Drift.Enabled,
		Drift: chaos.DriftOptions{
			AffectedResourceCount: cfg.Drift.ResourceCount,
			MinInterval:           cfg.Drift.MinInterval,
			MaxInterval:           cfg.Drift.MaxInterval,
			AffectsReady:          cfg.Drift.AffectsReady,
			AutoRecover:           cfg.Drift.AutoRecover,
			RecoveryDelay:         cfg.Drift.RecoveryDelay,
		},
		FailureEnabled: cfg.Failure.Enabled,
		Failure: chaos.FailureOptions{
			AffectedResourceCount: cfg.Failure.ResourceCount,
			MinInterval:           cfg.Failure.MinInterval,
			MaxInterval:           cfg.Failure.MaxInterval,
			Probability:           cfg.Failure.Probability,
			AutoRecover:           cfg.Failure.AutoRecover,
			RecoveryDelay:         cfg.Failure.RecoveryDelay,
		},
	})
	if err != nil {
		return fmt.Errorf("creating simulator manager: %w", err)
	}

	logger.Info("Starting manager")
	return mgr.Start(ctx)
}
