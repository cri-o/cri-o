#!/usr/bin/env bash

# Pod checkpoint/restore test script for CRI-O
# Based on containerd's checkpoint-restore-cri-test.sh
# Tests pod-level checkpoint and restore functionality with multiple containers

set -eu -o pipefail

# Source common test utilities and variables
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=test/common.sh
source "${SCRIPT_DIR}/common.sh"

# Test-specific configuration
CRIO_SOCKET="/var/run/crio/crio.sock"
CONTAINER1_IMAGE="quay.io/adrianreber/wildfly-hello"
CONTAINER2_IMAGE="quay.io/adrianreber/counter"
CHECKPOINT_IMAGE="localhost/checkpoint-pod:test-$$"

# Test state
CRIO_PID=""
CRIO_SOCKET_PREEXISTED=""
POD_ID=""
CONTAINER_ID=""
CONTAINER2_ID=""
RESTORED_POD_ID=""
TEST_DIR=""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Cleanup function
cleanup() {
    local exit_code=$?

    log "Cleaning up test environment..."

    # Stop and remove all pods
    if [ -n "${CRICTL_BINARY}" ] && [ -S "${CRIO_SOCKET}" ]; then
        "${CRICTL_BINARY}" -t 5s rmp -fa 2>/dev/null || true
    fi

    # Remove checkpoint image
    if [ -n "${CHECKPOINT_IMAGE}" ]; then
        buildah rmi "${CHECKPOINT_IMAGE}" 2>/dev/null || true
    fi

    # Stop CRI-O
    if [ -n "${CRIO_PID}" ]; then
        log "Stopping CRI-O (PID: ${CRIO_PID})..."
        # Send SIGTERM first
        if [ -d "/proc/${CRIO_PID}" ]; then
            kill "${CRIO_PID}" 2>/dev/null || true

            # Wait up to 5 seconds for graceful shutdown
            local count=0
            while [ -d "/proc/${CRIO_PID}" ] && [ ${count} -lt 50 ]; do
                sleep 0.1
                count=$((count + 1))
            done

            # If still running, force kill
            if [ -d "/proc/${CRIO_PID}" ]; then
                warn "CRI-O did not stop gracefully, forcing termination..."
                kill -9 "${CRIO_PID}" 2>/dev/null || true
                sleep 0.5
            fi
        fi
    fi

    if [ ${exit_code} -eq 0 ]; then
        log "Test completed successfully!"
        # Clean up test directory on success
        if [ -n "${TEST_DIR}" ] && [ -d "${TEST_DIR}" ]; then
            rm -rf "${TEST_DIR}" 2>/dev/null || true
        fi
    else
        error "Test failed with exit code ${exit_code}"
        # Show CRI-O logs on failure
        if [ -n "${TEST_DIR}" ] && [ -f "${TEST_DIR}/crio.log" ]; then
            error "CRI-O logs (last 50 lines):"
            tail -50 "${TEST_DIR}/crio.log" >&2 || true
            error "Full logs available at: ${TEST_DIR}/crio.log"
        fi
    fi

    # Remove socket only if it was created by this script
    if [ -z "${CRIO_SOCKET_PREEXISTED}" ]; then
        rm -f "${CRIO_SOCKET}" 2>/dev/null || true
    fi

    exit ${exit_code}
}

