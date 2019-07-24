#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "crictl runtimeversion" {
	start_crio
	run crictl info
	echo "$output"
	[ "$status" -eq 0 ]
	stop_crio
}
