#!/usr/bin/env bats
# vim: set syntax=sh:

# this is a canary test for the CI job for kata: if this one fails, all the others
# are dubious, as it means they probably run without kata

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "container run with kata should have containerd-shim-kata-v2 process running" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip Not running with kata
	fi
	start_crio

	# make sure no kata process is running before we start a container
	output="$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)"
	echo "$output"
	[[ "$output" == "0" ]]

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# verify that the shim is running
	[ "$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)" == "1" ]

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# verify that the shim goes away with the pod
	[ "$(ps --no-headers -C containerd-shim-kata-v2 | wc -l)" == "0" ]
}

@test "crio restore with vm runtime and dead shim" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_RUNNING"* ]]

	stop_crio_no_clean ""

	# simulate dead shim by removing address files from bundle directories
	find "$TESTDIR"/ -name address -exec rm \{\} \;

	start_crio_no_setup

	# container should be marked exited after restore
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]
	[[ "${output}" == *"Exit Code: 255"* ]]

	# stop and remove should succeed without panic
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore with vm runtime and stale shim address" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_RUNNING"* ]]

	stop_crio_no_clean ""

	# simulate dead shim with stale address file: replace address contents
	# with a socket path that will get connection refused
	find "$TESTDIR"/ -name address -exec sh -c 'echo "unix:///nonexistent/dead-shim.sock" > "$1"' _ {} \;

	start_crio_no_setup

	# container should be marked exited after restore
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]
	[[ "${output}" == *"Exit Code: 255"* ]]

	# stop and remove should succeed without panic
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio restore with vm runtime and dead shim already stopped" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl stop "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]

	stop_crio_no_clean ""

	# simulate dead shim by removing address files
	find "$TESTDIR"/ -name address -exec rm \{\} \;

	start_crio_no_setup

	# already stopped container should remain exited
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]

	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio stop vm runtime container with signal trapping" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Container traps TERM and kills its process group. In production
	# (through kubelet), this triggers a race where the kata shim exits
	# before the Kill RPC response reaches CRI-O, causing ttrpc.ErrClosed.
	# This test does not reliably reproduce the race (crictl talks directly
	# to CRI-O without kubelet's concurrency), but validates the stop/remove
	# lifecycle works for signal-trapping kata containers.
	jq '.command = ["/bin/sh", "-c", "trap '\''kill 0'\'' TERM; sleep 6000"]
		| .args = []' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container_trap.json"

	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_trap.json" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_RUNNING"* ]]

	# Without kill() ttrpc.ErrClosed fix: fails with ttrpc: closed
	crictl stop "$ctr_id"

	# Without StopContainer status fix: container stays RUNNING (CRI violation)
	output=$(crictl inspect -o table "$ctr_id")
	[[ "${output}" == *"CONTAINER_EXITED"* ]]

	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "crio vm pod with signal trapping container deletes gracefully" {
	if [[ $RUNTIME_TYPE != vm ]]; then
		skip "only applicable to vm runtime type"
	fi

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# See note in "crio stop vm runtime container with signal trapping"
	# about the ttrpc.ErrClosed race. This test validates the full pod
	# teardown lifecycle completes for signal-trapping kata containers.
	jq '.command = ["/bin/sh", "-c", "trap '\''kill 0'\'' TERM; sleep 6000"]
		| .args = []' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container_trap.json"

	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_trap.json" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Full pod teardown: stop container, remove container, stop pod, remove pod
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify pod is gone
	output=$(crictl pods --quiet)
	[[ -z "${output}" ]]
}
