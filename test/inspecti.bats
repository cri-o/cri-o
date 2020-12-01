#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
	start_crio
}

function teardown() {
	cleanup_test
}

@test "inspect image should succed contain all necessary information" {
	output=$(crictl inspecti quay.io/crio/redis:alpine)
	[ "$output" != "" ]
	jq -e '.status.size' <<< "$output"
	jq -e '.info.imageSpec.config.Cmd' <<< "$output"
}
