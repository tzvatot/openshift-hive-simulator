#!/bin/bash
# Restart Hive Simulator and regenerate provision shard configuration
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "==> Stopping existing hive-simulator processes..."
# Kill all hive-simulator and related processes
pkill -9 -f "bin/hive-simulator" || true
pkill -9 -f "make run-hive-simulator" || true
sleep 2

# Clean up any leftover envtest processes
pkill -9 -f "kube-apiserver.*envtest" || true
pkill -9 -f "etcd.*k8s_test_framework" || true
sleep 1

echo "==> Starting hive-simulator..."
cd "$PROJECT_ROOT"
make run-hive-simulator > /tmp/hive-simulator-restart.log 2>&1 &
SIMULATOR_PID=$!

echo "==> Waiting for simulator to be ready..."
sleep 5

# Check if kubeconfig was created
if [ ! -f /tmp/hive-simulator-kubeconfig.yaml ]; then
    echo "ERROR: Kubeconfig not found. Check /tmp/hive-simulator-restart.log for errors"
    exit 1
fi

echo "==> Regenerating provision shard configuration..."
cd "$SCRIPT_DIR"
./generate-provision-shard-config.sh

echo ""
echo "✓ Hive Simulator restarted successfully"
echo "  PID: $SIMULATOR_PID"
echo "  Logs: /tmp/hive-simulator-restart.log"
echo "  Provision shard config: $SCRIPT_DIR/provision_shards_simulator.yaml"
echo ""
echo "To restart clusters-service with the new config:"
echo "  pkill clusters-service"
echo "  cd $PROJECT_ROOT && ./clusters-service serve --config run.toml"
