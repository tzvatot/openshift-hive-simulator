# Understanding Kubeconfig in the Hive Simulator

## What is the Kubeconfig?

The **kubeconfig** is a configuration file that contains connection details for accessing a Kubernetes cluster. It includes:
- The Kubernetes API server address
- Authentication credentials (certificates/keys)
- Cluster CA certificates
- Context information (which cluster to use)

## In the Hive Simulator Context

When you start the Hive Simulator, it:

1. **Starts a lightweight Kubernetes API server** using envtest (etcd + kube-apiserver)
2. **Automatically generates a kubeconfig file** at `/tmp/hive-simulator-kubeconfig.yaml`
3. **Prints the kubeconfig location** in the startup logs

## Quick Start

The easiest way to get started is using the helper script:

```bash
# After starting the simulator
cd cmd/hive-simulator
./generate-provision-shard-config.sh

# This generates provision_shards_simulator.yaml with the kubeconfig embedded
# Use it with clusters-service:
../../bin/clusters-service serve --provision-shards-config provision_shards_simulator.yaml
```

## How to Get the Kubeconfig

### Method 1: Check the Startup Logs

When you run the simulator, it outputs:

```
INFO: Hive Simulator started successfully
INFO:   Kubernetes API: Use kubeconfig at /tmp/hive-simulator-kubeconfig.yaml
INFO:   Configuration API: http://localhost:8080
```

The kubeconfig path is shown in the second line.

### Method 2: Use the Default Location

The kubeconfig is **always** created at:
```
/tmp/hive-simulator-kubeconfig.yaml
```

## How to Use the Kubeconfig

### With kubectl

```bash
# Set the KUBECONFIG environment variable
export KUBECONFIG=/tmp/hive-simulator-kubeconfig.yaml

# Now kubectl commands will talk to the simulator
kubectl get clusterdeployments
kubectl get accountclaims
kubectl get projectclaims
kubectl get clusterimagesets

# Watch resources as they transition
watch kubectl get clusterdeployments
```

### With Clusters Service

To configure the clusters-service to use the simulator, you need to provide the kubeconfig **content** in your provision shard configuration file.

#### Option 1: Use the Helper Script (Recommended)

The easiest way is to use the provided helper script:

```bash
# Generate provision shard configuration automatically
cd cmd/hive-simulator
./generate-provision-shard-config.sh

# This creates provision_shards_simulator.yaml
# Start clusters-service with it
../../bin/clusters-service serve --provision-shards-config provision_shards_simulator.yaml
```

The script reads the kubeconfig from `/tmp/hive-simulator-kubeconfig.yaml` and generates a complete provision shard configuration file.

#### Option 2: Manual Configuration

1. **Read the simulator kubeconfig**:
   ```bash
   cat /tmp/hive-simulator-kubeconfig.yaml
   ```

2. **Create or update your provision shard configuration YAML**:

   Create a file `provision_shards_simulator.yaml`:
   ```yaml
   provision_shards:
   - id: hive-simulator-shard
     hive_config: |
       apiVersion: v1
       kind: Config
       clusters:
       - cluster:
           certificate-authority-data: <BASE64_ENCODED_CA>
           server: https://127.0.0.1:<PORT>
         name: hive-simulator
       contexts:
       - context:
           cluster: hive-simulator
           user: hive-simulator
         name: hive-simulator
       current-context: hive-simulator
       users:
       - name: hive-simulator
         user:
           client-certificate-data: <BASE64_ENCODED_CERT>
           client-key-data: <BASE64_ENCODED_KEY>
     aws_account_operator_config: |
       # Same kubeconfig content as hive_config
       apiVersion: v1
       kind: Config
       ...
     gcp_project_operator_config: |
       # Same kubeconfig content as hive_config
       apiVersion: v1
       kind: Config
       ...
     status: active
     region: us-east-1
     cloud_provider: aws
     aws_base_domain: example.com
   ```

   **Important**: Copy the **entire kubeconfig content** from `/tmp/hive-simulator-kubeconfig.yaml` into the `hive_config`, `aws_account_operator_config`, and `gcp_project_operator_config` fields.

