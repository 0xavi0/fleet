# Fleet Agent Simulator - Implementation Plan

Incremental implementation plan. Each phase builds on the previous one and includes tests. The simulator runs as a standalone binary outside any Kubernetes cluster -- it only needs a kubeconfig to reach the management cluster API.

Reference: [communication.md](./communication.md) for full field-level details.

---

## Phase 1: Skeleton + Upstream Connection + Heartbeat ✅ DONE

**Goal**: A binary that connects to the upstream management cluster and sends periodic Cluster status heartbeats. No BundleDeployment logic yet.

### 1.1 Project structure

```
agent-simulator/
  cmd/
    simulator/
      main.go           # Entry point, flag parsing, signal handling
  pkg/
    config/
      config.go         # Config struct + YAML loader
      config_test.go
    heartbeat/
      heartbeat.go      # Cluster status patcher
      heartbeat_test.go
  config.yaml           # Example config
```

### 1.2 Config struct (minimal for phase 1)

```go
type Config struct {
    Kubeconfig        string        `yaml:"kubeconfig"`
    ClusterNamespace  string        `yaml:"clusterNamespace"`
    ClusterName       string        `yaml:"clusterName"`
    AgentNamespace    string        `yaml:"agentNamespace"`
    HeartbeatInterval time.Duration `yaml:"heartbeatInterval"` // default 20s
    InitialDelay      time.Duration `yaml:"initialDelay"`      // default 5s
}
```

### 1.3 main.go

- Parse flags: `--config` (path to YAML), `--kubeconfig` (override)
- Load config from YAML
- Build upstream `client.Client` using kubeconfig (controller-runtime client with Fleet scheme)
- Start heartbeat goroutine
- Block on context cancellation (SIGINT/SIGTERM)

### 1.4 Heartbeat

Replicate the exact JSONPatch from `internal/cmd/agent/clusterstatus/ticker.go`:

