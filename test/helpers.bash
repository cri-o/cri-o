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
# Path of the runc binary.
RUNC_PATH=$(command -v runc || true)
RUNC_BINARY=${RUNC_PATH:-/usr/local/sbin/runc}
# Path of the apparmor_parser binary.
APPARMOR_PARSER_BINARY=${APPARMOR_PARSER_BINARY:-/sbin/apparmor_parser}
# Path of the apparmor profile for test.
APPARMOR_TEST_PROFILE_PATH=${APPARMOR_TEST_PROFILE_PATH:-${TESTDATA}/apparmor_test_deny_write}
# Name of the apparmor profile for test.
APPARMOR_TEST_PROFILE_NAME=${APPARMOR_TEST_PROFILE_NAME:-apparmor-test-deny-write}
# Path of boot config.
BOOT_CONFIG_FILE_PATH=${BOOT_CONFIG_FILE_PATH:-/boot/config-`uname -r`}
# Path of apparmor parameters file.
APPARMOR_PARAMETERS_FILE_PATH=${APPARMOR_PARAMETERS_FILE_PATH:-/sys/module/apparmor/parameters/enabled}

TESTDIR=$(mktemp -d)
if [ -e /usr/sbin/selinuxenabled ] && /usr/sbin/selinuxenabled; then
    . /etc/selinux/config
    filelabel=$(awk -F'"' '/^file.*=.*/ {print $2}' /etc/selinux/${SELINUXTYPE}/contexts/lxc_contexts)
    chcon -R ${filelabel} $TESTDIR
fi
OCID_SOCKET="$TESTDIR/ocid.sock"
OCID_CONFIG="$TESTDIR/ocid.conf"

cp "$CONMON_BINARY" "$TESTDIR/conmon"

PATH=$PATH:$TESTDIR

# Run ocid using the binary specified by $OCID_BINARY.
# This must ONLY be run on engines created with `start_ocid`.
function ocid() {
	"$OCID_BINARY" "$@"
}

# Run ocic using the binary specified by $OCID_BINARY.
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

	"$OCID_BINARY" --conmon "$CONMON_BINARY" --pause "$PAUSE_BINARY" --listen "$OCID_SOCKET" --runtime "$RUNC_BINARY" --root "$TESTDIR/ocid" --sandboxdir "$TESTDIR/sandboxes" --containerdir "$TESTDIR/ocid/containers" --seccomp-profile "$seccomp" --apparmor-profile "$apparmor" config >$OCID_CONFIG
	"$OCID_BINARY" --debug --config "$OCID_CONFIG" & OCID_PID=$!
	wait_until_reachable
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
		rm -f "$OCID_CONFIG"
	fi
}

function cleanup_test() {
	rm -rf "$TESTDIR"
}


function load_apparmor_test_profile() {
	"$APPARMOR_PARSER_BINARY" -r "$APPARMOR_TEST_PROFILE_PATH"
}

function remove_apparmor_test_profile() {
	"$APPARMOR_PARSER_BINARY" -R "$APPARMOR_TEST_PROFILE_PATH"
}

function is_seccomp_enabled() {
	if [[ -f "$BOOT_CONFIG_FILE_PATH" ]]; then
		out=$(cat "$BOOT_CONFIG_FILE_PATH" | grep CONFIG_SECCOMP=)
		if [[ "$out" =~ "CONFIG_SECCOMP=y" ]]; then
			echo 1
		else
			echo 0
		fi
	else
		echo 0
	fi
}

function is_apparmor_enabled() {
	if [[ -f "$APPARMOR_PARAMETERS_FILE_PATH" ]]; then
		out=$(cat "$APPARMOR_PARAMETERS_FILE_PATH")
		if [[ "$out" =~ "Y" ]]; then
			echo 1
		else
			echo 0
		fi
	else
		echo 0
	fi
}
