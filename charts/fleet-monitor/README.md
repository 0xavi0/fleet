# Fleet Monitor Helm Chart

Fleet Monitor provides read-only monitoring controllers that observe Fleet's reconciliation behavior without impacting production workloads.

## Overview

Fleet Monitor deploys controllers that:
- **Mirror production controller watches** - Trigger on the same events as production Fleet controllers
- **Log detailed change information** - Spec diffs, status changes, related resource triggers
- **Are completely read-only** - No writes to Kubernetes API (except leader election leases)
- **Are individually configurable** - Enable/disable each controller independently
- **Have minimal resource footprint** - Lightweight memory and CPU usage

## Use Cases

- **Debugging reconciliation loops** - Understand why controllers are being triggered repeatedly
- **Understanding controller behavior** - See what changes cause reconciliations
- **Production observability** - Monitor Fleet behavior without impacting performance
- **Development and testing** - Verify controller behavior in test environments

## Installation

### Quick Start

```bash
helm install fleet-monitor ./charts/fleet-monitor \
  --namespace cattle-fleet-system \
  --create-namespace
```

### With Specific Controllers

```bash
helm install fleet-monitor ./charts/fleet-monitor \
  --namespace cattle-fleet-system \
  --set controllers.bundle=true \
  --set controllers.cluster=true \
  --set controllers.bundledeployment=false \
  --set controllers.gitrepo=false \
  --set controllers.helmapp=false
```

### With Sharding

```bash
helm install fleet-monitor-shard0 ./charts/fleet-monitor \
  --namespace cattle-fleet-system \
  --set shardID=shard0
```

### With Custom Resource Limits

```bash
helm install fleet-monitor ./charts/fleet-monitor \
  --namespace cattle-fleet-system \
  --set resources.limits.memory=512Mi \
  --set resources.requests.cpu=200m
```

## Configuration

### Controllers

Enable or disable individual monitor controllers:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controllers.bundle` | Monitor Bundle controller | `true` |
| `controllers.bundledeployment` | Monitor BundleDeployment controller | `true` |
| `controllers.cluster` | Monitor Cluster controller | `true` |
| `controllers.gitrepo` | Monitor GitRepo controller | `true` |
| `controllers.helmapp` | Monitor HelmApp controller | `true` |

### Workers

Configure worker counts for each controller:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `workers.bundle` | Bundle controller workers | `5` |
| `workers.bundledeployment` | BundleDeployment controller workers | `5` |
| `workers.cluster` | Cluster controller workers | `5` |
| `workers.gitrepo` | GitRepo controller workers | `5` |
| `workers.helmapp` | HelmApp controller workers | `5` |

### Image

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Image repository | `rancher/fleet-monitor` |
| `image.tag` | Image tag | `dev` |
| `image.imagePullPolicy` | Image pull policy | `IfNotPresent` |

### Resources

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |

### Sharding

| Parameter | Description | Default |
|-----------|-------------|---------|
| `shardID` | Shard ID for multi-instance deployments | `""` |

### Leader Election

| Parameter | Description | Default |
|-----------|-------------|---------|
| `leaderElection.enabled` | Enable leader election | `true` |
| `leaderElection.leaseDuration` | Lease duration | `30s` |
| `leaderElection.retryPeriod` | Retry period | `10s` |
| `leaderElection.renewDeadline` | Renew deadline | `25s` |

### Other

| Parameter | Description | Default |
|-----------|-------------|---------|
| `namespace` | Namespace to watch | `cattle-fleet-system` |
| `debug` | Enable debug mode | `false` |
| `debugLevel` | Debug level | `0` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `priorityClassName` | Priority class name | `""` |
| `extraEnv` | Extra environment variables | `[]` |

## Log Output

### Bundle Spec Change
```json
{
  "level": "info",
  "logger": "bundle-monitor",
  "msg": "Spec changed - Generation update detected",
  "bundle": "fleet-local/test-bundle",
  "event": "generation-change",
  "oldGeneration": 1,
  "newGeneration": 2,
  "specDiff": "..."
}
```

### Status Change
```json
{
  "level": "info",
  "logger": "bundle-monitor",
  "msg": "Status changed",
  "bundle": "fleet-local/test-bundle",
  "event": "status-change",
  "diff": "..."
}
```

### Related Resource Trigger
```json
{
  "level": "info",
  "logger": "bundle-monitor",
  "msg": "Bundle reconciliation triggered by BundleDeployment change",
  "bundledeployment": "cluster-local-1234/test-bundle",
  "bundle": "fleet-local/test-bundle"
}
```

## RBAC Permissions

Fleet Monitor has **strictly read-only** RBAC permissions:

- **Fleet Resources**: get, list, watch only
- **Core Resources**: get, list, watch only (secrets, configmaps, namespaces, etc.)
- **RBAC Resources**: get, list, watch only
- **Jobs**: get, list, watch only
- **Deployments**: get, list, watch only
- **Leader Election Leases**: full access (required for HA)

## Viewing Logs

```bash
# View logs for fleet-monitor
kubectl logs -n cattle-fleet-system deployment/fleet-monitor -f

# View logs for specific shard
kubectl logs -n cattle-fleet-system deployment/fleet-monitor-shard-shard0 -f

# Filter for specific events
kubectl logs -n cattle-fleet-system deployment/fleet-monitor | grep "generation-change"
```

## Uninstallation

```bash
helm uninstall fleet-monitor --namespace cattle-fleet-system
```

## Troubleshooting

### No logs appearing
- Check that controllers are enabled in values
- Verify RBAC permissions are applied
- Check if there are actual changes happening in Fleet resources

### High memory usage
- Reduce worker counts
- Disable unnecessary controllers
- Adjust resource limits

### Missing trigger sources
- Some triggers can't be detected due to controller-runtime limitations
- Check logs at fan-out mapping points for related resource information

## Architecture

Each monitor controller:
1. Replicates exact watch configuration from production controllers
2. Stores previous object state in cache
3. Compares current state with cached state on reconciliation
4. Logs all detected changes with detailed diffs
5. Updates cache with new state

**Key Design**: Monitors never write to Kubernetes API (except leader election leases).
