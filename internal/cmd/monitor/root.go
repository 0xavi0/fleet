// Package monitor starts the fleet monitor.
package monitor

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	ctrl "sigs.k8s.io/controller-runtime"
	clog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	command "github.com/rancher/fleet/internal/cmd"
	"github.com/rancher/fleet/internal/cmd/monitor/reconciler"
	"github.com/rancher/fleet/pkg/version"
)

type FleetMonitor struct {
	command.DebugConfig
	Kubeconfig string `usage:"Kubeconfig file"`
	Namespace  string `usage:"namespace to watch" default:"cattle-fleet-system" env:"NAMESPACE"`
	ShardID    string `usage:"only monitor resources labeled with a specific shard ID" name:"shard-id"`

	// Controller toggles
	EnableBundleMonitor           bool `usage:"Enable bundle monitoring" env:"ENABLE_BUNDLE_MONITOR"`
	EnableBundleDeploymentMonitor bool `usage:"Enable bundledeployment monitoring" env:"ENABLE_BUNDLEDEPLOYMENT_MONITOR"`
	EnableClusterMonitor          bool `usage:"Enable cluster monitoring" env:"ENABLE_CLUSTER_MONITOR"`
	EnableGitRepoMonitor          bool `usage:"Enable gitrepo monitoring" env:"ENABLE_GITREPO_MONITOR"`
	EnableHelmAppMonitor          bool `usage:"Enable helmapp monitoring" env:"ENABLE_HELMAPP_MONITOR"`

	// Per-controller logging modes
	BundleDetailedLogs           bool   `usage:"Enable detailed logging for Bundle controller" env:"FLEET_MONITOR_BUNDLE_DETAILED" default:"false"`
	BundleDeploymentDetailedLogs bool   `usage:"Enable detailed logging for BundleDeployment controller" env:"FLEET_MONITOR_BUNDLEDEPLOYMENT_DETAILED" default:"false"`
	ClusterDetailedLogs          bool   `usage:"Enable detailed logging for Cluster controller" env:"FLEET_MONITOR_CLUSTER_DETAILED" default:"false"`
	GitRepoDetailedLogs          bool   `usage:"Enable detailed logging for GitRepo controller" env:"FLEET_MONITOR_GITREPO_DETAILED" default:"false"`
	HelmAppDetailedLogs          bool   `usage:"Enable detailed logging for HelmApp controller" env:"FLEET_MONITOR_HELMAPP_DETAILED" default:"false"`

	// Bundle event filters
	BundleEventFilterGenerationChange      bool `usage:"Show generation-change events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_GENERATION_CHANGE"`
	BundleEventFilterStatusChange          bool `usage:"Show status-change events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_STATUS_CHANGE"`
	BundleEventFilterAnnotationChange      bool `usage:"Show annotation-change events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_ANNOTATION_CHANGE"`
	BundleEventFilterLabelChange           bool `usage:"Show label-change events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_LABEL_CHANGE"`
	BundleEventFilterResourceVersionChange bool `usage:"Show resourceversion-change events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_RESVER_CHANGE"`
	BundleEventFilterDeletion              bool `usage:"Show deletion events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_DELETION"`
	BundleEventFilterNotFound              bool `usage:"Show not-found events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_NOT_FOUND"`
	BundleEventFilterCreate                bool `usage:"Show create events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_CREATE"`
	BundleEventFilterTriggeredBy           bool `usage:"Show triggered-by events for Bundle" env:"FLEET_MONITOR_BUNDLE_EVENT_TRIGGERED_BY"`

	// BundleDeployment event filters
	BundleDeploymentEventFilterGenerationChange      bool `usage:"Show generation-change events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_GENERATION_CHANGE"`
	BundleDeploymentEventFilterStatusChange          bool `usage:"Show status-change events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_STATUS_CHANGE"`
	BundleDeploymentEventFilterAnnotationChange      bool `usage:"Show annotation-change events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_ANNOTATION_CHANGE"`
	BundleDeploymentEventFilterLabelChange           bool `usage:"Show label-change events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_LABEL_CHANGE"`
	BundleDeploymentEventFilterResourceVersionChange bool `usage:"Show resourceversion-change events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_RESVER_CHANGE"`
	BundleDeploymentEventFilterDeletion              bool `usage:"Show deletion events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_DELETION"`
	BundleDeploymentEventFilterNotFound              bool `usage:"Show not-found events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_NOT_FOUND"`
	BundleDeploymentEventFilterCreate                bool `usage:"Show create events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_CREATE"`
	BundleDeploymentEventFilterTriggeredBy           bool `usage:"Show triggered-by events for BundleDeployment" env:"FLEET_MONITOR_BD_EVENT_TRIGGERED_BY"`

	// Cluster event filters
	ClusterEventFilterGenerationChange      bool `usage:"Show generation-change events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_GENERATION_CHANGE"`
	ClusterEventFilterStatusChange          bool `usage:"Show status-change events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_STATUS_CHANGE"`
	ClusterEventFilterAnnotationChange      bool `usage:"Show annotation-change events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_ANNOTATION_CHANGE"`
	ClusterEventFilterLabelChange           bool `usage:"Show label-change events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_LABEL_CHANGE"`
	ClusterEventFilterResourceVersionChange bool `usage:"Show resourceversion-change events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_RESVER_CHANGE"`
	ClusterEventFilterDeletion              bool `usage:"Show deletion events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_DELETION"`
	ClusterEventFilterNotFound              bool `usage:"Show not-found events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_NOT_FOUND"`
	ClusterEventFilterCreate                bool `usage:"Show create events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_CREATE"`
	ClusterEventFilterTriggeredBy           bool `usage:"Show triggered-by events for Cluster" env:"FLEET_MONITOR_CLUSTER_EVENT_TRIGGERED_BY"`

	// GitRepo event filters
	GitRepoEventFilterGenerationChange      bool `usage:"Show generation-change events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_GENERATION_CHANGE"`
	GitRepoEventFilterStatusChange          bool `usage:"Show status-change events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_STATUS_CHANGE"`
	GitRepoEventFilterAnnotationChange      bool `usage:"Show annotation-change events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_ANNOTATION_CHANGE"`
	GitRepoEventFilterLabelChange           bool `usage:"Show label-change events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_LABEL_CHANGE"`
	GitRepoEventFilterResourceVersionChange bool `usage:"Show resourceversion-change events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_RESVER_CHANGE"`
	GitRepoEventFilterDeletion              bool `usage:"Show deletion events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_DELETION"`
	GitRepoEventFilterNotFound              bool `usage:"Show not-found events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_NOT_FOUND"`
	GitRepoEventFilterCreate                bool `usage:"Show create events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_CREATE"`
	GitRepoEventFilterTriggeredBy           bool `usage:"Show triggered-by events for GitRepo" env:"FLEET_MONITOR_GITREPO_EVENT_TRIGGERED_BY"`

	// HelmApp event filters
	HelmAppEventFilterGenerationChange      bool `usage:"Show generation-change events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_GENERATION_CHANGE"`
	HelmAppEventFilterStatusChange          bool `usage:"Show status-change events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_STATUS_CHANGE"`
	HelmAppEventFilterAnnotationChange      bool `usage:"Show annotation-change events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_ANNOTATION_CHANGE"`
	HelmAppEventFilterLabelChange           bool `usage:"Show label-change events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_LABEL_CHANGE"`
	HelmAppEventFilterResourceVersionChange bool `usage:"Show resourceversion-change events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_RESVER_CHANGE"`
	HelmAppEventFilterDeletion              bool `usage:"Show deletion events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_DELETION"`
	HelmAppEventFilterNotFound              bool `usage:"Show not-found events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_NOT_FOUND"`
	HelmAppEventFilterCreate                bool `usage:"Show create events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_CREATE"`
	HelmAppEventFilterTriggeredBy           bool `usage:"Show triggered-by events for HelmApp" env:"FLEET_MONITOR_HELMAPP_EVENT_TRIGGERED_BY"`

	SummaryInterval string `usage:"How often to print summary (e.g., 5s, 30s, 1m)" env:"FLEET_MONITOR_SUMMARY_INTERVAL" default:"30s"`
	SummaryReset    bool   `usage:"Reset counters after each summary" env:"FLEET_MONITOR_SUMMARY_RESET" default:"false"`
}

