#!/usr/bin/env bash
# Setup local registry for CRI-O integration tests
# This script creates a Podman-based local registry and pre-pulls test images
# to speed up integration tests and reduce flakiness from network issues.

set -euo pipefail

# Registry configuration
REGISTRY_NAME="${REGISTRY_NAME:-crio-test-registry}"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"
REGISTRY_HOST="localhost:${REGISTRY_PORT}"

# Get image list from test configuration
SCRIPT_DIR="$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")"
TEST_DIR="${SCRIPT_DIR}/../test"

# Extract IMAGES array from common.sh without sourcing the entire file
# This avoids dependencies on crictl and other test binaries
mapfile -t IMAGES < <(sed -n '/^IMAGES=(/,/)$/p' "${TEST_DIR}/common.sh" | grep -v '^IMAGES=\|^)$' | sed 's/^[[:space:]]*//' | tr -d '"')

log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

# Check if Podman is available
check_podman() {
    if ! command -v podman &> /dev/null; then
        log_error "Podman is not installed. Please install Podman first."
        log_info "On Ubuntu/Debian: sudo apt-get install podman"
        log_info "On Fedora/RHEL: sudo dnf install podman"
        exit 1
    fi
    log_info "Found Podman: $(podman --version)"
}

# Create and start the local registry container
setup_registry() {
    local registry_running
    registry_running=$(podman inspect -f '{{.State.Running}}' "${REGISTRY_NAME}" 2>/dev/null || echo "false")

    if [ "${registry_running}" = "true" ]; then
        log_info "Registry '${REGISTRY_NAME}' is already running"
        return 0
    fi

    # Check if container exists but is stopped
    if podman inspect "${REGISTRY_NAME}" &>/dev/null; then
        log_info "Starting existing registry container '${REGISTRY_NAME}'"
        podman start "${REGISTRY_NAME}"
        return 0
    fi

    # Create new registry container
    log_info "Creating new registry container '${REGISTRY_NAME}' on port ${REGISTRY_PORT}"
    podman run -d \
        --restart=always \
        -p "${REGISTRY_PORT}:5000" \
        --name "${REGISTRY_NAME}" \
        docker.io/library/registry:2

    # Wait for registry to be ready
    local max_wait=30
    local count=0
    while ! curl -sf "http://${REGISTRY_HOST}/v2/" >/dev/null 2>&1; do
        if [ ${count} -ge ${max_wait} ]; then
            log_error "Registry failed to start within ${max_wait} seconds"
            podman logs "${REGISTRY_NAME}"
            exit 1
        fi
        log_info "Waiting for registry to be ready... ($((count + 1))/${max_wait})"
        sleep 1
        ((count++))
    done

    log_info "Registry is ready at http://${REGISTRY_HOST}"
}

# Pull image from remote registry and push to local registry
mirror_image() {
    local remote_image="$1"
    local image_name
    local image_tag

    # Extract image name and tag
    image_name=$(echo "${remote_image}" | sed -e 's|^.*/||' -e 's/:.*$//')
    image_tag=$(echo "${remote_image}" | sed -e 's/.*://')

    local local_image="${REGISTRY_HOST}/${image_name}:${image_tag}"

    log_info "Processing ${remote_image}"

    # Pull from remote registry
    if ! podman pull "${remote_image}"; then
        log_error "Failed to pull ${remote_image}"
        return 1
    fi

    # Tag for local registry
    if ! podman tag "${remote_image}" "${local_image}"; then
        log_error "Failed to tag ${remote_image} as ${local_image}"
        return 1
    fi

    # Push to local registry
    if ! podman push "${local_image}" --tls-verify=false; then
        log_error "Failed to push ${local_image}"
        return 1
    fi

    log_info "Successfully mirrored ${remote_image} to ${local_image}"
    return 0
}

# Mirror all test images to local registry
mirror_all_images() {
    local failed=0

    log_info "Mirroring ${#IMAGES[@]} test images to local registry"

    for img in "${IMAGES[@]}"; do
        if ! mirror_image "${img}"; then
            log_warn "Failed to mirror ${img}"
            ((failed++))
        fi
    done

    if [ ${failed} -gt 0 ]; then
        log_warn "${failed} images failed to mirror"
        return 1
    fi

    log_info "All images mirrored successfully"
    return 0
}

