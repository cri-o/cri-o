#!/usr/bin/env bash

. common.sh
bats_require_minimum_version 1.9.0
export BATS_VERBOSE_RUN=1

function setup_test() {
    TESTDIR=$(mktemp -d)

    # Setup default hooks dir
    HOOKSDIR=$TESTDIR/hooks
    mkdir "$HOOKSDIR"

    HOOKSCHECK=$TESTDIR/hookscheck
    CONTAINER_EXITS_DIR=$TESTDIR/containers/exits
    CONTAINER_ATTACH_SOCKET_DIR=$TESTDIR/containers

    # We may need to set some default storage options.
    case "$(stat -f -c %T "$TESTDIR")" in
    aufs)
        # None of device mapper, overlay, or aufs can be used dependably over aufs, and of course btrfs and zfs can't,
        # and we have to explicitly specify the "vfs" driver in order to use it, so do that now.
        STORAGE_OPTIONS=${STORAGE_OPTIONS:--s vfs}
        ;;
    *)
        STORAGE_OPTIONS=${STORAGE_OPTIONS:-}
        ;;
    esac

    if [ -e /usr/sbin/selinuxenabled ] && /usr/sbin/selinuxenabled; then
        # shellcheck disable=SC1091
        . /etc/selinux/config
        filelabel=$(awk -F'"' '/^file.*=.*/ {print $2}' "/etc/selinux/${SELINUXTYPE}/contexts/lxc_contexts")
        chcon -R "$filelabel" "$TESTDIR"
    fi
    CRIO_SOCKET="$TESTDIR/crio.sock"
    CRIO_CONFIG_DIR="$TESTDIR/crio.conf.d"
    mkdir "$CRIO_CONFIG_DIR"
    CRIO_CONFIG="$TESTDIR/crio.conf"
    CRIO_CUSTOM_CONFIG="$CRIO_CONFIG_DIR/00-crio-custom.conf"
    CRIO_CNI_CONFIG="$TESTDIR/cni/net.d/"
    CRIO_LOG="$TESTDIR/crio.log"

    # Override NRI socket to a testcase-specific location.
    CRIO_NRI_CONFIG="$CRIO_CONFIG_DIR/10-crio-nri.conf"
    NRI_SOCKET="$TESTDIR/nri.sock"
    cat <<EOF >"$CRIO_NRI_CONFIG"