trap cleanup EXIT

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."

    # Check if running as root
    if [ "$(id -u)" -ne 0 ]; then
        error "This script must be run as root"
        exit 1
    fi

    # Build checkcriu if needed
    if [ ! -f "${CHECKCRIU_BINARY}" ]; then
        log "Building checkcriu tool..."
        (cd "${SCRIPT_DIR}/checkcriu" && go build -o checkcriu checkcriu.go)
    fi

    # Check for CRIU
    log "Checking for CRIU..."
    if ! "${CHECKCRIU_BINARY}"; then
        error "CRIU is not available or version is too old"
        error "Pod checkpointing requires CRIU with pod checkpoint support"
        exit 1
    fi

    # Check for CRI-O binary
    if [ ! -x "${CRIO_BINARY_PATH}" ]; then
        error "CRI-O binary not found at ${CRIO_BINARY_PATH}"
        error "Please build CRI-O first: make bin/crio"
        exit 1
    fi

    # Check for pinns binary
    if [ ! -x "${PINNS_BINARY_PATH}" ]; then
        error "pinns binary not found at ${PINNS_BINARY_PATH}"
        error "Please build pinns first: make bin/pinns"
        exit 1
    fi

    # Check for crictl
    if [ -z "${CRICTL_BINARY}" ] || [ ! -x "${CRICTL_BINARY}" ]; then
        error "crictl binary not found"
        error "Please install crictl or build from cri-tools: (cd ../cri-tools && make crictl)"
        exit 1
    fi

    # Check if crictl supports pod checkpoint/restore commands
    # Note: We capture output first to avoid SIGPIPE issues with grep -q and pipefail
    log "Checking crictl pod checkpoint/restore support..."
    local crictl_help
    crictl_help=$("${CRICTL_BINARY}" --help 2>&1 || true)

    if ! echo "${crictl_help}" | grep -q "checkpointp"; then
        error "crictl does not support 'checkpointp' command"
        error "This test requires crictl with pod checkpoint support"
        error "Please build crictl from cri-tools with pod checkpoint support"
        exit 1
    fi

    if ! echo "${crictl_help}" | grep -q "restorep"; then
        error "crictl does not support 'restorep' command"
        error "This test requires crictl with pod restore support"
        error "Please build crictl from cri-tools with pod restore support"
        exit 1
    fi
    log "crictl supports pod checkpoint/restore commands"

    # Check for buildah
    if ! command -v buildah >/dev/null 2>&1; then
        error "buildah is required for checkpoint image management"
        exit 1
    fi

    # Check for jq
    if ! command -v jq >/dev/null 2>&1; then
        error "jq is required for JSON parsing"
        exit 1
    fi

    # Check for curl
    if ! command -v curl >/dev/null 2>&1; then
        error "curl is required for testing containers"
        exit 1
    fi

    log "All prerequisites satisfied"
}

# Start CRI-O server
start_crio() {
    log "Starting CRI-O server..."

    # Create test directory for CRI-O runtime
    TEST_DIR="$(mktemp -d /tmp/crio-test.XXXXXX)"
    log "Test directory: ${TEST_DIR}"

    # Create crictl config to suppress warnings
    cat >"${TEST_DIR}/crictl.yaml" <<EOF
runtime-endpoint: unix://${CRIO_SOCKET}
image-endpoint: unix://${CRIO_SOCKET}
timeout: 20
EOF

    # Export crictl configuration
    export CRI_CONFIG_FILE="${TEST_DIR}/crictl.yaml"

    # Check for a preexisting socket (e.g., a system CRI-O already running)
    if [ -S "${CRIO_SOCKET}" ]; then
        CRIO_SOCKET_PREEXISTED=1
        error "CRI-O socket already exists at ${CRIO_SOCKET}"
        error "Another CRI-O instance may be running. Stop it before running this test."
        exit 1
    fi

    # Start CRI-O in background (output redirected to log file)
    "${CRIO_BINARY_PATH}" \
        --pinns-path "${PINNS_BINARY_PATH}" \
        --default-runtime runc \
        --log-level debug \
        --enable-pod-events \
        >"${TEST_DIR}/crio.log" 2>&1 &

    CRIO_PID=$!
    log "CRI-O started with PID ${CRIO_PID} (logs: ${TEST_DIR}/crio.log)"

    # Wait for CRI-O socket
    log "Waiting for CRI-O socket..."
    local timeout=30
    local count=0
    while [ ! -S "${CRIO_SOCKET}" ]; do
        sleep 1
        count=$((count + 1))
        if [ ${count} -ge ${timeout} ]; then
            error "Timeout waiting for CRI-O socket"
            exit 1
        fi
    done

    log "CRI-O socket ready"
}

# Pull test images
pull_test_images() {
    log "Pulling container 1 image: ${CONTAINER1_IMAGE}"
    "${CRICTL_BINARY}" pull "${CONTAINER1_IMAGE}"

    log "Pulling container 2 image: ${CONTAINER2_IMAGE}"
    "${CRICTL_BINARY}" pull "${CONTAINER2_IMAGE}"
}

