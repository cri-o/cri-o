#!/usr/bin/env bash
# Verify that the local registry is running and was used during tests

set -euo pipefail

REGISTRY_NAME="crio-test-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"
REGISTRY_HOST="localhost:${REGISTRY_PORT}"

# Check if registry container is running
if ! podman inspect "${REGISTRY_NAME}" &>/dev/null; then
    echo "ERROR: Local registry '${REGISTRY_NAME}' not found"
    exit 1
fi

# Check registry is responding
if ! curl -sf "http://${REGISTRY_HOST}/v2/" >/dev/null 2>&1; then
    echo "ERROR: Local registry not responding at http://${REGISTRY_HOST}"
    exit 1
fi

# List images in registry
echo "Local registry catalog:"
curl -sf "http://${REGISTRY_HOST}/v2/_catalog" | jq -r '.repositories[]' 2>/dev/null | sed 's/^/  /' || true

# Check registry logs for manifest requests
pull_count=$(podman logs "${REGISTRY_NAME}" 2>&1 | grep -c '/manifests/' || true)
echo "Registry manifest requests: ${pull_count}"

if [ "${pull_count}" -gt 0 ]; then
    echo "OK: Local registry was used"
else
    echo "WARN: No manifest requests seen in registry logs"
fi
