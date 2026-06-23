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
# This includes images from common.sh, BATS tests, and critest
IMAGES=(
    # Core images from test/common.sh (pre-loaded by test_runner.sh)
    "registry.k8s.io/pause:3.10.1"
    "quay.io/crio/fedora-crio-ci:latest"
    "quay.io/crio/hello-wasm:latest"

    # Additional images referenced in test/*.bats files
    "quay.io/crio/alpine:3.9"
    "quay.io/crio/artifact:v1"
    "quay.io/saschagrunert/hello-world:latest"
    "quay.io/fedora/fedora:latest"

    # critest images (gcr.io/k8s-staging-cri-tools)
    "gcr.io/k8s-staging-cri-tools/test-image-predefined-group:latest"
    "gcr.io/k8s-staging-cri-tools/hostnet-nginx-arm64:latest"
    "gcr.io/k8s-staging-cri-tools/hostnet-nginx-amd64:latest"
    "gcr.io/k8s-staging-cri-tools/test-image-tags:1"
    "gcr.io/k8s-staging-cri-tools/test-image-tags:2"
    "gcr.io/k8s-staging-cri-tools/test-image-latest:latest"
    "gcr.io/k8s-staging-cri-tools/test-image-tag:test"
    "gcr.io/k8s-staging-cri-tools/test-image-digest@sha256:9d242c6ffa4b72cfc037d88f975969defe6ba3f1e6aca35fea7497207a1210ab"
    "k8s.gcr.io/pause:3.10.1"

    # registry.k8s.io e2e test images
    "registry.k8s.io/e2e-test-images/busybox:1.29-2"
    "registry.k8s.io/e2e-test-images/nginx:1.14-2"
    "registry.k8s.io/e2e-test-images/httpd:2.4.39-4"
    "registry.k8s.io/e2e-test-images/nonewprivs:1.3"
)

# OCI artifacts and special images that may not work with podman pull
# These are handled by CRI-O directly during tests
SKIP_IMAGES=(
    "quay.io/crio/artifact:singlefile"      # OCI artifact, not a container image
    "quay.io/crio/artifact:multiplefiles"   # OCI artifact
    "quay.io/crio/artifact:exec"            # OCI artifact
    "quay.io/crio/seccomp:v2"               # OCI artifact
    "registry.access.redhat.com/rhel7-atomic:latest"  # No ARM64 variant
    "quay.io/crio/nginx@sha256:960355a671fb88ef18a85f92ccf2ccf8e12186216c86337ad808c204d69d512d"  # Digest-based
)

log_info() {
    echo "[INFO] $*"
}

