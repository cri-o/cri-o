#!/usr/bin/env bash
# shellcheck disable=SC2034

set -e

# Root directory of integration tests.
INTEGRATION_ROOT=${INTEGRATION_ROOT:-$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")}

# Test data path.
TESTDATA="${INTEGRATION_ROOT}/testdata"

# Root directory of the repository.
CRIO_ROOT=${CRIO_ROOT:-$(
    cd "$INTEGRATION_ROOT/.." || exit
    pwd -P
)}

CNI_PLUGIN_NAME=${CNI_PLUGIN_NAME:-log_cni_plugin}

# Path to the crio binary.
CRIO_BINARY=${CRIO_BINARY:-crio}
CRIO_BINARY_PATH=${CRIO_BINARY_PATH:-${CRIO_ROOT}/bin/$CRIO_BINARY}

# Path to the pinns binary
PINNS_BINARY_PATH=${PINNS_BINARY_PATH:-${CRIO_ROOT}/bin/pinns}

# Path of the crictl binary.
CRICTL_BINARY=${CRICTL_BINARY:-$(command -v crictl)}
CRICTL_TIMEOUT=${CRICTL_TIMEOUT:-30s}
# Path of the conmon binary set as a variable to allow overwriting.
CONMON_BINARY=${CONMON_BINARY:-$(command -v conmon)}
# Cgroup for the conmon process
CONTAINER_CONMON_CGROUP=${CONTAINER_CONMON_CGROUP:-pod}
# Path of the default seccomp profile.
CONTAINER_SECCOMP_PROFILE=${CONTAINER_SECCOMP_PROFILE:-${CRIO_ROOT}/vendor/go.podman.io/common/pkg/seccomp/seccomp.json}
CONTAINER_UID_MAPPINGS=${CONTAINER_UID_MAPPINGS:-}
CONTAINER_GID_MAPPINGS=${CONTAINER_GID_MAPPINGS:-}
OVERRIDE_OPTIONS=${OVERRIDE_OPTIONS:-}
# CNI path
if command -v host-local >/dev/null; then
    CONTAINER_CNI_PLUGIN_DIR=${CONTAINER_CNI_PLUGIN_DIR:-$(dirname "$(readlink "$(command -v host-local)")")}
else
    CONTAINER_CNI_PLUGIN_DIR=${CONTAINER_CNI_PLUGIN_DIR:-/opt/cni/bin}
fi
# Runtime
CONTAINER_DEFAULT_RUNTIME=${CONTAINER_DEFAULT_RUNTIME:-crun}
RUNTIME_BINARY_PATH=$(command -v "$CONTAINER_DEFAULT_RUNTIME")
RUNTIME_TYPE=${RUNTIME_TYPE:-oci}
PRIVILEGED_WITHOUT_HOST_DEVICES=${PRIVILEGED_WITHOUT_HOST_DEVICES:-}
RUNTIME_CONFIG_PATH=${RUNTIME_CONFIG_PATH:-""}
# Path of the apparmor_parser binary.
APPARMOR_PARSER_BINARY=${APPARMOR_PARSER_BINARY:-/sbin/apparmor_parser}
# Path of the apparmor profile for test.
APPARMOR_TEST_PROFILE_PATH=${APPARMOR_TEST_PROFILE_PATH:-${TESTDATA}/apparmor_test_deny_write}
# Path of the apparmor profile for unloading crio-default.
FAKE_CRIO_DEFAULT_PROFILE_PATH=${FAKE_CRIO_DEFAULT_PROFILE_PATH:-${TESTDATA}/fake_crio_default}
# Name of the default apparmor profile.
FAKE_CRIO_DEFAULT_PROFILE_NAME=${FAKE_CRIO_DEFAULT_PROFILE_NAME:-crio-default-fake}
# Name of the apparmor profile for test.
APPARMOR_TEST_PROFILE_NAME=${APPARMOR_TEST_PROFILE_NAME:-apparmor-test-deny-write}
# Path of boot config.
BOOT_CONFIG_FILE_PATH=${BOOT_CONFIG_FILE_PATH:-/boot/config-$(uname -r)}
# Path of apparmor parameters file.
APPARMOR_PARAMETERS_FILE_PATH=${APPARMOR_PARAMETERS_FILE_PATH:-/sys/module/apparmor/parameters/enabled}
# Path of the copyimg binary.
COPYIMG_BINARY=${COPYIMG_BINARY:-${CRIO_ROOT}/test/copyimg/copyimg}
# Path of tests artifacts.
ARTIFACTS_PATH=${ARTIFACTS_PATH:-${CRIO_ROOT}/.artifacts}
# Path of the checkseccomp binary.
CHECKSECCOMP_BINARY=${CHECKSECCOMP_BINARY:-${CRIO_ROOT}/test/checkseccomp/checkseccomp}
# Path of the checkcriu binary.
CHECKCRIU_BINARY=${CHECKCRIU_BINARY:-${CRIO_ROOT}/test/checkcriu/checkcriu}
# The default log directory where all logs will go unless directly specified by the kubelet
DEFAULT_LOG_PATH=${DEFAULT_LOG_PATH:-/var/log/crio/pods}
# Cgroup manager to be used
CONTAINER_CGROUP_MANAGER=${CONTAINER_CGROUP_MANAGER:-systemd}
# Image volumes handling
CONTAINER_IMAGE_VOLUMES=${CONTAINER_IMAGE_VOLUMES:-mkdir}
# Container pids limit
CONTAINER_PIDS_LIMIT=${CONTAINER_PIDS_LIMIT:-1024}
# Log size max limit
CONTAINER_LOG_SIZE_MAX=${CONTAINER_LOG_SIZE_MAX:--1}
# Stream Port
STREAM_PORT=${STREAM_PORT:-10010}
# Metrics Host
CONTAINER_METRICS_HOST=${CONTAINER_METRICS_HOST:-127.0.0.1}
# Metrics Port
CONTAINER_METRICS_PORT=${CONTAINER_METRICS_PORT:-9090}
# The default signature policy to be used
SIGNATURE_POLICY=${SIGNATURE_POLICY:-${INTEGRATION_ROOT}/policy.json}
# The default signature policy namespace root to be used
SIGNATURE_POLICY_DIR=${SIGNATURE_POLICY_DIR:-${TESTDATA}/policies}
# irqbalance options
IRQBALANCE_CONFIG_FILE=${IRQBALANCE_CONFIG_FILE:-/etc/sysconfig/irqbalance}
IRQBALANCE_CONFIG_RESTORE_FILE=${IRQBALANCE_CONFIG_RESTORE_FILE:-disable}

POD_IPV4_CIDR="10.88.0.0/16"
# shellcheck disable=SC2034
POD_IPV4_CIDR_START="10.88."
POD_IPV4_DEF_ROUTE="0.0.0.0/0"

POD_IPV6_CIDR="1100:200::/24"
# shellcheck disable=SC2034
POD_IPV6_CIDR_START="1100:2"
POD_IPV6_DEF_ROUTE="1100:200::1/24"

ARCH=$(uname -m)
ARCH_X86_64=x86_64

IMAGES=(
    registry.k8s.io/pause:3.10.1
    quay.io/crio/fedora-crio-ci:latest
    quay.io/crio/hello-wasm:latest
)

function img2dir() {
    local dir
    dir=$(echo "$@" | sed -e 's|^.*/||' -e 's/:.*$//' -e 's/-/_/' -e 's/$/-image/')
    echo "$ARTIFACTS_PATH/$dir"
}

function get_img() {
    local img="docker://$1" dir
    dir="$(img2dir "$img")"

    if ! [ -d "$dir" ]; then
        mkdir -p "$dir"
        if ! "$COPYIMG_BINARY" \
            --import-from="$img" \
            --export-to="dir:$dir" \
            --signature-policy="$INTEGRATION_ROOT"/policy.json; then
            echo "Error pulling $img" >&2
            rm -fr "$dir"
            exit 1
        fi
    fi
}

function get_images() {
    for img in "${IMAGES[@]}"; do
        get_img "$img"
    done
}
