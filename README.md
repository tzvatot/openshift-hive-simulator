# OpenShift Hive Simulator

A lightweight Kubernetes testing environment that simulates OpenShift Hive and related operators for OCM (OpenShift Cluster Manager) integration testing, specifically designed for the clusters-service.

## Overview

The Hive Simulator provides a complete testing environment for OCM ([OpenShift Cluster Manager API](https://api.openshift.com)) clusters-service without requiring a full Kubernetes cluster or cloud provider infrastructure. It uses [envtest](https://book.kubebuilder.io/reference/envtest.html) to run a real Kubernetes API server with etcd, and simulates the behavior of:

- **[Hive](https://github.com/openshift/hive)** - OpenShift cluster provisioning operator
- **[AWS Account Operator](https://github.com/openshift/aws-account-operator)** - AWS account management for OSD
- **[GCP Project Operator](https://github.com/openshift/gcp-project-operator)** - GCP project management for OSD

## Supported Custom Resources

The simulator fully supports the following CRDs that OCM clusters-service depends on:

### ClusterDeployment (Hive)
Represents an OpenShift cluster being provisioned via Hive. The simulator:
- Progresses through realistic states: Pending → Provisioning → Installing → Running
- Sets appropriate conditions at each state
- Respects AccountClaim/ProjectClaim dependencies
- Supports configurable timing and failure scenarios

### ClusterImageSet (Hive)
Represents available OpenShift versions. The simulator:
- Pre-populates ClusterImageSets from configuration
- Adds `api.openshift.com/version` annotation (used by clusters-service for `raw_id`)
- Adds `api.openshift.com/channel-group` label (stable, candidate, fast, nightly)
- Supports all version naming conventions:
  - Stable: `openshift-v4.17.0`
  - Candidate: `openshift-v4.17.0-ec.0-candidate`
  - Fast: `openshift-v4.17.0-fc.0-fast`
  - Nightly: `openshift-v4.17.0-0.nightly-2024-08-01-120000-nightly`

### AccountClaim (AWS Account Operator)
Represents AWS account allocation for a cluster. The simulator:
- Progresses from Pending → Ready
- Links to ClusterDeployment via `api.openshift.com/id` label
- Supports configurable timing

### ProjectClaim (GCP Project Operator)
Represents GCP project allocation for a cluster. The simulator:
- Progresses through Pending → PendingProject → Ready
- Links to ClusterDeployment via `api.openshift.com/id` label
- Supports configurable timing

## Quick Start

### Prerequisites

- Go 1.24.7 or later
- Make
- setup-envtest (installed automatically)

### Installation

```bash
# Clone the repository
git clone https://github.com/tzvatot/openshift-hive-simulator.git
cd openshift-hive-simulator

# Download dependencies
make deps

# Install envtest binaries
make setup-envtest
```

### Building

```bash
# Build the simulator
make build
```

### Running

```bash
# Run the simulator
make run
```

The simulator will:
1. Start a Kubernetes API server on a dynamic port
2. Write kubeconfig to `/tmp/hive-simulator-kubeconfig.yaml`
3. Start a configuration API on port 8080
4. Pre-populate 11 ClusterImageSets with proper OCM labels/annotations

### Cloud Provider Credentials

The simulator can use real cloud provider credentials to enable full cluster provisioning through clusters-service:

**AWS Credentials:**
```bash
export AWS_ACCESS_KEY_ID="your-access-key-id"
export AWS_SECRET_ACCESS_KEY="your-secret-access-key"
make run
```

**GCP Credentials:**
```bash
export GCP_SERVICE_ACCOUNT_JSON='{"type":"service_account",...}'
make run
```

**Note:** If these environment variables are not set, the simulator will use placeholder credentials. This allows basic testing but clusters will fail AWS/GCP credential validation in clusters-service.

### Accessing the Simulated Cluster

```bash
# Use kubectl with the generated kubeconfig
export KUBECONFIG=/tmp/hive-simulator-kubeconfig.yaml
kubectl get clusterimageset
kubectl get clusterdeployment
```

## OCM Clusters-Service Integration

### Provision Shard Configuration

To use the simulator as a Hive provision shard for clusters-service, generate the provision shard configuration:

```bash
# Generate provision shard config
./cmd/generate-provision-shard-config.sh

# This creates: provision_shards_simulator.yaml
```

The generated configuration includes:
- `hive_config`: Kubeconfig for Hive API access
- `aws_account_operator_config`: Kubeconfig for AWS Account Operator
- `gcp_project_operator_config`: Kubeconfig for GCP Project Operator

**Note**: The simulator uses dynamic ports. After each restart, regenerate the provision shard configuration.

### Restart Helper Script

For development, use the restart script to automatically regenerate configuration:

```bash
./cmd/restart-simulator.sh
```

This script:
1. Stops any running simulator processes
2. Cleans up envtest processes
3. Rebuilds the simulator
4. Starts the simulator
5. Automatically regenerates provision shard configuration

## Configuration

The simulator behavior is controlled via `config/hive-simulator.yaml`:

### ClusterDeployment Configuration

```yaml
clusterDeployment:
  defaultDelaySeconds: 5  # Total time from creation to ready
  dependsOnAccountClaim: true  # Wait for AccountClaim before progressing
  dependsOnProjectClaim: true  # Wait for ProjectClaim before progressing

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
    # ... more states
```

### ClusterImageSets

Pre-populate ClusterImageSets with OCM-compatible labels:

```yaml
clusterImageSets:
  - name: "openshift-v4.17.0"
    visible: true
  - name: "openshift-v4.17.0-ec.0-candidate"
    visible: true
  # ... more versions
```

Each ClusterImageSet automatically gets:
- `api.openshift.com/version`: Extracted version (e.g., "4.17.0")
- `api.openshift.com/channel-group`: Inferred channel (stable/candidate/fast/nightly)

### Failure Scenarios (Optional)

Simulate random failures for testing error handling:

```yaml
clusterDeployment:
  failureScenarios:
    - probability: 0.1  # 10% chance
      condition: ProvisionFailed
      message: "Simulated AWS capacity error"
      reason: InsufficientCapacity
```

## API Endpoints

The simulator exposes a REST API on port 8080:

- `GET /api/hive-simulator/v1/health` - Health check
- `GET /api/hive-simulator/v1/config` - Current configuration
- `PUT /api/hive-simulator/v1/config` - Update configuration dynamically

## Architecture

```
┌─────────────────────────────────────────┐
│         Hive Simulator                  │
├─────────────────────────────────────────┤
│  ┌─────────────────────────────────┐   │
│  │   Envtest Environment           │   │
│  │   (etcd + kube-apiserver)       │   │
│  └─────────────────────────────────┘   │
│                                         │
│  ┌─────────────────────────────────┐   │
│  │   Controller Manager            │   │
│  │   - ClusterDeployment           │   │
│  │   - AccountClaim                │   │
│  │   - ProjectClaim                │   │
│  └─────────────────────────────────┘   │
│                                         │
│  ┌─────────────────────────────────┐   │
│  │   State Machines                │   │
│  │   - Configurable progression    │   │
│  │   - Behavior engine             │   │
│  └─────────────────────────────────┘   │
│                                         │
│  ┌─────────────────────────────────┐   │
│  │   Configuration API (8080)      │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
```

## Development

### Project Structure

```
.
├── cmd/
│   └── main.go                      # Entry point
├── pkg/
│   ├── server.go                    # Main simulator server
│   ├── api/                         # REST API handlers
│   ├── behavior/                    # Behavior engine
│   ├── config/                      # Configuration types
│   ├── controllers/                 # Kubernetes controllers
│   ├── state_machine/               # State machines for CRs
│   ├── labels/                      # OCM label constants
│   └── externalapis/                # Vendored CRD types
│       ├── aws-account-operator/
│       └── gcp-project-operator/
├── crds/                            # CRD definitions
└── config/
    └── hive-simulator.yaml          # Default configuration
```

### Testing

```bash
# Run unit tests
make test

# Test with a real clusters-service instance
./bin/clusters-service serve --provision-shards-config provision_shards_simulator.yaml
```

## Use Cases

1. **Local Development**: Test clusters-service changes without cloud infrastructure
2. **Integration Testing**: Validate ClusterDeployment provisioning logic
3. **CI/CD**: Fast, reliable testing in CI pipelines
4. **Failure Testing**: Simulate various failure scenarios
5. **Performance Testing**: Test clusters-service under load

## Key Features

- ✅ **Fast**: No real cloud resources, starts in seconds
- ✅ **Realistic**: Uses real Kubernetes API and CRDs
- ✅ **Configurable**: Adjust timing, states, and failures
- ✅ **OCM Compatible**: Proper labels, annotations, and version formats
- ✅ **Stateful**: Controllers maintain CR state across reconciliations
- ✅ **Dynamic**: Update behavior without restart via API

## Troubleshooting

### Port Already in Use
Envtest uses dynamic ports. If you see port conflicts, ensure no other simulator instances are running:

```bash
pkill -9 -f "bin/hive-simulator"
```

### ClusterImageSet Version Errors
Ensure ClusterImageSet names follow the supported patterns. The simulator extracts versions and channels from the name.

### CRDs Not Loading
Verify the `crds/` directory exists and contains the CRD YAML files. The simulator looks for CRDs relative to the binary location.

## References

- **[OpenShift Hive](https://github.com/openshift/hive)** - OpenShift cluster provisioning operator
- **[OCM API Documentation](https://api.openshift.com)** - OpenShift Cluster Manager API
- **[AWS Account Operator](https://github.com/openshift/aws-account-operator)** - Manages AWS accounts for OSD clusters
- **[GCP Project Operator](https://github.com/openshift/gcp-project-operator)** - Manages GCP projects for OSD clusters
- **[Envtest](https://book.kubebuilder.io/reference/envtest.html)** - Integration testing with real Kubernetes API
- **[Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)** - Kubernetes controller framework

## License

Apache License 2.0

## Contributing

Contributions welcome! Please open issues or pull requests on GitHub.
