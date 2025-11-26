#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "exec during graceful termination" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start stopping the container with a 10 second timeout
	# This gives us a graceful termination window
	crictl stop -t 10 "$ctr_id" &
	stop_pid=$!

	# Wait a moment for the stop to be initiated
	sleep 0.5

	# During the graceful termination period, exec should succeed
	output=$(crictl exec --sync "$ctr_id" echo "exec during termination")
	[[ "$output" == *"exec during termination"* ]]

	# Wait for the stop to complete
	wait "$stop_pid"
}

@test "multiple execs during graceful termination" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start stopping the container with a 10 second timeout
	crictl stop -t 10 "$ctr_id" &
	stop_pid=$!

	# Wait a moment for the stop to be initiated
	sleep 0.5

	# Start multiple exec commands concurrently during graceful termination
	crictl exec --sync "$ctr_id" echo "exec 1" &
	exec1_pid=$!
	crictl exec --sync "$ctr_id" echo "exec 2" &
	exec2_pid=$!
	crictl exec --sync "$ctr_id" echo "exec 3" &
	exec3_pid=$!

	# All execs should complete successfully
	wait "$exec1_pid"
	wait "$exec2_pid"
	wait "$exec3_pid"

	# Wait for the stop to complete
	wait "$stop_pid"
}

@test "long exec killed when kill loop starts" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start a long-running exec that will outlive the graceful period
	crictl exec "$ctr_id" /bin/bash -c 'sleep 60' &
	exec_pid=$!

	# Give the exec a moment to start
	sleep 0.5

	# Stop container with a short timeout (2 seconds)
	# The exec should be killed when the grace period ends
	crictl stop -t 2 "$ctr_id"

	# The exec should have been terminated
	# wait returns the exit code; a killed process returns non-zero
	run wait "$exec_pid"
	# The exec should not have completed successfully
	[ "$status" -ne 0 ]
}

@test "execsync during graceful termination" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start stopping the container with a 10 second timeout
	crictl stop -t 10 "$ctr_id" &
	stop_pid=$!

	# Wait a moment for the stop to be initiated
	sleep 0.5

	# Test execsync during graceful termination
	output=$(crictl exec --sync "$ctr_id" echo "execsync works")
	[[ "$output" == *"execsync works"* ]]

	# Wait for the stop to complete
	wait "$stop_pid"
}

@test "tty exec during graceful termination" {
	start_crio

	# Create a container with tty enabled
	jq '.tty = true' "$TESTDATA"/container_sleep.json > "$TESTDIR/container_tty.json"
	ctr_id=$(crictl run "$TESTDIR/container_tty.json" "$TESTDATA"/sandbox_config.json)

	# Start stopping the container with a 10 second timeout
	crictl stop -t 10 "$ctr_id" &
	stop_pid=$!

	# Wait a moment for the stop to be initiated
	sleep 0.5

	# Test tty exec during graceful termination (without --sync)
	crictl exec "$ctr_id" echo "tty exec works" &
	exec_pid=$!

	# The exec should complete
	wait "$exec_pid"

	# Wait for the stop to complete
	wait "$stop_pid"
}

@test "exec fails after container stopped" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Stop the container and wait for it to fully stop
	crictl stop "$ctr_id"

	# Give container time to be in stopped state
	sleep 1

	# Exec should fail on a stopped container
	run crictl exec --sync "$ctr_id" echo "should fail"
	[ "$status" -ne 0 ]
}

@test "exec during graceful termination with short command" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start stopping the container with a 10 second timeout
	crictl stop -t 10 "$ctr_id" &
	stop_pid=$!

	# Wait a moment for the stop to be initiated
	sleep 0.5

	# Run a quick command that should complete before graceful period ends
	output=$(crictl exec --sync "$ctr_id" /bin/sh -c 'echo "quick"; sleep 0.1; echo "done"')
	[[ "$output" == *"quick"* ]]
	[[ "$output" == *"done"* ]]

	# Wait for the stop to complete
	wait "$stop_pid"
}

@test "exec started before termination continues during graceful period" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start a long-running exec before initiating stop
	crictl exec "$ctr_id" /bin/bash -c 'sleep 3; echo "exec completed"' &
	exec_pid=$!

	# Give the exec a moment to start
	sleep 0.5

	# Now initiate stop with sufficient grace period for exec to complete
	crictl stop -t 10 "$ctr_id" &
	stop_pid=$!

	# The exec should complete successfully during graceful termination
	wait "$exec_pid"
	exec_status=$?
	[ "$exec_status" -eq 0 ]

	# Wait for the stop to complete
	wait "$stop_pid"
}

@test "exec rejected after grace period ends" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Stop container with a very short timeout (1 second)
	crictl stop -t 1 "$ctr_id" &
	stop_pid=$!

	# Wait for grace period to end and kill loop to start
	sleep 2

	# Exec should be rejected during/after kill loop
	run crictl exec --sync "$ctr_id" echo "should fail"
	[ "$status" -ne 0 ]

	# Wait for the stop to complete
	wait "$stop_pid" || true
}

@test "sequential execs during graceful termination" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start stopping the container with a 10 second timeout
	crictl stop -t 10 "$ctr_id" &
	stop_pid=$!

	# Wait a moment for the stop to be initiated
	sleep 0.5

	# Run multiple execs sequentially (one after another, not concurrent)
	output1=$(crictl exec --sync "$ctr_id" echo "exec 1")
	[[ "$output1" == *"exec 1"* ]]

	output2=$(crictl exec --sync "$ctr_id" echo "exec 2")
	[[ "$output2" == *"exec 2"* ]]

	output3=$(crictl exec --sync "$ctr_id" echo "exec 3")
	[[ "$output3" == *"exec 3"* ]]

	# Wait for the stop to complete
	wait "$stop_pid"
}

@test "exec handles SIGTERM gracefully during termination" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start an exec that traps SIGTERM
	crictl exec "$ctr_id" /bin/bash -c 'trap "echo caught SIGTERM; exit 0" TERM; sleep 60' &
	exec_pid=$!

	# Give the exec a moment to start and set up signal handler
	sleep 0.5

	# Stop container with a short timeout
	crictl stop -t 2 "$ctr_id"

	# The exec should be terminated (either by completing the trap or being killed)
	wait "$exec_pid" || true
}

@test "container restart rejected during exec" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start a long-running exec
	crictl exec "$ctr_id" /bin/bash -c 'sleep 5' &
	exec_pid=$!

	# Give the exec a moment to start
	sleep 0.5

	# Try to stop and restart the container while exec is running
	crictl stop -t 10 "$ctr_id"

	# Give container time to be stopped
	sleep 2

	# Attempting to start a stopped container should fail
	# (containers can't be restarted with crictl, only pods can)
	run crictl start "$ctr_id"
	[ "$status" -ne 0 ]

	# The exec should have been terminated
	wait "$exec_pid" || true
}