# Update registries.conf to use local registry as mirror
configure_registry_mirror() {
    local registries_conf="${TEST_DIR}/registries.conf"
    local backup_file="${registries_conf}.backup"

    # Create backup if it doesn't exist
    if [ ! -f "${backup_file}" ]; then
        log_info "Creating backup of registries.conf"
        cp "${registries_conf}" "${backup_file}"
    fi

    # Check if mirror configuration already exists
    if grep -q "location = \"${REGISTRY_HOST}\"" "${registries_conf}"; then
        log_info "Registry mirror already configured in ${registries_conf}"
        return 0
    fi

    log_info "Adding local registry mirror to ${registries_conf}"

    # Add mirror configuration for quay.io
    cat >> "${registries_conf}" <<EOF

# Local registry mirror for testing (added by setup-local-registry.sh)
[[registry]]
prefix = "quay.io/crio"
location = "${REGISTRY_HOST}"
insecure = true

[[registry]]
prefix = "registry.k8s.io"
location = "${REGISTRY_HOST}"
insecure = true
EOF

    log_info "Registry mirror configured"
}

# List images in local registry
list_registry_images() {
    log_info "Images in local registry:"

    if ! curl -sf "http://${REGISTRY_HOST}/v2/_catalog" | jq -r '.repositories[]' 2>/dev/null; then
        log_warn "Failed to list images (jq may not be installed)"
        curl -sf "http://${REGISTRY_HOST}/v2/_catalog" || log_error "Registry is not accessible"
    fi
}

# Stop and remove the local registry
cleanup_registry() {
    log_info "Stopping and removing registry container '${REGISTRY_NAME}'"
    podman stop "${REGISTRY_NAME}" 2>/dev/null || true
    podman rm "${REGISTRY_NAME}" 2>/dev/null || true
    log_info "Registry cleanup complete"
}

# Restore original registries.conf
restore_registry_config() {
    local registries_conf="${TEST_DIR}/registries.conf"
    local backup_file="${registries_conf}.backup"

    if [ -f "${backup_file}" ]; then
        log_info "Restoring original registries.conf"
        mv "${backup_file}" "${registries_conf}"
    fi
}

# Show usage information
usage() {
    cat <<EOF
Usage: $(basename "$0") [COMMAND]

Setup local registry for CRI-O integration tests to reduce network dependencies.

Commands:
    setup       Create registry and mirror all test images (default)
    start       Start existing registry container
    stop        Stop registry container
    cleanup     Stop and remove registry container
    list        List images in local registry
    mirror      Mirror all test images to local registry
    restore     Restore original registries.conf
    help        Show this help message

Environment Variables:
    REGISTRY_NAME    Name of the registry container (default: crio-test-registry)
    REGISTRY_PORT    Port for the registry (default: 5000)

Examples:
    # Full setup (create registry and mirror images)
    $(basename "$0") setup

    # Just mirror images to existing registry
    $(basename "$0") mirror

    # Cleanup everything
    $(basename "$0") cleanup
    $(basename "$0") restore
EOF
}

# Main function
main() {
    local command="${1:-setup}"

    case "${command}" in
        setup)
            check_podman
            setup_registry
            mirror_all_images
            configure_registry_mirror
            list_registry_images
            log_info "Local registry setup complete!"
            log_info "Registry is available at http://${REGISTRY_HOST}"
            log_info "Run 'make localintegration' to use the local registry"
            ;;
        start)
            check_podman
            setup_registry
            ;;
        stop)
            log_info "Stopping registry container"
            podman stop "${REGISTRY_NAME}" 2>/dev/null || log_warn "Registry is not running"
            ;;
        cleanup)
            cleanup_registry
            ;;
        list)
            list_registry_images
            ;;
        mirror)
            check_podman
            if ! podman inspect "${REGISTRY_NAME}" &>/dev/null; then
                log_error "Registry container '${REGISTRY_NAME}' does not exist"
                log_info "Run '$(basename "$0") setup' first"
                exit 1
            fi
            mirror_all_images
            ;;
        restore)
            restore_registry_config
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: ${command}"
            usage
            exit 1
            ;;
    esac
}

main "$@"