```go
patch := `[{"op":"add","path":"/status/agent","value":{"lastSeen":"` +
    time.Now().UTC().Format(time.RFC3339) +
    `","namespace":"` + agentNamespace + `"}}]`
client.Status().Patch(ctx, cluster, client.RawPatch(types.JSONPatchType, []byte(patch)))
```

- Wait `initialDelay` before first patch
- Then tick every `heartbeatInterval`

### 1.5 Tests

| Test | Type | Description | Status |
|------|------|-------------|--------|
| `config_test.go` | Unit | Load YAML config, verify defaults, validate required fields | ✅ 8/8 pass |
| `heartbeat_test.go` | Unit (envtest) | Create a Cluster resource, run heartbeat, assert `Status.Agent.LastSeen` is updated. Assert it updates periodically. | ✅ 3/3 pass |

**Implementation notes:**
- Added `./agent-simulator` to `go.work` so the simulator is part of the workspace.
- Heartbeat test initialises the Cluster status subresource before patching, because the JSONPatch `op:add` on `/status/agent` requires the parent `/status` object to exist.

---

## Phase 2: BundleDeployment Watch + Instant Ready

**Goal**: Watch BundleDeployments in the cluster namespace and immediately respond with a fully-ready status.

### 2.1 New files

```
agent-simulator/
  pkg/
    simulator/
      simulator.go      # Controller-runtime manager setup + BD reconciler
      simulator_test.go
    status/
      status.go         # Status builder (constructs BundleDeployment status)
      status_test.go
    resources/
      resources.go      # Fake resource generator
      resources_test.go
```

### 2.2 Simulator (controller-runtime manager)

- Create a `ctrl.Manager` against the upstream cluster, scoped to `clusterNamespace` (same cache pattern as real agent: `cache.Options{DefaultNamespaces: ...}`)
- Register a reconciler for `BundleDeployment`
- Add heartbeat as a `manager.Runnable`
- Use event predicates to filter status-only changes (same as real agent)

### 2.3 BD Reconciler (instant ready)

On reconcile:
1. Read `bd.Spec.DeploymentID` -- if it matches `bd.Status.AppliedDeploymentID`, skip (already processed)
2. Skip if `Paused`, `OffSchedule`, or `WaitingForValues` is true
3. Build a fully-ready status (via `status.Builder`)
4. Patch `BundleDeployment/status` using MergeFrom

### 2.4 Status Builder

Constructs a `BundleDeploymentStatus` given parameters:

```go
type BuildParams struct {
    DeploymentID  string
    ResourceCount int
    ReadyCount    int
    AgentScope    string
    Namespace     string  // for release name
}
```

Produces:
- `AppliedDeploymentID` = DeploymentID
- `Release` = `<agentNamespace>/s-<hash>` (deterministic from BD name)
- `Ready` = (ReadyCount == ResourceCount)
- `NonModified` = true
- `Conditions`: Deployed=True, Installed=True, Monitored=True, Ready=(based on readiness)
- `Resources[]`: N fake resources (Deployment, Service, ConfigMap mix)
- `ResourceCounts`: computed from ReadyCount/ResourceCount
- `NonReadyStatus[]`: populated for non-ready resources
- `Display`: derived from conditions

### 2.5 Fake Resource Generator

Generates deterministic fake resource lists for a BundleDeployment:
- Given BD name + resourceCount, produces a stable list of resources (same names on every call)
- Resource types: mix of Deployment, Service, ConfigMap, ServiceAccount
- Names derived from BD name: `<bd-name>-deploy-0`, `<bd-name>-svc-0`, etc.

### 2.6 Config additions

```go
ResourceCount int `yaml:"resourceCount"` // default 10
```

### 2.7 Tests

| Test | Type | Description |
|------|------|-------------|
| `status_test.go` | Unit | Build status with various ReadyCount/ResourceCount combos, verify conditions, counts, resource lists |
| `resources_test.go` | Unit | Generate resources, verify determinism, verify correct count and types |
| `simulator_test.go` | Integration (envtest) | Create a BundleDeployment, run simulator, assert status is patched to ready. Assert Paused BDs are skipped. Assert DeploymentID change triggers re-reconcile. |

---

## Phase 3: Gradual Rollout (N steps to Ready)

**Goal**: Instead of instant ready, send N incremental status updates with increasing ready counts.

### 3.1 New files

```
agent-simulator/
  pkg/
    rollout/
      rollout.go       # Per-BD rollout state machine
      rollout_test.go
```

### 3.2 Rollout State Machine

Track per-BD state:

```go
type State struct {
    DeploymentID string
    CurrentStep  int
    TotalSteps   int
    NextUpdate   time.Time
}
```

- On new DeploymentID: reset to step 0
- Each step: compute `readyCount = resourceCount * currentStep / totalSteps` (final step always = resourceCount)
- After building status, schedule next update via `ctrl.Result{RequeueAfter: rolloutInterval}`
- When `currentStep == totalSteps`: transition to Ready, stop requeueing

### 3.3 Reconciler changes

- Replace instant-ready logic with rollout state lookup
- If BD has active rollout state, advance one step and requeue
- If `rolloutSteps == 1`, behave like phase 2 (instant ready)

### 3.4 Config additions

```go
RolloutSteps    int           `yaml:"rolloutSteps"`    // default 1
RolloutInterval time.Duration `yaml:"rolloutInterval"` // default 5s
```

### 3.5 Tests

| Test | Type | Description |
|------|------|-------------|
| `rollout_test.go` | Unit | Advance state through steps, verify ready counts at each step. Verify reset on DeploymentID change. Verify single-step mode. |
| `simulator_test.go` | Integration (envtest) | Create BD with rolloutSteps=3, verify 3 status patches with increasing ready counts, final patch has Ready=true. |

---

## Phase 4: Drift Simulation

**Goal**: Periodically report drift on ready BundleDeployments, with optional auto-recovery.

### 4.1 New files

```
agent-simulator/
  pkg/
    chaos/
      drift.go         # Drift event scheduler
      drift_test.go
```

### 4.2 Drift Scheduler

- Runs as a `manager.Runnable`
- Maintains a timer per ready BD, random interval in `[driftMinInterval, driftMaxInterval]`
- When timer fires:
  1. Pick `driftResourceCount` resources from the BD's fake resource list
  2. Patch BD status: `NonModified=false`, populate `ModifiedStatus[]` with synthetic patches, update `ResourceCounts.Modified`
  3. If `driftAffectsReady`: also set `Ready=false`, `Conditions[Ready]=False`
  4. If `driftAutoRecover`: schedule recovery after `driftRecoveryDelay`
- Recovery: restore `NonModified=true`, clear `ModifiedStatus[]`, restore Ready if affected

### 4.3 ModifiedStatus generation

Produce realistic-looking patches:

```go
ModifiedStatus{
    Kind:      "Deployment",
    Name:      "sim-resource-X",
    Namespace: targetNamespace,
    Patch:     `{"spec":{"replicas":3}}`, // synthetic
}
```

### 4.4 Config additions

```go
Drift struct {
    Enabled       bool          `yaml:"enabled"`       // default false
    MinInterval   time.Duration `yaml:"minInterval"`   // default 60s
    MaxInterval   time.Duration `yaml:"maxInterval"`   // default 300s
    ResourceCount int           `yaml:"resourceCount"` // default 1
    AffectsReady  bool          `yaml:"affectsReady"`  // default false
    AutoRecover   bool          `yaml:"autoRecover"`   // default true
    RecoveryDelay time.Duration `yaml:"recoveryDelay"` // default 30s
} `yaml:"drift"`
```

### 4.5 Tests

| Test | Type | Description |
|------|------|-------------|
| `drift_test.go` | Unit | Verify random interval is within bounds. Verify ModifiedStatus generation. Verify recovery clears drift state. Verify driftAffectsReady flag. |
| `drift_integration_test.go` | Integration (envtest) | Create BD, let it reach Ready, enable drift with short intervals, assert BD transitions to drifted state and back. |

---

## Phase 5: Random Failure Simulation

**Goal**: Randomly transition ready BDs to a failed state, simulating pod crashes and other failures.

### 5.1 New files

```
agent-simulator/
  pkg/
    chaos/
      failure.go       # Failure event scheduler
      failure_test.go
```

### 5.2 Failure Scheduler

- Runs as a `manager.Runnable`
- Periodic tick at random interval in `[failureMinInterval, failureMaxInterval]`
- On tick: iterate all ready BDs, apply `failureProbability` to each
- For selected BDs:
  1. Pick `failureResourceCount` resources from the fake resource list
  2. Patch BD status: `Ready=false`, `Conditions[Ready]={Status:False, Reason:SimulatedFailure, Message:...}`
  3. Populate `NonReadyStatus[]` with realistic error summaries (CrashLoopBackOff, ImagePullBackOff, OOMKilled -- randomly selected)
  4. Update `ResourceCounts` (decrement Ready, increment NotReady)
  5. If `failureAutoRecover`: schedule recovery after `failureRecoveryDelay`
- Recovery: restore to fully ready state

### 5.3 NonReadyStatus generation

Produce realistic error summaries:

```go
NonReadyStatus{
    Kind:      "Pod",
    Name:      "sim-resource-X-abc123",
    Namespace: targetNamespace,
    Summary: Summary{
        State:   "CrashLoopBackOff",
        Message: "back-off 5m0s restarting failed container",
    },
}
```

Error types pool: `CrashLoopBackOff`, `ImagePullBackOff`, `OOMKilled`, `CreateContainerError`, `ErrImageNeverPull`

### 5.4 Config additions

```go
Failure struct {
    Enabled       bool          `yaml:"enabled"`       // default false
    MinInterval   time.Duration `yaml:"minInterval"`   // default 120s
    MaxInterval   time.Duration `yaml:"maxInterval"`   // default 600s
    ResourceCount int           `yaml:"resourceCount"` // default 1
    Probability   float64       `yaml:"probability"`   // default 0.1
    AutoRecover   bool          `yaml:"autoRecover"`   // default true
    RecoveryDelay time.Duration `yaml:"recoveryDelay"` // default 60s
} `yaml:"failure"`
```

### 5.5 Tests

| Test | Type | Description |
|------|------|-------------|
| `failure_test.go` | Unit | Verify probability filtering (with seeded RNG). Verify NonReadyStatus error message generation. Verify recovery restores Ready state. Verify failure selects only Ready BDs. |
| `failure_integration_test.go` | Integration (envtest) | Create BD, let it reach Ready, trigger failure, assert BD goes non-ready, wait for recovery, assert Ready again. |

---

## Phase 6: Multi-Cluster Simulation ✅ DONE

**Goal**: Run a single simulator binary that simulates multiple agents (one per cluster).

### 6.1 Changes

- Config accepts a list of clusters, each with its own namespace/name and optional per-cluster overrides
- Main loop starts one manager per cluster (each with its own goroutine)
- Shared kubeconfig (all simulated clusters talk to the same management cluster)

### 6.2 Config additions

```yaml
simulator:
  kubeconfig: /path/to/upstream
  defaults:
    heartbeatInterval: 20s
    rolloutSteps: 3
    resourceCount: 10
    # ... all other defaults

  clusters:
    - clusterNamespace: cluster-fleet-default-sim-001-xyz
      clusterName: sim-001
    - clusterNamespace: cluster-fleet-default-sim-002-xyz
      clusterName: sim-002
      rolloutSteps: 5  # per-cluster override
```

### 6.3 Tests

| Test | Type | Description |
|------|------|-------------|
| `multi_cluster_test.go` | Unit | Verify config merging (defaults + per-cluster overrides). |
| `multi_cluster_integration_test.go` | Integration (envtest) | Start 2 simulated clusters, create BDs in each namespace, verify independent heartbeats and status updates. |

---

## Phase 7: Observability + CLI Polish ✅ DONE

**Goal**: Make the simulator usable for real performance testing.

### 7.1 Logging

- Structured logging via `sigs.k8s.io/controller-runtime/pkg/log` (consistent with Fleet)
- Log: heartbeats sent, BD transitions (Pending->RollingOut->Ready->Drifted->Failed), status patches sent
- Configurable log level

### 7.2 Metrics

- Prometheus metrics endpoint (reuse controller-runtime metrics server)
- Metrics:
  - `simulator_heartbeats_total` (counter, per cluster)
  - `simulator_status_patches_total` (counter, per cluster, per state)
  - `simulator_bd_state` (gauge, per cluster: count of BDs in each state)
  - `simulator_patch_duration_seconds` (histogram)
  - `simulator_patch_errors_total` (counter)

### 7.3 CLI flags

```
--config           Path to config YAML
--kubeconfig       Override kubeconfig path (also respects KUBECONFIG env)
--log-level        Log verbosity (0-5)
--metrics-addr     Metrics bind address (default :9090)
--dry-run          Log status updates without patching
```

### 7.4 Tests

| Test | Type | Description |
|------|------|-------------|
| `metrics_test.go` | Unit | Verify metric increments on heartbeat and status patch. |
| `dry_run_test.go` | Unit | Verify no API calls made in dry-run mode. |

---

## Implementation Order Summary

| Phase | Depends On | What You Get |
|-------|-----------|-------------|
| 1 | Nothing | Binary that connects + heartbeats |
| 2 | Phase 1 | Instant-ready BD responses |
| 3 | Phase 2 | Gradual rollout (N steps) |
| 4 | Phase 2 | Drift events on ready BDs |
| 5 | Phase 2 | Random failure events |
| 6 | Phases 1-5 | Multiple simulated clusters |
| 7 | Phases 1-5 | Metrics, logging, CLI polish |

Phases 4 and 5 are independent of each other and of phase 3 -- they can be done in any order after phase 2.

---

## Testing Strategy

### Unit tests
- Pure Go tests, no cluster needed
- Cover: config parsing, status building, resource generation, rollout state machine, drift/failure scheduling logic
- Use table-driven tests for status builder (many field combinations)

### Integration tests (envtest)
- Use `sigs.k8s.io/controller-runtime/tools/setup-envtest` (same as Fleet's existing integration tests)
- Register Fleet CRDs (Cluster, BundleDeployment) in envtest
- Create resources, run simulator components, assert status updates with `Eventually`/`Consistently` (Gomega)
- Test each phase's behavior in isolation

### Running tests

```bash
# Unit tests
go test ./agent-simulator/pkg/...

# Integration tests (requires setup-envtest)
KUBEBUILDER_ASSETS=$(setup-envtest use --use-env -p path 1.34) \
  ginkgo ./agent-simulator/pkg/simulator/...
```
