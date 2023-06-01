#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	start_crio
}

function teardown() {
	cleanup_test
}

@test "status should fail if no subcommand is provided" {
	run -1 "${CRIO_BINARY_PATH}" status
}

@test "status should succeed to retrieve the config" {
	# when
	run -0 "${CRIO_BINARY_PATH}" status --socket="${CRIO_SOCKET}" config

	# then
	[[ "$output" == *"[crio]"* ]]
}

@test "status should fail to retrieve the config with invalid socket" {
	run -1 "${CRIO_BINARY_PATH}" status --socket wrong.sock c
}

@test "status should succeed to retrieve the info" {
	# when
	run -0 "${CRIO_BINARY_PATH}" status --socket="${CRIO_SOCKET}" info

	# then
	[[ "$output" == *"storage driver"* ]]
}

@test "status should fail to retrieve the info with invalid socket" {
	run -1 "${CRIO_BINARY_PATH}" status --socket wrong.sock i
}

@test "succeed to retrieve the container info" {
	# given
	pod=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr=$(crictl create "$pod" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr"

	# when
	run -0 "${CRIO_BINARY_PATH}" status --socket="${CRIO_SOCKET}" containers --id "$ctr"

	# then
	[[ "$output" == *"sandbox: $pod"* ]]
}

@test "should fail to retrieve the container info without ID" {
	run -1 "${CRIO_BINARY_PATH}" status --socket="${CRIO_SOCKET}" containers
}

@test "should fail to retrieve the container with invalid socket" {
	run -1 "${CRIO_BINARY_PATH}" status --socket wrong.sock s
}
