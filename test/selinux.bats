#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "ctr termination reason Completed" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config_selinux.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config_selinux.json)
	crictl start "$ctr_id"
}
