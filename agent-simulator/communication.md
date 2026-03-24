# Fleet Agent Communication Analysis

Analysis of how the Fleet agent communicates with the upstream (management) cluster, focused on what a simulator needs to replicate.

## Communication Overview

The agent communicates exclusively via Kubernetes API calls to the management cluster using a kubeconfig obtained during registration. All operations target a single **cluster namespace** (e.g., `cluster-fleet-local/cluster-fleet-default-mycluster-xyz`).

```
DOWNSTREAM CLUSTER                      MANAGEMENT CLUSTER
+---------------------+                +----------------------+
| Agent Pod           |                | Fleet Controller     |
| (cattle-fleet-sys)  |                |                      |
|                     |                | Cluster Namespace    |
|                     |                | (cluster-fleet-{ID}) |
|                     |                |                      |
| WATCH  -------------|--- watch ------>  BundleDeployment    |
| PATCH  -------------|--- status ---->  BundleDeployment     |
| PATCH  -------------|--- status ---->  Cluster              |
| GET    -------------|--- read ------>  Secret (options)     |
+---------------------+                +----------------------+
```

---

## 1. Registration (Bootstrap)

**Purpose**: Obtain upstream kubeconfig for ongoing communication.

### Process
1. Agent reads local `fleet-agent-bootstrap` secret containing: API server URL, CA cert, namespace, token
2. Agent creates a `ClusterRegistration` resource on management cluster
3. Agent polls for a registration secret (named `c-{clientID}-{clientRandom}`)
4. Registration secret contains the permanent kubeconfig: `kubeconfig`, `clusterNamespace`, `clusterName`, `deploymentNamespace`
5. Agent stores credentials in local `fleet-agent` secret

### Simulator Implication
A simulator can **skip registration entirely** if provided a pre-configured kubeconfig with access to the cluster namespace. Registration is a one-time bootstrap process.

**Source**: `internal/cmd/agent/register/register.go`

---

## 2. BundleDeployment Watch (Upstream -> Agent)

**Purpose**: Receive deployment instructions from the management cluster.

### Operation
- **Type**: WATCH (continuous) on `BundleDeployment` resources
- **Namespace**: Cluster namespace (scoped cache)
- **Trigger**: Any spec change (status-only changes are filtered out)

### Fields Read from BundleDeployment.Spec

| Field | Type | Simulator Relevance |
|-------|------|---------------------|
| `Paused` | bool | If true, skip deployment |
| `OffSchedule` | bool | If true, skip deployment |
| `WaitingForValues` | bool | If true, skip deployment |
| `DeploymentID` | string | Identifies the current deployment version |
| `StagedDeploymentID` | string | Staged deployment waiting for approval |
| `Options` | BundleDeploymentOptions | Full deployment configuration |
| `Options.DefaultNamespace` | string | Target namespace for resources |
| `Options.TargetNamespace` | string | Force all resources to this namespace |
| `Options.Helm` | HelmOptions | Helm chart config (chart, repo, values) |
| `Options.ForceSyncGeneration` | *int64 | Force redeployment counter |
| `Options.CorrectDrift` | CorrectDrift | Drift correction settings |
| `Options.DownstreamResources` | []DownstreamResource | Resources to copy from upstream |
| `DependsOn` | []BundleDependsOn | Bundle dependencies |
| `OCIContents` | bool | Contents are in OCI registry |
| `HelmChartOptions` | HelmChartOptions | Helm chart download config |
| `CorrectDrift` | CorrectDrift | Top-level drift correction |
| `ValuesHash` | string | Hash to validate options secret |
| `DownstreamResourcesGeneration` | *int64 | Tracks DownstreamResources changes |

### Simulator Implication
The simulator needs to **watch** BundleDeployments and respond to spec changes. It does NOT need to actually deploy anything -- it just needs to read the spec and produce a realistic status response.

**Source**: `internal/cmd/agent/controller/bundledeployment_controller.go`

---

## 3. BundleDeployment Status Updates (Agent -> Upstream)

**Purpose**: Report deployment results back to the management cluster.

### Operation
- **Type**: PATCH (MergeFrom) on `BundleDeployment/status` subresource
- **Namespace**: Cluster namespace
- **Frequency**: After each reconciliation (deploy, monitor, drift detect)

### Fields Written to BundleDeployment.Status

