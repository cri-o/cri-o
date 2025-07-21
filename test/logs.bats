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
