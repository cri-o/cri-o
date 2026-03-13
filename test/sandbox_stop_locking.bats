#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

# Helper function to create container config that ignores SIGTERM
# This simulates a container that won't terminate gracefully, forcing
# CRI-O to wait through the full timeout period before sending SIGKILL
function create_nonterminating_container_config() {
	local output_path="$1"
	# This forces CRI-O to wait through full timeout before SIGKILL
	jq '.command = ["sh", "-c", "trap '"'"''"'"' TERM; while true; do sleep 1; done"]' \
		"$TESTDATA"/container_sleep.json > "$output_path"
}

@test "locking CreateContainer blocks when StopPodSandbox holds exclusive lock" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	create_nonterminating_container_config "$TESTDIR/container_noterminate.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_noterminate.json" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Container stopping in background
	crictl stopp "$pod_id" &
	stop_pid=$!

	# Wait for stop operation to acquire the lock
	sleep 1

	# Attempt to create another container while stop holds lock
	start_create=$(date +%s)
	timeout 5s crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json &
	create_pid=$!

	# Monitor for 3 seconds to show blocking
	for i in {1..3}; do
		sleep 1
		if ps -p "$create_pid" > /dev/null 2>&1; then
			echo "CreateContainer is BLOCKED (waiting ${i}s so far)" >&3
		else
			break
		fi
	done

	# Verify it was NOT blocked for 2+ seconds
	end_create=$(date +%s)
	blocked_time=$((end_create - start_create))
	echo "CreateContainer took $blocked_time seconds" >&3

	# The test will fail if its blocked for 2 seconds or more
	[ "$blocked_time" -lt 2 ]

	if ps -p "$create_pid" > /dev/null 2>&1; then
		kill -9 "$create_pid" 2> /dev/null || true
	fi

	wait "$stop_pid" || true
}
