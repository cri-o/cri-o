#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "pprof" {
	CONTAINER_PROFILE=true start_crio
	curl --silent --fail --show-error http://localhost:6060/debug/pprof/goroutine > /dev/null
}