#### Core Status
| Field | Type | Description |
|-------|------|-------------|
| `AppliedDeploymentID` | string | DeploymentID that was successfully applied |
| `Release` | string | Helm release name (e.g., `fleet-agent/s-12345abcde`) |
| `Ready` | bool | All deployed resources are ready |
| `NonModified` | bool | No resources have drifted from desired state |
| `IncompleteState` | bool | True if resource lists were truncated (>10 items) |
| `SyncGeneration` | *int64 | Mirrors ForceSyncGeneration after sync |
| `DownstreamResourcesGeneration` | *int64 | Tracks copied resources version |

#### Conditions
| Condition Type | Values | Description |
|---------------|--------|-------------|
| `Deployed` | True/False/Unknown | Whether Helm install/upgrade succeeded |
| `Monitored` | True/False/Unknown | Whether resource monitoring is active |
| `Ready` | True/False/Unknown | Whether all resources are ready |
| `Installed` | True/False/Unknown | Whether Helm release is installed |

Each condition has: `Type`, `Status`, `Reason`, `Message`, `LastUpdateTime`

#### Resource Lists
| Field | Type | Description |
|-------|------|-------------|
| `NonReadyStatus[]` | []NonReadyStatus | Resources that aren't ready |
| `ModifiedStatus[]` | []ModifiedStatus | Resources that have drifted |
| `Resources[]` | []BundleDeploymentResource | All deployed resources |

**NonReadyStatus fields:**
- `UID`, `Kind`, `APIVersion`, `Namespace`, `Name` -- resource identification
- `Summary` -- detailed status (conditions, reasons)

**ModifiedStatus fields:**
- `Kind`, `APIVersion`, `Namespace`, `Name` -- resource identification
- `Create` (bool) -- resource is missing
- `Exist` (bool) -- resource exists but not owned by bundle
- `Delete` (bool) -- resource is orphaned
- `Patch` (string) -- JSON patch description of changes

**Resources[] fields:**
- `Kind`, `APIVersion`, `Namespace`, `Name`, `CreatedAt`

#### Resource Counts
| Field | Type | Description |
|-------|------|-------------|
| `ResourceCounts.DesiredReady` | int | Total resources |
| `ResourceCounts.Ready` | int | Ready count |
| `ResourceCounts.NotReady` | int | Not ready count |
| `ResourceCounts.Missing` | int | Missing count |
| `ResourceCounts.Orphaned` | int | Orphaned count |
| `ResourceCounts.Modified` | int | Modified count |

#### Display
| Field | Type | Description |
|-------|------|-------------|
| `Display.Deployed` | string | Human-readable deployment status |
| `Display.Monitored` | string | Human-readable monitoring status |
| `Display.State` | string | Overall state string |

### Simulator Implication
This is the **most important part to simulate**. The simulator must produce realistic status updates. A minimal happy-path status update looks like:

```go
status.AppliedDeploymentID = spec.DeploymentID
status.Release = "fleet-agent/s-" + hash
status.Ready = true
status.NonModified = true
status.Conditions = []GenericCondition{
    {Type: "Deployed", Status: "True", ...},
    {Type: "Monitored", Status: "True", ...},
    {Type: "Ready", Status: "True", ...},
    {Type: "Installed", Status: "True", ...},
}
status.ResourceCounts = ResourceCounts{DesiredReady: N, Ready: N}
status.Resources = []BundleDeploymentResource{...} // list of "deployed" resources
```

**Source**: `internal/cmd/agent/deployer/monitor/updatestatus.go`, `internal/cmd/agent/controller/bundledeployment_controller.go`

---

## 4. Cluster Status Heartbeat (Agent -> Upstream)

**Purpose**: Signal agent liveness to the management cluster.

### Operation
- **Type**: JSONPatch on `Cluster/status` subresource
- **Namespace**: Cluster namespace
- **Patch path**: `/status/agent`

### Fields Written to Cluster.Status.Agent

| Field | Type | Description |
|-------|------|-------------|
| `LastSeen` | metav1.Time | Timestamp of last check-in (RFC3339) |
| `Namespace` | string | Agent deployment namespace (e.g., `cattle-fleet-system`) |

### Timing
| Event | Delay |
|-------|-------|
| Initial update after registration | 5 seconds |
| Periodic heartbeat interval | 20 seconds (default, configurable) |

### Simulator Implication
The simulator must periodically patch the Cluster resource with `LastSeen`. This is critical -- without it, the management cluster considers the agent offline.

**Source**: `internal/cmd/agent/clusterstatus/ticker.go`

---

## 5. Downstream Resource Copying (Upstream -> Local)

**Purpose**: Copy Secrets/ConfigMaps from upstream cluster namespace to downstream cluster.