# Create pod configuration
create_pod_config() {
    local config_file="${TEST_DIR}/pod-config.json"

    cat >"${config_file}" <<EOF
{
    "metadata": {
        "name": "test-pod-$$",
        "uid": "test-pod-uid-$$",
        "namespace": "default"
    },
    "log_directory": "${TEST_DIR}"
}
EOF

    echo "${config_file}"
}

# Create container configuration
create_container_config() {
    local container_name="$1"
    local container_image="$2"
    local config_file="${TEST_DIR}/${container_name}-config.json"

    cat >"${config_file}" <<EOF
{
    "metadata": {
        "name": "${container_name}"
    },
    "image": {
        "image": "${container_image}"
    },
    "log_path": "${container_name}.log",
    "linux": {}
}
EOF

    echo "${config_file}"
}

# Test pod checkpoint and restore
test_pod_checkpoint_restore() {
    log "=== Testing Pod Checkpoint and Restore ==="

    # Create configurations
    local pod_config
    pod_config="$(create_pod_config)"

    local container1_config
    container1_config="$(create_container_config "container1" "${CONTAINER1_IMAGE}")"

    local container2_config
    container2_config="$(create_container_config "container2" "${CONTAINER2_IMAGE}")"

    # Create pod
    log "Creating pod..."
    POD_ID=$("${CRICTL_BINARY}" runp "${pod_config}")
    log "Pod created: ${POD_ID}"

    # Create first container
    log "Creating first container..."
    CONTAINER_ID=$("${CRICTL_BINARY}" create "${POD_ID}" "${container1_config}" "${pod_config}")
    log "Container 1 created: ${CONTAINER_ID}"

    # Create second container
    log "Creating second container..."
    CONTAINER2_ID=$("${CRICTL_BINARY}" create "${POD_ID}" "${container2_config}" "${pod_config}")
    log "Container 2 created: ${CONTAINER2_ID}"

    # Start both containers
    log "Starting containers..."
    "${CRICTL_BINARY}" start "${CONTAINER_ID}"
    "${CRICTL_BINARY}" start "${CONTAINER2_ID}"

    # Wait for containers to be running
    log "Waiting for containers to be ready..."
    local container1_state="UNKNOWN"
    local container2_state="UNKNOWN"
    local wait_count=0
    while [ ${wait_count} -lt 30 ]; do
        container1_state=$("${CRICTL_BINARY}" inspect "${CONTAINER_ID}" 2>/dev/null | jq -r '.status.state' || echo "UNKNOWN")
        container2_state=$("${CRICTL_BINARY}" inspect "${CONTAINER2_ID}" 2>/dev/null | jq -r '.status.state' || echo "UNKNOWN")
        if [ "${container1_state}" = "CONTAINER_RUNNING" ] && [ "${container2_state}" = "CONTAINER_RUNNING" ]; then
            break
        fi
        sleep 1
        wait_count=$((wait_count + 1))
    done

    if [ "${container1_state}" != "CONTAINER_RUNNING" ]; then
        error "Container 1 is not running after 30s (state: ${container1_state})"
        exit 1
    fi

    if [ "${container2_state}" != "CONTAINER_RUNNING" ]; then
        error "Container 2 is not running after 30s (state: ${container2_state})"
        exit 1
    fi

    log "Both containers are running"

    # Get pod IP address
    local pod_ip
    pod_ip=$("${CRICTL_BINARY}" inspectp --output go-template --template '{{.status.network.ip}}' "${POD_ID}")
    log "Pod IP address: ${pod_ip}"

    # Test wildfly-hello container
    log "Testing wildfly-hello container at ${pod_ip}:8080/helloworld/..."
    local wildfly_response=""
    local count=0
    local max_attempts=30
    while [ ${count} -lt ${max_attempts} ]; do
        wildfly_response=$(curl -s --max-time 2 "http://${pod_ip}:8080/helloworld/" 2>/dev/null || echo "")
        if [[ "${wildfly_response}" =~ ^[0-9]+$ ]]; then
            log "wildfly-hello responded with counter: ${wildfly_response}"
            break
        fi
        count=$((count + 1))
        sleep 1
    done

    if ! [[ "${wildfly_response}" =~ ^[0-9]+$ ]]; then
        error "wildfly-hello did not respond correctly after ${max_attempts}s (response: ${wildfly_response})"
        exit 1
    fi

    # Test counter container
    log "Testing counter container at ${pod_ip}:8088/..."
    local counter_response=""
    count=0
    while [ ${count} -lt ${max_attempts} ]; do
        counter_response=$(curl -s --max-time 2 "http://${pod_ip}:8088/" 2>/dev/null || echo "")
        if [[ "${counter_response}" =~ ^counter:\ [0-9]+$ ]]; then
            log "counter responded with: ${counter_response}"
            break
        fi
        count=$((count + 1))
        sleep 1
    done

    if ! [[ "${counter_response}" =~ ^counter:\ [0-9]+$ ]]; then
        error "counter did not respond correctly after ${max_attempts}s (response: ${counter_response})"
        exit 1
    fi

    log "Both containers are responding correctly"

    # Checkpoint the pod
    log "Checkpointing pod to image: ${CHECKPOINT_IMAGE}"
    "${CRICTL_BINARY}" -t 20s checkpointp --export="${CHECKPOINT_IMAGE}" "${POD_ID}"

    # Verify checkpoint image was created
    # Note: Capture output first to avoid SIGPIPE issues with grep -q and pipefail
    log "Verifying checkpoint image..."
    local buildah_images
    buildah_images=$(buildah images 2>&1 || true)
    if ! echo "${buildah_images}" | grep -q "checkpoint-pod"; then
        error "Checkpoint image was not created"
        exit 1
    fi
    log "Checkpoint image created successfully"

    # Inspect checkpoint image annotations
    log "Inspecting checkpoint image annotations..."
    local annotations_output
    annotations_output=$(buildah inspect --format '{{range $key, $value := .ImageAnnotations}}  🏷️  {{$key}}={{$value}}{{println}}{{end}}' "${CHECKPOINT_IMAGE}" 2>/dev/null || echo "")

    # Print all annotations
    if [ -n "${annotations_output}" ]; then
        echo "${annotations_output}"
    fi

    # Count org.criu annotations
    local criu_count=0
    if [ -n "${annotations_output}" ]; then
        criu_count=$(echo "${annotations_output}" | grep -c "org\.criu\.checkpoint\." 2>/dev/null || echo "0")
        # Remove any whitespace/newlines
        criu_count=$(echo "${criu_count}" | tr -d '[:space:]')
    fi

    log "Found ${criu_count} CRIU checkpoint annotations"

    if [ "${criu_count}" -lt 3 ]; then
        error "Expected at least 3 org.criu annotations, found ${criu_count}"
        exit 1
    fi

    # Verify annotations in pod.options file inside the checkpoint image
    log "Mounting checkpoint image to verify pod.options annotations..."
    local cp_container
    cp_container=$(buildah from "${CHECKPOINT_IMAGE}")
    local cp_mountpoint
    cp_mountpoint=$(buildah mount "${cp_container}")
    log "Checkpoint image mounted at ${cp_mountpoint}"

    local pod_options_file="${cp_mountpoint}/pod.options"
    if [ ! -f "${pod_options_file}" ]; then
        buildah umount "${cp_container}" >/dev/null 2>&1 || true
        buildah rm "${cp_container}" >/dev/null 2>&1 || true
        error "pod.options file not found in checkpoint image"
        exit 1
    fi

    # Print all annotations from pod.options in the same format as OCI image annotations
    local pod_opts_annotations
    pod_opts_annotations=$(jq -r '.annotations // {} | to_entries[] | "  🏷️  \(.key)=\(.value)"' "${pod_options_file}" 2>/dev/null || echo "")

    if [ -n "${pod_opts_annotations}" ]; then
        echo "${pod_opts_annotations}"
    fi

    # Count org.criu annotations in pod.options
    local pod_opts_ann_count=0
    if [ -n "${pod_opts_annotations}" ]; then
        pod_opts_ann_count=$(echo "${pod_opts_annotations}" | grep -c "org\.criu\.checkpoint\." 2>/dev/null || echo "0")
        pod_opts_ann_count=$(echo "${pod_opts_ann_count}" | tr -d '[:space:]')
    fi

    log "Found ${pod_opts_ann_count} CRIU checkpoint annotations in pod.options"

    if [ "${pod_opts_ann_count}" -lt 3 ]; then
        buildah umount "${cp_container}" >/dev/null 2>&1 || true
        buildah rm "${cp_container}" >/dev/null 2>&1 || true
        error "Expected at least 3 org.criu annotations in pod.options, found ${pod_opts_ann_count}"
        exit 1
    fi

    # Check for specific required annotation keys
    for key in "org.criu.checkpoint.pod.name" "org.criu.checkpoint.pod.id" \
        "org.criu.checkpoint.pod.namespace" "org.criu.checkpoint.pod.uid" \
        "org.criu.checkpoint.engine.name"; do
        if ! jq -e --arg k "${key}" '.annotations[$k]' "${pod_options_file}" >/dev/null 2>&1; then
            buildah umount "${cp_container}" >/dev/null 2>&1 || true
            buildah rm "${cp_container}" >/dev/null 2>&1 || true
            error "Missing expected annotation '${key}' in pod.options"
            exit 1
        fi
    done

    log "All expected annotations found in pod.options"

    buildah umount "${cp_container}"
    buildah rm "${cp_container}"

    # Remove all pods
    log "Removing all pods..."
    "${CRICTL_BINARY}" -t 5s rmp -fa

    # Verify pod is removed
    sleep 2
    local pods_list
    pods_list=$("${CRICTL_BINARY}" pods 2>&1 || true)
    if echo "${pods_list}" | grep -q "${POD_ID}"; then
        error "Pod was not removed"
        exit 1
    fi
    log "Pod removed successfully"

    # Restore the pod
    log "Restoring pod from image: ${CHECKPOINT_IMAGE}"
    RESTORED_POD_ID=$("${CRICTL_BINARY}" -t 20s restorep -l "${CHECKPOINT_IMAGE}")
    log "Pod restored: ${RESTORED_POD_ID}"

    # Wait for restored containers to be running
    log "Waiting for restored containers to be ready..."

    local restored_containers=""
    local container_count=0
    wait_count=0
    while [ ${wait_count} -lt 30 ]; do
        restored_containers=$("${CRICTL_BINARY}" ps --pod="${RESTORED_POD_ID}" -q 2>/dev/null || echo "")
        if [ -n "${restored_containers}" ]; then
            container_count=$(echo "${restored_containers}" | wc -l)
            if [ "${container_count}" -ge 2 ]; then
                # Check that all containers are running
                local all_running=true
                for ctr_id in ${restored_containers}; do
                    local state
                    state=$("${CRICTL_BINARY}" inspect "${ctr_id}" 2>/dev/null | jq -r '.status.state' || echo "UNKNOWN")
                    if [ "${state}" != "CONTAINER_RUNNING" ]; then
                        all_running=false
                        break
                    fi
                done
                if [ "${all_running}" = true ]; then
                    break
                fi
            fi
        fi
        sleep 1
        wait_count=$((wait_count + 1))
    done

    # Verify restored pod and containers
    log "Verifying restored pod and containers..."
    "${CRICTL_BINARY}" pods
    "${CRICTL_BINARY}" ps -a

    if [ "${container_count}" -ne 2 ]; then
        error "Expected 2 containers in restored pod, found ${container_count}"
        exit 1
    fi

    log "Found ${container_count} containers in restored pod"

    # Verify both containers are running
    for ctr_id in ${restored_containers}; do
        local state
        state=$("${CRICTL_BINARY}" inspect "${ctr_id}" 2>/dev/null | jq -r '.status.state' || echo "UNKNOWN")

        if [ "${state}" != "CONTAINER_RUNNING" ]; then
            error "Restored container ${ctr_id} is not running after 30s (state: ${state})"
            exit 1
        fi

        local name
        name=$("${CRICTL_BINARY}" inspect "${ctr_id}" 2>/dev/null | jq -r '.status.metadata.name' || echo "unknown")
        log "Container ${name} (${ctr_id}) is running"
    done

    # Get restored pod IP address
    local restored_pod_ip
    restored_pod_ip=$("${CRICTL_BINARY}" inspectp --output go-template --template '{{.status.network.ip}}' "${RESTORED_POD_ID}")
    log "Restored pod IP address: ${restored_pod_ip}"

    # Test restored wildfly-hello container
    log "Testing restored wildfly-hello container at ${restored_pod_ip}:8080/helloworld/..."
    local restored_wildfly_response=""
    local count=0
    local max_attempts=30
    while [ ${count} -lt ${max_attempts} ]; do
        restored_wildfly_response=$(curl -s --max-time 2 "http://${restored_pod_ip}:8080/helloworld/" 2>/dev/null || echo "")
        if [[ "${restored_wildfly_response}" =~ ^[0-9]+$ ]]; then
            log "Restored wildfly-hello responded with counter: ${restored_wildfly_response}"
            break
        fi
        count=$((count + 1))
        sleep 1
    done

    if ! [[ "${restored_wildfly_response}" =~ ^[0-9]+$ ]]; then
        error "Restored wildfly-hello did not respond correctly after ${max_attempts}s (response: ${restored_wildfly_response})"
        exit 1
    fi

    # Test restored counter container
    log "Testing restored counter container at ${restored_pod_ip}:8088/..."
    local restored_counter_response=""
    count=0
    while [ ${count} -lt ${max_attempts} ]; do
        restored_counter_response=$(curl -s --max-time 2 "http://${restored_pod_ip}:8088/" 2>/dev/null || echo "")
        if [[ "${restored_counter_response}" =~ ^counter:\ [0-9]+$ ]]; then
            log "Restored counter responded with: ${restored_counter_response}"
            break
        fi
        count=$((count + 1))
        sleep 1
    done

    if ! [[ "${restored_counter_response}" =~ ^counter:\ [0-9]+$ ]]; then
        error "Restored counter did not respond correctly after ${max_attempts}s (response: ${restored_counter_response})"
        exit 1
    fi

    log "Both restored containers are responding correctly"

    # Verify that counter values are incremented after restore
    log "Verifying that counters are incremented after restore..."

    # Extract numeric value from wildfly-hello responses
    local wildfly_count_before="${wildfly_response}"
    local wildfly_count_after="${restored_wildfly_response}"

    # Extract numeric value from counter responses (format: "counter: X")
    local counter_count_before
    counter_count_before=$(echo "${counter_response}" | sed 's/counter: //')
    local counter_count_after
    counter_count_after=$(echo "${restored_counter_response}" | sed 's/counter: //')

    log "wildfly-hello counter: before=${wildfly_count_before}, after=${wildfly_count_after}"
    log "counter container: before=${counter_count_before}, after=${counter_count_after}"

    # Verify wildfly-hello counter is incremented
    if [ "${wildfly_count_after}" -le "${wildfly_count_before}" ]; then
        error "wildfly-hello counter did not increment (before: ${wildfly_count_before}, after: ${wildfly_count_after})"
        exit 1
    fi
    log "✓ wildfly-hello counter incremented from ${wildfly_count_before} to ${wildfly_count_after}"

    # Verify counter container is incremented
    if [ "${counter_count_after}" -le "${counter_count_before}" ]; then
        error "counter container did not increment (before: ${counter_count_before}, after: ${counter_count_after})"
        exit 1
    fi
    log "✓ counter container incremented from ${counter_count_before} to ${counter_count_after}"

    log "Pod checkpoint and restore test completed successfully!"
}

# Main test execution
main() {
    log "=== CRI-O Pod Checkpoint/Restore Test ==="
    log "Test timestamp: $(date)"
    log "CRI-O binary: ${CRIO_BINARY_PATH}"
    log "crictl binary: ${CRICTL_BINARY}"
    log "pinns binary: ${PINNS_BINARY_PATH}"
    log "Container 1 image: ${CONTAINER1_IMAGE}"
    log "Container 2 image: ${CONTAINER2_IMAGE}"

    check_prerequisites
    start_crio
    pull_test_images
    test_pod_checkpoint_restore

    log "All tests passed!"
}

main "$@"
