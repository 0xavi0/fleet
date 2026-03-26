# Fleet Agent Simulator

A standalone binary that impersonates one or more Fleet agents against a management cluster without running any real downstream Kubernetes cluster.

Useful for:
- **Load / scale testing** — spin up hundreds of simulated agents to stress-test the Fleet controller.
- **Development** — iterate on controller logic without needing real downstream clusters.
- **Demo environments** — produce realistic agent activity (heartbeats, BundleDeployment status updates) from a laptop.

The simulator speaks the exact same API as a real Fleet agent:
- Periodic `Cluster/status` JSONPatch heartbeats (Phase 1)
- `BundleDeployment/status` patches with realistic ready/non-ready/drifted/failed states (Phases 2-5)

---

## Quick start

### 1. Build

From the fleet repo root:

```bash
./agent-simulator/build
# binary written to ./bin/fleet-agent-simulator
```

Cross-compile for Linux amd64:

```bash
GOOS=linux GOARCH=amd64 ./agent-simulator/build
```

Write the binary to a custom path:

```bash
OUTPUT=/usr/local/bin/fleet-agent-simulator ./agent-simulator/build
```

### 2. Prerequisites

You need:

1. A running Fleet management cluster with the Fleet controller installed.
2. A kubeconfig for the management cluster scoped to a ServiceAccount with the permissions listed in the [RBAC](#rbac-permissions) section below.
3. A `Cluster` resource and its cluster namespace set up on the management cluster (see [Setting up a cluster target](#setting-up-a-cluster-target)).

### 3. Configure

Copy the example config and fill in your values:

```bash
cp agent-simulator/config.yaml my-sim.yaml
$EDITOR my-sim.yaml
```

Minimum required fields:

```yaml
kubeconfig: /path/to/upstream-kubeconfig   # or set KUBECONFIG env var
clusterNamespace: fleet-default            # registration namespace — where the Cluster CR lives
clusterName: mycluster
agentNamespace: cattle-fleet-system
bdNamespace: cluster-fleet-default-mycluster-abc12  # cluster namespace — where BundleDeployments live (Cluster.Status.Namespace)
```

### 4. Run

```bash
./bin/fleet-agent-simulator --config my-sim.yaml
```

Or override the kubeconfig on the command line:

```bash
./bin/fleet-agent-simulator --config my-sim.yaml --kubeconfig ~/.kube/upstream
```

The `KUBECONFIG` environment variable is also respected if `--kubeconfig` is not set and `kubeconfig` is empty in the config file.

Stop the simulator with `Ctrl-C` (SIGINT) or `SIGTERM`.

---

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `config.yaml` | Path to the simulator YAML config file |
| `--kubeconfig` | `$KUBECONFIG` | Path to the upstream management cluster kubeconfig |

---

## Configuration reference

```yaml
# Upstream connection
kubeconfig: ""                          # path to kubeconfig; falls back to KUBECONFIG env
clusterNamespace: fleet-default         # registration namespace where the Cluster CR lives (e.g. fleet-default or fleet-local)
clusterName: sim-cluster
agentNamespace: cattle-fleet-system     # agent deployment namespace reported in heartbeats
bdNamespace: cluster-fleet-default-sim-cluster-a1b2c3d4  # Cluster.Status.Namespace — where BundleDeployments live

# Timing
heartbeatInterval: 20s                  # default: 20s
initialDelay: 5s                        # default: 5s

# BundleDeployment simulation (Phase 2+)
resourceCount: 10                       # fake resources reported per BD; default: 10

# Gradual rollout (Phase 3)
rolloutSteps: 1                         # incremental patches before a BD is Ready; 1 = instant; default: 1
rolloutInterval: 5s                     # delay between rollout steps; default: 5s

# Drift simulation (Phase 4)
drift:
  enabled: false                        # activate the drift scheduler; default: false
  minInterval: 60s                      # minimum time between drift events per BD; default: 60s
  maxInterval: 300s                     # maximum time between drift events per BD; default: 300s
  resourceCount: 1                      # resources to mark as drifted per event; default: 1
  affectsReady: false                   # also set Ready=false when drifted; default: false
  autoRecover: false                    # automatically restore the BD after recoveryDelay; default: false
  recoveryDelay: 30s                    # delay before auto-recovery fires; default: 30s

# Failure simulation (Phase 5)
failure:
  enabled: false                        # activate the failure scheduler; default: false
  minInterval: 120s                     # minimum time between global failure ticks; default: 120s
  maxInterval: 600s                     # maximum time between global failure ticks; default: 600s
  resourceCount: 1                      # resources to mark as failed per event; default: 1
  probability: 0.1                      # per-BD probability of being selected on each tick; default: 0.1
  autoRecover: true                     # automatically restore the BD after recoveryDelay; default: true
  recoveryDelay: 60s                    # delay before auto-recovery fires; default: 60s

# Multi-cluster simulation (Phase 6)
# When clusters is non-empty all top-level fields above become global defaults.
# Each cluster entry can override any of them. The single-cluster fields
# (clusterNamespace, clusterName, bdNamespace) are ignored.
# clusters:
#   - clusterNamespace: fleet-default
#     clusterName: sim-001
#     bdNamespace: cluster-fleet-default-sim-001-abc123
#     kubeconfig: sim-cluster-1/kubeconfig.yaml   # required when using per-cluster ServiceAccounts
#   - clusterNamespace: fleet-default
#     clusterName: sim-002
#     bdNamespace: cluster-fleet-default-sim-002-def456
#     kubeconfig: sim-cluster-2/kubeconfig.yaml
#     rolloutSteps: 5            # per-cluster override; inherits everything else from global defaults
```

---

## Setting up a cluster target

The simulator skips the real agent registration bootstrap entirely. Instead it needs:

- A `Cluster` resource in a Fleet workspace (e.g. `fleet-default`).
- A cluster namespace created by the Fleet controller (`cluster-fleet-default-<name>-<hash>`).
- A kubeconfig whose ServiceAccount has the minimum RBAC to patch `Cluster/status` (and, in later phases, `BundleDeployment/status`).

### Namespace concepts

Fleet uses **two different namespaces** per cluster. Do not confuse them:

| Concept | Config key | Typical value | Contains |
|---------|-----------|--------------|---------|
| **Registration namespace** | `clusterNamespace` | `fleet-default` | The `Cluster` resource itself |
| **Cluster namespace** | `bdNamespace` | `cluster-fleet-default-<name>-<hash>` | `BundleDeployment` resources; where the ServiceAccount lives |

The heartbeat patches `Cluster/status`, so `clusterNamespace` must be the **registration namespace** (e.g. `fleet-default`). The `bdNamespace` is `Cluster.Status.Namespace` — the per-cluster namespace provisioned by the Fleet controller. Both are required.

Choose the path that fits your situation.

---

### Option A: Point at an existing registered cluster

If you already have a real cluster registered with Fleet, the namespace and Cluster resource already exist. You only need to create a dedicated ServiceAccount and kubeconfig for the simulator.

**1. Find the cluster details**

```bash
kubectl get clusters -A
# NAMESPACE      NAME         READY   ...
# fleet-default  mycluster    True
```

```bash
# The cluster namespace is stored in Cluster.Status.Namespace
CLUSTER_REG_NS=fleet-default
CLUSTER_NAME=mycluster
CLUSTER_NS=$(kubectl get cluster "$CLUSTER_NAME" -n "$CLUSTER_REG_NS" \
  -o jsonpath='{.status.namespace}')
echo "cluster namespace: $CLUSTER_NS"
# e.g. cluster-fleet-default-mycluster-abc12
```

**2. Create a simulator ServiceAccount**

```bash
kubectl create serviceaccount fleet-agent-sim -n "$CLUSTER_NS"
```

**3. Bind the minimum RBAC**

```bash
# Role: patch Cluster/status (for the heartbeat)
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: fleet-agent-sim
  namespace: $CLUSTER_REG_NS
rules:
- apiGroups: [fleet.cattle.io]
  resources: [clusters/status]
  resourceNames: [$CLUSTER_NAME]
  verbs: [patch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: fleet-agent-sim
  namespace: $CLUSTER_REG_NS
subjects:
- kind: ServiceAccount
  name: fleet-agent-sim
  namespace: $CLUSTER_NS
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: fleet-agent-sim
EOF
```

**4. Generate a kubeconfig**

```bash
# Create a long-lived token (adjust --duration as needed)
TOKEN=$(kubectl create token fleet-agent-sim -n "$CLUSTER_NS" --duration=8760h)

# Extract the server URL and CA from the current context
SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CA_DATA=$(kubectl config view --minify --raw \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')

cat > sim-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
- name: upstream
  cluster:
    server: $SERVER
    certificate-authority-data: $CA_DATA
users:
- name: fleet-agent-sim
  user:
    token: $TOKEN
contexts:
- name: default
  context:
    cluster: upstream
    user: fleet-agent-sim
current-context: default
EOF
```

**5. Configure the simulator**

```yaml
# my-sim.yaml
kubeconfig: sim-kubeconfig.yaml
clusterNamespace: fleet-default   # registration namespace — where the Cluster CR lives
clusterName: mycluster
agentNamespace: cattle-fleet-system
bdNamespace: cluster-fleet-default-mycluster-abc12  # Cluster.Status.Namespace
```

---

### Option B: Create a new simulated cluster target from scratch

No real downstream cluster is needed. You create a `Cluster` resource and the Fleet controller automatically provisions the cluster namespace and RBAC.

The `create-cluster` script automates all of the steps below:

```bash
./agent-simulator/create-cluster sim-cluster
# writes sim-cluster/kubeconfig.yaml and sim-cluster/config.yaml

./bin/fleet-agent-simulator --config sim-cluster/config.yaml
```

Options:

```
./agent-simulator/create-cluster <name> [--namespace <ns>] [--agent-namespace <ns>]
                                         [--token-duration <d>] [--out-dir <dir>]
```

To tear down: `kubectl delete cluster sim-cluster -n fleet-default`

Or follow the steps manually:

**1. Apply a Cluster resource**

```bash
kubectl apply -f - <<EOF
apiVersion: fleet.cattle.io/v1alpha1
kind: Cluster
metadata:
  name: sim-cluster
  namespace: fleet-default
  labels:
    fleet.cattle.io/cluster: sim-cluster
spec: {}
EOF
```

**2. Wait for Fleet to create the cluster namespace**

The Fleet controller sets `Cluster.Status.Namespace` once the namespace is provisioned (usually within a few seconds):

```bash
kubectl wait cluster sim-cluster -n fleet-default \
  --for=jsonpath='{.status.namespace}' --timeout=60s

CLUSTER_NS=$(kubectl get cluster sim-cluster -n fleet-default \
  -o jsonpath='{.status.namespace}')
echo "cluster namespace: $CLUSTER_NS"
# e.g. cluster-fleet-default-sim-cluster-a1b2c3d4
```

**3. Create a simulator ServiceAccount and bind the minimum RBAC**

```bash
kubectl create serviceaccount fleet-agent-sim -n "$CLUSTER_NS"

kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: fleet-agent-sim
  namespace: fleet-default
rules:
- apiGroups: [fleet.cattle.io]
  resources: [clusters/status]
  resourceNames: [sim-cluster]
  verbs: [patch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: fleet-agent-sim
  namespace: fleet-default
subjects:
- kind: ServiceAccount
  name: fleet-agent-sim
  namespace: $CLUSTER_NS
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: fleet-agent-sim
EOF
```

**4. Generate a kubeconfig**

```bash
TOKEN=$(kubectl create token fleet-agent-sim -n "$CLUSTER_NS" --duration=8760h)
SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
CA_DATA=$(kubectl config view --minify --raw \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')

cat > sim-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
- name: upstream
  cluster:
    server: $SERVER
    certificate-authority-data: $CA_DATA
users:
- name: fleet-agent-sim
  user:
    token: $TOKEN
contexts:
- name: default
  context:
    cluster: upstream
    user: fleet-agent-sim
current-context: default
EOF
```

**5. Configure and run the simulator**

```yaml
# my-sim.yaml
kubeconfig: sim-kubeconfig.yaml
clusterNamespace: fleet-default   # registration namespace — where the Cluster CR lives, NOT $CLUSTER_NS
clusterName: sim-cluster
agentNamespace: cattle-fleet-system
bdNamespace: $CLUSTER_NS          # Cluster.Status.Namespace — where BundleDeployments live
```

```bash
./bin/fleet-agent-simulator --config my-sim.yaml
```

**To remove the simulated cluster**, delete the Cluster resource and the controller will clean up the namespace and all associated RBAC:

```bash
kubectl delete cluster sim-cluster -n fleet-default
```

---

## RBAC permissions

The simulator's ServiceAccount needs different permissions depending on which phases are active.

| Phase | Resource | Namespace | Verbs |
|-------|----------|-----------|-------|
| 1 — Heartbeat | `clusters/status` | cluster registration namespace (e.g. `fleet-default`) | `patch` |
| 2+ — BD status | `bundledeployments` | cluster namespace | `get`, `list`, `watch` |
| 2+ — BD status | `bundledeployments/status` | cluster namespace | `update`, `patch` |
| 2+ — Values secrets | `secrets` | cluster namespace | `get` |

For Phase 1 only, the single `Role` + `RoleBinding` shown above is sufficient. Phases 2–5 also need access to `BundleDeployment` resources in the cluster namespace. The `fleet-bundle-deployment` ClusterRole (created automatically by the Fleet controller at startup) covers these permissions and can be bound with an additional `RoleBinding`:

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: fleet-agent-sim-bd
  namespace: $CLUSTER_NS
subjects:
- kind: ServiceAccount
  name: fleet-agent-sim
  namespace: $CLUSTER_NS
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: fleet-bundle-deployment
EOF
```

---

## What the simulator does

### Phase 1 — Heartbeat

After startup the simulator:

1. Waits `initialDelay` (default 5 s).
2. Sends an initial `Cluster/status` JSONPatch to set `Status.Agent.LastSeen` and `Status.Agent.Namespace`. This is the exact same patch format the real Fleet agent sends.
3. Repeats the patch every `heartbeatInterval` (default 20 s).
4. Shuts down cleanly on SIGINT / SIGTERM.

Without regular heartbeats the Fleet controller marks the agent as offline. The simulator keeps the simulated cluster appearing online indefinitely.

### Phase 2 & 3 — BundleDeployment status

The simulator watches `BundleDeployment` resources in the cluster namespace and patches their status. With `rolloutSteps: 1` (the default) each BD transitions to `Ready=true` immediately. With a higher value the simulator sends N incremental patches with increasing ready-resource counts, mimicking a real Helm deploy rolling out across pods.

### Phase 4 — Drift simulation

When `drift.enabled: true` the simulator runs a background scheduler that periodically injects drift into ready BundleDeployments:

- A random timer fires for each ready BD at an interval chosen uniformly in `[minInterval, maxInterval]`.
- On fire: `NonModified` is set to `false` and `ModifiedStatus` is populated with `resourceCount` synthetic patch entries (one per fake resource, e.g. `{"spec":{"replicas":3}}`). `ResourceCounts.Modified` is updated to match.
- If `affectsReady: true`, `Ready` is also set to `false` and the `Ready` condition is updated to `Reason: Drifted`.
- If `autoRecover: true`, after `recoveryDelay` the scheduler restores `NonModified: true`, clears `ModifiedStatus`, and (if `affectsReady` was set) restores `Ready: true`. After recovery a new drift timer is scheduled.
- If `autoRecover: false`, the BD stays drifted until a real deployment (DeploymentID change) triggers the reconciler to overwrite the status.

**Example — slow drift, no ready impact, auto-recover:**

```yaml
drift:
  enabled: true
  minInterval: 60s
  maxInterval: 300s
  resourceCount: 1
  affectsReady: false
  autoRecover: true
  recoveryDelay: 30s
```

**Example — aggressive drift that marks BDs not-ready, no recovery (useful for alerting tests):**

```yaml
drift:
  enabled: true
  minInterval: 10s
  maxInterval: 30s
  resourceCount: 3
  affectsReady: true
  autoRecover: false
```

### Phase 6 — Multi-cluster simulation

When the `clusters` list is non-empty the simulator runs **one independent manager per cluster** within a single binary. Each manager:

- Watches `BundleDeployment` resources only in its own cluster namespace (no cross-cluster visibility).
- Sends heartbeats for its own `Cluster` resource.
- Maintains its own rollout, drift, and failure state.

All managers share the same upstream kubeconfig and run concurrently. A single SIGINT/SIGTERM shuts them all down gracefully.

**Example — two simulated clusters, each with its own kubeconfig:**

```yaml
agentNamespace: cattle-fleet-system
heartbeatInterval: 20s
resourceCount: 10
rolloutSteps: 3

clusters:
  - clusterNamespace: fleet-default
    clusterName: sim-001
    bdNamespace: cluster-fleet-default-sim-001-abc123
    kubeconfig: sim-cluster-1/kubeconfig.yaml   # credentials for sim-001's ServiceAccount
  - clusterNamespace: fleet-default
    clusterName: sim-002
    bdNamespace: cluster-fleet-default-sim-002-def456
    kubeconfig: sim-cluster-2/kubeconfig.yaml   # credentials for sim-002's ServiceAccount
    rolloutSteps: 5                             # per-cluster override
```

If you use `create-cluster` to provision each simulated cluster, it writes a `kubeconfig.yaml` into the cluster's output directory. Reference those paths in the `clusters` list as shown above — each cluster's ServiceAccount only has RBAC access to its own namespace, so a per-cluster kubeconfig is required.

A shared top-level `kubeconfig` can still be used when a single ServiceAccount has access to all cluster namespaces (e.g. a cluster-admin credential in a dev environment). In that case omit `kubeconfig` from each cluster entry and set it at the top level.

**Per-cluster overrides** — any top-level field (`kubeconfig`, `agentNamespace`, `heartbeatInterval`, `initialDelay`, `resourceCount`, `rolloutSteps`, `rolloutInterval`, `drift`, `failure`) can be overridden inside a cluster entry. Fields not overridden are inherited from the global defaults.

---

### Phase 5 — Failure simulation

When `failure.enabled: true` the simulator runs a background scheduler that randomly transitions ready BundleDeployments to a failed state with realistic pod error messages:

- A global tick fires at a random interval chosen uniformly in `[minInterval, maxInterval]`.
- On each tick, every ready BD is evaluated independently: it is selected for failure with probability `probability`.
- For each selected BD: `Ready` is set to `false`, `NonReadyStatus` is populated with `resourceCount` synthetic pod failure entries drawn from the error pool (`CrashLoopBackOff`, `ImagePullBackOff`, `OOMKilled`, `CreateContainerError`, `ErrImageNeverPull`), and `ResourceCounts.Ready`/`NotReady` are updated accordingly. The `Ready` condition is set to `Reason: SimulatedFailure`.
- BDs that are not ready (e.g. mid-rollout, paused) are never selected.
- If `autoRecover: true` (the default), after `recoveryDelay` the scheduler restores `Ready: true`, clears `NonReadyStatus`, and resets `ResourceCounts`. The BD is then eligible for failure again on the next tick.
- If `autoRecover: false`, the BD stays failed until a new deployment (DeploymentID change) triggers the reconciler to overwrite the status.

**Example — low-probability background noise, auto-recover (good baseline for dashboards):**

```yaml
failure:
  enabled: true
  minInterval: 120s
  maxInterval: 600s
  resourceCount: 1
  probability: 0.1
  autoRecover: true
  recoveryDelay: 60s
```

**Example — high failure rate, no recovery (useful for alert and on-call runbook testing):**

```yaml
failure:
  enabled: true
  minInterval: 10s
  maxInterval: 30s
  resourceCount: 2
  probability: 0.5
  autoRecover: false
```

---

## Running tests

Unit tests (no cluster needed):

```bash
go test ./agent-simulator/pkg/config/...
go test ./agent-simulator/pkg/status/...
go test ./agent-simulator/pkg/resources/...
go test ./agent-simulator/pkg/rollout/...
go test ./agent-simulator/pkg/chaos/...   # drift + failure unit tests
```

Integration tests (uses envtest — requires `setup-envtest`):

```bash
SETUP_ENVTEST_VER=v0.0.0-20251014082336-b8f11375258f
ENVTEST_K8S_VERSION=1.34

go install sigs.k8s.io/controller-runtime/tools/setup-envtest@"$SETUP_ENVTEST_VER"
KUBEBUILDER_ASSETS=$(setup-envtest use --use-env -p path "$ENVTEST_K8S_VERSION")
export KUBEBUILDER_ASSETS

ginkgo ./agent-simulator/pkg/heartbeat/...
ginkgo ./agent-simulator/pkg/simulator/...   # includes drift + failure integration tests
```

Run everything at once:

```bash
go test ./agent-simulator/...
```

---

## Project layout

```
agent-simulator/
  build                       # build the simulator binary
  create-cluster              # provision a simulated cluster + generate kubeconfig and config
  config.yaml                 # example configuration
  cmd/
    simulator/
      main.go                 # entry point: flag parsing, client setup, signal handling
  pkg/
    config/
      config.go               # Config struct, YAML loader, defaults, validation
      config_test.go
      multi_cluster_test.go   # multi-cluster config merging tests
    heartbeat/
      heartbeat.go            # Cluster/status JSONPatch (Ticker + Patch)
      heartbeat_test.go       # envtest integration tests
      suite_test.go           # envtest setup/teardown
    simulator/
      simulator.go            # controller-runtime manager + BD reconciler
      simulator_test.go       # envtest integration tests (rollout)
      drift_integration_test.go           # envtest integration tests (drift)
      failure_integration_test.go         # envtest integration tests (failure)
      multi_cluster_integration_test.go   # envtest integration tests (multi-cluster)
      suite_test.go           # envtest setup/teardown
    status/
      status.go               # BundleDeploymentStatus builder
      status_test.go
    resources/
      resources.go            # deterministic fake resource generator
      resources_test.go
    rollout/
      rollout.go              # per-BD rollout state machine
      rollout_test.go
    chaos/
      drift.go                # DriftScheduler (Phase 4)
      drift_test.go           # unit tests for RandomInterval and BuildModifiedStatus
      failure.go              # FailureScheduler (Phase 5)
      failure_test.go         # unit tests for BuildNonReadyStatus
  plan.md                     # phased implementation plan
  communication.md            # Fleet agent communication protocol reference
```

---

## Roadmap

| Phase | Status | Description |
|-------|--------|-------------|
| 1 — Heartbeat | ✅ Done | Connects to management cluster, sends periodic `Cluster/status` heartbeats |
| 2 — BD Watch + Instant Ready | ✅ Done | Watches `BundleDeployment` resources and immediately responds with a fully-ready status |
| 3 — Gradual Rollout | ✅ Done | Sends N incremental status updates with increasing ready resource counts |
| 4 — Drift Simulation | ✅ Done | Periodically reports drift on ready BundleDeployments with optional auto-recovery |
| 5 — Failure Simulation | ✅ Done | Randomly transitions ready BDs to a failed state with realistic pod error messages |
| 6 — Multi-Cluster | ✅ Done | Single binary simulating multiple independent agent clusters |
| 7 — Observability | Planned | Prometheus metrics, structured logging, dry-run mode |

See [plan.md](./plan.md) for full implementation details and [communication.md](./communication.md) for the Fleet agent communication protocol reference.
