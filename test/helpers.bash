#!/bin/bash

# Root directory of integration tests.
INTEGRATION_ROOT=$(dirname "$(readlink -f "$BASH_SOURCE")")

# Test data path.
TESTDATA="${INTEGRATION_ROOT}/../testdata"

# Root directory of the repository.
OCID_ROOT=${OCID_ROOT:-$(cd "$INTEGRATION_ROOT/../.."; pwd -P)}

# Path of the ocid binary.
OCID_BINARY=${OCID_BINARY:-${OCID_ROOT}/ocid/ocid}
# Path of the ocic binary.
OCIC_BINARY=${OCIC_BINARY:-${OCID_ROOT}/ocid/ocic}
# Path of the conmon binary.
CONMON_BINARY=${CONMON_BINARY:-${OCID_ROOT}/ocid/conmon/conmon}

PATH=$PATH:$BATS_TMPDIR

# Run ocid using the binary specified by $OCID_BINARY.
# This must ONLY be run on engines created with `start_ocid`.
function ocid() {
	"$OCID_BINARY" "$@"
}

# Run ocic using the binary specified by $OCID_BINARY.
function ocic() {
	"$OCIC_BINARY" "$@"
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
	retry 15 1 "$OCIC_BINARY" runtimeversion
}

# Start ocid.
function start_ocid() {
	cp "$CONMON_BINARY" "$BATS_TMPDIR/conmon"
	mkdir -p "$BATS_TMPDIR/ocid/containers"
	mkdir -p "$BATS_TMPDIR/ocid/sandboxes"

	"$OCID_BINARY" --debug --runtime /usr/local/bin/runc  --containerdir "$BATS_TMPDIR/ocid/containers" --sandboxdir "$BATS_TMPDIR/ocid/sandboxes" & OCID_PID=$!
	wait_until_reachable
}

# Stop ocid.
function stop_ocid() {
	# TODO(runcom): why a greceful kill doesn't work?!
	kill -9 "$OCID_PID"
	# TODO(runcom): remove the whole /var/lib/ocid
	rm -rf "$BATS_TMPDIR/ocid/{containers,sandboxes}"
	# TODO(runcom): runc list and kill/delete everything!
}