type MonitorReconcilerWorkers struct {
	Bundle           int
	BundleDeployment int
	Cluster          int
	GitRepo          int
	HelmApp          int
}

var (
	setupLog = ctrl.Log.WithName("setup")
	zopts    = &zap.Options{
		Development: true,
	}
)

func (f *FleetMonitor) PersistentPre(_ *cobra.Command, _ []string) error {
	if err := f.SetupDebug(); err != nil {
		return fmt.Errorf("failed to setup debug logging: %w", err)
	}
	zopts = f.OverrideZapOpts(zopts)

	return nil
}

func (f *FleetMonitor) Run(cmd *cobra.Command, args []string) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(zopts)))
	ctx := clog.IntoContext(cmd.Context(), ctrl.Log)

	kubeconfig := ctrl.GetConfigOrDie()
	workersOpts := MonitorReconcilerWorkers{}

	leaderOpts, err := command.NewLeaderElectionOptions()
	if err != nil {
		return err
	}

	if d := os.Getenv("BUNDLE_RECONCILER_WORKERS"); d != "" {
		w, err := strconv.Atoi(d)
		if err != nil {
			setupLog.Error(err, "failed to parse BUNDLE_RECONCILER_WORKERS", "value", d)
		}
		workersOpts.Bundle = w
	}

	if d := os.Getenv("BUNDLEDEPLOYMENT_RECONCILER_WORKERS"); d != "" {
		w, err := strconv.Atoi(d)
		if err != nil {
			setupLog.Error(err, "failed to parse BUNDLEDEPLOYMENT_RECONCILER_WORKERS", "value", d)
		}
		workersOpts.BundleDeployment = w
	}

	if d := os.Getenv("CLUSTER_RECONCILER_WORKERS"); d != "" {
		w, err := strconv.Atoi(d)
		if err != nil {
			setupLog.Error(err, "failed to parse CLUSTER_RECONCILER_WORKERS", "value", d)
		}
		workersOpts.Cluster = w
	}

	if d := os.Getenv("GITREPO_RECONCILER_WORKERS"); d != "" {
		w, err := strconv.Atoi(d)
		if err != nil {
			setupLog.Error(err, "failed to parse GITREPO_RECONCILER_WORKERS", "value", d)
		}
		workersOpts.GitRepo = w
	}

	if d := os.Getenv("HELMAPP_RECONCILER_WORKERS"); d != "" {
		w, err := strconv.Atoi(d)
		if err != nil {
			setupLog.Error(err, "failed to parse HELMAPP_RECONCILER_WORKERS", "value", d)
		}
		workersOpts.HelmApp = w
	}

	// Parse per-controller detailed logging flags from environment
	// The wrangler command framework may not parse boolean env vars correctly,
	// so we parse them manually here
	parseBoolEnv := func(key string, defaultValue bool) bool {
		if val := os.Getenv(key); val != "" {
			b, err := strconv.ParseBool(val)
			if err != nil {
				setupLog.Error(err, "failed to parse boolean env var", "key", key, "value", val)
				return defaultValue
			}
			return b
		}
		return defaultValue
	}

	// Parse controller enable flags
	enableBundle := parseBoolEnv("ENABLE_BUNDLE_MONITOR", f.EnableBundleMonitor)
	enableBundleDeployment := parseBoolEnv("ENABLE_BUNDLEDEPLOYMENT_MONITOR", f.EnableBundleDeploymentMonitor)
	enableCluster := parseBoolEnv("ENABLE_CLUSTER_MONITOR", f.EnableClusterMonitor)
	enableGitRepo := parseBoolEnv("ENABLE_GITREPO_MONITOR", f.EnableGitRepoMonitor)
	enableHelmApp := parseBoolEnv("ENABLE_HELMAPP_MONITOR", f.EnableHelmAppMonitor)

	bundleDetailed := parseBoolEnv("FLEET_MONITOR_BUNDLE_DETAILED", f.BundleDetailedLogs)
	bundleDeploymentDetailed := parseBoolEnv("FLEET_MONITOR_BUNDLEDEPLOYMENT_DETAILED", f.BundleDeploymentDetailedLogs)
	clusterDetailed := parseBoolEnv("FLEET_MONITOR_CLUSTER_DETAILED", f.ClusterDetailedLogs)
	gitRepoDetailed := parseBoolEnv("FLEET_MONITOR_GITREPO_DETAILED", f.GitRepoDetailedLogs)
	helmAppDetailed := parseBoolEnv("FLEET_MONITOR_HELMAPP_DETAILED", f.HelmAppDetailedLogs)

	// Parse event filters for each controller
	bundleEventFilters := reconciler.EventTypeFilters{
		GenerationChange:      parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_GENERATION_CHANGE", f.BundleEventFilterGenerationChange),
		StatusChange:          parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_STATUS_CHANGE", f.BundleEventFilterStatusChange),
		AnnotationChange:      parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_ANNOTATION_CHANGE", f.BundleEventFilterAnnotationChange),
		LabelChange:           parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_LABEL_CHANGE", f.BundleEventFilterLabelChange),
		ResourceVersionChange: parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_RESVER_CHANGE", f.BundleEventFilterResourceVersionChange),
		Deletion:              parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_DELETION", f.BundleEventFilterDeletion),
		NotFound:              parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_NOT_FOUND", f.BundleEventFilterNotFound),
		Create:                parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_CREATE", f.BundleEventFilterCreate),
		TriggeredBy:           parseBoolEnv("FLEET_MONITOR_BUNDLE_EVENT_TRIGGERED_BY", f.BundleEventFilterTriggeredBy),
	}

	bundleDeploymentEventFilters := reconciler.EventTypeFilters{
		GenerationChange:      parseBoolEnv("FLEET_MONITOR_BD_EVENT_GENERATION_CHANGE", f.BundleDeploymentEventFilterGenerationChange),
		StatusChange:          parseBoolEnv("FLEET_MONITOR_BD_EVENT_STATUS_CHANGE", f.BundleDeploymentEventFilterStatusChange),
		AnnotationChange:      parseBoolEnv("FLEET_MONITOR_BD_EVENT_ANNOTATION_CHANGE", f.BundleDeploymentEventFilterAnnotationChange),
		LabelChange:           parseBoolEnv("FLEET_MONITOR_BD_EVENT_LABEL_CHANGE", f.BundleDeploymentEventFilterLabelChange),
		ResourceVersionChange: parseBoolEnv("FLEET_MONITOR_BD_EVENT_RESVER_CHANGE", f.BundleDeploymentEventFilterResourceVersionChange),
		Deletion:              parseBoolEnv("FLEET_MONITOR_BD_EVENT_DELETION", f.BundleDeploymentEventFilterDeletion),
		NotFound:              parseBoolEnv("FLEET_MONITOR_BD_EVENT_NOT_FOUND", f.BundleDeploymentEventFilterNotFound),
		Create:                parseBoolEnv("FLEET_MONITOR_BD_EVENT_CREATE", f.BundleDeploymentEventFilterCreate),
		TriggeredBy:           parseBoolEnv("FLEET_MONITOR_BD_EVENT_TRIGGERED_BY", f.BundleDeploymentEventFilterTriggeredBy),
	}

	clusterEventFilters := reconciler.EventTypeFilters{
		GenerationChange:      parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_GENERATION_CHANGE", f.ClusterEventFilterGenerationChange),
		StatusChange:          parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_STATUS_CHANGE", f.ClusterEventFilterStatusChange),
		AnnotationChange:      parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_ANNOTATION_CHANGE", f.ClusterEventFilterAnnotationChange),
		LabelChange:           parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_LABEL_CHANGE", f.ClusterEventFilterLabelChange),
		ResourceVersionChange: parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_RESVER_CHANGE", f.ClusterEventFilterResourceVersionChange),
		Deletion:              parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_DELETION", f.ClusterEventFilterDeletion),
		NotFound:              parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_NOT_FOUND", f.ClusterEventFilterNotFound),
		Create:                parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_CREATE", f.ClusterEventFilterCreate),
		TriggeredBy:           parseBoolEnv("FLEET_MONITOR_CLUSTER_EVENT_TRIGGERED_BY", f.ClusterEventFilterTriggeredBy),
	}

	gitRepoEventFilters := reconciler.EventTypeFilters{
		GenerationChange:      parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_GENERATION_CHANGE", f.GitRepoEventFilterGenerationChange),
		StatusChange:          parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_STATUS_CHANGE", f.GitRepoEventFilterStatusChange),
		AnnotationChange:      parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_ANNOTATION_CHANGE", f.GitRepoEventFilterAnnotationChange),
		LabelChange:           parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_LABEL_CHANGE", f.GitRepoEventFilterLabelChange),
		ResourceVersionChange: parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_RESVER_CHANGE", f.GitRepoEventFilterResourceVersionChange),
		Deletion:              parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_DELETION", f.GitRepoEventFilterDeletion),
		NotFound:              parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_NOT_FOUND", f.GitRepoEventFilterNotFound),
		Create:                parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_CREATE", f.GitRepoEventFilterCreate),
		TriggeredBy:           parseBoolEnv("FLEET_MONITOR_GITREPO_EVENT_TRIGGERED_BY", f.GitRepoEventFilterTriggeredBy),
	}

	helmAppEventFilters := reconciler.EventTypeFilters{
		GenerationChange:      parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_GENERATION_CHANGE", f.HelmAppEventFilterGenerationChange),
		StatusChange:          parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_STATUS_CHANGE", f.HelmAppEventFilterStatusChange),
		AnnotationChange:      parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_ANNOTATION_CHANGE", f.HelmAppEventFilterAnnotationChange),
		LabelChange:           parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_LABEL_CHANGE", f.HelmAppEventFilterLabelChange),
		ResourceVersionChange: parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_RESVER_CHANGE", f.HelmAppEventFilterResourceVersionChange),
		Deletion:              parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_DELETION", f.HelmAppEventFilterDeletion),
		NotFound:              parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_NOT_FOUND", f.HelmAppEventFilterNotFound),
		Create:                parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_CREATE", f.HelmAppEventFilterCreate),
		TriggeredBy:           parseBoolEnv("FLEET_MONITOR_HELMAPP_EVENT_TRIGGERED_BY", f.HelmAppEventFilterTriggeredBy),
	}

	// Parse resource filters for each controller
	bundleResourceFilter := &reconciler.ResourceFilter{
		NamespacePattern: os.Getenv("FLEET_MONITOR_BUNDLE_RESOURCE_FILTER_NAMESPACE"),
		NamePattern:      os.Getenv("FLEET_MONITOR_BUNDLE_RESOURCE_FILTER_NAME"),
	}

	bundleDeploymentResourceFilter := &reconciler.ResourceFilter{
		NamespacePattern: os.Getenv("FLEET_MONITOR_BUNDLEDEPLOYMENT_RESOURCE_FILTER_NAMESPACE"),
		NamePattern:      os.Getenv("FLEET_MONITOR_BUNDLEDEPLOYMENT_RESOURCE_FILTER_NAME"),
	}

	clusterResourceFilter := &reconciler.ResourceFilter{
		NamespacePattern: os.Getenv("FLEET_MONITOR_CLUSTER_RESOURCE_FILTER_NAMESPACE"),
		NamePattern:      os.Getenv("FLEET_MONITOR_CLUSTER_RESOURCE_FILTER_NAME"),
	}

	gitRepoResourceFilter := &reconciler.ResourceFilter{
		NamespacePattern: os.Getenv("FLEET_MONITOR_GITREPO_RESOURCE_FILTER_NAMESPACE"),
		NamePattern:      os.Getenv("FLEET_MONITOR_GITREPO_RESOURCE_FILTER_NAME"),
	}

	helmAppResourceFilter := &reconciler.ResourceFilter{
		NamespacePattern: os.Getenv("FLEET_MONITOR_HELMAPP_RESOURCE_FILTER_NAMESPACE"),
		NamePattern:      os.Getenv("FLEET_MONITOR_HELMAPP_RESOURCE_FILTER_NAME"),
	}

	// Log the parsed configuration for debugging
	setupLog.Info("parsed per-controller logging configuration",
		"bundle", bundleDetailed,
		"bundleDeployment", bundleDeploymentDetailed,
		"cluster", clusterDetailed,
		"gitRepo", gitRepoDetailed,
		"helmApp", helmAppDetailed,
	)

	// Parse summary interval
	summaryInterval, err := time.ParseDuration(f.SummaryInterval)
	if err != nil {
		setupLog.Error(err, "invalid summary interval, using default 30s", "value", f.SummaryInterval)
		summaryInterval = 30 * time.Second
	}

	monitorOpts := MonitorOptions{
		EnableBundle:           enableBundle,
		EnableBundleDeployment: enableBundleDeployment,
		EnableCluster:          enableCluster,
		EnableGitRepo:          enableGitRepo,
		EnableHelmApp:          enableHelmApp,
		Workers:                workersOpts,

		// Per-controller logging configuration
		ControllerLogging: ControllerLoggingConfig{
			Bundle: ControllerLogConfig{
				Detailed:       bundleDetailed,
				EventFilters:   bundleEventFilters,
				ResourceFilter: bundleResourceFilter,
			},
			BundleDeployment: ControllerLogConfig{
				Detailed:       bundleDeploymentDetailed,
				EventFilters:   bundleDeploymentEventFilters,
				ResourceFilter: bundleDeploymentResourceFilter,
			},
			Cluster: ControllerLogConfig{
				Detailed:       clusterDetailed,
				EventFilters:   clusterEventFilters,
				ResourceFilter: clusterResourceFilter,
			},
			GitRepo: ControllerLogConfig{
				Detailed:       gitRepoDetailed,
				EventFilters:   gitRepoEventFilters,
				ResourceFilter: gitRepoResourceFilter,
			},
			HelmApp: ControllerLogConfig{
				Detailed:       helmAppDetailed,
				EventFilters:   helmAppEventFilters,
				ResourceFilter: helmAppResourceFilter,
			},
		},

		SummaryInterval: summaryInterval,
		SummaryReset:    f.SummaryReset,
	}

	if err := start(
		ctx,
		f.Namespace,
		kubeconfig,
		leaderOpts,
		monitorOpts,
		f.ShardID,
	); err != nil {
		return err
	}

	<-cmd.Context().Done()
	return nil
}

func App() *cobra.Command {
	root := command.Command(&FleetMonitor{}, cobra.Command{
		Version: version.FriendlyVersion(),
		Use:     "fleetmonitor",
		Short:   "Fleet read-only monitoring controllers",
	})
	fs := flag.NewFlagSet("", flag.ExitOnError)
	zopts.BindFlags(fs)
	ctrl.RegisterFlags(fs)
	root.Flags().AddGoFlagSet(fs)

	return root
}
