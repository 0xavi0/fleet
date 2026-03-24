# Fleet Agent Simulator

A standalone binary that impersonates one or more Fleet agents against a management cluster without running any real downstream Kubernetes cluster.

Useful for:
- **Load / scale testing** — spin up hundreds of simulated agents to stress-test the Fleet controller.
- **Development** — iterate on controller logic without needing real downstream clusters.
- **Demo environments** — produce realistic agent activity (heartbeats, BundleDeployment status updates) from a laptop.

The simulator speaks the exact same API as a real Fleet agent:
- Periodic `Cluster/status` JSONPatch heartbeats (Phase 1)
- `BundleDeployment/status` patches with realistic ready/non-ready/drifted states (Phases 2-5)

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
clusterNamespace: cluster-fleet-default-mycluster-abc12
clusterName: mycluster
agentNamespace: cattle-fleet-system
```

Optional timing overrides (shown with defaults):

```yaml
heartbeatInterval: 20s   # how often to send Cluster.Status.Agent heartbeat
initialDelay: 5s         # pause before the first heartbeat
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

# Timing
heartbeatInterval: 20s                  # default: 20s
initialDelay: 5s                        # default: 5s
```

---

## Setting up a cluster target

The simulator skips the real agent registration bootstrap entirely. Instead it needs:

- A `Cluster` resource in a Fleet workspace (e.g. `fleet-default`).
- A cluster namespace created by the Fleet controller (`cluster-fleet-default-<name>-<hash>`).
- A kubeconfig whose ServiceAccount has the minimum RBAC to patch `Cluster/status` (and, in later phases, `BundleDeployment/status`).

### Namespace concepts

Fleet uses **two different namespaces** per cluster. Do not confuse them:

| Concept | Typical value | Contains |
|---------|--------------|---------|
| **Registration namespace** — `clusterNamespace` in the simulator config | `fleet-default` | The `Cluster` resource itself |
| **Cluster namespace** — `cluster.Status.Namespace` | `cluster-fleet-default-<name>-<hash>` | `BundleDeployment` resources; where the ServiceAccount lives |

The heartbeat patches `Cluster/status`, so `clusterNamespace` in the config must be the **registration namespace** (e.g. `fleet-default`), not the cluster namespace.

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

For Phase 1 only (current), the single `Role` + `RoleBinding` shown above is sufficient. The `fleet-bundle-deployment` ClusterRole (created automatically by the Fleet controller at startup) covers the Phase 2+ permissions and can be bound with an additional `RoleBinding`:

```bash
# Add when implementing Phase 2+
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

## What the simulator does (Phase 1)

After startup the simulator:

1. Waits `initialDelay` (default 5 s).
2. Sends an initial `Cluster/status` JSONPatch to set `Status.Agent.LastSeen` and `Status.Agent.Namespace`. This is the exact same patch format the real Fleet agent sends.
3. Repeats the patch every `heartbeatInterval` (default 20 s).
4. Shuts down cleanly on SIGINT / SIGTERM.

Without regular heartbeats the Fleet controller marks the agent as offline. The simulator keeps the simulated cluster appearing online indefinitely.

---

## Running tests

Unit tests (no cluster needed):

```bash
go test ./agent-simulator/pkg/config/...
```

Integration tests (uses envtest — requires `setup-envtest`):

```bash
SETUP_ENVTEST_VER=v0.0.0-20251014082336-b8f11375258f
ENVTEST_K8S_VERSION=1.34

go install sigs.k8s.io/controller-runtime/tools/setup-envtest@"$SETUP_ENVTEST_VER"
KUBEBUILDER_ASSETS=$(setup-envtest use --use-env -p path "$ENVTEST_K8S_VERSION")
export KUBEBUILDER_ASSETS

go test ./agent-simulator/pkg/heartbeat/...
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
    heartbeat/
      heartbeat.go            # Cluster/status JSONPatch (Ticker + Patch)
      heartbeat_test.go       # envtest integration tests
      suite_test.go           # envtest setup/teardown
  plan.md                     # phased implementation plan
  communication.md            # Fleet agent communication protocol reference
```

---

## Roadmap

| Phase | Status | Description |
|-------|--------|-------------|
| 1 — Heartbeat | ✅ Done | Connects to management cluster, sends periodic `Cluster/status` heartbeats |
| 2 — BD Watch + Instant Ready | Planned | Watches `BundleDeployment` resources and immediately responds with a fully-ready status |
| 3 — Gradual Rollout | Planned | Sends N incremental status updates with increasing ready resource counts |
| 4 — Drift Simulation | Planned | Periodically reports drift on ready BundleDeployments with optional auto-recovery |
| 5 — Failure Simulation | Planned | Randomly transitions ready BDs to a failed state with realistic error messages |
| 6 — Multi-Cluster | Planned | Single binary simulating multiple independent agent clusters |
| 7 — Observability | Planned | Prometheus metrics, structured logging, dry-run mode |

See [plan.md](./plan.md) for full implementation details and [communication.md](./communication.md) for the Fleet agent communication protocol reference.
