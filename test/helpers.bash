#!/bin/bash

# Root directory of integration tests.
INTEGRATION_ROOT=$(dirname "$(readlink -f "$BASH_SOURCE")")

# Test data path.
TESTDATA="${INTEGRATION_ROOT}/testdata"

# Root directory of the repository.
CRIO_ROOT=${CRIO_ROOT:-$(cd "$INTEGRATION_ROOT/../.."; pwd -P)}

# Path of the crio binary.
CRIO_BINARY=${CRIO_BINARY:-${CRIO_ROOT}/cri-o/bin/crio}
# Path of the crictl binary.
CRICTL_PATH=$(command -v crictl || true)
CRICTL_BINARY=${CRICTL_PATH:-/usr/bin/crictl}
# Path of the conmon binary.
CONMON_BINARY=${CONMON_BINARY:-${CRIO_ROOT}/cri-o/bin/conmon}
# Path of the pause binary.
PAUSE_BINARY=${PAUSE_BINARY:-${CRIO_ROOT}/cri-o/bin/pause}
# Path of the default seccomp profile.
SECCOMP_PROFILE=${SECCOMP_PROFILE:-${CRIO_ROOT}/cri-o/seccomp.json}
# Name of the default apparmor profile.
APPARMOR_PROFILE=${APPARMOR_PROFILE:-crio-default}
# Runtime
DEFAULT_RUNTIME=${DEFAULT_RUNTIME:-runc}
RUNTIME_NAME=${RUNTIME_NAME:-runc}
RUNTIME=${RUNTIME:-runc}
UID_MAPPINGS=${UID_MAPPINGS:-}
GID_MAPPINGS=${GID_MAPPINGS:-}
ULIMITS=${ULIMITS:-}
DEVICES=${DEVICES:-}
OVERRIDE_OPTIONS=${OVERRIDE_OPTIONS:-}
RUNTIME_PATH=$(command -v $RUNTIME || true)
RUNTIME_BINARY=${RUNTIME_PATH:-/usr/local/sbin/runc}
# Path of the apparmor_parser binary.
APPARMOR_PARSER_BINARY=${APPARMOR_PARSER_BINARY:-/sbin/apparmor_parser}
# Path of the apparmor profile for test.
APPARMOR_TEST_PROFILE_PATH=${APPARMOR_TEST_PROFILE_PATH:-${TESTDATA}/apparmor_test_deny_write}
# Path of the apparmor profile for unloading crio-default.
FAKE_CRIO_DEFAULT_PROFILE_PATH=${FAKE_CRIO_DEFAULT_PROFILE_PATH:-${TESTDATA}/fake_crio_default}
# Name of the apparmor profile for test.
APPARMOR_TEST_PROFILE_NAME=${APPARMOR_TEST_PROFILE_NAME:-apparmor-test-deny-write}
# Path of boot config.
BOOT_CONFIG_FILE_PATH=${BOOT_CONFIG_FILE_PATH:-/boot/config-`uname -r`}
# Path of apparmor parameters file.
APPARMOR_PARAMETERS_FILE_PATH=${APPARMOR_PARAMETERS_FILE_PATH:-/sys/module/apparmor/parameters/enabled}
# Path of the bin2img binary.
BIN2IMG_BINARY=${BIN2IMG_BINARY:-${CRIO_ROOT}/cri-o/test/bin2img/bin2img}
# Path of the copyimg binary.
COPYIMG_BINARY=${COPYIMG_BINARY:-${CRIO_ROOT}/cri-o/test/copyimg/copyimg}
# Path of tests artifacts.
ARTIFACTS_PATH=${ARTIFACTS_PATH:-${CRIO_ROOT}/cri-o/.artifacts}
# Path of the checkseccomp binary.
CHECKSECCOMP_BINARY=${CHECKSECCOMP_BINARY:-${CRIO_ROOT}/cri-o/test/checkseccomp/checkseccomp}
# XXX: This is hardcoded inside cri-o at the moment.
DEFAULT_LOG_PATH=/var/log/crio/pods
# Cgroup manager to be used
CGROUP_MANAGER=${CGROUP_MANAGER:-cgroupfs}
# Image volumes handling
IMAGE_VOLUMES=${IMAGE_VOLUMES:-mkdir}
# Container pids limit
PIDS_LIMIT=${PIDS_LIMIT:-1024}
# Log size max limit
LOG_SIZE_MAX_LIMIT=${LOG_SIZE_MAX_LIMIT:--1}
# Stream Port
STREAM_PORT=${STREAM_PORT:-10010}