### Operation
- **Type**: GET on Secret/ConfigMap in cluster namespace (upstream)
- **Triggered by**: `BundleDeployment.Spec.Options.DownstreamResources` list

### Simulator Implication
The simulator does NOT have a downstream cluster, so this can be **skipped entirely**. Just track `DownstreamResourcesGeneration` in the status to signal completion.

---

## 6. Drift Detection Reporting

**Purpose**: Report when deployed resources diverge from desired state.

### How Drift is Reported
Drift is reported via the same BundleDeployment status update mechanism (section 3):
- `ModifiedStatus[]` populated with drifted resources and their patches
- `NonModified` set to `false`
- `Ready` condition may be affected

### Simulator Implication
The simulator can optionally simulate drift by populating `ModifiedStatus[]` and setting `NonModified = false`. For a happy-path simulator, always report `NonModified = true`.

---

## 7. Summary: Minimum Simulator Requirements

### Must Implement
1. **BundleDeployment Watch** -- watch for new/changed BundleDeployments in cluster namespace
2. **BundleDeployment Status Patch** -- update status with deployment results (conditions, ready state, resource counts)
3. **Cluster Status Heartbeat** -- periodic JSONPatch of `Cluster.Status.Agent.LastSeen`

### Can Skip
- Registration (use pre-configured kubeconfig)
- Helm deployment logic
- Local resource monitoring
- Downstream resource copying (just update generation counter)
- Image scanning (not agent-side)
- Leader election (only one simulator instance per cluster)

### API Calls Summary

| Priority | Operation | Resource | Verb | Frequency |
|----------|-----------|----------|------|-----------|
| Required | Watch | BundleDeployment | WATCH | Continuous |
| Required | Status update | BundleDeployment/status | PATCH | On each BD change |
| Required | Heartbeat | Cluster/status | PATCH (JSON) | Every 20s |
| Optional | Read options | Secret | GET | Per BD with ValuesHash |

---

## 8. Simulator Behavior Configuration

The simulator needs configurable variables to control how it behaves. These variables shape the status update lifecycle for each BundleDeployment.

### 8.1 Deployment Rollout Simulation

Instead of instantly reporting a BundleDeployment as ready, the simulator sends **N incremental status updates** that progressively increase the number of ready resources until all are ready.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `rolloutSteps` | int | 1 | Number of status updates before a BD reaches fully ready. 1 = instant ready. |
| `rolloutInterval` | duration | 5s | Time between each incremental status update during rollout |
| `resourceCount` | int | 10 | Number of fake resources to report per BundleDeployment |

**Rollout sequence for `rolloutSteps=4`, `resourceCount=10`:**

| Step | Status Update |
|------|--------------|
| 1 | `Deployed=True`, `Ready=False`, `ResourceCounts{DesiredReady:10, Ready:3, NotReady:7}` |
| 2 | `Deployed=True`, `Ready=False`, `ResourceCounts{DesiredReady:10, Ready:5, NotReady:5}` |
| 3 | `Deployed=True`, `Ready=False`, `ResourceCounts{DesiredReady:10, Ready:8, NotReady:2}` |
| 4 | `Deployed=True`, `Ready=True`, `ResourceCounts{DesiredReady:10, Ready:10, NotReady:0}` |

During rollout:
- `Conditions[Ready]` = `False` (until final step)
- `Conditions[Deployed]` = `True` (from step 1)
- `Conditions[Installed]` = `True` (from step 1)
- `Conditions[Monitored]` = `True` (from step 1)
- `NonReadyStatus[]` is populated with remaining non-ready resources (shrinks each step)
- `AppliedDeploymentID` is set from step 1

### 8.2 Drift Simulation

The simulator can periodically report drift on BundleDeployments that are in a ready state.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `driftEnabled` | bool | false | Enable drift simulation |
| `driftMinInterval` | duration | 60s | Minimum time between drift events |
| `driftMaxInterval` | duration | 300s | Maximum time between drift events (random within [min, max]) |
| `driftResourceCount` | int | 1 | Number of resources to report as modified per drift event |
| `driftAutoRecover` | bool | true | Automatically recover from drift after a delay |
| `driftRecoveryDelay` | duration | 30s | Time before drift is auto-corrected (if autoRecover=true) |

**Drift event status update:**
```
NonModified = false
Ready = false (or true, depending on severity -- controlled by driftAffectsReady below)
ModifiedStatus[] = [{Kind: "Deployment", Name: "sim-resource-X", Patch: "<synthetic diff>"}]
ResourceCounts.Modified = driftResourceCount
Conditions[Ready] = False (if driftAffectsReady)
```

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `driftAffectsReady` | bool | false | Whether drift events flip the Ready condition to False |

