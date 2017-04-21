#!/bin/bash

# Root directory of integration tests.
INTEGRATION_ROOT=$(dirname "$(readlink -f "$BASH_SOURCE")")

# Test data path.
TESTDATA="${INTEGRATION_ROOT}/testdata"

# Root directory of the repository.
OCID_ROOT=${OCID_ROOT:-$(cd "$INTEGRATION_ROOT/../.."; pwd -P)}

# Path of the ocid binary.
OCID_BINARY=${OCID_BINARY:-${OCID_ROOT}/cri-o/ocid}
# Path of the ocic binary.
OCIC_BINARY=${OCIC_BINARY:-${OCID_ROOT}/cri-o/ocic}
# Path of the conmon binary.
CONMON_BINARY=${CONMON_BINARY:-${OCID_ROOT}/cri-o/conmon/conmon}
# Path of the pause binary.
PAUSE_BINARY=${PAUSE_BINARY:-${OCID_ROOT}/cri-o/pause/pause}
# Path of the default seccomp profile.
SECCOMP_PROFILE=${SECCOMP_PROFILE:-${OCID_ROOT}/cri-o/seccomp.json}
# Name of the default apparmor profile.
APPARMOR_PROFILE=${APPARMOR_PROFILE:-ocid-default}
# Runtime
RUNTIME=${RUNTIME:-runc}
RUNTIME_PATH=$(command -v $RUNTIME || true)
RUNTIME_BINARY=${RUNTIME_PATH:-/usr/local/sbin/runc}
# Path of the apparmor_parser binary.
APPARMOR_PARSER_BINARY=${APPARMOR_PARSER_BINARY:-/sbin/apparmor_parser}
# Path of the apparmor profile for test.
APPARMOR_TEST_PROFILE_PATH=${APPARMOR_TEST_PROFILE_PATH:-${TESTDATA}/apparmor_test_deny_write}
# Path of the apparmor profile for unloading ocid-default.
FAKE_OCID_DEFAULT_PROFILE_PATH=${FAKE_OCID_DEFAULT_PROFILE_PATH:-${TESTDATA}/fake_ocid_default}
# Name of the apparmor profile for test.
APPARMOR_TEST_PROFILE_NAME=${APPARMOR_TEST_PROFILE_NAME:-apparmor-test-deny-write}
# Path of boot config.
BOOT_CONFIG_FILE_PATH=${BOOT_CONFIG_FILE_PATH:-/boot/config-`uname -r`}
# Path of apparmor parameters file.
APPARMOR_PARAMETERS_FILE_PATH=${APPARMOR_PARAMETERS_FILE_PATH:-/sys/module/apparmor/parameters/enabled}
# Path of the bin2img binary.
BIN2IMG_BINARY=${BIN2IMG_BINARY:-${OCID_ROOT}/cri-o/test/bin2img/bin2img}
# Path of the copyimg binary.
COPYIMG_BINARY=${COPYIMG_BINARY:-${OCID_ROOT}/cri-o/test/copyimg/copyimg}
# Path of tests artifacts.
ARTIFACTS_PATH=${ARTIFACTS_PATH:-${OCID_ROOT}/cri-o/.artifacts}
# Path of the checkseccomp binary.
CHECKSECCOMP_BINARY=${CHECKSECCOMP_BINARY:-${OCID_ROOT}/cri-o/test/checkseccomp/checkseccomp}
# XXX: This is hardcoded inside cri-o at the moment.
DEFAULT_LOG_PATH=/var/log/ocid/pods

TESTDIR=$(mktemp -d)
if [ -e /usr/sbin/selinuxenabled ] && /usr/sbin/selinuxenabled; then
    . /etc/selinux/config
    filelabel=$(awk -F'"' '/^file.*=.*/ {print $2}' /etc/selinux/${SELINUXTYPE}/contexts/lxc_contexts)
    chcon -R ${filelabel} $TESTDIR
fi
OCID_SOCKET="$TESTDIR/ocid.sock"
OCID_CONFIG="$TESTDIR/ocid.conf"
OCID_CNI_CONFIG="$TESTDIR/cni/net.d/"
OCID_CNI_PLUGIN="/opt/cni/bin/"
POD_CIDR="10.88.0.0/16"
POD_CIDR_MASK="10.88.*.*"

cp "$CONMON_BINARY" "$TESTDIR/conmon"

PATH=$PATH:$TESTDIR

# Make sure we have a copy of the redis:latest image.
if ! [ -d "$ARTIFACTS_PATH"/redis-image ]; then
    mkdir -p "$ARTIFACTS_PATH"/redis-image
    if ! "$COPYIMG_BINARY" --import-from=docker://redis --export-to=dir:"$ARTIFACTS_PATH"/redis-image --signature-policy="$INTEGRATION_ROOT"/policy.json ; then
        echo "Error pulling docker://redis"
        rm -fr "$ARTIFACTS_PATH"/redis-image
        exit 1
    fi
fi

# Make sure we have a copy of the busybox:latest image.
if ! [ -d "$ARTIFACTS_PATH"/busybox-image ]; then
    mkdir -p "$ARTIFACTS_PATH"/busybox-image
    if ! "$COPYIMG_BINARY" --import-from=docker://busybox --export-to=dir:"$ARTIFACTS_PATH"/busybox-image --signature-policy="$INTEGRATION_ROOT"/policy.json ; then
        echo "Error pulling docker://busybox"
        rm -fr "$ARTIFACTS_PATH"/busybox-image
        exit 1
    fi
fi

# Run ocid using the binary specified by $OCID_BINARY.
# This must ONLY be run on engines created with `start_ocid`.
function ocid() {
	"$OCID_BINARY" --listen "$OCID_SOCKET" "$@"
}