[crio.nri]
nri_listen = "$NRI_SOCKET"
EOF

    # Copy all the CNI dependencies around to ensure encapsulated tests
    CRIO_CNI_PLUGIN="$TESTDIR/cni-bin"
    mkdir "$CRIO_CNI_PLUGIN"
    cp "$CONTAINER_CNI_PLUGIN_DIR"/* "$CRIO_CNI_PLUGIN"
    cp "$INTEGRATION_ROOT"/cni_plugin_helper.bash "$CRIO_CNI_PLUGIN"
    sed -i "s;%TEST_DIR%;$TESTDIR;" "$CRIO_CNI_PLUGIN"/cni_plugin_helper.bash

    # Configure crictl to not try pulling images on create/run,
    # as $IMAGES are already preloaded, and eliminating network
    # interaction results in less flakes when creating containers.
    #
    # A test case that requires an image not listed in $IMAGES
    # should either do an explicit "crictl pull", or use --with-pull.
    #
    # Make sure concurrent test cases don't stomp on each other by
    # updating the configuration file in place while another test
    # case is using it.

    CRICTL_CONFIG_FILE="$TESTDIR"/crictl.yaml
    touch "$CRICTL_CONFIG_FILE"
    crictl config \
        --set pull-image-on-create=false \
        --set disable-pull-on-run=true

    PATH=$PATH:$TESTDIR
}

# Run crio using the binary specified by $CRIO_BINARY_PATH.
# This must ONLY be run on engines created with `start_crio`.
function crio() {
    "$CRIO_BINARY_PATH" --listen "$CRIO_SOCKET" "$@"
}

# Run crictl using the binary specified by $CRICTL_BINARY.
function crictl() {
    "$CRICTL_BINARY" -t 10m --config "$CRICTL_CONFIG_FILE" -r "unix://$CRIO_SOCKET" -i "unix://$CRIO_SOCKET" "$@"
}

# Run the runtime binary with the specified RUNTIME_ROOT
function runtime() {
    "$RUNTIME_BINARY_PATH" --root "$RUNTIME_ROOT" "$@"
}

# Communicate with Docker on the host machine.
# Should rarely use this.
function docker_host() {
    command docker "$@"
}

# Retry a command $1 times until it succeeds. Wait $2 seconds between retries.
function retry() {
    local attempts=$1
    shift
    local delay=$1
    shift
    local i

    for ((i = 0; i < attempts; i++)); do
        if output=$("$@"); then
            return 0
        fi
        sleep "$delay"
    done

    echo "Command \"$*\" failed $attempts times; last output :: $output"
    false
}

# Waits until crio becomes reachable.
function wait_until_reachable() {
    retry 15 1 crictl info
}

function copyimg() {
    # Don't forget: copyimg and crio have their own default drivers,
    # so if you override any, you probably need to override them all.

    # shellcheck disable=SC2086
    "$COPYIMG_BINARY" \
        --root "$TESTDIR/crio" \
        --runroot "$TESTDIR/crio-run" \
        --signature-policy="$INTEGRATION_ROOT"/policy.json \
        $STORAGE_OPTIONS \
        "$@"
}

function setup_img() {
    local name="$1" dir
    dir="$(img2dir "$name")"

    copyimg --image-name="$name" --import-from="dir:$dir"
}

function setup_crio() {
    apparmor=""
    if [[ -n "$1" ]]; then
        apparmor="$1"
    fi

    for img in "${IMAGES[@]}"; do
        setup_img "$img"
    done

    # Prepare the CNI configuration files, we're running with non host
    # networking by default
    CNI_DEFAULT_NETWORK=${CNI_DEFAULT_NETWORK:-crio}
    CNI_TYPE=${CNI_TYPE:-bridge}

    RUNTIME_ROOT=${RUNTIME_ROOT:-"$TESTDIR/crio-runtime-root"}
    # export here so direct calls to crio later inherit the variable
    export CONTAINER_RUNTIMES=${CONTAINER_RUNTIMES:-$CONTAINER_DEFAULT_RUNTIME:$RUNTIME_BINARY_PATH:$RUNTIME_ROOT:$RUNTIME_TYPE:$PRIVILEGED_WITHOUT_HOST_DEVICES:$RUNTIME_CONFIG_PATH}

    # generate the default config file
    "$CRIO_BINARY_PATH" config --default >"$CRIO_CONFIG"

    # shellcheck disable=SC2086
    "$CRIO_BINARY_PATH" \
        --hooks-dir="$HOOKSDIR" \
        --apparmor-profile "$apparmor" \
        --cgroup-manager "$CONTAINER_CGROUP_MANAGER" \
        --conmon "$CONMON_BINARY" \
        --container-attach-socket-dir "$CONTAINER_ATTACH_SOCKET_DIR" \
        --container-exits-dir "$CONTAINER_EXITS_DIR" \
        --listen "$CRIO_SOCKET" \
        --irqbalance-config-file "$IRQBALANCE_CONFIG_FILE" \
        --irqbalance-config-restore-file "$IRQBALANCE_CONFIG_RESTORE_FILE" \
        --signature-policy "$SIGNATURE_POLICY" \
        --signature-policy-dir "$SIGNATURE_POLICY_DIR" \
        -r "$TESTDIR/crio" \
        --runroot "$TESTDIR/crio-run" \
        --cni-default-network "$CNI_DEFAULT_NETWORK" \
        --cni-config-dir "$CRIO_CNI_CONFIG" \
        --cni-plugin-dir "$CRIO_CNI_PLUGIN" \
        --pinns-path "$PINNS_BINARY_PATH" \
        $STORAGE_OPTIONS \
        -c "" \
        -d "" \
        $OVERRIDE_OPTIONS \
        config >"$CRIO_CUSTOM_CONFIG"
    # make sure we don't run with nodev, or else mounting a readonly rootfs will fail: https://github.com/cri-o/cri-o/issues/1929#issuecomment-474240498
    sed -r -e 's/nodev(,)?//g' -i "$CRIO_CONFIG"
    sed -r -e 's/nodev(,)?//g' -i "$CRIO_CUSTOM_CONFIG"
    prepare_network_conf
}

function check_images() {
    local img json list

    # check that images are there
    json=$(crictl images -o json)
    [ -n "$json" ]
    list=$(jq -r '.images[] | .repoTags[]' <<<"$json")
    for img in "${IMAGES[@]}"; do
        if [[ "$list" != *"$img"* ]]; then
            echo "Image $img is not present but it should!" >&2
            exit 1
        fi
    done

    # these two variables are used by a few tests
    eval "$(jq -r '.images[] |
        select(.repoTags[0] == "quay.io/crio/fedora-crio-ci:latest") |
        "REDIS_IMAGEID=" + .id + "\n" +
	"REDIS_IMAGEREF=" + .repoDigests[0]' <<<"$json")"
}

function start_crio_no_setup() {
    "$CRIO_BINARY_PATH" \
        --default-mounts-file "$TESTDIR/containers/mounts.conf" \
        -l debug \
        -c "$CRIO_CONFIG" \
        -d "$CRIO_CONFIG_DIR" \
        &>"$CRIO_LOG" &
    CRIO_PID=$!
    wait_until_reachable
}

# Start crio.
# shellcheck disable=SC2120
function start_crio() {
    setup_crio "$@"
    start_crio_no_setup
    check_images
}

# Check if journald is supported by runtime.
function check_journald() {
    "$CONMON_BINARY" \
        -l journald:42 \
        --cid 1234567890123 \
        --cuuid 42 \
        --runtime /bin/true &&
        journalctl --version
}

# get a random available port
function free_port() {
    python -c 'import socket; s=socket.socket(); s.bind(("", 0)); print(s.getsockname()[1]); s.close()'
}

# Check whether a port is listening
function port_listens() {
    netstat -ln46 | grep -q ":$1\b"
}

# Check whether a specific pair of IP address or hostname and port number listens.
function host_and_port_listens() {
    local host="$1"
    local port="$2"

    netstat -ln46 | grep -E -q "${host}:${port}\b"
}

function check_kernel_version() {
    local version="$1"

    required_major=${version%%.*}
    required_minor=${version##*.}

    [[ $(uname -r) =~ ([0-9]+)\.([0-9]+) ]]
    major=${BASH_REMATCH[1]}
    minor=${BASH_REMATCH[2]}

    ((major > required_major)) || ((major == required_major && minor >= required_minor))
}

function check_crictl_version() {
    local version="$1"

    required_major=${version%%.*}
    required_minor=${version##*.}

    crictl_binary=${CRICTL_BINARY:-/usr/bin/crictl}

    [[ $($crictl_binary --version) =~ ([0-9]+)\.([0-9]+) ]]
    major=${BASH_REMATCH[1]}
    minor=${BASH_REMATCH[2]}

    ((major > required_major)) || ((major == required_major && minor >= required_minor))
}

function requires_kernel() {
    check_kernel_version "$@" ||
        skip "requires kernel version \"$1\" or newer"
}

function requires_crictl() {
    check_crictl_version "$@" ||
        skip "requires crictl version \"$1\" or newer"
}

function cleanup_ctrs() {
    crictl rm -a -f
    rm -f "$HOOKSCHECK"
}

function cleanup_images() {
    crictl rmi -a -q
}

function cleanup_pods() {
    crictl rmp -a -f
}

function stop_crio_no_clean() {
    local signal="$1"
    if [ -n "${CRIO_PID+x}" ]; then
        kill "$signal" "$CRIO_PID" >/dev/null 2>&1 || true
        wait "$CRIO_PID"
        unset CRIO_PID
    fi
}

# Stop crio.
function stop_crio() {
    stop_crio_no_clean ""
    cleanup_network_conf
}

function restart_crio() {
    if [ "$CRIO_PID" != "" ]; then
        kill "$CRIO_PID" >/dev/null 2>&1
        wait "$CRIO_PID"
        start_crio
    else
        echo "you must start crio first"
        exit 1
    fi
}

function cleanup_testdir() {
    # shellcheck disable=SC2013
    # Note: By using 'sort -r' we're ensuring longer paths go first, which
    # means that if there are nested mounts, the innermost mountpoints get
    # unmounted first
    for mnt in $(awk '{print $2}' /proc/self/mounts | grep ^"$TESTDIR" | sort -r); do
        umount "$mnt"
    done
    rm -rf "$TESTDIR" || true
    unset TESTDIR
}

function cleanup_test() {
    [ -z "$TESTDIR" ] && return
    # show crio log (only shown by bats in case of test failure)
    if [ -f "$CRIO_LOG" ]; then
        echo "# --- crio.log :: ---"
        cat "$CRIO_LOG"
        echo "# --- --- ---"
    fi
    if [[ $RUNTIME_TYPE == pod ]]; then
        echo "# --- conmonrs logs :: ---"
        CONMONRS_PID=$(sed -nr 's/.*Running conmonrs with PID: ([0-9]+).*/\1/p' "$CRIO_LOG")
        journalctl _COMM=conmonrs _PID="$CONMONRS_PID" --no-pager
        echo "# --- --- ---"
    fi

    # Leave the test artifacts intact for failing tests if requested.
    #
    # BATS_TEST_COMPLETED is set by BATS to 1 if the test passed, otherwise
    # it is left unset. The variable is also set if the test was skipped.
    # See https://bats-core.readthedocs.io/en/stable/faq.html#how-can-i-check-if-a-test-failed-succeeded-during-teardown for more details.
    if [ -z "$TEST_KEEP_ON_FAILURE" ] || [ "${BATS_TEST_COMPLETED:-}" = "1" ]; then
        cleanup_ctrs
        cleanup_pods
        stop_crio
        cleanup_testdir
        if [ "$RUNTIME_TYPE" == "vm" ]; then
            # cleanup left over kata processes
            # don't fail if there is none
            run killall containerd-shim-kata-v2
        fi
    else
        echo >&3 "* Failed \"$BATS_TEST_DESCRIPTION\", TESTDIR=$TESTDIR, LVM_DEVICE=${LVM_DEVICE:-}"
    fi
}

