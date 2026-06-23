#!/usr/bin/env bash
# CI script to setup local registry and pre-load test images
# This runs before integration tests to reduce network dependencies

set -euo pipefail

SCRIPT_DIR="$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")"
CRIO_ROOT="${SCRIPT_DIR}/.."
TEST_DIR="${CRIO_ROOT}/test"

# Registry configuration
REGISTRY_NAME="crio-test-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"
REGISTRY_HOST="localhost:${REGISTRY_PORT}"

# Retry configuration for image pulls
MAX_RETRIES="${MAX_RETRIES:-3}"
RETRY_DELAY="${RETRY_DELAY:-5}"

# Complete list of all images used in tests
# This includes images from common.sh and all BATS tests that pull images
IMAGES=(
    # Core images from test/common.sh (pre-loaded by test_runner.sh)
    "registry.k8s.io/pause:3.10.1"
    "quay.io/crio/fedora-crio-ci:latest"
    "quay.io/crio/hello-wasm:latest"

    # Additional images referenced in test/*.bats files
    # These may be pulled on-demand during tests
    "quay.io/crio/pause:latest"
    "quay.io/crio/alpine:3.9"
    "quay.io/crio/artifact:v1"
    "quay.io/crio/artifact:singlefile"
    "quay.io/crio/artifact:multiplefiles"
    "quay.io/crio/artifact:exec"
    "quay.io/crio/seccomp:v2"
    "quay.io/crio/nginx@sha256:960355a671fb88ef18a85f92ccf2ccf8e12186216c86337ad808c204d69d512d"
    "quay.io/saschagrunert/hello-world:latest"
    "quay.io/fedora/fedora:latest"
    "registry.access.redhat.com/rhel7-atomic:latest"
)

log_info() {
    echo "[INFO] $*"
}

log_error() {
    echo "[ERROR] $*" >&2
}

# Check if podman is available
check_podman() {
    if ! command -v podman &> /dev/null; then
        log_error "Podman not found. Falling back to standard image pull."
        return 1
    fi
    log_info "Using Podman: $(podman --version)"
    return 0
}

# Setup registry container
setup_registry() {
    log_info "Setting up local registry at ${REGISTRY_HOST}"

    # Remove existing registry if present
    podman rm -f "${REGISTRY_NAME}" 2>/dev/null || true

    # Pull registry image with retry (in case of network issues)
    local pull_attempt=1
    while [ ${pull_attempt} -le ${MAX_RETRIES} ]; do
        log_info "Pulling registry image (attempt ${pull_attempt}/${MAX_RETRIES})"
        if podman pull docker.io/library/registry:2; then
            break
        fi

        if [ ${pull_attempt} -ge ${MAX_RETRIES} ]; then
            log_error "Failed to pull registry image after ${MAX_RETRIES} attempts"
            return 1
        fi

        log_warn "Registry image pull failed, retrying in ${RETRY_DELAY} seconds..."
        sleep "${RETRY_DELAY}"
        ((pull_attempt++))
    done

    # Create registry container
    if ! podman run -d \
        --restart=always \
        -p "${REGISTRY_PORT}:5000" \
        --name "${REGISTRY_NAME}" \
        docker.io/library/registry:2; then
        log_error "Failed to create registry container"
        podman logs "${REGISTRY_NAME}" 2>/dev/null || true
        return 1
    fi

    # Wait for registry to be ready
    local max_wait=30
    local count=0
    log_info "Waiting for registry to be ready..."
    while ! curl -sf "http://${REGISTRY_HOST}/v2/" >/dev/null 2>&1; do
        if [ ${count} -ge ${max_wait} ]; then
            log_error "Registry failed to start within ${max_wait} seconds"
            podman logs "${REGISTRY_NAME}"
            return 1
        fi
        sleep 1
        ((count++))
    done

    log_info "Registry ready at http://${REGISTRY_HOST}"
    return 0
}

# Pull image with retry logic
pull_image_with_retry() {
    local image="$1"
    local attempt=1

    while [ ${attempt} -le ${MAX_RETRIES} ]; do
        log_info "Pulling ${image} (attempt ${attempt}/${MAX_RETRIES})"

        if podman pull "${image}"; then
            log_info "Successfully pulled ${image}"
            return 0
        fi

        if [ ${attempt} -lt ${MAX_RETRIES} ]; then
            log_warn "Pull failed, retrying in ${RETRY_DELAY} seconds..."
            sleep "${RETRY_DELAY}"
        fi

        ((attempt++))
    done

    log_error "Failed to pull ${image} after ${MAX_RETRIES} attempts"
    return 1
}

