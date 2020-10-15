#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "pid_namespace_mode_pod_test" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_pidnamespacemode_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis_namespace.json "$TESTDATA"/sandbox_pidnamespacemode_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" cat /proc/1/cmdline)
	[[ "$output" == *"pause"* ]]
}
