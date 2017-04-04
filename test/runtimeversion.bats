#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "ocic runtimeversion" {
	start_ocid
	run ocic runtimeversion
	echo "$output"
	[ "$status" -eq 0 ]
	stop_ocid
}
