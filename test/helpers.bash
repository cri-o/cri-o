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
# Path of the runc binary.
RUNC_PATH=$(command -v runc || true)
RUNC_BINARY=${RUNC_PATH:-/usr/local/sbin/runc}

TESTDIR=$(mktemp -d)
OCID_SOCKET="$TESTDIR/ocid.sock"

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
	"$OCID_BINARY" --conmon "$CONMON_BINARY" --pause "$PAUSE_BINARY" --debug --listen "$TESTDIR/ocid.sock" --runtime "$RUNC_BINARY" --root "$TESTDIR/ocid" --sandboxdir "$TESTDIR/sandboxes" --containerdir "$TESTDIR/ocid/containers" & OCID_PID=$!
	wait_until_reachable
}

function cleanup_ctrs() {
	run ocic ctr list --quiet
	if [ "$status" -eq 0 ]; then
		if [ "$output" != "" ]; then
			printf '%s\n' "$output" | while IFS= read -r line
			do
			   ocic ctr stop --id "$line"
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
			   ocic pod stop --id "$line"
			   ocic pod remove --id "$line"
			done
		fi
	fi
}

# Stop ocid.
function stop_ocid() {
	if [ "$OCID_PID" != "" ]; then
		kill "$OCID_PID" >/dev/null 2>&1
	fi
}

function cleanup_test() {
	rm -rf "$TESTDIR"
}
