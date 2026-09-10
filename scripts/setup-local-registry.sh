#!/usr/bin/env bash
#
# Setup a local registry mirror for CRI-O integration tests.
#
# Starts a registry:2 container on localhost:5000 and copies all test images
# into it using skopeo. The test runner (test_runner.sh) configures CRI-O to
# use this registry as a mirror via CONTAINER_REGISTRIES_CONF_DIR, so that
# tests pull from localhost instead of upstream registries.
#
# Mirror layout follows the [[registry.mirror]] convention: the upstream
# registry prefix is stripped, so quay.io/crio/alpine:3.9 becomes
# localhost:5000/crio/alpine:3.9.
#
# Usage: scripts/setup-local-registry.sh
# Requires: podman, skopeo, curl

set -euo pipefail

REGISTRY_NAME="crio-test-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"
REGISTRY_HOST="localhost:${REGISTRY_PORT}"
MAX_RETRIES="${MAX_RETRIES:-3}"
RETRY_DELAY="${RETRY_DELAY:-5}"

# All images used in integration tests and critest
IMAGES=(
    # Core test images (common.sh / test_runner.sh)
    "registry.k8s.io/pause:3.10.1"
    "quay.io/crio/fedora-crio-ci:latest"
    "quay.io/crio/hello-wasm:latest"

    # BATS test images
    "quay.io/crio/alpine:3.9"
    "quay.io/crio/artifact:v1"
    "quay.io/saschagrunert/hello-world:latest"
    "quay.io/fedora/fedora:latest"

    # critest images
    "gcr.io/k8s-staging-cri-tools/test-image-predefined-group:latest"
    "gcr.io/k8s-staging-cri-tools/hostnet-nginx-arm64:latest"
    "gcr.io/k8s-staging-cri-tools/hostnet-nginx-amd64:latest"
    "gcr.io/k8s-staging-cri-tools/test-image-tags:1"
    "gcr.io/k8s-staging-cri-tools/test-image-tags:2"
    "gcr.io/k8s-staging-cri-tools/test-image-latest:latest"
    "gcr.io/k8s-staging-cri-tools/test-image-tag:test"
    "gcr.io/k8s-staging-cri-tools/test-image-1:latest"
    "gcr.io/k8s-staging-cri-tools/test-image-2:latest"
    "gcr.io/k8s-staging-cri-tools/test-image-3:latest"
    "k8s.gcr.io/pause:3.10.1"

    # registry.k8s.io e2e test images
    "registry.k8s.io/e2e-test-images/busybox:1.29-2"
    "registry.k8s.io/e2e-test-images/nginx:1.14-2"
    "registry.k8s.io/e2e-test-images/httpd:2.4.39-4"
    "registry.k8s.io/e2e-test-images/nonewprivs:1.3"

    # Other
    "registry.access.redhat.com/rhel7-atomic:latest"
    "quay.io/crio/nginx@sha256:960355a671fb88ef18a85f92ccf2ccf8e12186216c86337ad808c204d69d512d"

    # OCI artifacts
    "quay.io/crio/artifact:singlefile"
    "quay.io/crio/artifact:multiplefiles"
    "quay.io/crio/artifact:exec"
    "quay.io/crio/seccomp:v2"
)

# Retry a command up to MAX_RETRIES times
retry() {
    local attempt=1
    while [ ${attempt} -le ${MAX_RETRIES} ]; do
        if "$@"; then
            return 0
        fi
        if [ ${attempt} -lt ${MAX_RETRIES} ]; then
            echo "[WARN] Attempt ${attempt}/${MAX_RETRIES} failed, retrying in ${RETRY_DELAY}s..." >&2
            sleep "${RETRY_DELAY}"
        fi
        ((attempt++))
    done
    return 1
}

# Copy a single image into the local registry, preserving manifest lists
mirror_image() {
    local remote_image="$1"
    local repo_path local_image

    if [[ "${remote_image}" =~ @ ]]; then
        repo_path="${remote_image%%@*}"
        repo_path="${repo_path#*/}"
        local digest="${remote_image##*@}"
        local short_hash="sha256-${digest##*:}"
        short_hash="${short_hash:0:20}"
        local_image="${REGISTRY_HOST}/${repo_path}:${short_hash}"
    else
        repo_path="${remote_image#*/}"
        local_image="${REGISTRY_HOST}/${repo_path}"
    fi

    echo "[INFO] Mirroring ${remote_image} -> ${local_image}"
    retry skopeo copy --all "docker://${remote_image}" "docker://${local_image}" --dest-tls-verify=false
}

setup_registry() {
    echo "[INFO] Setting up local registry at ${REGISTRY_HOST}"
    podman rm -f "${REGISTRY_NAME}" 2>/dev/null || true

    retry podman pull docker.io/library/registry:2

    podman run -d \
        --restart=always \
        --network=host \
        --name "${REGISTRY_NAME}" \
        -e "REGISTRY_HTTP_ADDR=0.0.0.0:${REGISTRY_PORT}" \
        docker.io/library/registry:2

    local count=0
    while ! curl -sf "http://${REGISTRY_HOST}/v2/" >/dev/null 2>&1; do
        if [ ${count} -ge 30 ]; then
            echo "[ERROR] Registry failed to start within 30 seconds" >&2
            podman logs "${REGISTRY_NAME}" >&2
            return 1
        fi
        sleep 1
        ((count++))
    done
    echo "[INFO] Registry ready at http://${REGISTRY_HOST}"
}

mirror_all_images() {
    echo "[INFO] Mirroring ${#IMAGES[@]} images"
    local failed=0 succeeded=0

    for img in "${IMAGES[@]}"; do
        if mirror_image "${img}"; then
            ((succeeded++)) || true
        else
            echo "[WARN] Failed to mirror ${img}" >&2
            ((failed++)) || true
        fi
    done

    echo "[INFO] Mirror summary: ${succeeded} succeeded, ${failed} failed"
    if [ ${succeeded} -lt 3 ]; then
        echo "[ERROR] Too few images mirrored (need at least 3 core images)" >&2
        return 1
    fi
}

main() {
    if ! command -v podman &>/dev/null; then
        echo "[INFO] Podman not found, skipping local registry setup"
        exit 0
    fi

    setup_registry
    mirror_all_images

    echo "[INFO] Local registry setup complete"
    curl -sf "http://${REGISTRY_HOST}/v2/_catalog" 2>/dev/null || true
}

main "$@"