function load_apparmor_profile() {
    "$APPARMOR_PARSER_BINARY" -r "$1"
}

function remove_apparmor_profile() {
    "$APPARMOR_PARSER_BINARY" -R "$1"
}

function is_apparmor_enabled() {
    grep -q Y "$APPARMOR_PARAMETERS_FILE_PATH" 2>/dev/null
}

function is_selinux_enabled() {
    selinuxenabled 2>/dev/null || false
}

function is_selinux_enforcing() {
    command -v getenforce 1>/dev/null || false

    [[ $(getenforce) == "Enforcing" ]]
}

function prepare_network_conf() {
    mkdir -p "$CRIO_CNI_CONFIG"
    cat >"$CRIO_CNI_CONFIG/10-crio.conf" <<-EOF
{
    "cniVersion": "0.3.1",
    "name": "$CNI_DEFAULT_NETWORK",
    "type": "$CNI_TYPE",
    "bridge": "cni0",
    "isGateway": true,
    "ipMasq": true,
    "ipam": {
        "type": "host-local",
        "routes": [
            { "dst": "$POD_IPV4_DEF_ROUTE" },
            { "dst": "$POD_IPV6_DEF_ROUTE" }
        ],
        "ranges": [
            [{ "subnet": "$POD_IPV4_CIDR" }],
            [{ "subnet": "$POD_IPV6_CIDR" }]
        ]
    }
}
EOF
}

