#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

function pid_namespace_test() {
	start_crio

	run crictl runs "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" cat /proc/1/cmdline
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "${EXPECTED_INIT:-redis}" ]]

	run crictl exec --sync "$pod_id" cat /proc/*/cmdline
	echo "$output"
	[ "$status" -eq 0 ]
	if [ -n "${REDIS_IN_INFRA}" ]
	then
		[[ "$output" =~ "redis" ]]
	else
		! [[ "$output" =~ "redis" ]]
	fi

	run crictl stops "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rms "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_crio
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

@test "pod disable shared pid namespace" {
	ADDITIONAL_CRIO_OPTIONS=--enable-shared-pid-namespace=false pid_namespace_test
}

@test "pod enable shared pid namespace" {
	ADDITIONAL_CRIO_OPTIONS=--enable-shared-pid-namespace=true EXPECTED_INIT=pause pid_namespace_test
}