TESTDIR=$(mktemp -d)
RANDOM_STRING=${TESTDIR: -10}

# podman pull needs a configuration file for shortname pulls
export REGISTRIES_CONFIG_PATH="$INTEGRATION_ROOT/registries.conf"

# Setup default hooks dir
HOOKSDIR=$TESTDIR/hooks
mkdir ${HOOKSDIR}
HOOKS_OPTS="--hooks-dir=$HOOKSDIR"

# Setup default mounts using deprecated --default-mounts flag
# should be removed, once the flag is removed
MOUNT_PATH="$TESTDIR/secrets"
mkdir ${MOUNT_PATH}
MOUNT_FILE="${MOUNT_PATH}/test.txt"
touch ${MOUNT_FILE}
echo "Testing secrets mounts!" > ${MOUNT_FILE}
DEFAULT_MOUNTS_OPTS="--default-mounts=${MOUNT_PATH}:/container/path1"

# Setup default secrets mounts
mkdir $TESTDIR/containers
touch $TESTDIR/containers/mounts.conf
echo "$TESTDIR/rhel/secrets:/run/secrets" > $TESTDIR/containers/mounts.conf
mkdir -p $TESTDIR/rhel/secrets
touch $TESTDIR/rhel/secrets/test.txt
echo "Testing secrets mounts. I am mounted!" > $TESTDIR/rhel/secrets/test.txt
mkdir -p $TESTDIR/symlink/target
touch $TESTDIR/symlink/target/key.pem
ln -s $TESTDIR/symlink/target $TESTDIR/rhel/secrets/mysymlink

# We may need to set some default storage options.
case "$(stat -f -c %T ${TESTDIR})" in
    aufs)
        # None of device mapper, overlay, or aufs can be used dependably over aufs, and of course btrfs and zfs can't,
        # and we have to explicitly specify the "vfs" driver in order to use it, so do that now.
        STORAGE_OPTIONS=${STORAGE_OPTIONS:---storage-driver vfs}
        ;;
esac

if [ -e /usr/sbin/selinuxenabled ] && /usr/sbin/selinuxenabled; then
    . /etc/selinux/config
    filelabel=$(awk -F'"' '/^file.*=.*/ {print $2}' /etc/selinux/${SELINUXTYPE}/contexts/lxc_contexts)
    chcon -R ${filelabel} $TESTDIR
fi
CRIO_SOCKET="$TESTDIR/crio.sock"
CRIO_CONFIG="$TESTDIR/crio.conf"
CRIO_CNI_CONFIG="$TESTDIR/cni/net.d/"
CRIO_LOG="$TESTDIR/crio.log"

