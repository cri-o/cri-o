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

@test "pprof over unix socket" {
	ENABLE_PROFILE_UNIX_SOCKET=true start_crio
	curl --silent --fail --show-error --unix-socket "$CRIO_SOCKET" http://localhost/debug/pprof/goroutine > /dev/null
}