# Run ocic using the binary specified by $OCIC_BINARY.
function ocic() {
	"$OCIC_BINARY" --connect "$OCID_SOCKET" "$@"
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

# Waits until the given ocid becomes reachable.
function wait_until_reachable() {
	retry 15 1 ocic runtimeversion
}

# Start ocid.
function start_ocid() {
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

	# Don't forget: bin2img, copyimg, and ocid have their own default drivers, so if you override any, you probably need to override them all
	if ! [ "$3" = "--no-pause-image" ] ; then
		"$BIN2IMG_BINARY" --root "$TESTDIR/ocid" $STORAGE_OPTS --runroot "$TESTDIR/ocid-run" --source-binary "$PAUSE_BINARY"
	fi
	"$COPYIMG_BINARY" --root "$TESTDIR/ocid" $STORAGE_OPTS --runroot "$TESTDIR/ocid-run" --image-name=redis --import-from=dir:"$ARTIFACTS_PATH"/redis-image --add-name=docker://docker.io/library/redis:latest --signature-policy="$INTEGRATION_ROOT"/policy.json
	"$OCID_BINARY" --conmon "$CONMON_BINARY" --listen "$OCID_SOCKET" --runtime "$RUNTIME_BINARY" --root "$TESTDIR/ocid" --runroot "$TESTDIR/ocid-run" $STORAGE_OPTS --seccomp-profile "$seccomp" --apparmor-profile "$apparmor" --cni-config-dir "$OCID_CNI_CONFIG" --signature-policy "$INTEGRATION_ROOT"/policy.json --config /dev/null config >$OCID_CONFIG

	# Prepare the CNI configuration files, we're running with non host networking by default
	prepare_network_conf $POD_CIDR

	"$OCID_BINARY" --debug --config "$OCID_CONFIG" & OCID_PID=$!
	wait_until_reachable

	run ocic image status --id=redis
	if [ "$status" -ne 0 ] ; then
		ocic image pull redis:latest
	fi
	REDIS_IMAGEID=$(ocic image status --id=redis | head -1 | sed -e "s/ID: //g")
	run ocic image status --id=busybox
	if [ "$status" -ne 0 ] ; then
		ocic image pull busybox:latest
	fi
	BUSYBOX_IMAGEID=$(ocic image status --id=busybox | head -1 | sed -e "s/ID: //g")
}

function cleanup_ctrs() {
	run ocic ctr list --quiet
	if [ "$status" -eq 0 ]; then
		if [ "$output" != "" ]; then
			printf '%s\n' "$output" | while IFS= read -r line
			do
			   ocic ctr stop --id "$line" || true
			   ocic ctr remove --id "$line"
			done
		fi
	fi
}

function cleanup_images() {
	run ocic image list --quiet
	if [ "$status" -eq 0 ]; then
		if [ "$output" != "" ]; then
			printf '%s\n' "$output" | while IFS= read -r line
			do
			   ocic image remove --id "$line"
			done
		fi
	fi
}

function cleanup_pods() {
	run ocic pod list --quiet
	if [ "$status" -eq 0 ]; then
		if [ "$output" != "" ]; then
			printf '%s\n' "$output" | while IFS= read -r line
			do
			   ocic pod stop --id "$line" || true
			   ocic pod remove --id "$line"
			done
		fi
	fi
}

# Stop ocid.
function stop_ocid() {
	if [ "$OCID_PID" != "" ]; then
		kill "$OCID_PID" >/dev/null 2>&1
		wait "$OCID_PID"
		rm -f "$OCID_CONFIG"
	fi

	cleanup_network_conf
}

function restart_ocid() {
	if [ "$OCID_PID" != "" ]; then
		kill "$OCID_PID" >/dev/null 2>&1
		wait "$OCID_PID"
		start_ocid
	else
		echo "you must start ocid first"
		exit 1
	fi
}

function cleanup_test() {
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
	mkdir -p $OCID_CNI_CONFIG
	cat >$OCID_CNI_CONFIG/10-ocid.conf <<-EOF
{
    "cniVersion": "0.2.0",
    "name": "ocidnet",
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

	cat >$OCID_CNI_CONFIG/99-loopback.conf <<-EOF
{
    "cniVersion": "0.2.0",
    "type": "loopback"
}
EOF

	echo 0
}

function check_pod_cidr() {
        fullnetns=`ocic pod status --id $1 | grep namespace | cut -d ' ' -f 3`
	netns=`basename $fullnetns`

	run ip netns exec $netns ip addr show dev eth0 scope global 2>&1
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
		fi
	done
}

function ping_pod() {
	netns=`ocic pod status --id $1 | grep namespace | cut -d ' ' -f 3`
	inet=`ip netns exec \`basename $netns\` ip addr show dev eth0 scope global | grep inet`

	IFS=" "
	ip=`parse_pod_ip $inet`

	ping -W 1 -c 5 $ip

	echo $?
}

function ping_pod_from_pod() {
	pod_ip=`ocic pod status --id $1 | grep "IP Address" | cut -d ' ' -f 3`
	netns=`ocic pod status --id $2 | grep namespace | cut -d ' ' -f 3`

	ip netns exec `basename $netns` ping -W 1 -c 2 $pod_ip

	echo $?
}


function cleanup_network_conf() {
	rm -rf $OCID_CNI_CONFIG

	echo 0
}

function temp_sandbox_conf() {
	sed -e s/\"namespace\":.*/\"namespace\":\ \"$1\",/g "$TESTDATA"/sandbox_config.json > $TESTDIR/sandbox_config_$1.json
}
