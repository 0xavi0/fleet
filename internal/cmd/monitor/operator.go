package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rancher/fleet/internal/cmd"
	"github.com/rancher/fleet/internal/cmd/monitor/reconciler"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

// ControllerLogConfig holds logging configuration for a single controller
type ControllerLogConfig struct {
	Detailed     bool                         // true = detailed logs, false = summary only
	EventFilters reconciler.EventTypeFilters // Which event types to show in detailed mode
}

// ControllerLoggingConfig holds logging configuration for all controllers
type ControllerLoggingConfig struct {
	Bundle           ControllerLogConfig
	BundleDeployment ControllerLogConfig
	Cluster          ControllerLogConfig
	GitRepo          ControllerLogConfig
	HelmApp          ControllerLogConfig
}

type MonitorOptions struct {
	EnableBundle           bool
	EnableBundleDeployment bool
	EnableCluster          bool
	EnableGitRepo          bool
	EnableHelmApp          bool
	Workers                MonitorReconcilerWorkers

	// Per-controller logging configuration
	ControllerLogging ControllerLoggingConfig

	// Summary configuration
	SummaryInterval time.Duration
	SummaryReset    bool
}

func start(
	ctx context.Context,
	systemNamespace string,
	config *rest.Config,
	leaderOpts cmd.LeaderElectionOptions,
	monitorOpts MonitorOptions,
	shardID string,
) error {
	setupLog.Info("starting fleet monitor",
		"namespace", systemNamespace,
		"shardID", shardID,
		"enableBundle", monitorOpts.EnableBundle,
		"enableBundleDeployment", monitorOpts.EnableBundleDeployment,
		"enableCluster", monitorOpts.EnableCluster,
		"enableGitRepo", monitorOpts.EnableGitRepo,
		"enableHelmApp", monitorOpts.EnableHelmApp,
		"bundleDetailedLogs", monitorOpts.ControllerLogging.Bundle.Detailed,
		"bundleDeploymentDetailedLogs", monitorOpts.ControllerLogging.BundleDeployment.Detailed,
		"clusterDetailedLogs", monitorOpts.ControllerLogging.Cluster.Detailed,
		"gitRepoDetailedLogs", monitorOpts.ControllerLogging.GitRepo.Detailed,
		"helmAppDetailedLogs", monitorOpts.ControllerLogging.HelmApp.Detailed,
		"summaryInterval", monitorOpts.SummaryInterval,
		"summaryReset", monitorOpts.SummaryReset,
	)

	// Start summary printer (always runs, prints stats for all controllers)
	go startSummaryPrinter(ctx, monitorOpts.SummaryInterval, monitorOpts.SummaryReset)

	// No metrics for monitoring controllers
	metricServerOptions := metricsserver.Options{BindAddress: "0"}

	var leaderElectionSuffix string
	if shardID != "" {
		leaderElectionSuffix = fmt.Sprintf("-%s", shardID)
	}

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricServerOptions,
		HealthProbeBindAddress:  "0", // No health probes
		LeaderElection:          true,
		LeaderElectionID:        fmt.Sprintf("fleet-monitor-leader-election-shard%s", leaderElectionSuffix),
		LeaderElectionNamespace: systemNamespace,
		LeaseDuration:           leaderOpts.LeaseDuration,
		RenewDeadline:           leaderOpts.RenewDeadline,
		RetryPeriod:             leaderOpts.RetryPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	// Register enabled monitor controllers with per-controller logging mode
	if monitorOpts.EnableBundle {
		if err := (&reconciler.BundleMonitorReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			ShardID:      shardID,
			Workers:      monitorOpts.Workers.Bundle,
			DetailedLogs: monitorOpts.ControllerLogging.Bundle.Detailed,
			EventFilters: monitorOpts.ControllerLogging.Bundle.EventFilters,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create monitor controller", "controller", "Bundle")
			return err
		}
		setupLog.Info("registered monitor controller", "controller", "Bundle", "workers", monitorOpts.Workers.Bundle, "mode", logMode(monitorOpts.ControllerLogging.Bundle.Detailed))
	}

	if monitorOpts.EnableCluster {
		if err := (&reconciler.ClusterMonitorReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			ShardID:      shardID,
			Workers:      monitorOpts.Workers.Cluster,
			DetailedLogs: monitorOpts.ControllerLogging.Cluster.Detailed,
			EventFilters: monitorOpts.ControllerLogging.Cluster.EventFilters,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create monitor controller", "controller", "Cluster")
			return err
		}
		setupLog.Info("registered monitor controller", "controller", "Cluster", "workers", monitorOpts.Workers.Cluster, "mode", logMode(monitorOpts.ControllerLogging.Cluster.Detailed))
	}

	if monitorOpts.EnableBundleDeployment {
		if err := (&reconciler.BundleDeploymentMonitorReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			ShardID:      shardID,
			Workers:      monitorOpts.Workers.BundleDeployment,
			DetailedLogs: monitorOpts.ControllerLogging.BundleDeployment.Detailed,
			EventFilters: monitorOpts.ControllerLogging.BundleDeployment.EventFilters,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create monitor controller", "controller", "BundleDeployment")
			return err
		}
		setupLog.Info("registered monitor controller", "controller", "BundleDeployment", "workers", monitorOpts.Workers.BundleDeployment, "mode", logMode(monitorOpts.ControllerLogging.BundleDeployment.Detailed))
	}

	if monitorOpts.EnableGitRepo {
		if err := (&reconciler.GitRepoMonitorReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			ShardID:      shardID,
			Workers:      monitorOpts.Workers.GitRepo,
			DetailedLogs: monitorOpts.ControllerLogging.GitRepo.Detailed,
			EventFilters: monitorOpts.ControllerLogging.GitRepo.EventFilters,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create monitor controller", "controller", "GitRepo")
			return err
		}
		setupLog.Info("registered monitor controller", "controller", "GitRepo", "workers", monitorOpts.Workers.GitRepo, "mode", logMode(monitorOpts.ControllerLogging.GitRepo.Detailed))
	}

	if monitorOpts.EnableHelmApp {
		if err := (&reconciler.HelmAppMonitorReconciler{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			ShardID:      shardID,
			Workers:      monitorOpts.Workers.HelmApp,
			DetailedLogs: monitorOpts.ControllerLogging.HelmApp.Detailed,
			EventFilters: monitorOpts.ControllerLogging.HelmApp.EventFilters,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create monitor controller", "controller", "HelmApp")
			return err
		}
		setupLog.Info("registered monitor controller", "controller", "HelmApp", "workers", monitorOpts.Workers.HelmApp, "mode", logMode(monitorOpts.ControllerLogging.HelmApp.Detailed))
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}

func logMode(detailed bool) string {
	if detailed {
		return "detailed"
	}
	return "summary"
}

// startSummaryPrinter periodically prints statistics summary
func startSummaryPrinter(ctx context.Context, interval time.Duration, reset bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	statsTracker := reconciler.GetStatsTracker()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			summary := statsTracker.GetSummary()

			// Convert to JSON and print
			jsonStr, err := summary.ToJSON()
			if err != nil {
				setupLog.Error(err, "failed to marshal summary to JSON")
				continue
			}

			// Print as structured log (will be formatted as JSON by zap)
			setupLog.Info("Fleet Monitor Summary", "summary", json.RawMessage(jsonStr))

			// Reset or just update timestamp
			if reset {
				statsTracker.Reset()
			} else {
				statsTracker.UpdateLastSummaryTime()
			}
		}
	}
}
