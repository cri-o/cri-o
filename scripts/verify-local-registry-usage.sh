#!/usr/bin/env bash
# Verify that the local registry was used during tests
# Run this after integration tests complete

set -euo pipefail

REGISTRY_NAME="crio-test-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"

log_info() {
    echo "[INFO] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

# Check if registry container exists
if ! podman inspect "${REGISTRY_NAME}" &>/dev/null; then
    log_error "Local registry '${REGISTRY_NAME}' not found"
    log_error "Did you run ci-setup-local-registry.sh before tests?"
    exit 1
fi

log_info "Checking local registry usage..."

# Get registry logs
log_info "Registry access logs (last 50 lines):"
podman logs --tail 50 "${REGISTRY_NAME}" 2>&1 | grep -E "GET|HEAD|POST" || {
    log_info "  No HTTP access logs found"
}

# Count GET requests (image pulls)
pull_count=$(podman logs "${REGISTRY_NAME}" 2>&1 | grep -c "GET.*manifests" || echo "0")
blob_count=$(podman logs "${REGISTRY_NAME}" 2>&1 | grep -c "GET.*blobs" || echo "0")

log_info ""
log_info "========================================"
log_info "LOCAL REGISTRY USAGE SUMMARY"
log_info "========================================"
log_info "Manifest requests (image pulls): ${pull_count}"
log_info "Blob requests (layer downloads): ${blob_count}"
log_info "========================================"

if [ "${pull_count}" -gt 0 ] || [ "${blob_count}" -gt 0 ]; then
    log_info "✓ Local registry WAS used during tests"
    log_info "  Tests pulled images from localhost:${REGISTRY_PORT}"
else
    log_error "✗ Local registry was NOT used during tests"
    log_error "  No image pulls detected in registry logs"
    log_error "  Check test/registries.conf configuration"
fi

# Show what images are in the registry
log_info ""
log_info "Images in local registry:"
curl -sf "http://localhost:${REGISTRY_PORT}/v2/_catalog" | jq -r '.repositories[]' 2>/dev/null || {
    curl -sf "http://localhost:${REGISTRY_PORT}/v2/_catalog" || log_error "  Registry not accessible"
}

log_info "========================================"