log_warn() {
    echo "[WARN] $*" >&2
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
    local repo_path
    local image_ref
    local local_image

    # Strip the registry prefix to match how registries.conf mirroring works
    # Examples:
    #   quay.io/crio/fedora-crio-ci:latest -> fedora-crio-ci:latest (strip "quay.io/crio/")
    #   registry.k8s.io/e2e-test-images/busybox:1.29-2 -> e2e-test-images/busybox:1.29-2 (strip "registry.k8s.io/")
    #   gcr.io/k8s-staging-cri-tools/test-image-tags:1 -> k8s-staging-cri-tools/test-image-tags:1 (strip "gcr.io/")

    if [[ "${remote_image}" =~ @ ]]; then
        # Digest-based image
        repo_path="${remote_image%%@*}"  # Remove digest

        local digest="${remote_image##*@}"
        image_ref="sha256-${digest##*:}"
        image_ref="${image_ref:0:20}"  # Truncate to reasonable length
    else
        # Tagged image
        repo_path="${remote_image%:*}"   # Remove tag
        image_ref="${remote_image##*:}"  # Extract tag
    fi

    # Strip prefix based on source registry
    case "${repo_path}" in
        quay.io/crio/*)
            repo_path="${repo_path#quay.io/crio/}"
            ;;
        quay.io/saschagrunert/*)
            repo_path="${repo_path#quay.io/saschagrunert/}"
            ;;
        quay.io/fedora/*)
            repo_path="${repo_path#quay.io/fedora/}"
            ;;
        registry.k8s.io/*)
            repo_path="${repo_path#registry.k8s.io/}"
            ;;
        gcr.io/*)
            repo_path="${repo_path#gcr.io/}"
            ;;
        k8s.gcr.io/*)
            repo_path="${repo_path#k8s.gcr.io/}"
            ;;
        *)
            # Fallback: just strip registry hostname
            repo_path="${repo_path#*/}"
            ;;
    esac

    local_image="${REGISTRY_HOST}/${repo_path}:${image_ref}"
    log_info "Mirroring ${remote_image} -> ${local_image}"

    # Pull from remote with retry
    if ! pull_image_with_retry "${remote_image}"; then
        log_warn "Skipping ${remote_image} (pull failed)"
        return 1
    fi

    # Tag for local registry
    if ! podman tag "${remote_image}" "${local_image}" 2>/dev/null; then
        log_warn "Failed to tag ${remote_image}, trying alternate method"
        # Try with image ID
        local image_id
        image_id=$(podman images --format "{{.ID}}" "${remote_image}" | head -1)
        if [ -n "${image_id}" ] && ! podman tag "${image_id}" "${local_image}"; then
            log_error "Failed to tag ${remote_image}"
            return 1
        fi
    fi

    # Push to local registry (with retry)
    local push_attempt=1
    while [ ${push_attempt} -le ${MAX_RETRIES} ]; do
        if podman push "${local_image}" --tls-verify=false 2>&1; then
            log_info "Successfully pushed ${local_image}"
            return 0
        fi

        if [ ${push_attempt} -lt ${MAX_RETRIES} ]; then
            log_warn "Push failed, retrying in ${RETRY_DELAY} seconds..."
            sleep "${RETRY_DELAY}"
        fi

        ((push_attempt++))
    done

    log_warn "Failed to push ${local_image} after ${MAX_RETRIES} attempts"
    return 1
}

# Mirror all test images
mirror_all_images() {
    log_info "Mirroring ${#IMAGES[@]} test images"
    log_info "Skipping ${#SKIP_IMAGES[@]} OCI artifacts and incompatible images"

    local failed=0
    local succeeded=0

    for img in "${IMAGES[@]}"; do
        if mirror_image "${img}"; then
            ((succeeded++))
        else
            log_warn "Failed to mirror ${img}"
            ((failed++))
        fi
    done

    log_info "Mirror summary: ${succeeded} succeeded, ${failed} failed"

    # Don't fail if at least core images succeeded
    if [ ${succeeded} -lt 3 ]; then
        log_error "Too few images mirrored successfully (${succeeded}/3 core images minimum)"
        return 1
    fi

    if [ ${failed} -gt 0 ]; then
        log_warn "${failed} images failed to mirror, but continuing with ${succeeded} images"
    else
        log_info "Successfully mirrored all images"
    fi

    return 0
}

# Configure registries.conf to use local registry
configure_registries() {
    local registries_conf="/etc/containers/registries.conf"

    log_info "Configuring registry mirror in ${registries_conf}"

    # Backup original
    cp "${registries_conf}" "${registries_conf}.ci-backup"

    # Add mirror configuration
    # Use [[registry.mirror]] instead of prefix-based matching for better compatibility
    cat >> "${registries_conf}" <<EOF

# CI local registry mirror (added by ci-setup-local-registry.sh)
# Mirror for quay.io/crio images
[[registry]]
prefix = "quay.io/crio"
location = "${REGISTRY_HOST}"
insecure = true

# Mirror for gcr.io (including k8s-staging-cri-tools)
[[registry]]
prefix = "gcr.io"
location = "${REGISTRY_HOST}"
insecure = true

# Mirror for k8s.gcr.io (legacy k8s registry)
[[registry]]
prefix = "k8s.gcr.io"
location = "${REGISTRY_HOST}"
insecure = true

# Mirror for registry.k8s.io (current k8s registry)
[[registry]]
prefix = "registry.k8s.io"
location = "${REGISTRY_HOST}"
insecure = true

# Mirror for quay.io/saschagrunert and quay.io/fedora
[[registry]]
prefix = "quay.io/saschagrunert"
location = "${REGISTRY_HOST}"
insecure = true

[[registry]]
prefix = "quay.io/fedora"
location = "${REGISTRY_HOST}"
insecure = true
EOF

    log_info "Registry configuration updated"
    log_info "Registry mirror configuration:"
    tail -10 "${registries_conf}" || true
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