# Copy all the CNI dependencies around to ensure encapsulated tests
CRIO_CNI_PLUGIN="$TESTDIR/cni-bin"
mkdir "$CRIO_CNI_PLUGIN"
cp /opt/cni/bin/* "$CRIO_CNI_PLUGIN"
cp "$INTEGRATION_ROOT"/cni_plugin_helper.bash "$CRIO_CNI_PLUGIN"
sed -i "s;%TEST_DIR%;$TESTDIR;" "$CRIO_CNI_PLUGIN"/cni_plugin_helper.bash

POD_CIDR="10.88.0.0/16"
POD_CIDR_MASK="10.88.*.*"

cp "$CONMON_BINARY" "$TESTDIR/conmon"

PATH=$PATH:$TESTDIR

DEFAULT_CAPABILITIES="CHOWN,DAC_OVERRIDE,FSETID,FOWNER,NET_RAW,SETGID,SETUID,SETPCAP,NET_BIND_SERVICE,SYS_CHROOT,KILL"

# Make sure we have a copy of the redis:alpine image.
if ! [ -d "$ARTIFACTS_PATH"/redis-image ]; then
    mkdir -p "$ARTIFACTS_PATH"/redis-image
    if ! "$COPYIMG_BINARY" --import-from=docker://quay.io/crio/redis:alpine --export-to=dir:"$ARTIFACTS_PATH"/redis-image --signature-policy="$INTEGRATION_ROOT"/policy.json ; then
        echo "Error pulling quay.io/crio/redis"
        rm -fr "$ARTIFACTS_PATH"/redis-image
        exit 1
    fi
fi

# Make sure we have a copy of the runcom/stderr-test image.
if ! [ -d "$ARTIFACTS_PATH"/stderr-test ]; then
    mkdir -p "$ARTIFACTS_PATH"/stderr-test
    if ! "$COPYIMG_BINARY" --import-from=docker://quay.io/crio/stderr-test:latest --export-to=dir:"$ARTIFACTS_PATH"/stderr-test --signature-policy="$INTEGRATION_ROOT"/policy.json ; then
        echo "Error pulling quay.io/crio/stderr-test"
        rm -fr "$ARTIFACTS_PATH"/stderr-test
        exit 1
    fi
fi

# Make sure we have a copy of the busybox:latest image.
if ! [ -d "$ARTIFACTS_PATH"/busybox-image ]; then
    mkdir -p "$ARTIFACTS_PATH"/busybox-image
    if ! "$COPYIMG_BINARY" --import-from=docker://quay.io/crio/busybox:latest --export-to=dir:"$ARTIFACTS_PATH"/busybox-image --signature-policy="$INTEGRATION_ROOT"/policy.json ; then
        echo "Error pulling quay.io/crio/busybox"
        rm -fr "$ARTIFACTS_PATH"/busybox-image
        exit 1
    fi
fi

# Make sure we have a copy of the mrunalp/oom:latest image.
if ! [ -d "$ARTIFACTS_PATH"/oom-image ]; then
    mkdir -p "$ARTIFACTS_PATH"/oom-image
    if ! "$COPYIMG_BINARY" --import-from=docker://quay.io/crio/oom:latest --export-to=dir:"$ARTIFACTS_PATH"/oom-image --signature-policy="$INTEGRATION_ROOT"/policy.json ; then
        echo "Error pulling quay.io/crio/oom"
        rm -fr "$ARTIFACTS_PATH"/oom-image
        exit 1
    fi
fi

# Make sure we have a copy of the mrunalp/image-volume-test:latest image.
if ! [ -d "$ARTIFACTS_PATH"/image-volume-test-image ]; then
    mkdir -p "$ARTIFACTS_PATH"/image-volume-test-image
    if ! "$COPYIMG_BINARY" --import-from=docker://quay.io/crio/image-volume-test:latest --export-to=dir:"$ARTIFACTS_PATH"/image-volume-test-image --signature-policy="$INTEGRATION_ROOT"/policy.json ; then
        echo "Error pulling quay.io/crio/image-volume-test-image"
        rm -fr "$ARTIFACTS_PATH"/image-volume-test-image
        exit 1
    fi
fi
# Run crio using the binary specified by $CRIO_BINARY.
# This must ONLY be run on engines created with `start_crio`.
function crio() {
	"$CRIO_BINARY" --listen "$CRIO_SOCKET" "$@"
}

# Run crictl using the binary specified by $CRICTL_BINARY.
function crictl() {
	"$CRICTL_BINARY" -r "unix://$CRIO_SOCKET" -i "unix://$CRIO_SOCKET" "$@"
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

	for ((i=0; i < attempts; i++)); do
		run "$@"
		if [[ "$status" -eq 0 ]] ; then
			return 0
		fi
		sleep $delay
	done

	echo "Command \"$@\" failed $attempts times. Output: $output"
	false
}

# Waits until crio becomes reachable.
function wait_until_reachable() {
	retry 15 1 crictl info
}

# Start crio.
function start_crio() {
	if [[ -n "$1" ]]; then
		seccomp="$1"
	else
		seccomp="$SECCOMP_PROFILE"
	fi

	if [[ -n "$2" ]]; then
		apparmor="$2"
	else
		apparmor="$APPARMOR_PROFILE"
	fi

	# Don't forget: bin2img, copyimg, and crio have their own default drivers, so if you override any, you probably need to override them all
	if ! [ "$3" = "--no-pause-image" ] ; then
		"$BIN2IMG_BINARY" --root "$TESTDIR/crio" $STORAGE_OPTIONS --runroot "$TESTDIR/crio-run" --source-binary "$PAUSE_BINARY"
	fi

	if [[ -n "$4" ]]; then
		capabilities="$4"
	else
		capabilities="$DEFAULT_CAPABILITIES"
	fi

	"$COPYIMG_BINARY" --root "$TESTDIR/crio" $STORAGE_OPTIONS --runroot "$TESTDIR/crio-run" --image-name=quay.io/crio/redis:alpine --import-from=dir:"$ARTIFACTS_PATH"/redis-image --signature-policy="$INTEGRATION_ROOT"/policy.json
	"$COPYIMG_BINARY" --root "$TESTDIR/crio" $STORAGE_OPTIONS --runroot "$TESTDIR/crio-run" --image-name=quay.io/crio/oom:latest --import-from=dir:"$ARTIFACTS_PATH"/oom-image --signature-policy="$INTEGRATION_ROOT"/policy.json
	"$COPYIMG_BINARY" --root "$TESTDIR/crio" $STORAGE_OPTIONS --runroot "$TESTDIR/crio-run" --image-name=quay.io/crio/image-volume-test:latest --import-from=dir:"$ARTIFACTS_PATH"/image-volume-test-image --signature-policy="$INTEGRATION_ROOT"/policy.json
	"$COPYIMG_BINARY" --root "$TESTDIR/crio" $STORAGE_OPTIONS --runroot "$TESTDIR/crio-run" --image-name=quay.io/crio/busybox:latest --import-from=dir:"$ARTIFACTS_PATH"/busybox-image --signature-policy="$INTEGRATION_ROOT"/policy.json
	"$COPYIMG_BINARY" --root "$TESTDIR/crio" $STORAGE_OPTIONS --runroot "$TESTDIR/crio-run" --image-name=quay.io/crio/stderr-test:latest --import-from=dir:"$ARTIFACTS_PATH"/stderr-test --signature-policy="$INTEGRATION_ROOT"/policy.json
	"$CRIO_BINARY" ${DEFAULT_MOUNTS_OPTS} ${HOOKS_OPTS} --default-capabilities "$capabilities" --conmon "$CONMON_BINARY" --listen "$CRIO_SOCKET" --stream-port "$STREAM_PORT" --cgroup-manager "$CGROUP_MANAGER" --default-mounts-file "$TESTDIR/containers/mounts.conf" --registry "quay.io" --default-runtime $DEFAULT_RUNTIME --runtimes "$RUNTIME_NAME:$RUNTIME_BINARY" --root "$TESTDIR/crio" --runroot "$TESTDIR/crio-run" $STORAGE_OPTIONS --seccomp-profile "$seccomp" --apparmor-profile "$apparmor" --cni-config-dir "$CRIO_CNI_CONFIG" --cni-plugin-dir "$CRIO_CNI_PLUGIN" --signature-policy "$INTEGRATION_ROOT"/policy.json --image-volumes "$IMAGE_VOLUMES" --pids-limit "$PIDS_LIMIT" --log-size-max "$LOG_SIZE_MAX_LIMIT" $DEVICES $ULIMITS --uid-mappings "$UID_MAPPINGS" --gid-mappings "$GID_MAPPINGS" --default-sysctls "$TEST_SYSCTL" $OVERRIDE_OPTIONS --config /dev/null config >$CRIO_CONFIG
	sed -r -e 's/^(#)?root =/root =/g' -e 's/^(#)?runroot =/runroot =/g' -e 's/^(#)?storage_driver =/storage_driver =/g' -e '/^(#)?storage_option = (\[)?[ \t]*$/,/^#?$/s/^(#)?//g' -e '/^(#)?registries = (\[)?[ \t]*$/,/^#?$/s/^(#)?//g' -e '/^(#)?default_ulimits = (\[)?[ \t]*$/,/^#?$/s/^(#)?//g' -i $CRIO_CONFIG
	# make sure we don't run with nodev, or else mounting a readonly rootfs will fail: https://github.com/cri-o/cri-o/issues/1929#issuecomment-474240498
	sed -r -e 's/nodev(,)?//g' -i $CRIO_CONFIG
	# Prepare the CNI configuration files, we're running with non host networking by default
	if [[ -n "$5" ]]; then
		netfunc="$5"
	else
		netfunc="prepare_network_conf"
	fi
	${netfunc} $POD_CIDR

	"$CRIO_BINARY" --default-mounts-file "$TESTDIR/containers/mounts.conf" --log-level debug --config "$CRIO_CONFIG" & CRIO_PID=$!
	wait_until_reachable

	run crictl inspecti quay.io/crio/redis:alpine
	if [ "$status" -ne 0 ] ; then
		crictl pull quay.io/crio/redis:alpine
	fi
	REDIS_IMAGEID=$(crictl inspecti --output=table quay.io/crio/redis:alpine | grep ^ID: | head -n 1 | sed -e "s/ID: //g")
	REDIS_IMAGEREF=$(crictl inspecti --output=table quay.io/crio/redis:alpine | grep ^Digest: | head -n 1 | sed -e "s/Digest: //g")
	run crictl inspecti quay.io/crio/oom
	if [ "$status" -ne 0 ] ; then
		  crictl pull quay.io/crio/oom
	fi
	OOM_IMAGEID=$(crictl inspecti quay.io/crio/oom | grep ^ID: | head -n 1 | sed -e "s/ID: //g")
	run crictl inspecti quay.io/crio/stderr-test
	if [ "$status" -ne 0 ] ; then
		crictl pull quay.io/crio/stderr-test:latest
	fi
	STDERR_IMAGEID=$(crictl inspecti quay.io/crio/stderr-test | grep ^ID: | head -n 1 | sed -e "s/ID: //g")
	run crictl inspecti quay.io/crio/busybox
	if [ "$status" -ne 0 ] ; then
		crictl pull quay.io/crio/busybox:latest
	fi
	BUSYBOX_IMAGEID=$(crictl inspecti quay.io/crio/busybox | grep ^ID: | head -n 1 | sed -e "s/ID: //g")
	run crictl inspecti quay.io/crio/image-volume-test
	if [ "$status" -ne 0 ] ; then
		  crictl pull quay.io/crio/image-volume-test:latest
	fi
	VOLUME_IMAGEID=$(crictl inspecti quay.io/crio/image-volume-test | grep ^ID: | head -n 1 | sed -e "s/ID: //g")
}

function cleanup_ctrs() {
	output=$(crictl ps --quiet)
	if [ $? -eq 0 ]; then
		if [ "$output" != "" ]; then
			printf '%s\n' "$output" | while IFS= read -r line
			do
			   crictl stop "$line"
			   crictl rm "$line"
			done
		fi
	fi
	rm -f /run/hookscheck
}

function cleanup_images() {
	output=$(crictl images --quiet)
	if [ $? -eq 0 ]; then
		if [ "$output" != "" ]; then
			printf '%s\n' "$output" | while IFS= read -r line
			do
			   crictl rmi "$line"
			done
		fi
	fi
}

function cleanup_pods() {
	output=$(crictl pods --quiet)
	if [ $? -eq 0 ]; then
		if [ "$output" != "" ]; then
			printf '%s\n' "$output" | while IFS= read -r line
			do
			   crictl stopp "$line"
			   crictl rmp "$line"
			done
		fi
	fi
}

# Stop crio.
function stop_crio() {
	if [ "$CRIO_PID" != "" ]; then
		kill "$CRIO_PID" >/dev/null 2>&1
		wait "$CRIO_PID"
		rm -f "$CRIO_CONFIG"
		CRIO_PID=
	fi

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

function cleanup_lvm() {
	if [ "$LVM_DEVICE" != "" ]; then
		lvm lvremove -y storage/thinpool
		lvm vgremove -y storage
		lvm pvremove -y $LVM_DEVICE
	fi
}

function cleanup_test() {
	cleanup_ctrs
	cleanup_pods
	stop_crio
	cleanup_lvm
	rm -rf "$TESTDIR"
}


function load_apparmor_profile() {
	"$APPARMOR_PARSER_BINARY" -r "$1"
}

function remove_apparmor_profile() {
	"$APPARMOR_PARSER_BINARY" -R "$1"
}

function is_seccomp_enabled() {
	if ! "$CHECKSECCOMP_BINARY" ; then
		echo 0
		return
	fi
	echo 1
}

function is_apparmor_enabled() {
	if [[ -f "$APPARMOR_PARAMETERS_FILE_PATH" ]]; then
		out=$(cat "$APPARMOR_PARAMETERS_FILE_PATH")
		if [[ "$out" =~ "Y" ]]; then
			echo 1
			return
		fi
	fi
	echo 0
}

function prepare_network_conf() {
	mkdir -p $CRIO_CNI_CONFIG
	cat >$CRIO_CNI_CONFIG/10-crio.conf <<-EOF
{
    "cniVersion": "0.2.0",
    "name": "crionet",
    "type": "bridge",
    "bridge": "cni0",
    "isGateway": true,
    "ipMasq": true,
    "ipam": {
        "type": "host-local",
        "subnet": "$1",
        "routes": [
            { "dst": "0.0.0.0/0"  }
        ]
    }
}
EOF

	echo 0
}

function write_plugin_test_args_network_conf() {
	mkdir -p $CRIO_CNI_CONFIG
	cat >$CRIO_CNI_CONFIG/10-plugin-test-args.conf <<-EOF
{
    "cniVersion": "0.2.0",
    "name": "crionet_test_args_$RANDOM_STRING",
    "type": "cni_plugin_helper.bash",
    "bridge": "cni0",
    "isGateway": true,
    "ipMasq": true,
    "ipam": {
        "type": "host-local",
        "subnet": "$1",
        "routes": [
            { "dst": "0.0.0.0/0"  }
        ]
    }
}
EOF

	if [[ -n "$2" ]]; then
		echo "DEBUG_ARGS=$2" > "$TESTDIR"/cni_plugin_helper_input.env
	fi

	echo 0
}

function prepare_plugin_test_args_network_conf() {
	write_plugin_test_args_network_conf $1 ""
}

function prepare_plugin_test_args_network_conf_malformed_result() {
	write_plugin_test_args_network_conf $1 "malformed-result"
}

function check_pod_cidr() {
	run crictl exec --sync $1 ip addr show dev eth0 scope global 2>&1
	echo "$output"
	[ "$status" -eq 0  ]
	[[ "$output" =~ $POD_CIDR_MASK  ]]
}

function parse_pod_ip() {
	for arg
	do
		cidr=`echo "$arg" | grep $POD_CIDR_MASK`
		if [ "$cidr" == "$arg" ]
		then
			echo `echo "$arg" | sed "s/\/[0-9][0-9]//"`
			break
		fi
	done
}

function get_host_ip() {
	gateway_dev=`ip -o route show default 0.0.0.0/0 | sed 's/.*dev \([^[:space:]]*\).*/\1/'`
	[ "$gateway_dev" ]
	host_ip=`ip -o -4 addr show dev $gateway_dev scope global | sed 's/.*inet \([0-9.]*\).*/\1/'`
}

