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
	run crictl runp "$TESTDATA"/sandbox_pidnamespacemode_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis_namespace.json "$TESTDATA"/sandbox_pidnamespacemode_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" cat /proc/1/cmdline
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ pause ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}
