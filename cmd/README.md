# Hive Simulator

A standalone service that simulates Hive and its operators (aws-account-operator, gcp-project-operator) with configurable behavior for testing and development of the OCM Clusters Service.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Usage Examples](#usage-examples)
- [Development](#development)

## Overview

The Hive Simulator provides a lightweight, controllable environment for testing cluster provisioning flows without requiring a real Hive installation. It simulates:

- **Hive ClusterDeployment** - Cluster provisioning lifecycle
- **AWS Account Operator (AccountClaim)** - AWS account provisioning
- **GCP Project Operator (ProjectClaim)** - GCP project provisioning
- **ClusterImageSet** - Available OpenShift versions

### Key Features

- **Realistic State Transitions**: Follows actual Hive behavior with proper dependencies
- **Configurable Timing**: Control how long each state transition takes
- **Failure Injection**: Test error scenarios and edge cases
- **Runtime Configuration**: Modify behavior via REST API without restart
- **Lightweight**: Uses envtest (etcd + kube-apiserver) for minimal overhead

## Prerequisites

### Envtest Binaries

The Hive Simulator uses **envtest** from Kubernetes controller-runtime to provide a real Kubernetes API server for testing. Envtest requires Kubernetes binaries (etcd and kube-apiserver).

**What is Envtest?**

Envtest is a testing library that runs a real, lightweight Kubernetes control plane:
- Real etcd for storage
- Real kube-apiserver for API semantics
- Support for Custom Resource Definitions
- No kubelet, scheduler, or controllers (just the API)

This gives the simulator realistic Kubernetes API behavior without needing a full cluster.

**Setup (Automatic via Makefile)**

The easiest way is to use the Makefile targets:

```bash
# This automatically installs setup-envtest and downloads K8s binaries
make envtest-setup

# Then run the simulator
make run-hive-simulator
```

The Makefile will:
1. Install the `setup-envtest` tool to `bin/setup-envtest`
2. Download Kubernetes 1.28.0 binaries (~80MB) to `bin/k8s/`
3. Set the `KUBEBUILDER_ASSETS` environment variable automatically

**Manual Setup**

If you need to set up envtest manually:

```bash
# Install setup-envtest
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Download and install K8s binaries
setup-envtest use 1.28.0

# Get the path to binaries
export KUBEBUILDER_ASSETS=$(setup-envtest use 1.28.0 -p path)

# Run the simulator
./bin/hive-simulator
```

**Troubleshooting**

If you see this error:
```
fork/exec /usr/local/kubebuilder/bin/etcd: no such file or directory
```

This means envtest binaries are not installed. Run `make envtest-setup` to fix it.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│              Hive Simulator Service                 │
├─────────────────────────────────────────────────────┤
│                                                     │
│  ┌─────────────────────────────────────────────┐  │
│  │     Kubernetes API Server (envtest)         │  │
│  │  - Lightweight etcd storage                 │  │
│  │  - CRD definitions (Hive, AAO, GPO)         │  │
│  │  - Standard k8s API semantics               │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│  ┌─────────────────────────────────────────────┐  │
│  │     Mock Controllers (Reconcilers)          │  │
│  │  • ClusterDeployment Controller             │  │
│  │  • AccountClaim Controller                  │  │
│  │  • ProjectClaim Controller                  │  │
│  │  • ClusterImageSet Pre-population           │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│  ┌─────────────────────────────────────────────┐  │
│  │     Configuration API (REST)                │  │
│  │  - Configure timing/delays                  │  │
│  │  - Inject failures                          │  │
│  │  - Override behavior per resource           │  │
│  │  - Reset state                              │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│  ┌─────────────────────────────────────────────┐  │
│  │     Behavior Engine                         │  │
│  │  - State machines for each CR type          │  │
│  │  - Resource dependency management           │  │
│  │  - Failure injection logic                  │  │
│  └─────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### Resource Dependencies

The simulator respects real-world dependencies:

1. **AccountClaim (AWS)**:
   - Created first for AWS clusters
   - Must reach `Ready` state before ClusterDeployment progresses

2. **ProjectClaim (GCP)**:
   - Created first for GCP clusters
   - Must reach `Ready` state before ClusterDeployment progresses

3. **ClusterDeployment**:
   - Waits for AccountClaim/ProjectClaim to be ready
   - Progresses through: Pending → Provisioning → Installing → Running
   - Sets `Spec.Installed=true` when ready
   - Populates InfraId, API URL, Console URL

### State Machines

#### ClusterDeployment States

```
Pending
  ↓ (1s default)
Provisioning
  - Condition: DeprovisionLaunchError=False
  ↓ (2s default)
Installing
  - Condition: DNSNotReady=False (DNS Ready)
  ↓ (1s default)
Running
  - Spec.Installed=true
  - InfraId populated
  - API/Console URLs set
  - Condition: ClusterDeploymentCompleted=True
```

#### AccountClaim States

```
Pending
  ↓ (2s default)
Ready
  - Status.State=Ready
  - Condition: Claimed=True
```

#### ProjectClaim States

```
Pending
  ↓ (1s default)
PendingProject
  ↓ (2s default)
Ready
  - Status.State=Ready
  - Condition: Ready=True
```

## Quick Start

### Build and Run

```bash
# Step 1: Set up envtest (only needed once)
make envtest-setup

# Step 2: Build and run the simulator (uses default 5-second provisioning)
make run-hive-simulator

# Or run with fast configuration (1-second provisioning)
make run-hive-simulator-fast
```

### Connect Clusters Service

Update your provision shard configuration to point to the simulator:

```bash
# Get the kubeconfig from simulator logs
# The simulator outputs the path to kubeconfig on startup

# Use the helper script to generate provision shard configuration
cd cmd/hive-simulator
./generate-provision-shard-config.sh

# This creates provision_shards_simulator.yaml with the kubeconfig content inline
# Start clusters-service with the provision shard config
../../bin/clusters-service serve --provision-shards-config provision_shards_simulator.yaml

# See KUBECONFIG.md for manual configuration details
```

#### Important: Dynamic Port Behavior

**The Kubernetes API server uses a random port on each start.** This is a limitation of envtest - it doesn't support static ports without accessing internal (non-public) APIs.

**What this means:**
- Each time you restart the simulator, the API server gets a new port
- You must regenerate the provision shard config after simulator restarts
- The REST API (port 8080) is always static

**Easy Restart Workflow:**

Use the `restart-simulator.sh` script to automatically handle everything:

```bash
cd cmd/hive-simulator
./restart-simulator.sh
```

This script will:
1. Stop any running simulator processes
2. Start a fresh simulator instance
3. Wait for it to be ready
4. Regenerate the provision shard configuration automatically

After the script completes, just restart clusters-service to pick up the new configuration.

## Configuration

### Configuration File

Create a YAML configuration file to customize behavior:

```yaml
# hive-simulator-config.yaml

clusterDeployment:
  defaultDelaySeconds: 5
  dependsOnAccountClaim: true
  dependsOnProjectClaim: true
  states:
    - name: Pending
      durationSeconds: 1
    - name: Provisioning
      durationSeconds: 2
      conditions:
        - type: DeprovisionLaunchError
          status: "False"
          reason: Provisioning
          message: "Cluster is provisioning"
    - name: Installing
      durationSeconds: 1
      conditions:
        - type: DNSNotReady
          status: "False"
          reason: DNSReady
          message: "DNS is ready"
    - name: Running
      durationSeconds: 1
      conditions:
        - type: ClusterDeploymentCompleted
          status: "True"
          reason: ClusterDeploymentCompleted
          message: "Cluster deployment is complete"
  failureScenarios:
    - probability: 0.1  # 10% chance of failure
      condition: ProvisionFailed
      message: "Simulated AWS capacity error"
      reason: InsufficientCapacity

accountClaim:
  defaultDelaySeconds: 3
  states:
    - name: Pending
      durationSeconds: 2
    - name: Ready
      durationSeconds: 1

projectClaim:
  defaultDelaySeconds: 4
  states:
    - name: Pending
      durationSeconds: 1
    - name: PendingProject
      durationSeconds: 2
    - name: Ready
      durationSeconds: 1

clusterImageSets:
  - name: "openshift-v4.12.0"
    visible: true
  - name: "openshift-v4.13.0"
    visible: true
  - name: "openshift-v4.14.0"
    visible: true
  - name: "openshift-v4.15.0"
    visible: true
```

### Environment Variables

```bash
# API port for configuration endpoint
HIVE_SIMULATOR_API_PORT=8080

# Kubernetes API port
HIVE_SIMULATOR_KUBE_PORT=6443

# Log level (debug, info, warn, error)
HIVE_SIMULATOR_LOG_LEVEL=debug

# Configuration file path
HIVE_SIMULATOR_CONFIG=/path/to/config.yaml
```

## API Reference

The simulator exposes a REST API for runtime configuration on port 8080 (configurable).

### Global Configuration

#### Get Current Configuration
```bash
GET /api/v1/config
```

Response:
```json
{
  "clusterDeployment": {
    "defaultDelaySeconds": 5,
    "states": [...],
    "dependsOnAccountClaim": true,
    "dependsOnProjectClaim": true
  },
  "accountClaim": {...},
  "projectClaim": {...}
}
```

#### Update ClusterDeployment Configuration
```bash
POST /api/v1/config/clusterdeployment
Content-Type: application/json

{
  "defaultDelaySeconds": 10
}
```

#### Update AccountClaim Configuration
```bash
POST /api/v1/config/accountclaim
Content-Type: application/json

{
  "defaultDelaySeconds": 5
}
```

#### Update ProjectClaim Configuration
```bash
POST /api/v1/config/projectclaim
Content-Type: application/json

{
  "defaultDelaySeconds": 6
}
```

### Per-Resource Overrides

#### Force Failure for Specific ClusterDeployment
```bash
POST /api/v1/overrides/clusterdeployment/{namespace}/{name}/failure
Content-Type: application/json

{
  "condition": "ProvisionFailed",
  "message": "AWS VPC limit exceeded",
  "reason": "AWSVPCLimitExceeded"
}
```

#### Override Delay for Specific Resource
```bash
POST /api/v1/overrides/clusterdeployment/{namespace}/{name}/delay
Content-Type: application/json

{
  "delaySeconds": 30
}
```

#### Force Success (Skip Probabilistic Failures)
```bash
POST /api/v1/overrides/clusterdeployment/{namespace}/{name}/success
```

#### Clear Overrides for Resource
```bash
DELETE /api/v1/overrides/clusterdeployment/{namespace}/{name}
```

### State Management

#### Reset All State
```bash
POST /api/v1/reset
```

Clears all overrides and resets to configuration file defaults.

#### Get Simulator Status
```bash
GET /api/v1/status
```

Response:
```json
{
  "healthy": true,
  "uptime": "1h23m45s",
  "resources": {
    "clusterDeployments": 5,
    "accountClaims": 3,
    "projectClaims": 2
  }
}
```

## Usage Examples

### Example 1: Basic Local Development

```bash
# Terminal 1: Start simulator
./bin/hive-simulator --config config/hive-simulator.yaml

# Terminal 2: Generate provision shard config
cd cmd/hive-simulator
./generate-provision-shard-config.sh
cd ../..

# Terminal 3: Start clusters-service with simulator provision shard
./bin/clusters-service serve --provision-shards-config cmd/hive-simulator/provision_shards_simulator.yaml

# Terminal 4: Create a cluster via OCM API
# The cluster will provision through the simulator
```

### Example 2: Test Failure Scenarios

```bash
# Start simulator
./bin/hive-simulator

# Force next ClusterDeployment to fail
curl -X POST http://localhost:8080/api/v1/config/clusterdeployment \
  -H "Content-Type: application/json" \
  -d '{
    "failureScenarios": [{
      "probability": 1.0,
      "condition": "ProvisionFailed",
      "message": "Testing failure handling",
      "reason": "TestFailure"
    }]
  }'

# Create cluster - it will fail as configured
# Test clusters-service error handling
```

### Example 3: Slow Provisioning Test

```bash
# Simulate slow provisioning (60 seconds)
curl -X POST http://localhost:8080/api/v1/config/clusterdeployment \
  -H "Content-Type: application/json" \
  -d '{
    "defaultDelaySeconds": 60
  }'

# Create cluster and verify clusters-service handles long provisioning
```

### Example 4: Per-Cluster Override

```bash
# Force specific cluster to fail
curl -X POST http://localhost:8080/api/v1/overrides/clusterdeployment/default/my-cluster-abc123/failure \
  -H "Content-Type: application/json" \
  -d '{
    "condition": "ProvisionFailed",
    "message": "This specific cluster should fail",
    "reason": "CustomTestFailure"
  }'

# Other clusters will succeed, only my-cluster-abc123 will fail
```

## Troubleshooting

### Envtest Binary Not Found

**Error:**
```
Server failed: failed to setup envtest: failed to start envtest:
unable to start control plane itself: failed to start the controlplane.
retried 5 times: fork/exec /usr/local/kubebuilder/bin/etcd: no such file or directory
```

**Cause:** Envtest binaries (etcd, kube-apiserver) are not installed.

**Solution:**
```bash
# Install envtest binaries
make envtest-setup

# Then run the simulator
make run-hive-simulator
```

### Port Already in Use

**Error:**
```
API server failed: listen tcp :8080: bind: address already in use
```

**Cause:** Another process is using port 8080.

**Solution:**
```bash
# Option 1: Kill the process using port 8080
lsof -ti:8080 | xargs kill

# Option 2: Use a different port
./bin/hive-simulator --api-port 8081
```

### Kubeconfig Not Generated

**Symptom:** `/tmp/hive-simulator-kubeconfig.yaml` doesn't exist.

**Cause:** Simulator failed to start or envtest setup failed.

**Solution:**
1. Check simulator logs for errors during startup
2. Ensure envtest binaries are installed: `make envtest-setup`
3. Check that envtest started successfully (look for "Envtest started" in logs)

### Connection Refused from Clusters Service

**Error:** Clusters service can't connect to the simulator.

**Cause:**
- Simulator not running
- Wrong server address in kubeconfig
- Certificates from old simulator instance

**Solution:**
```bash
# 1. Verify simulator is running
ps aux | grep hive-simulator

# 2. Regenerate provision shard config
cd cmd/hive-simulator
./generate-provision-shard-config.sh

# 3. Restart clusters-service with new config
cd ../..
./bin/clusters-service serve --provision-shards-config cmd/hive-simulator/provision_shards_simulator.yaml
```

### KUBEBUILDER_ASSETS Not Set (Manual Run)

**Error:**
```
fork/exec /usr/local/kubebuilder/bin/etcd: no such file or directory
```

**Cause:** Running simulator manually without setting `KUBEBUILDER_ASSETS`.

**Solution:**
```bash
# Option 1: Use Makefile (sets automatically)
make run-hive-simulator

# Option 2: Set manually
export KUBEBUILDER_ASSETS=$(bin/setup-envtest use 1.28.0 -p path)
./bin/hive-simulator
```

### Go Version Mismatch

**Error:**
```
go: sigs.k8s.io/controller-runtime/tools/setup-envtest requires go >= 1.25.0
```

**Cause:** The setup-envtest tool requires a newer Go version than what you have.

**Solution:** This is usually not an issue - Go will automatically download the required version for the tool. If it fails, update your Go installation.

### Envtest Binaries Download Fails

**Error:** Timeout or network error downloading envtest binaries.

**Solution:**
```bash
# Retry with explicit version
make ENVTEST_K8S_VERSION=1.28.0 envtest-setup

# Or download manually
bin/setup-envtest use 1.28.0 --bin-dir bin/k8s
```

## Development

### Running Tests

```bash
# Run simulator tests
make test-hive-simulator

# Run integration tests with simulator
make integration-test-simulator
```

### Adding New Custom Resources

To add support for additional CRs:

1. Add CR type definition to `pkg/hive_simulator/state_machine/`
2. Create controller in `pkg/hive_simulator/controllers/`
3. Add configuration in `pkg/hive_simulator/config/config.go`
4. Register controller in `pkg/hive_simulator/server.go`
5. Update documentation

### Debug Logging

Enable debug logging to see detailed state transitions:

```bash
./bin/hive-simulator --log-level debug
```

Debug logs include:
- State transition events
- Resource creation/updates
- Condition changes
- Dependency checks
- API requests

### Troubleshooting

#### Simulator won't start

Check that ports are available:
```bash
lsof -i :8080  # API port
lsof -i :6443  # Kubernetes API port
```

#### Clusters-service can't connect

Verify kubeconfig path:
```bash
export KUBECONFIG=/tmp/hive-simulator-kubeconfig.yaml
kubectl get clusterdeployments
```

#### Resources stuck in Pending

Check logs for dependency issues:
```bash
./bin/hive-simulator --log-level debug 2>&1 | grep "dependency"
```

## CI/CD Integration

### Using in CI Pipelines

```yaml
# .gitlab-ci.yml example
test-with-simulator:
  script:
    - make hive-simulator
    - ./bin/hive-simulator --config ci-config.yaml &
    - SIMULATOR_PID=$!
    - export KUBECONFIG=/tmp/hive-simulator-kubeconfig.yaml
    - make integration-test
    - kill $SIMULATOR_PID
```

### Fast CI Configuration

For CI, use a fast configuration to speed up tests:

```yaml
# ci-config.yaml
clusterDeployment:
  defaultDelaySeconds: 1
  dependsOnAccountClaim: false  # Skip dependencies for speed
  dependsOnProjectClaim: false

accountClaim:
  defaultDelaySeconds: 1

projectClaim:
  defaultDelaySeconds: 1
```

## Roadmap

Future enhancements planned:

- [ ] SyncSet/SelectorSyncSet support
- [ ] Advanced failure injection (network issues, API throttling)
- [ ] Event emission to match real Hive
- [ ] Prometheus metrics endpoint
- [ ] Multi-shard simulation
- [ ] Timeline recording and replay
- [ ] Scenario presets (upgrade flows, migrations, etc.)
- [ ] Chaos mode for random failures

## Contributing

See main [CONTRIBUTING.md](../../CONTRIBUTING.md) for contribution guidelines.

## License

See main repository LICENSE file.
