# Provision Shard Integration Guide

## Overview

The Hive Simulator integrates with OCM Clusters Service through **provision shards**. This guide explains how provision shards work and how to configure them to use the simulator.

## How Provision Shards Work

Provision shards are configurations that tell clusters-service how to connect to different Hive/Hypershift/Maestro instances. They are configured via a YAML file passed to clusters-service at startup.

**Key Point**: Provision shards store the **kubeconfig content** (not a file path) directly in the YAML configuration.

## Quick Setup (Recommended)

Use the provided helper script to automatically generate the provision shard configuration:

```bash
# 1. Start the simulator
make run-hive-simulator

# 2. Generate provision shard config
cd cmd/hive-simulator
./generate-provision-shard-config.sh

# 3. Start clusters-service with the simulator
cd ../..
./bin/clusters-service serve --provision-shards-config cmd/hive-simulator/provision_shards_simulator.yaml
```

## What the Helper Script Does

The `generate-provision-shard-config.sh` script:

1. Reads the kubeconfig from `/tmp/hive-simulator-kubeconfig.yaml`
2. Embeds the kubeconfig content into a provision shard YAML file
3. Creates `provision_shards_simulator.yaml` ready to use with clusters-service

## Manual Configuration

If you need to manually create the provision shard configuration:

### Step 1: Get the Kubeconfig Content

```bash
cat /tmp/hive-simulator-kubeconfig.yaml
```

### Step 2: Create Provision Shard YAML

Create `provision_shards_simulator.yaml`:

```yaml
provision_shards:
- id: hive-simulator-shard
  hive_config: |
    # Paste the ENTIRE kubeconfig content here, indented by 4 spaces
    apiVersion: v1
    kind: Config
    clusters:
    - cluster:
        certificate-authority-data: <BASE64_CA_DATA>
        server: https://127.0.0.1:XXXXX
      name: hive-simulator
    # ... rest of kubeconfig
  aws_account_operator_config: |
    # Same kubeconfig content as hive_config
    apiVersion: v1
    kind: Config
    # ...
  gcp_project_operator_config: |
    # Same kubeconfig content as hive_config
    apiVersion: v1
    kind: Config
    # ...
  status: active
  region: us-east-1
  cloud_provider: aws
  aws_base_domain: simulator.example.com
```

### Step 3: Start Clusters Service

```bash
./bin/clusters-service serve --provision-shards-config provision_shards_simulator.yaml
```

## Why Not File Paths?

You might wonder why we don't just use a file path to the kubeconfig. Here's why:

1. **API Design**: The provision shard API accepts kubeconfig **content** as a string field in the `ServerConfig` struct
2. **Flexibility**: Content can be stored in the database and synchronized across multiple clusters-service instances
3. **Security**: Secrets can be injected via environment variables in the YAML
4. **Consistency**: Same pattern works for different deployment scenarios (local, CI/CD, production)

## Common Issues

### "Connection refused" when creating clusters

**Problem**: Clusters-service can't connect to the simulator.

**Solution**:
1. Ensure the simulator is running: `ps aux | grep hive-simulator`
2. Check the server address in the kubeconfig matches what the simulator is listening on
3. Regenerate the provision shard config: `./generate-provision-shard-config.sh`

### "Unauthorized" errors

**Problem**: Authentication fails with the simulator.

**Solution**:
- The simulator generates new certificates each time it starts
- Regenerate the provision shard config after restarting the simulator

### Provision shard not found for cluster

**Problem**: Clusters-service can't find an appropriate provision shard.

**Solution**:
- Ensure `status: active` is set in the provision shard config
- Check that `region` and `cloud_provider` match your cluster configuration
- For AWS clusters, ensure `hive_config` and `aws_account_operator_config` are set
- For GCP clusters, ensure `hive_config` and `gcp_project_operator_config` are set

## Example: Complete Local Development Setup

```bash
# Terminal 1: Start the simulator
cd /home/user/clusters-service
make run-hive-simulator

# Output shows:
# INFO: Hive Simulator started successfully
# INFO:   Kubernetes API: Use kubeconfig at /tmp/hive-simulator-kubeconfig.yaml
# INFO:   Configuration API: http://localhost:8080

# Terminal 2: Generate provision shard config
cd cmd/hive-simulator
./generate-provision-shard-config.sh

# Output shows:
# ✓ Provision shard configuration generated: provision_shards_simulator.yaml
# To use with clusters-service:
#   ./bin/clusters-service serve --provision-shards-config provision_shards_simulator.yaml

# Terminal 3: Start clusters-service
cd ../..
./bin/clusters-service serve --provision-shards-config cmd/hive-simulator/provision_shards_simulator.yaml

# Terminal 4: Create a test cluster via OCM API
# The cluster will be provisioned through the simulator
```

## CI/CD Example

```yaml
# .gitlab-ci.yml
test-with-simulator:
  script:
    # Build binaries
    - make hive-simulator
    - make cmds

    # Start simulator in background
    - ./bin/hive-simulator --config config/hive-simulator-fast.yaml &
    - sleep 5

    # Generate provision shard config
    - cd cmd/hive-simulator
    - ./generate-provision-shard-config.sh
    - cd ../..

    # Start clusters-service in background
    - ./bin/clusters-service serve --provision-shards-config cmd/hive-simulator/provision_shards_simulator.yaml &
    - sleep 3

    # Run tests
    - make integration-test

    # Cleanup
    - killall hive-simulator clusters-service
```

## Advanced: Multiple Simulators

To run multiple simulator instances (e.g., for different regions):

```bash
# Start simulator 1 for us-east-1
./bin/hive-simulator --config config/hive-simulator.yaml --api-port 8080 &

# Generate config for shard 1
./cmd/hive-simulator/generate-provision-shard-config.sh \
  /tmp/hive-simulator-kubeconfig.yaml \
  provision_shard_us_east_1.yaml \
  us-east-1-shard

# Start simulator 2 for eu-west-1
./bin/hive-simulator --config config/hive-simulator.yaml --api-port 8081 &

# Generate config for shard 2
./cmd/hive-simulator/generate-provision-shard-config.sh \
  /tmp/hive-simulator-kubeconfig.yaml \
  provision_shard_eu_west_1.yaml \
  eu-west-1-shard

# Combine configs manually or use multiple --provision-shards-config flags
```

Note: Multiple simulators will overwrite `/tmp/hive-simulator-kubeconfig.yaml`. For production use, modify the simulator to write to different paths.

## Reference

- **Provision Shard Documentation**: `docs/common/PROVISION_SHARDS.md`
- **Kubeconfig Guide**: `cmd/hive-simulator/KUBECONFIG.md`
- **Simulator README**: `cmd/hive-simulator/README.md`
- **API Model**: `pkg/api/provision_shards.go`
- **Domain Model**: `pkg/models/provision_shard.go`
