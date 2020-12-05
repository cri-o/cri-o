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
	crictl info
}