# Pull and mirror a single image
mirror_image() {
    local remote_image="$1"
    local image_name
    local image_ref

    # Handle digest-based images (image@sha256:...) differently from tagged images
    if [[ "${remote_image}" =~ @ ]]; then
        # Digest-based image: quay.io/crio/nginx@sha256:abc...
        image_name=$(basename "${remote_image%%@*}")
        # Use the full digest as reference
        image_ref="${remote_image##*@}"
        local local_image="${REGISTRY_HOST}/${image_name}@${image_ref}"
    else
        # Tagged image: quay.io/crio/pause:latest
        image_name=$(basename "${remote_image%%:*}")
        image_ref="${remote_image##*:}"
        local local_image="${REGISTRY_HOST}/${image_name}:${image_ref}"
    fi

    log_info "Mirroring ${remote_image} -> ${local_image}"

    # Pull from remote with retry
    if ! pull_image_with_retry "${remote_image}"; then
        return 1
    fi

    # Tag for local registry
    if ! podman tag "${remote_image}" "${local_image}"; then
        log_error "Failed to tag ${remote_image}"
        return 1
    fi

    # Push to local registry (with retry)
    local push_attempt=1
    while [ ${push_attempt} -le ${MAX_RETRIES} ]; do
        if podman push "${local_image}" --tls-verify=false; then
            log_info "Successfully pushed ${local_image}"
            return 0
        fi

        if [ ${push_attempt} -lt ${MAX_RETRIES} ]; then
            log_warn "Push failed, retrying in ${RETRY_DELAY} seconds..."
            sleep "${RETRY_DELAY}"
        fi

        ((push_attempt++))
    done

    log_error "Failed to push ${local_image} after ${MAX_RETRIES} attempts"
    return 1
}

# Mirror all test images
mirror_all_images() {
    log_info "Mirroring ${#IMAGES[@]} test images"

    local failed=0
    for img in "${IMAGES[@]}"; do
        if ! mirror_image "${img}"; then
            ((failed++))
        fi
    done

    if [ ${failed} -gt 0 ]; then
        log_error "${failed} images failed to mirror"
        return 1
    fi

    log_info "Successfully mirrored all images"
    return 0
}

# Configure registries.conf to use local registry
configure_registries() {
    local registries_conf="${TEST_DIR}/registries.conf"

    log_info "Configuring registry mirror in ${registries_conf}"

    # Backup original
    cp "${registries_conf}" "${registries_conf}.ci-backup"

    # Add mirror configuration
    cat >> "${registries_conf}" <<EOF

# CI local registry mirror (added by ci-setup-local-registry.sh)
[[registry]]
prefix = "quay.io/crio"
location = "${REGISTRY_HOST}"
insecure = true

[[registry]]
prefix = "registry.k8s.io"
location = "${REGISTRY_HOST}"
insecure = true
EOF

    log_info "Registry configuration updated"
    log_info "Registry mirror configuration:"
    grep -A2 "CI local registry mirror" "${registries_conf}" || true
}

# Main execution
main() {
    log_info "Starting CI local registry setup"

    if ! check_podman; then
        log_info "Skipping local registry setup (podman not available)"
        exit 0
    fi

    if ! setup_registry; then
        log_error "Failed to setup registry"
        exit 1
    fi

    if ! mirror_all_images; then
        log_error "Failed to mirror images"
        podman rm -f "${REGISTRY_NAME}" 2>/dev/null || true
        exit 1
    fi

    configure_registries

    # Verify registry and list images
    log_info "Verifying local registry contents..."
    local image_count
    image_count=$(curl -sf "http://${REGISTRY_HOST}/v2/_catalog" | grep -o '"repositories":\[.*\]' | grep -o ',' | wc -l)
    ((image_count++)) || true

    log_info "========================================"
    log_info "LOCAL REGISTRY SETUP COMPLETE"
    log_info "========================================"
    log_info "Registry URL: http://${REGISTRY_HOST}"
    log_info "Images pre-loaded: ${#IMAGES[@]}"
    log_info "Images in registry: ${image_count}"
    log_info "Registry catalog:"
    curl -sf "http://${REGISTRY_HOST}/v2/_catalog" 2>/dev/null || echo "  (catalog unavailable)"
    log_info "========================================"
    log_info "Tests will use local registry via registries.conf mirror"
    log_info "Monitor 'podman pull' or 'crictl pull' commands in test output"
    log_info "Local pulls will show: localhost:${REGISTRY_PORT}/<image>"
    log_info "========================================"
}

main "$@"
