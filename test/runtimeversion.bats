#!/usr/bin/env bats

load helpers

function teardown() {
	stop_ocid
	cleanup_test
}

@test "ocic runtimeversion" {
	start_ocid
	ocic runtimeversion
	[ "$status" -eq 0 ]
}
