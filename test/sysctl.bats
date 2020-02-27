#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}


@test "pass pod sysctls to runtime" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	export TEST_SYSCTL="net.ipv4.ip_forward=1"
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config_sysctl.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config_sysctl.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" sysctl kernel.shm_rmid_forced
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "kernel.shm_rmid_forced = 1" ]]

	run crictl exec --sync "$ctr_id" sysctl kernel.msgmax
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "kernel.msgmax = 4096" ]]

	run crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_local_port_range
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "net.ipv4.ip_local_port_range = 1024	65000" ]]

	run crictl exec --sync "$ctr_id" sysctl net.ipv4.ip_forward
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "net.ipv4.ip_forward = 1" ]]
}

