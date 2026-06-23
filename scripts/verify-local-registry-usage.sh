#!/usr/bin/env bash
# Verify that the local registry was used during tests
# Run this after integration tests complete

set -euo pipefail

REGISTRY_NAME="crio-test-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"
CRIO_LOG_DIR="${CRIO_LOG_DIR:-/var/log/crio}"

log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*" >&2
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

# Count GET requests from local registry
pull_count=$(podman logs "${REGISTRY_NAME}" 2>&1 | grep -c "GET.*manifests" || echo "0")
blob_count=$(podman logs "${REGISTRY_NAME}" 2>&1 | grep -c "GET.*blobs" || echo "0")

# Extract unique images pulled from local registry
log_info ""
log_info "Images pulled from local registry (localhost:${REGISTRY_PORT}):"
local_images=$(podman logs "${REGISTRY_NAME}" 2>&1 | grep "GET.*manifests" | sed -E 's|.*GET /v2/([^/]+(/[^/]+)*)/manifests/.*|\1|' | sort -u || echo "")
if [ -n "$local_images" ]; then
    echo "$local_images" | sed 's/^/  ✓ /'
    local_image_count=$(echo "$local_images" | wc -l)
else
    log_info "  (none)"
    local_image_count=0
fi

# Check for external registry pulls (fallback to real registries)
log_info ""
log_info "Checking for external registry fallbacks..."

# Look for pulls to real registries in recent logs
external_pulls=""

# Check systemd journal if available
if command -v journalctl &>/dev/null && journalctl -u crio --no-pager -n 10000 &>/dev/null 2>&1; then
    external_pulls=$(journalctl -u crio --no-pager -n 10000 --since "10 minutes ago" 2>/dev/null | \
        grep -E "Pulling image:|PullImage from image service" | \
        grep -vE "localhost:${REGISTRY_PORT}" | \
        sed -E 's/.*image[=:] ?"?([^ "]+)"?.*/\1/' | \
        grep -E "registry.k8s.io|gcr.io|quay.io|k8s.gcr.io" | \
        sort -u || echo "")
fi

# Also check any crio log files in common locations
for log_file in "${CRIO_LOG_DIR}/crio.log" /tmp/tmp.*/crio.log /tmp/crio*.log; do
    if [ -f "$log_file" ]; then
        external_pulls="${external_pulls}
$(grep -E "Pulling image:|PullImage from image service" "$log_file" 2>/dev/null | \
    grep -vE "localhost:${REGISTRY_PORT}" | \
    sed -E 's/.*image[=:] ?"?([^ "]+)"?.*/\1/' | \
    grep -E "registry.k8s.io|gcr.io|quay.io|k8s.gcr.io" | \
    sort -u || echo "")"
    fi
done

# Deduplicate and clean up
external_pulls=$(echo "$external_pulls" | grep -v "^$" | sort -u || echo "")

log_info ""
log_info "========================================"
log_info "LOCAL REGISTRY USAGE SUMMARY"
log_info "========================================"
log_info "Manifest requests from mirror: ${pull_count}"
log_info "Blob requests from mirror: ${blob_count}"
log_info "Unique images from mirror: ${local_image_count}"

if [ -n "$external_pulls" ]; then
    external_count=$(echo "$external_pulls" | wc -l)
    log_info "External registry fallbacks: ${external_count}"
    log_info "========================================"
    log_warn ""
    log_warn "⚠ Images pulled from EXTERNAL registries (not in mirror):"
    echo "$external_pulls" | sed 's/^/  ⚠ /'
    log_warn ""
    log_warn "These images fell back to external registries:"
    log_warn "  - Slower (network latency)"
    log_warn "  - Less reliable (network failures)"
    log_warn "  - Should be added to ci-setup-local-registry.sh IMAGES array"
    log_warn ""
else
    log_info "External registry fallbacks: 0"
    log_info "========================================"
    log_info "✓ All images served from local registry!"
fi

if [ "${pull_count}" -gt 0 ] || [ "${blob_count}" -gt 0 ]; then
    log_info "✓ Local registry was used (${pull_count} manifest pulls)"
else
    log_error "✗ Local registry was NOT used"
    log_error "  Check /etc/containers/registries.conf configuration"
fi

# Show what images are available in the registry
log_info ""
log_info "Images available in local registry:"
curl -sf "http://localhost:${REGISTRY_PORT}/v2/_catalog" | jq -r '.repositories[]' 2>/dev/null | sed 's/^/  /' || {
    curl -sf "http://localhost:${REGISTRY_PORT}/v2/_catalog" 2>/dev/null || log_error "  Registry not accessible"
}

log_info "========================================"