3. **Start clusters-service with the simulator provision shard**:
   ```bash
   ./bin/clusters-service serve --provision-shards-config provision_shards_simulator.yaml
   ```

### With Client-Go Applications

If you're writing Go code that needs to connect to the simulator:

```go
import (
    "k8s.io/client-go/tools/clientcmd"
)

func main() {
    // Load the kubeconfig
    config, err := clientcmd.BuildConfigFromFlags("", "/tmp/hive-simulator-kubeconfig.yaml")
    if err != nil {
        // handle error
    }

    // Use config to create a Kubernetes client
    // ...
}
```

## What the Kubeconfig Contains

The generated kubeconfig includes:

```yaml
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: <BASE64_ENCODED_CA>
    server: https://127.0.0.1:<PORT>
  name: hive-simulator
contexts:
- context:
    cluster: hive-simulator
    user: hive-simulator
  name: hive-simulator
current-context: hive-simulator
users:
- name: hive-simulator
  user:
    client-certificate-data: <BASE64_ENCODED_CERT>
    client-key-data: <BASE64_ENCODED_KEY>
```

## Important Notes

### Temporary File Location

⚠️ The kubeconfig is in `/tmp/`, which means:
- It gets **recreated** each time you start the simulator
- It may be **deleted** on system reboot
- It's **temporary** and not meant for long-term storage

### Security

The kubeconfig contains:
- **Client certificates** for authentication
- **Private keys** for TLS
- **CA certificates** for server verification

These are **self-signed and generated** by envtest, so they're safe for local development but should not be used in production.

### Multiple Simulators

If you run multiple simulator instances:
- Each creates its own kubeconfig at the **same path**
- The file gets **overwritten** by each new instance
- Only the **most recent** simulator's kubeconfig will be valid

To run multiple simulators:
1. Modify the simulator to write to different locations
2. Or use separate directories/machines
3. Or stop one before starting another

## Troubleshooting

### "Connection refused" errors

If you get connection errors:

```bash
# Check if the simulator is running
ps aux | grep hive-simulator

# Check the API server address in the kubeconfig
grep server: /tmp/hive-simulator-kubeconfig.yaml

# Verify the port is listening
lsof -i :<PORT_FROM_KUBECONFIG>
```

### "File not found" errors

If the kubeconfig doesn't exist:

```bash
# Check if the simulator is running
ps aux | grep hive-simulator

# Check simulator logs for errors during startup
# The kubeconfig is created during the setupEnvtest() step
```

### "Unauthorized" errors

This usually means:
- The simulator was restarted (new certificates were generated)
- You're using an old kubeconfig from a previous run

**Solution**: Just restart your client/clusters-service to pick up the new kubeconfig.

## Example Workflow

Here's a complete example of using the simulator:

```bash
# Terminal 1: Start the simulator
make run-hive-simulator
# Output shows: Use kubeconfig at /tmp/hive-simulator-kubeconfig.yaml

# Terminal 2: Use kubectl to interact
export KUBECONFIG=/tmp/hive-simulator-kubeconfig.yaml

# List all custom resources
kubectl get clusterdeployments
kubectl get accountclaims
kubectl get projectclaims
kubectl get clusterimagesets

# Watch a ClusterDeployment transition through states
kubectl get clusterdeployment my-cluster -o yaml -w

# Terminal 3: Use the configuration API
curl http://localhost:8080/api/v1/status
```

## Summary

The kubeconfig is:
- **Automatically generated** when the simulator starts
- **Located at** `/tmp/hive-simulator-kubeconfig.yaml`
- **Used to connect** kubectl, clusters-service, or any Kubernetes client to the simulator
- **Temporary** and recreated each time the simulator starts
- **Printed in the logs** when the simulator starts up

Just set `export KUBECONFIG=/tmp/hive-simulator-kubeconfig.yaml` and you're ready to use kubectl or configure clusters-service to talk to the simulator!
