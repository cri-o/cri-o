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


	run crictl exec --sync "$pod_id" cat /proc/*/cmdline
	echo "$output"
	[ "$status" -eq 0 ]
	if [ -n "${REDIS_IN_INFRA}" ]
	then
		[[ "$output" =~ "redis" ]]
	else
		! [[ "$output" =~ "redis" ]]
	fi

}

@test "container pid namespace" {
	ADDITIONAL_CRIO_OPTIONS=--pid-namespace=container pid_namespace_test
}

@test "pod pid namespace" {
	ADDITIONAL_CRIO_OPTIONS=--pid-namespace=pod REDIS_IN_INFRA=1 EXPECTED_INIT=pause pid_namespace_test
}

@test "pod-container pid namespace" {
	ADDITIONAL_CRIO_OPTIONS=--pid-namespace=pod-container REDIS_IN_INFRA=1 pid_namespace_test
}