# Usage: ip=$(pod_ip -4|-6 "$ctr_id")
function pod_ip() {
    [ $# -eq 2 ]
    [ "$1" = "-4" ] || [ "$1" = "-6" ]
    crictl exec --sync "$2" ip "$1" addr show dev eth0 scope global |
        awk '/^ +inet/ {sub("/.*","",$2); print $2; exit}'
}

function get_host_ip() {
    gateway_dev=$(ip -o route show default $POD_IPV4_DEF_ROUTE | sed 's/.*dev \([^[:space:]]*\).*/\1/')
    [ "$gateway_dev" ]
    ip -o -4 addr show dev "$gateway_dev" scope global | sed 's/.*inet \([0-9.]*\).*/\1/' | head -1
}

function ping_pod() {
    local ip

    ip=$(pod_ip -4 "$1")
    ping -W 1 -c 2 "$ip"

    ip=$(pod_ip -6 "$1")
    ping6 -W 1 -c 2 "$ip"
}

function ping_pod_from_pod() {
    ip=$(pod_ip -4 "$1")
    crictl exec --sync "$2" ping -W 1 -c 2 "$ip"

    # since RHEL kernels don't mirror ipv4.ip_forward sysctl to ipv6, this fails
    # in such an environment without giving all containers NET_RAW capability
    # rather than reducing the security of the tests for all cases, skip this check
    # instead
    if is_rhel_7; then
        return
    fi

    ip=$(pod_ip -6 "$1")
    crictl exec --sync "$2" ping6 -W 1 -c 2 "$ip"
}

function is_rhel_7() {
    grep -i 'Red Hat\|CentOS' /etc/redhat-release | grep -q " 7"
}

function cleanup_network_conf() {
    rm -rf "$CRIO_CNI_CONFIG"
}

function reload_crio() {
    kill -HUP "$CRIO_PID"
}

function wait_for_log() {
    CNT=0
    while true; do
        if [[ $CNT -gt 50 ]]; then
            echo wait for log timed out
            exit 1
        fi

        if grep -iq "$1" "$CRIO_LOG"; then
            break
        fi

        echo "waiting for log entry to appear ($CNT): $1"
        sleep 0.1
        CNT=$((CNT + 1))
    done
}

function replace_config() {
    sed -i -e 's;\('"$1"' = "\).*\("\);\1'"$2"'\2;' "$CRIO_CONFIG"
    sed -i -e 's;\('"$1"' = "\).*\("\);\1'"$2"'\2;' "$CRIO_CUSTOM_CONFIG"
}

# tests whether the node is configured to use cgroupv2
function is_cgroup_v2() {
    test "$(stat -f -c%T /sys/fs/cgroup)" = "cgroup2fs"
}

function create_runtime_with_allowed_annotation() {
    local NAME="$1"
    local ANNOTATION="$2"
    unset CONTAINER_DEFAULT_RUNTIME
    unset CONTAINER_RUNTIMES
    cat <<EOF >"$CRIO_CONFIG_DIR/01-$NAME.conf"
[crio.runtime]
default_runtime = "$NAME"
[crio.runtime.runtimes.$NAME]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
allowed_annotations = ["$ANNOTATION"]
EOF
}

function create_workload_with_allowed_annotation() {
    local act="$2"
    # Fallback on the specified allowed annotation if
    # a specific activation annotation wasn't specified.
    if [[ -z "$act" ]]; then
        act="$1"
    fi
    cat <<EOF >"$CRIO_CONFIG_DIR/01-workload.conf"
[crio.runtime.workloads.management]
activation_annotation = "$act"
allowed_annotations = ["$1"]
EOF
}

function set_swap_fields_given_cgroup_version() {
    # set memory {,swap} max file for cgroupv1 or v2
    export CGROUP_MEM_SWAP_FILE="/sys/fs/cgroup/memory/memory.memsw.limit_in_bytes"
    export CGROUP_MEM_FILE="/sys/fs/cgroup/memory/memory.limit_in_bytes"
    if is_cgroup_v2; then
        export CGROUP_MEM_SWAP_FILE="/sys/fs/cgroup/memory.swap.max"
        export CGROUP_MEM_FILE="/sys/fs/cgroup/memory.max"
    fi
}

function set_container_pod_cgroup_root() {
    controller="$1"
    ctr_id="$2"
    CGROUP_ROOT="/sys/fs/cgroup"
    if is_cgroup_v2; then
        controller=""
    fi

    export POD_CGROUP="$CGROUP_ROOT"/"$controller"/pod_123.slice/pod_123-456.slice
    export CTR_CGROUP="$POD_CGROUP"/crio-"$ctr_id".scope
    if [ "$CONTAINER_CGROUP_MANAGER" != "systemd" ]; then
        export POD_CGROUP="$CGROUP_ROOT"/"$controller"/pod_123-456
        export CTR_CGROUP="$POD_CGROUP"/crio-"$ctr_id"
    fi
}

function check_conmon_cpuset() {
    local ctr_id="$1"
    local cpuset="$2"
    systemd_supports_cpuset=$(systemctl show --property=AllowedCPUs systemd || true)

    if [[ "$CONTAINER_CGROUP_MANAGER" == "cgroupfs" ]]; then
        if is_cgroup_v2; then
            cpuset_path="/sys/fs/cgroup"
            # see https://github.com/containers/crun/blob/e5874864918f8f07acdff083f83a7a59da8abb72/crun.1.md#cpu-controller for conversion
            cpushares=$((1 + ((cpushares - 2) * 9999) / 262142))
        else
            cpuset_path="/sys/fs/cgroup/cpuset"
        fi

        found_cpuset=$(cat "$cpuset_path/pod_123-456/crio-conmon-$ctr_id/cpuset.cpus")
        if [ -z "$cpuset" ]; then
            [[ $(cat "$cpuset_path/pod_123-456/cpuset.cpus") == *"$found_cpuset"* ]]
        else
            [[ "$cpuset" == *"$found_cpuset"* ]]
        fi
    else
        # don't test cpuset if it's not supported by systemd
        if [[ -n "$systemd_supports_cpuset" ]]; then
            info="$(systemctl show --property=AllowedCPUs crio-conmon-"$ctr_id".scope)"
            if [ -z "$cpuset" ]; then
                echo "$info" | grep -E '^AllowedCPUs=$'
            else
                [[ "$info" == *"AllowedCPUs=$cpuset"* ]]
            fi
        fi
    fi
}

function setup_kubensmnt() {
    if [[ -z $PIN_ROOT ]]; then
        PIN_ROOT=$TESTDIR/kubens
    fi
    PINNED_MNT_NS=$PIN_ROOT/mntns/mnt
    $PINNS_BINARY_PATH -d "$PIN_ROOT" -f mnt -m
    export KUBENSMNT=$PINNED_MNT_NS
}

function has_criu() {
    if [[ "$TEST_USERNS" == "1" ]]; then
        skip "Cannot run CRIU tests in user namespace."
    fi

    if [[ "$CONTAINER_DEFAULT_RUNTIME" != "runc" ]]; then
        skip "Checkpoint/Restore with pods only works in runc."
    fi

    if ! "$CHECKCRIU_BINARY"; then
        skip "CRIU check failed"
    fi
}

function has_buildah() {
    if [ ! -e "$(command -v buildah)" ]; then
        skip "buildah binary not found"
    fi
}

# Run buildah with the specified root directory (same as CRI-O)
function run_buildah() {
    buildah --log-level debug --root "$TESTDIR/crio" "$@"
}

function wait_until_exit() {
    ctr_id=$1
    # Wait for container to exit
    attempt=0
    while [ $attempt -le 100 ]; do
        attempt=$((attempt + 1))
        output=$(crictl inspect -o table "$ctr_id")
        if [[ "$output" == *"State: CONTAINER_EXITED"* ]]; then
            [[ "$output" == *"Exit Code: ${EXPECTED_EXIT_STATUS:-0}"* ]]
            return 0
        fi
        sleep 1
    done
    return 1
}

# Helpers for pod annotations tests
function prepare_cni_plugin() {
    # name the config with prefix 001 to ensure the corresponding cni plugin will be invoked when pod is created
    mkdir -p "$CRIO_CNI_CONFIG"
    cat >"$CRIO_CNI_CONFIG"/001-"$CNI_PLUGIN_NAME".conf <<-EOF
{
  "cniVersion": "0.3.1",
  "name": "$CNI_PLUGIN_NAME",
  "type": "$CNI_PLUGIN_NAME.sh",
  "config": {
    "log_path": "$1"
  },
  "capabilities": {
    "io.kubernetes.cri.pod-annotations": $2
  }
}
EOF
    chmod 755 "$TESTDATA"/"$CNI_PLUGIN_NAME".sh
    # copy the cni plugin into cni plugin binary directory
    cp "$TESTDATA"/"$CNI_PLUGIN_NAME".sh "$CRIO_CNI_PLUGIN"/"$CNI_PLUGIN_NAME".sh
}

function prepare_chained_cni_plugins() {
    # create a chained cni plugin configuration file
    mkdir -p "$CRIO_CNI_CONFIG"
    cat >"$CRIO_CNI_CONFIG"/001-"$CNI_PLUGIN_NAME".conflist <<-EOF
{
  "cniVersion": "0.3.1",
  "name": "$CNI_PLUGIN_NAME",
  "plugins": [
    {
      "type": "$CNI_PLUGIN_NAME.sh",
      "config": {
        "log_path": "$1"
      },
      "capabilities": {
        "io.kubernetes.cri.pod-annotations": $2
      }
    },
    {
      "type": "$CNI_PLUGIN_NAME.sh",
      "config": {
        "log_path": "$3"
      },
      "capabilities": {
        "io.kubernetes.cri.pod-annotations": $4
      }
    }
  ]
}
EOF
    chmod 777 "$TESTDATA"/"$CNI_PLUGIN_NAME".sh
    cp "$TESTDATA"/"$CNI_PLUGIN_NAME".sh "$CRIO_CNI_PLUGIN"/"$CNI_PLUGIN_NAME".sh
}

function contains() {
    # this function checks whether b contains a
    a=$1
    b=$2
    # if a and b are both null or empty, we consider them equal
    if { [[ $a == null ]] || [[ $a == '{}' ]]; } && { [[ $b == null ]] || [[ $b == '{}' ]]; }; then
        return 0
    fi
    if [[ $a == null ]] || [[ $a == '{}' ]] || [[ $b == null ]] || [[ $b == '{}' ]]; then
        return 1
    fi
    for key in $(echo "$a" | jq 'keys[]'); do
        value=$(jq -e ."$key" <<<"$b")
        # value is null means b does not have this key
        if [[ $value == null ]]; then
            return 1
        # if b has this key, checks their value
        elif [[ $value != $(jq -e ."$key" <<<"$a") ]]; then
            return 1
        fi
    done
    return 0
}

function annotations_equal() {
    cni_plugin_received=$1
    expected=$2
    contains "$cni_plugin_received" "$expected"
    expected_contains_received=$?
    contains "$expected" "$cni_plugin_received"
    received_contains_expected=$?
    [[ $expected_contains_received -eq 0 ]] && [[ $received_contains_expected -eq 0 ]]
}

function remove_random_storage_layer() {
    find "$TESTDIR"/crio/overlay -maxdepth 1 | grep '.*/[a-f0-9\-]\{64\}.*' | head -1 | xargs rm -Rf
}

function is_using_crun() {
    runtime --version | grep -q crun
}