**Recovery status update** (after `driftRecoveryDelay`):
```
NonModified = true
ModifiedStatus[] = []
ResourceCounts.Modified = 0
Conditions[Ready] = True (restored)
```

### 8.3 Random Failure Simulation

The simulator can randomly transition ready BundleDeployments to a non-ready/error state, simulating real-world failures (pod crashes, OOM, image pull errors, etc.).

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `failureEnabled` | bool | false | Enable random failure simulation |
| `failureMinInterval` | duration | 120s | Minimum time between failure events |
| `failureMaxInterval` | duration | 600s | Maximum time between failure events (random within [min, max]) |
| `failureResourceCount` | int | 1 | Number of resources to report as non-ready per failure |
| `failureAutoRecover` | bool | true | Automatically recover from failure after a delay |
| `failureRecoveryDelay` | duration | 60s | Time before auto-recovery (if autoRecover=true) |
| `failureProbability` | float | 0.1 | Probability (0.0-1.0) that a given ready BD is selected for failure when a failure event fires |

**Failure event status update:**
```
Ready = false
Conditions[Ready] = {Status: "False", Reason: "SimulatedFailure", Message: "Pod sim-resource-X: CrashLoopBackOff"}
NonReadyStatus[] = [{Kind: "Pod", Name: "sim-resource-X", Summary: {State: "CrashLoopBackOff", ...}}]
ResourceCounts = {DesiredReady: 10, Ready: 9, NotReady: 1}  // adjusted by failureResourceCount
```

**Recovery** (after `failureRecoveryDelay`):
```
Ready = true
Conditions[Ready] = {Status: "True"}
NonReadyStatus[] = []
ResourceCounts = {DesiredReady: 10, Ready: 10}
```

### 8.4 General Timing

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `heartbeatInterval` | duration | 20s | Cluster status heartbeat interval |
| `initialDelay` | duration | 5s | Delay before first heartbeat after startup |

### 8.5 Lifecycle State Machine

Each BundleDeployment tracked by the simulator follows this state machine:

```
                    +-- new BD spec change (DeploymentID changes)
                    |
                    v
  [Pending] --rolloutInterval--> [RollingOut] --step N--> [Ready]
                                      |                     |  ^
                                      |                     |  |
                                      |    failureInterval  |  failureRecoveryDelay
                                      |       (random)      v  |
                                      |                  [Failed]
                                      |
                                      |                  [Ready]
                                      |                     |  ^
                                      |    driftInterval    |  |
                                      |      (random)       v  |
                                      |                  [Drifted]
                                      |                     |
                                      |              driftRecoveryDelay
                                      |                     |
                                      +---------------------+
```

States:
- **Pending**: BD received, not yet started rollout. Transitions to RollingOut after first `rolloutInterval`.
- **RollingOut**: Incrementally increasing ready resources. Sends `rolloutSteps` status updates spaced by `rolloutInterval`. Transitions to Ready after final step.
- **Ready**: Fully deployed. `Ready=true`, `NonModified=true`. Subject to drift and failure events.
- **Drifted**: Some resources modified. `NonModified=false`. Auto-recovers to Ready after `driftRecoveryDelay` (if enabled).
- **Failed**: Some resources non-ready. `Ready=false`. Auto-recovers to Ready after `failureRecoveryDelay` (if enabled).

A new spec change (DeploymentID change) resets the BD back to **Pending** regardless of current state.

### 8.6 Configuration Example

```yaml
simulator:
  # Upstream connection
  kubeconfig: /path/to/upstream-kubeconfig
  clusterNamespace: cluster-fleet-default-sim-cluster-abc123
  clusterName: sim-cluster
  agentNamespace: cattle-fleet-system

  # Timing
  heartbeatInterval: 20s
  initialDelay: 5s

  # Rollout
  rolloutSteps: 4
  rolloutInterval: 5s
  resourceCount: 10

  # Drift
  drift:
    enabled: true
    minInterval: 60s
    maxInterval: 300s
    resourceCount: 1
    affectsReady: false
    autoRecover: true
    recoveryDelay: 30s

  # Failures
  failure:
    enabled: true
    minInterval: 120s
    maxInterval: 600s
    resourceCount: 1
    probability: 0.1
    autoRecover: true
    recoveryDelay: 60s
```
