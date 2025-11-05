#!/bin/bash
# generate-provision-shard-config.sh
# Helper script to generate provision shard configuration for the Hive Simulator

set -e

KUBECONFIG_PATH="${1:-/tmp/hive-simulator-kubeconfig.yaml}"
OUTPUT_FILE="${2:-provision_shards_simulator.yaml}"
SHARD_ID="${3:-hive-simulator-shard}"

# Check if kubeconfig exists
if [ ! -f "$KUBECONFIG_PATH" ]; then
    echo "ERROR: Kubeconfig not found at $KUBECONFIG_PATH"
    echo "Make sure the Hive Simulator is running."
    exit 1
fi

echo "Generating provision shard configuration..."
echo "  Kubeconfig: $KUBECONFIG_PATH"
echo "  Output: $OUTPUT_FILE"
echo "  Shard ID: $SHARD_ID"

# Read kubeconfig and indent it for YAML
KUBECONFIG_CONTENT=$(sed 's/^/    /' "$KUBECONFIG_PATH")

# Generate the provision shard configuration
cat > "$OUTPUT_FILE" <<EOF
# Provision Shard Configuration for Hive Simulator
# Generated: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
# Kubeconfig source: $KUBECONFIG_PATH

provision_shards:
- id: $SHARD_ID
  hive_config: |
$KUBECONFIG_CONTENT
  aws_account_operator_config: |
$KUBECONFIG_CONTENT
  gcp_project_operator_config: |
$KUBECONFIG_CONTENT
  status: active
  region: us-east-1
  cloud_provider: aws
  aws_base_domain: simulator.example.com
  gcp_base_domain: simulator.example.com
EOF

echo ""
echo "✓ Provision shard configuration generated: $OUTPUT_FILE"
echo ""
echo "To regenerate after simulator restart:"
echo "  $0 $KUBECONFIG_PATH $OUTPUT_FILE $SHARD_ID"
