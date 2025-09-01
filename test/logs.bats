#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "ReopenContainerLog should succeed when the container is running" {
	start_crio

	jq '.metadata.name = "sleep"
		| .command = ["/bin/sh", "-c", "sleep 600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/trap.json

	ctr_id=$(crictl run "$TESTDIR"/trap.json "$TESTDATA"/sandbox_config.json)
	crictl logs -r "$ctr_id"
}

@test "ReopenContainerLog should fail when the container is stopped" {
	start_crio

	jq '.metadata.name = "sleep"
		| .command = ["/bin/sh", "-c", "sleep 600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/trap.json

	ctr_id=$(crictl run "$TESTDIR"/trap.json "$TESTDATA"/sandbox_config.json)
	crictl stop "$ctr_id"
	run ! crictl logs -r "$ctr_id"
}

@test "ReopenContainerLog should not be blocked during deletion" {
	start_crio

	jq '.metadata.name = "trap"
		| .command = ["/bin/sh", "-c", "trap \"sleep 600\" TERM && sleep 600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/trap.json

	ctr_id=$(crictl run "$TESTDIR"/trap.json "$TESTDATA"/sandbox_config.json)
	# Especially when using kata, it sometimes takes a few seconds to actually run container
	sleep 5

	crictl stop -t 10 "$ctr_id" &
	wait_for_log "Request: &v1.StopContainerRequest"
	crictl logs -r "$ctr_id"
	output=$(crictl inspect "$ctr_id" | jq -r ".status.state")
	[[ "$output" == "CONTAINER_RUNNING" ]]
}

@test "Log file rotation should work" {
	start_crio

	jq '.metadata.name = "logger"
		| .command = ["/bin/sh", "-c", "while true; do echo hello; sleep 1; done"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/logger.json

	ctr_id=$(crictl run "$TESTDIR"/logger.json "$TESTDATA"/sandbox_config.json)
	# Especially when using kata, it sometimes takes a few seconds to actually run container
	sleep 5

	logpath=$(crictl inspect "$ctr_id" | jq -r ".status.logPath")
	[[ -f "$logpath" ]]

	# Move log file away, then ask for re-open.
	# It will fail if the new log file is not created
	mv "$logpath" "$logpath".rotated
	crictl logs -r "$ctr_id"

	[[ -f "$logpath" ]]

	# Verify that the rotated log file is not written to anymore
	initial_size=$(stat -c %s "$logpath.rotated")
	[ "$initial_size" -gt 0 ]
	sleep 2 # our logger writes every second, leave enough time for at least one write
	new_size=$(stat -c %s "$logpath.rotated")
	[ "$new_size" -eq "$initial_size" ]
}
