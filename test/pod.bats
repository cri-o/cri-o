#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

# PR#59
@test "pod release name on remove" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	id="$output"
	run ocic pod stop --id "$id"
	echo "$output"
	[ "$status" -eq 0 ]
	echo "$output"
	run ocic pod remove --id "$id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	id="$output"
	run ocic pod stop --id "$id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "pod remove" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "pod list filtering" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json -name pod1 --label "a=b" --label "c=d" --label "e=f"
	echo "$output"
	[ "$status" -eq 0 ]
	pod1_id="$output"
	run ocic pod run --config "$TESTDATA"/sandbox_config.json -name pod2 --label "a=b" --label "c=d"
	echo "$output"
	[ "$status" -eq 0 ]
	pod2_id="$output"
	run ocic pod run --config "$TESTDATA"/sandbox_config.json -name pod3 --label "a=b"
	echo "$output"
	[ "$status" -eq 0 ]
	pod3_id="$output"
	run ocic pod list --label "a=b" --label "c=d" --label "e=f" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id"  ]]
	run ocic pod list --label "g=h" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run ocic pod list --label "a=b" --label "c=d" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id"  ]]
	[[ "$output" =~ "$pod2_id"  ]]
	run ocic pod list --label "a=b" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id"  ]]
	[[ "$output" =~ "$pod2_id"  ]]
	[[ "$output" =~ "$pod3_id"  ]]
	run ocic pod list --id "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id"  ]]
	# filter by truncated id should work as well
	run ocic pod list --id "${pod1_id:0:4}"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id" ]]
	run ocic pod list --id "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod2_id"  ]]
	run ocic pod list --id "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod3_id"  ]]
	run ocic pod list --id "$pod1_id" --label "a=b"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod1_id"  ]]
	run ocic pod list --id "$pod2_id" --label "a=b"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod2_id"  ]]
	run ocic pod list --id "$pod3_id" --label "a=b"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$pod3_id"  ]]
	run ocic pod list --id "$pod3_id" --label "c=d"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run ocic pod remove --id "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_pods
	stop_ocid
}

@test "pod metadata in list & status" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run ocic pod list --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "Name: podsandbox1" ]]
	[[ "$output" =~ "UID: redhat-test-ocid" ]]
	[[ "$output" =~ "Namespace: redhat.test.ocid" ]]
	[[ "$output" =~ "Attempt: 1" ]]

	run ocic pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "Name: podsandbox1" ]]
	[[ "$output" =~ "UID: redhat-test-ocid" ]]
	[[ "$output" =~ "Namespace: redhat.test.ocid" ]]
	[[ "$output" =~ "Attempt: 1" ]]

	cleanup_pods
	stop_ocid
}

@test "pass pod sysctls to runtime" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run ocic ctr create --pod "$pod_id" --config "$TESTDATA"/container_redis.json
	echo "$output"
	[ "$status" -eq 0 ]
	container_id="$output"

	run ocic ctr start --id "$container_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run ocic ctr execsync --id "$container_id" sysctl kernel.shm_rmid_forced
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "kernel.shm_rmid_forced = 1" ]]

	run ocic ctr execsync --id "$container_id" sysctl kernel.msgmax
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "kernel.msgmax = 8192" ]]

	run ocic ctr execsync --id "$container_id" sysctl net.ipv4.ip_local_port_range
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "net.ipv4.ip_local_port_range = 1024	65000" ]]

	cleanup_pods
	stop_ocid
}

@test "pod stop idempotent" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "pod remove idempotent" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "pod stop idempotent with ctrs already stopped" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "restart ocid and still get pod status" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	restart_ocid
	run ocic pod status --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}