function ping_pod() {
	inet=`crictl exec --sync $1 ip addr show dev eth0 scope global 2>&1 | grep inet`

	IFS=" "
	ip=`parse_pod_ip $inet`

	ping -W 1 -c 5 $ip

	echo $?
}

function ping_pod_from_pod() {
	inet=`crictl exec --sync $1 ip addr show dev eth0 scope global 2>&1 | grep inet`

	IFS=" "
	ip=`parse_pod_ip $inet`

	run crictl exec --sync $2 ping -W 1 -c 2 $ip
	echo "$output"
	[ "$status" -eq 0   ]
}


function cleanup_network_conf() {
	rm -rf $CRIO_CNI_CONFIG

	echo 0
}

function temp_sandbox_conf() {
	sed -e s/\"namespace\":.*/\"namespace\":\ \"$1\",/g "$TESTDATA"/sandbox_config.json > $TESTDIR/sandbox_config_$1.json
}

function wait_until_exit() {
	ctr_id=$1
	# Wait for container to exit
	attempt=0
	while [ $attempt -le 100 ]; do
		attempt=$((attempt+1))
		run crictl inspect "$ctr_id" --output table
		echo "$output"
		[ "$status" -eq 0 ]
		if [[ "$output" =~ "State: CONTAINER_EXITED" ]]; then
			[[ "$output" =~ "Exit Code: ${EXPECTED_EXIT_STATUS:-0}" ]]
			return 0
		fi
		sleep 1
	done
	return 1
}
